// Examples of using RS-232/Virtual-Serial over USB
// as the transport layer for the protocol.

package protocol

import (
	"github.com/tarm/serial"
	"log"
	"time"
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
func (com *comGateway) ServeCOM() {
	for {
		var port *serial.Port
		firstTryDone := false
		// Attempt to open the COM port on the system.
		for {
			p, err := serial.OpenPort(com.comConfig)
			if err == nil {
				port = p
				break
			}
			if !firstTryDone {
				log.Printf("Gateway @ '%s': Error opening COM port -> %v\nRetrying every 5s..\n", com.comConfig.Name, err)
				firstTryDone = true
			}
			time.Sleep(time.Second * 5)
		}

		// Open success.
		log.Printf("Gateway @ '%s': Started service.\n", com.comConfig.Name)
		com.Listen(port)
		log.Printf("Gateway @ '%s': Fatal error. Closing COM port\n", com.comConfig.Name)
		time.Sleep(time.Second * 2)
	}
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
