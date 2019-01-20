package main

import (
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/al1img/dgilog/dgi"
	"github.com/google/gousb"
	log "github.com/sirupsen/logrus"
)

func init() {
	log.SetFormatter(&log.TextFormatter{
		DisableTimestamp: false,
		TimestampFormat:  "2006-01-02 15:04:05.000",
		FullTimestamp:    true})
	log.SetLevel(log.DebugLevel)
	log.SetOutput(os.Stdout)
}

func main() {
	log.Info("Start")

	ctx := gousb.NewContext()
	defer ctx.Close()

	ctx.Debug(3)

	dgiInstance, err := dgi.New(ctx)
	if err != nil {
		log.Fatalf("Can't create DGI instance: %s", err)
	}
	defer dgiInstance.Close()

	ack, err := dgiInstance.SignOn()
	if err != nil {
		log.Fatalf("Can't sign on: %s", err)
	}
	defer dgiInstance.SignOff()

	maj, min, err := dgiInstance.GetVersion()
	if err != nil {
		log.Fatalf("Can't get version: %s", err)
	}

	log.Infof("%s v%d.%d", ack, maj, min)

	itfList, err := dgiInstance.InterfaceList()
	if err != nil {
		log.Fatalf("Can't get interface list: %s", err)
	}

	itfNames := []string{}

	for _, itf := range itfList {
		name, err := dgiInstance.InterfaceName(itf)
		if err != nil {
			log.Warnf("Unknown interface: 0x%02X", itf)
			continue
		}

		itfNames = append(itfNames, name)
	}

	log.Infof("Interfaces: %s", strings.Join(itfNames, ", "))

	if err := dgiInstance.InterfacesSetConfig(dgi.DGI_ITF_ID_SPI, map[uint16]uint32{dgi.DGI_CFG_SPI_CHARLEN: 8}); err != nil {
		log.Fatalf("Can't set interfaces status: %s", err)
	}

	if err = dgiInstance.InterfacesEnable(map[uint8]uint8{dgi.DGI_ITF_ID_SPI: dgi.DGI_ITF_STATE_ON}); err != nil {
		log.Fatalf("Can't enable interface: %s", err)
	}

	config, err := dgiInstance.InterfacesGetConfig(dgi.DGI_ITF_ID_SPI)
	if err != nil {
		log.Fatalf("Can't get interfaces status: %s", err)
	}

	for id, value := range config {
		name, err := dgiInstance.ConfigName(dgi.DGI_ITF_ID_SPI, id)
		if err != nil {
			log.Errorf("Can't get config id name: %s", err)
			continue
		}

		log.Debugf("%s: %d", name, value)
	}

	stopChannel := make(chan bool)

	go func() {
		for {
			select {
			case <-stopChannel:
				return
			default:
				data, _, err := dgiInstance.InterfacesPollData(dgi.DGI_ITF_ID_SPI)
				if err != nil {
					log.Errorf("Polling data error: %s", err)
				}

				if len(data) != 0 {
					fmt.Print(string(data))
				} else {
					time.Sleep(100 * time.Millisecond)
				}
			}
		}
	}()

	terminateChannel := make(chan os.Signal, 1)
	signal.Notify(terminateChannel, os.Interrupt, syscall.SIGTERM)

	<-terminateChannel
	stopChannel <- true

	log.Info("Stop")
}
