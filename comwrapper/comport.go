// Examples of using RS-232/Virtual-Serial over USB
// as the transport layer for the protocol.

package comwrapper

import (
	"github.com/RoanBrand/SerialToTCPBridgeProtocol/protocol"
	"github.com/tarm/serial"
	"log"
	"net"
	"time"
)

// Connect to a address on a tcp network, using a protocol Client.
// We connect to a Gateway over a serial connection first.
func Dial(portName string, baudRate int, address string) (net.Conn, error) {
	p, err := serial.OpenPort(&serial.Config{Name: portName, Baud: baudRate})
	if err != nil {
		return nil, err
	}
	return protocol.Dial(p, address)
}

// A Protocol Gateway listening on a COM port.
type comGateway struct {
	protocol.Gateway
	ComConfig *serial.Config
}

func NewComPortGateway(portName string, baudRate int) *comGateway {
	return &comGateway{
		ComConfig: &serial.Config{Name: portName, Baud: baudRate},
	}
}

// Start Gateway on a COM port interface to service single protocol Client.
func (com *comGateway) ListenAndServe() {
	for {
		var port *serial.Port
		firstTryDone := false
		// Attempt to open the COM port on the system.
		for {
			p, err := serial.OpenPort(com.ComConfig)
			if err == nil {
				port = p
				break
			}
			if !firstTryDone {
				log.Printf("Gateway @ '%s': Error opening COM port -> %v\nRetrying every 5s..\n", com.ComConfig.Name, err)
				firstTryDone = true
			}
			time.Sleep(time.Second * 5)
		}

		// Open success.
		log.Printf("Gateway @ '%s': Started service.\n", com.ComConfig.Name)
		com.Listen(port)
		log.Printf("Gateway @ '%s': Fatal error. Closing COM port\n", com.ComConfig.Name)
		time.Sleep(time.Second * 2)
	}
}
