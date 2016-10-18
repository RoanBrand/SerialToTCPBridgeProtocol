package protocol

import (
	"log"
	"net"
)

// Implementation of the Protocol Gateway.
type gateway struct {
	protocolTransport          // Connection between Protocol Gateway & Client.
	uStream           net.Conn // Upstream connection to tcp Server.
}

// Initialize downstream RX and listen for a protocol Client.
func (g *gateway) Listen(ds serialInterface) {
	g.com = ds
	g.rxBuff = make(chan byte, 512)
	g.acknowledgeEvent = make(chan bool)
	g.state = Disconnected

	g.session.Add(2)
	go g.rxSerial(g.dropGateway)
	go g.packetParser(g.handleRxPacket, g.dropLink)
	g.session.Wait()
}

// Packet RX done. Handle it.
func (g *gateway) handleRxPacket(packet *Packet) {
	var rxSeqFlag bool = (packet.command & 0x80) > 0
	switch packet.command & 0x7F {
	case publish:
		// Payload from serial client
		if g.state == Connected {
			g.txBuff <- Packet{command: acknowledge | (packet.command & 0x80)}
			if rxSeqFlag == g.expectedRxSeqFlag {
				g.expectedRxSeqFlag = !g.expectedRxSeqFlag
				_, err := g.uStream.Write(packet.payload)
				if err != nil {
					log.Printf("Error sending upstream: %v Disconnecting client\n", err)
					g.txBuff <- Packet{command: disconnect}
					g.dropLink()
				}
			}
		}
	case acknowledge:
		if g.state == Connected {
			g.acknowledgeEvent <- rxSeqFlag
		}
	case connect:
		if g.state != Disconnected {
			return
		}
		if len(packet.payload) != 6 {
			return
		}

		dstStr := makeTCPConnString(packet.payload)

		g.txBuff = make(chan Packet, 2)
		g.expectedRxSeqFlag = false

		// Start downstream TX
		g.session.Add(1)
		go g.txSerial(g.dropGateway)

		// Open connection to upstream server on behalf of client
		// log.Printf("Gateway: Connect request from client. Dialing to: %v\n", dstStr)
		var err error
		if g.uStream, err = net.Dial("tcp", dstStr); err != nil { // TODO: add timeout
			log.Printf("Gateway: Failed to connect to: %v\n", dstStr)
			g.txBuff <- Packet{command: disconnect} // TODO: payload to contain error or timeout
			g.dropLink()
			return
		}

		// Start link session
		g.session.Add(1)
		tx := make([]byte, 512)
		go g.packetSender(func() (p Packet, err error) {
			// Publish data downstream received from upstream tcp server.
			n, err := g.uStream.Read(tx)
			p = Packet{command: publish, payload: tx[:n]}
			return
		}, g.dropLink)
		g.state = Connected
		g.txBuff <- Packet{command: connack}
	case disconnect:
		if g.state == Connected {
			log.Println("Client wants to disconnect. Ending link session")
			g.dropLink()
		}
	}
}

// End link session between upstream server and downstream client.
func (g *gateway) dropLink() {
	if g.uStream != nil {
		g.uStream.Close()
	}
	if g.txBuff != nil {
		close(g.txBuff)
		g.txBuff = nil
	}
	g.state = Disconnected
}

// Stop activity and release downstream interface.
func (g *gateway) dropGateway() {
	g.dropLink()
	g.com.Close()
	close(g.rxBuff)
	g.state = TransportNotReady
}
