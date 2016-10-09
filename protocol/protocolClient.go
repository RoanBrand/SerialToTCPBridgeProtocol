package protocol

import (
	"bytes"
	"encoding/binary"
	"log"
	"time"
)

// Implementation of the Protocol Client.
type client struct {
	protocolSession                   // Connection session between Protocol Server/Gateway & Client.
	com             protocolTransport // Connection to Protocol Server/Gateway.
	userData_rx     bytes.Buffer
	userData_tx     chan Packet
}

/*
 * SUCCESS 1
 * TIMED_OUT -1
 * INVALID_SERVER -2
 * TRUNCATED -3
 * INVALID_RESPONSE -4
 */
func (c *client) Connect(IP []byte, port uint16) int {
	if c.com == nil {
		return 0
	}

	c.rxBuff = make(chan byte, 512)
	c.acknowledgeEvent = make(chan bool)
	c.state = Disconnected

	c.txBuff = make(chan Packet, 2)
	c.expectedRxSeqFlag = false

	c.userData_tx = make(chan Packet, 10)

	c.session.Add(3)
	go c.rxSerial()
	go c.packetReader()
	go c.txSerial()
	//c.session.Wait()

	p := IP
	p = append(p, byte(port&0x00FF), byte((port>>8)&0x00FF))
	c.txBuff <- Packet{command: connect, payload: p}

	select {
	case reply, ok := <-c.acknowledgeEvent:
		if ok && reply {
			return 1
		}
	case <-time.After(time.Second * 5):
		c.Stop()
		return -1
	}

	return -4
}

func (c *client) Connected() bool {
	return c.state == Connected
}

func (c *client) Available() int {
	return c.userData_rx.Len()
}

func (c *client) Read() int {
	if c.Available() > 0 {
		b, err := c.userData_rx.ReadByte()
		if err != nil {
			c.Stop()
		}
		return int(b)
	}
	return 0
}

func (c *client) Write(payload []byte, pLength int) int {
/*
		paySlice := make([]byte, pLength)
		for i := 0; i < pLength; i++ {
			paySlice[i] = payload[i]
		}
*/
	if c.state != Connected {
		return 0
	}

	select {
	case c.userData_tx <- Packet{command: publish, payload: payload[:pLength]}:
		return pLength
	default:
		return 0
	}
}

func (c *client) Flush() {
	c.userData_rx.Reset()
}

func (c *client) Stop() {
	if c.txBuff != nil {
		close(c.txBuff)
		c.txBuff = nil
	}
	if c.rxBuff != nil {
		close(c.rxBuff)
		c.rxBuff = nil
	}
	//close(c.acknowledgeEvent)
	c.state = Disconnected
}

// Receive from Protocol Server over Serial interface.
func (c *client) rxSerial() {
	defer c.session.Done()
	rx := make([]byte, 128)
	c.com.Flush()
	for {
		nRx, err := c.com.Read(rx)
		if err != nil {
			log.Printf("Error receiving from COM: %v\n", err)
			// c.dropServer() what we need here? equivalent of dropping server: maybe just drop port
			return
		}
		for _, v := range rx[:nRx] {
			c.rxBuff <- v
		}
	}
}

// Read from TX buffer and write out Serial interface.
func (c *client) txSerial() {
	defer c.session.Done()
	for txPacket := range c.txBuff {
		txPacket.length = byte(len(txPacket.payload) + 5)
		txPacket.crc = txPacket.calcCrc()
		serialPacket := txPacket.serialize()

		nTx, err := c.com.Write(serialPacket)
		if err != nil {
			log.Printf("Error writing downstream: %v\n", err)
			// c.dropServer()
			return
		}

		if nTx != len(serialPacket) {
			log.Printf("TX mismatch. Want to send %v bytes. Sent: %v bytes.", len(serialPacket), nTx)
		}
	}
}

// Parse RX buffer for legitimate packets.
func (c *client) packetReader() {
	defer c.session.Done()
	timeouts := 0
PACKET_RX_LOOP:
	for {
		if timeouts >= 5 {
			if c.state == Connected {
				log.Println("COM timeout. Disconnecting from server")
				c.txBuff <- Packet{command: disconnect}
				c.Stop()
				return
			}
			timeouts = 0
		}

		p := Packet{}
		var ok bool

		// Length byte
		p.length, ok = <-c.rxBuff
		if !ok {
			return
		}

		// Command byte
		select {
		case p.command, ok = <-c.rxBuff:
			if !ok {
				return
			}
		case <-time.After(time.Millisecond * 100):
			timeouts++
			continue PACKET_RX_LOOP // discard
		}

		// Payload
		for i := 0; i < int(p.length)-5; i++ {
			select {
			case payloadByte, ok := <-c.rxBuff:
				if !ok {
					return
				}
				p.payload = append(p.payload, payloadByte)
			case <-time.After(time.Millisecond * 100):
				timeouts++
				continue PACKET_RX_LOOP
			}
		}

		// CRC32
		rxCrc := make([]byte, 0, 4)
		for i := 0; i < 4; i++ {
			select {
			case crcByte, ok := <-c.rxBuff:
				if !ok {
					return
				}
				rxCrc = append(rxCrc, crcByte)
			case <-time.After(time.Millisecond * 100):
				timeouts++
				continue PACKET_RX_LOOP
			}
		}
		p.crc = binary.LittleEndian.Uint32(rxCrc)

		// Integrity Checking
		if p.calcCrc() != p.crc {
			log.Println("COM packet RX CRCFAIL")
			timeouts++
			continue PACKET_RX_LOOP
		}
		timeouts = 0
		c.handleRxPacket(&p)
	}
}

// Packet RX done. Handle it.
func (c *client) handleRxPacket(packet *Packet) {
	var rxSeqFlag bool = (packet.command & 0x80) > 0
	switch packet.command & 0x7F {
	case publish:
		// Payload from serial client
		if c.state != Connected {
			return
		}

		c.txBuff <- Packet{command: acknowledge | (packet.command & 0x80)}
		if rxSeqFlag == c.expectedRxSeqFlag {
			c.expectedRxSeqFlag = !c.expectedRxSeqFlag
			c.userData_rx.Write(packet.payload)
		}
	case acknowledge:
		if c.state == Connected {
			c.acknowledgeEvent <- rxSeqFlag
		}
	case connack:
		log.Println("client got connack!!!!")
		if c.state != Disconnected {
			return
		}

		c.state = Connected
		c.acknowledgeEvent <- true
		c.session.Add(1)
		go c.packetSender()
	case disconnect:
		if c.state == Connected {
			log.Println("Client wants to disconnect. Ending link session")
			c.Stop()
		}
	}
}

// Publish data from user app over Serial interface.
// We need to get an Ack before sending the next publish packet.
// Resend same publish packet after timeout, and kill link after 5 retries.
func (c *client) packetSender() {
	defer c.session.Done()
	sequenceTxFlag := false
	retries := 0
	for {
		p, ok := <-c.userData_tx
		if !ok {
			if c.state == Connected {
				//log.Printf("Error receiving upstream: %v. Disconnecting client\n", err)
				c.txBuff <- Packet{command: disconnect}
				//c.dropLink()
			}
			return
		}
		if sequenceTxFlag {
			p.command |= 0x80
		}
	PUB_LOOP:
		for {
			c.txBuff <- p
			select {
			case ack, ok := <-c.acknowledgeEvent:
				if ok && ack == sequenceTxFlag {
					retries = 0
					sequenceTxFlag = !sequenceTxFlag
					break PUB_LOOP // success
				}
			case <-time.After(time.Millisecond * 500):
				retries++
				if retries >= 5 {
					log.Println("Too many downstream send retries. Disconnecting client")
					c.txBuff <- Packet{command: disconnect}
					c.Stop()
					return
				}
			}
		}
	}
}
