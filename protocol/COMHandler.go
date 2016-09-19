package protocol

import (
	"errors"
	"github.com/tarm/serial"
	"log"
)

// A protocol server listening on a COM port.
type comHandler struct {
	protocolServer server
	comConfig      *serial.Config
}

func NewComHandler(ComName string, ComBaudRate int) *comHandler {
	com := comHandler{
		comConfig: &serial.Config{Name: ComName, Baud: ComBaudRate},
	}
	return &com
}

// Start server on a COM port interface to service single protocol Client.
func (com *comHandler) ServeCOM() error {
	comPort, err := serial.OpenPort(com.comConfig)
	if err != nil {
		return err
	}

	log.Println("Listening on", com.comConfig.Name)
	com.protocolServer.Listen(comPort)
	return errors.New("COM Port Lost")
}
