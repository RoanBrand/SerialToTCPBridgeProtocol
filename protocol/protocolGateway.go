package protocol

import (
	"log"
	"net"
	"time"
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
		go g.packetSender()
		// log.Printf("Gateway: Connected to %v\n", dstStr)
		g.state = Connected
		g.txBuff <- Packet{command: connack}
	case disconnect:
		if g.state == Connected {
			log.Println("Client wants to disconnect. Ending link session")
			g.dropLink()
		}
	}
}

// Publish data downstream received from upstream tcp server.
// We need to get an Ack before sending the next publish packet.
// Resend same publish packet after timeout, and kill link after 5 retries.
func (g *gateway) packetSender() {
	defer g.session.Done()
	sequenceTxFlag := false
	retries := 0
	tx := make([]byte, 512)
	for {
		nRx, err := g.uStream.Read(tx)
		if err != nil {
			if g.state == Connected {
				log.Printf("Error receiving upstream: %v. Disconnecting client\n", err)
				g.txBuff <- Packet{command: disconnect}
				g.dropLink()
			}
			return
		}
		p := Packet{command: publish, payload: tx[:nRx]}
		if sequenceTxFlag {
			p.command |= 0x80
		}
	PUB_LOOP:
		for {
			g.txBuff <- p
			select {
			case ack, ok := <-g.acknowledgeEvent:
				if ok && ack == sequenceTxFlag {
					retries = 0
					sequenceTxFlag = !sequenceTxFlag
					break PUB_LOOP // success
				}
			case <-time.After(time.Millisecond * 500):
				retries++
				if retries >= 5 {
					log.Println("Too many downstream send retries. Disconnecting client")
					g.txBuff <- Packet{command: disconnect}
					g.dropLink()
					return
				}
			}
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
