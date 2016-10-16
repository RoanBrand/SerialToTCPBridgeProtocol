package protocol

import (
	"bytes"
	"errors"
	"log"
	"time"
)

// Implementation of the Protocol Client.
type client struct {
	protocolTransport // Connection between Protocol Client & Server/Gateway.
	userData_rx       bytes.Buffer
	userData_tx       chan Packet
}

// Public Protocol Client API
// Meant to match Arduino client API for now. Not Go idiomatic.
func (c *client) Connect(IP *[4]byte, port uint16) int {
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
	go c.rxSerial(nil)
	go c.packetParser(c.handleRxPacket, c.Stop)
	go c.txSerial(nil)
	//c.session.Wait()

	p := IP[:]
	p = append(p, byte(port&0x00FF), byte((port>>8)&0x00FF))
	c.txBuff <- Packet{command: connect, payload: p}
	/*
		logLock.Lock()
		log.Println("Client: Connect request sent to", *IP, port)
		logLock.Unlock()
	*/

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
	/*
	 * SUCCESS 1
	 * TIMED_OUT -1
	 * INVALID_SERVER -2
	 * TRUNCATED -3
	 * INVALID_RESPONSE -4
	 */
}

func (c *client) Connected() bool {
	return c.state == Connected
}

func (c *client) Available() int {
	return c.userData_rx.Len()
}

func (c *client) Read() int {
	if c.Available() == 0 {
		return -1
	}
	b, err := c.userData_rx.ReadByte()
	if err != nil {
		c.Stop()
	}
	return int(b)
}

func (c *client) Write(payload []byte, pLength int) int {
	if c.state != Connected {
		return 0
	}

	c.userData_tx <- Packet{command: publish, payload: payload[:pLength]}
	return pLength
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
	// close(c.acknowledgeEvent)
	c.state = Disconnected
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
		if c.state != Disconnected {
			return
		}

		c.state = Connected
		c.acknowledgeEvent <- true
		c.session.Add(1)
		go c.packetSender(func() (p Packet, err error) {
			p, ok := <-c.userData_tx
			if ok {
				err = nil
			} else {
				err = errors.New("channel closed")
			}
			return
		}, c.Stop)
		// log.Println("Client: Connected")
	case disconnect:
		if c.state == Connected {
			log.Println("Client wants to disconnect. Ending link session")
			c.Stop()
		}
	}
}
