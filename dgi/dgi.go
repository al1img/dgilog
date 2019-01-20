package dgi

import (
	"bytes"
	"encoding/binary"
	"errors"

	"github.com/google/gousb"
	log "github.com/sirupsen/logrus"
)

/*******************************************************************************
 * Consts
 ******************************************************************************/

const (
	vid = 0x03eb
	pid = 0x2111

	inBufferSize = 16384
)

const (
	DGI_CMD_SIGN_ON               = 0x00
	DGI_CMD_SIGN_OFF              = 0x01
	DGI_CMD_GET_VERSION           = 0x02
	DGI_CMD_INTERFACES_LIST       = 0x08
	DGI_CMD_SET_MODE              = 0x0A
	DGI_CMD_TARGET_RESET          = 0x20
	DGI_CMD_INTERFACES_ENABLE     = 0x10
	DGI_CMD_INTERFACES_STATUS     = 0x11
	DGI_CMD_INTERFACES_SET_CONFIG = 0x12
	DGI_CMD_INTERFACES_GET_CONFIG = 0x13
	DGI_CMD_INTERFACES_SEND_DATA  = 0x14
	DGI_CMD_INTERFACES_POLL_DATA  = 0x15
)

const (
	DGI_RESP_DATA = 0xA0
	DGI_RESP_OK   = 0x80
)

const (
	DGI_MODE_4BYTES_LEN         = 1 << 2
	DGI_MODE_OVERFLOW_INDICATOR = 1 << 0
)

const (
	DGI_RESET_HIGH = 0
	DGI_RESET_LOW  = 1
)

const (
	DGI_ITF_STATE_OFF       = 0
	DGI_ITF_STATE_ON        = 1
	DGI_ITF_STATE_TIMESTAMP = 2
)

const (
	DGI_ITF_ID_TIMESTAMP    = 0x00
	DGI_ITF_ID_SPI          = 0x20
	DGI_ITF_ID_UART         = 0x21
	DGI_ITF_ID_I2C          = 0x22
	DGI_ITF_ID_GPIO         = 0x30
	DGI_ITF_ID_POWER_DATA   = 0x40
	DGI_ITF_ID_POWER_EVENTS = 0x41
	DGI_ITF_ID_RESERVED     = 0xFF
)

const (
	DGI_ITF_STATUS_STARTED     = 1 << 0
	DGI_ITF_STATUS_TIMESTAMPED = 1 << 1
	DGI_ITF_STATUS_OVERFLOW    = 1 << 2
)

const (
	DGI_CFG_SPI_CHARLEN = 0
	DGI_CFG_SPI_MODE    = 1
	DGI_CFG_SPI_FORCECS = 2
)

/*******************************************************************************
 * Vars
 ******************************************************************************/

var cfgString = map[uint8]map[uint16]string{
	DGI_ITF_ID_SPI: map[uint16]string{
		DGI_CFG_SPI_CHARLEN: "Character length",
		DGI_CFG_SPI_MODE:    "Mode",
		DGI_CFG_SPI_FORCECS: "Force CS",
	},
}

var itfString = map[uint8]string{
	DGI_ITF_ID_TIMESTAMP:    "Timestamp",
	DGI_ITF_ID_SPI:          "SPI",
	DGI_ITF_ID_UART:         "UART",
	DGI_ITF_ID_I2C:          "I2C",
	DGI_ITF_ID_GPIO:         "GPIO",
	DGI_ITF_ID_POWER_DATA:   "Power Data",
	DGI_ITF_ID_POWER_EVENTS: "Power Events",
	DGI_ITF_ID_RESERVED:     "Reserved",
}

// Instance DGI instance
type Instance struct {
	dev         *gousb.Device
	cfg         *gousb.Config
	itf         *gousb.Interface
	inEp        *gousb.InEndpoint
	outEp       *gousb.OutEndpoint
	inBuf       []byte
	len4bytes   bool
	overflowInd bool
}

/*******************************************************************************
 * Public
 ******************************************************************************/

// New creates new DGI instance
func New(ctx *gousb.Context) (instance *Instance, err error) {
	instance = &Instance{}

	if instance.dev, err = ctx.OpenDeviceWithVIDPID(vid, pid); err != nil {
		return nil, err
	}

	if instance.dev == nil {
		return nil, errors.New("No device found")
	}

	log.Debug(instance.dev, err)

	cfgNum, itfNum, altNum, inEpNum, outEpNum, err := findInterface(instance.dev)
	if err != nil {
		return nil, err
	}

	if instance.cfg, err = instance.dev.Config(cfgNum); err != nil {
		return nil, err
	}

	if instance.itf, err = instance.cfg.Interface(itfNum, altNum); err != nil {
		return nil, err
	}

	if instance.inEp, err = instance.itf.InEndpoint(inEpNum); err != nil {
		return nil, err
	}

	if instance.outEp, err = instance.itf.OutEndpoint(outEpNum); err != nil {
		return nil, err
	}

	instance.inBuf = make([]uint8, inBufferSize)

	return instance, nil
}

// Close closes DGI instance
func (instance *Instance) Close() {
	if instance.itf != nil {
		instance.itf.Close()
	}

	if instance.cfg != nil {
		instance.cfg.Close()
	}

	if instance.dev != nil {
		instance.dev.Close()
	}
}

// SignOn performs sign on
func (instance *Instance) SignOn() (ack string, err error) {
	data, err := instance.sendCommand(DGI_CMD_SIGN_ON, nil)
	if err != nil {
		return "", err
	}

	var len uint16

	buf := bytes.NewBuffer(data)

	if err = binary.Read(buf, binary.BigEndian, &len); err != nil {
		return "", err
	}

	return string(buf.Bytes()[:len]), nil
}

// SignOff performs sign on
func (instance *Instance) SignOff() (err error) {
	if _, err = instance.sendCommand(DGI_CMD_SIGN_OFF, nil); err != nil {
		return err
	}

	return nil
}

// GetVersion returns version
func (instance *Instance) GetVersion() (maj, min uint8, err error) {
	data, err := instance.sendCommand(DGI_CMD_GET_VERSION, nil)
	if err != nil {
		return 0, 0, err
	}

	if len(data) != 2 {
		return 0, 0, errors.New("Wrong data length")
	}

	return data[0], data[1], nil
}

// InterfaceList returns interface list
func (instance *Instance) InterfaceList() (list []uint8, err error) {
	data, err := instance.sendCommand(DGI_CMD_INTERFACES_LIST, nil)
	if err != nil {
		return nil, err
	}

	return data[1 : data[0]+1], nil
}

// InterfaceName returns interface name
func (instance *Instance) InterfaceName(id uint8) (name string, err error) {
	name, ok := itfString[id]
	if !ok {
		return "", errors.New("Unknown interface")
	}

	return name, nil
}

// SetMode sets operation mode
func (instance *Instance) SetMode(mode uint8) (err error) {
	if _, err = instance.sendCommand(DGI_CMD_SET_MODE, []uint8{mode}); err != nil {
		return err
	}

	if mode&DGI_MODE_4BYTES_LEN != 0 {
		instance.len4bytes = true
	}

	if mode&DGI_MODE_OVERFLOW_INDICATOR != 0 {
		instance.overflowInd = true
	}

	return nil
}

// TargetReset resets target
func (instance *Instance) TargetReset(reset uint8) (err error) {
	if _, err = instance.sendCommand(DGI_CMD_TARGET_RESET, []uint8{reset}); err != nil {
		return err
	}

	return nil
}

// InterfacesEnable enables interfaces
func (instance *Instance) InterfacesEnable(states map[uint8]uint8) (err error) {
	data := make([]uint8, 0, 2*len(states))

	for itf, state := range states {
		data = append(data, itf, state)
	}

	if _, err = instance.sendCommand(DGI_CMD_INTERFACES_ENABLE, data); err != nil {
		return err
	}

	return nil
}

// InterfacesStatus returns interfaces status
func (instance *Instance) InterfacesStatus() (statuses map[uint8]uint8, err error) {
	statuses = make(map[uint8]uint8)

	data, err := instance.sendCommand(DGI_CMD_INTERFACES_STATUS, nil)
	if err != nil {
		return nil, err
	}

	for i := 0; i < len(data)/2; i++ {
		statuses[data[i*2]] = data[i*2+1]
	}

	return statuses, nil
}

// InterfacesGetConfig returns config of specific interface
func (instance *Instance) InterfacesGetConfig(itf uint8) (config map[uint16]uint32, err error) {
	data, err := instance.sendCommand(DGI_CMD_INTERFACES_GET_CONFIG, []uint8{itf})
	if err != nil {
		return nil, err
	}

	rsp := struct {
		Len uint16
		Itf uint8
	}{}

	buf := bytes.NewBuffer(data)

	if err = binary.Read(buf, binary.BigEndian, &rsp); err != nil {
		return nil, err
	}

	if rsp.Itf != itf {
		return nil, errors.New("Wrong interface")
	}

	if rsp.Len < 1 {
		return nil, errors.New("Wrong length")
	}

	config = make(map[uint16]uint32)

	for i := 0; i < int(rsp.Len-1)/6; i++ {
		param := struct {
			ID    uint16
			Value uint32
		}{}

		if err = binary.Read(buf, binary.BigEndian, &param); err != nil {
			return nil, err
		}

		config[param.ID] = param.Value
	}

	return config, nil
}

// ConfigName returns config name
func (instance *Instance) ConfigName(itf uint8, cfg uint16) (name string, err error) {
	cfgMap, ok := cfgString[itf]
	if !ok {
		return "", errors.New("Unknown interface")
	}

	name, ok = cfgMap[cfg]
	if !ok {
		return "", errors.New("Unknown config ID")
	}

	return name, nil
}

// InterfacesSetConfig returns config of specific interface
func (instance *Instance) InterfacesSetConfig(itf uint8, config map[uint16]uint32) (err error) {
	buf := bytes.NewBuffer(nil)

	if err = binary.Write(buf, binary.BigEndian, &itf); err != nil {
		return err
	}

	for id, value := range config {
		param := struct {
			ID    uint16
			Value uint32
		}{id, value}

		if err = binary.Write(buf, binary.BigEndian, &param); err != nil {
			return err
		}
	}

	if _, err := instance.sendCommand(DGI_CMD_INTERFACES_SET_CONFIG, buf.Bytes()); err != nil {
		return err
	}

	return nil
}

// InterfacesSendData sends data over specific interface
func (instance *Instance) InterfacesSendData(itf uint8, data []uint8) (err error) {
	buf := make([]uint8, 0, len(data)+1)

	buf[0] = itf

	if _, err := instance.sendCommand(DGI_CMD_INTERFACES_SEND_DATA, append(buf, data...)); err != nil {
		return err
	}

	return nil
}

// InterfacesPollData retreives data from the interface
func (instance *Instance) InterfacesPollData(itf uint8) (data []uint8, overflow bool, err error) {
	if data, err = instance.sendCommand(DGI_CMD_INTERFACES_POLL_DATA, []uint8{itf}); err != nil {
		return nil, false, err
	}

	buf := bytes.NewBuffer(data)

	var dataItf uint8

	if err = binary.Read(buf, binary.BigEndian, &dataItf); err != nil {
		return nil, false, err
	}

	if dataItf != itf {
		return nil, false, errors.New("Wrong interface")
	}

	var len16 uint16
	var len32 uint32

	len := 0

	if instance.len4bytes {
		if err = binary.Read(buf, binary.BigEndian, &len32); err != nil {
			return nil, false, err
		}
		len = int(len32)
	} else {
		if err = binary.Read(buf, binary.BigEndian, &len16); err != nil {
			return nil, false, err
		}
		len = int(len16)
	}

	if instance.overflowInd {
		var overflowInd uint32
		if err = binary.Read(buf, binary.BigEndian, &overflowInd); err != nil {
			return nil, false, err
		}
		if overflowInd != 0 {
			overflow = true
		}
	}

	for len > buf.Len() {
		readLen, err := instance.inEp.Read(instance.inBuf)
		if err != nil {
			return nil, false, err
		}

		if _, err = buf.Write(instance.inBuf[:readLen]); err != nil {
			return nil, false, err
		}
	}

	return buf.Bytes(), overflow, nil
}

/*******************************************************************************
 * Private
 ******************************************************************************/

func (instance *Instance) sendCommand(cmd uint8, outData []byte) (inData []byte, err error) {
	req := struct {
		Cmd uint8
		Len uint16
	}{Cmd: cmd, Len: uint16(len(outData))}

	outBuf := bytes.NewBuffer(nil)

	if err = binary.Write(outBuf, binary.BigEndian, &req); err != nil {
		return nil, err
	}

	if _, err = outBuf.Write(outData); err != nil {
		return nil, err
	}

	if _, err = instance.outEp.Write(outBuf.Bytes()); err != nil {
		return nil, err
	}

	len, err := instance.inEp.Read(instance.inBuf)
	if err != nil {
		return nil, err
	}

	var rsp struct {
		Cmd uint8
		Rsp uint8
	}

	inBuf := bytes.NewBuffer(instance.inBuf[:len])

	if err = binary.Read(inBuf, binary.BigEndian, &rsp); err != nil {
		return nil, err
	}

	if rsp.Cmd != cmd {
		return nil, errors.New("Wrong command id")
	}

	if rsp.Rsp == DGI_RESP_DATA && inBuf.Len() == 0 {
		return nil, errors.New("No data")
	}

	return inBuf.Bytes(), nil
}

func findInterface(dev *gousb.Device) (cfgNum, itfNum, altNum, inEpNum, outEpNum int, err error) {
	for _, cfgDesc := range dev.Desc.Configs {
		log.Debug(cfgDesc)

		cfgNum = cfgDesc.Number

		for _, itfDesc := range cfgDesc.Interfaces {

			log.Debug(itfDesc)

			itfNum = itfDesc.Number

			for _, altDesc := range itfDesc.AltSettings {

				log.Debug(altDesc)
				log.Debugf("Class: %s, SubClass: %s", altDesc.Class, altDesc.SubClass)

				altNum = altDesc.Alternate

				if altDesc.Class == gousb.ClassVendorSpec && altDesc.SubClass == gousb.ClassVendorSpec {
					for _, ep := range altDesc.Endpoints {

						log.Debug(ep)

						if ep.Direction == gousb.EndpointDirectionIn {
							inEpNum = ep.Number
						}

						if ep.Direction == gousb.EndpointDirectionOut {
							outEpNum = ep.Number
						}
					}

					if inEpNum == 0 && outEpNum == 0 {
						return 0, 0, 0, 0, 0, errors.New("Endpoint not found")
					}

					return cfgNum, itfNum, altNum, inEpNum, outEpNum, nil
				}
			}
		}
	}

	return 0, 0, 0, 0, 0, errors.New("Interface not found")
}
