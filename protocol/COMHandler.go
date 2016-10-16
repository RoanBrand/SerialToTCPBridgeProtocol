// Examples of using RS-232/Virtual-Serial over USB
// as the transport layer for the protocol.

package protocol

import (
	"errors"
	"github.com/tarm/serial"
	"log"
)

// A Protocol Gateway listening on a COM port.
type comGateway struct {
	gateway
	comConfig *serial.Config
}

func NewComGateway(ComName string, ComBaudRate int) *comGateway {
	s := comGateway{
		comConfig: &serial.Config{Name: ComName, Baud: ComBaudRate},
	}
	return &s
}

// Start Gateway on a COM port interface to service single protocol Client.
func (com *comGateway) ServeCOM() error {
	comPort, err := serial.OpenPort(com.comConfig)
	if err != nil {
		return err
	}

	log.Println("Listening on", com.comConfig.Name)
	com.Listen(comPort)
	return errors.New("COM Port Lost")
}

// A protocol client dialing to Server/Gateway over a COM port.
type comClient struct {
	client
}

func NewComClient(ComName string, ComBaudRate int) (*comClient, error) {
	c := comClient{}
	var err error
	c.com, err = serial.OpenPort(&serial.Config{Name: ComName, Baud: ComBaudRate})
	if err != nil {
		return nil, err
	}
	return &c, nil
}
