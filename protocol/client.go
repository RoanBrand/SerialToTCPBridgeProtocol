package protocol

import (
	"bytes"
	"errors"
	"log"
	"net"
	"strconv"
	"sync"
	"time"
)

// Implementation of the Protocol Client.
type client struct {
	protocolTransport // Connection between Protocol Client & Server/Gateway.
	rxBuffer          bytes.Buffer
	rxBufLock         sync.RWMutex
	txBuffer          chan Packet
}

// Dial connection to server.
func Dial(com serialInterface, address string) (net.Conn, error) {
	if com == nil {
		return nil, errors.New("No serial com interface provided")
	}

	c := client{}
	c.com = com
	c.rxBuff = make(chan byte, 512)
	c.acknowledgeEvent = make(chan bool)
	c.state = Disconnected
	c.txBuff = make(chan Packet, 2)
	c.expectedRxSeqFlag = false
	c.txBuffer = make(chan Packet, 10)

	c.session.Add(3)
	go c.rxSerial(nil)
	go c.packetParser(c.handleRxPacket, c.Stop)
	go c.txSerial(nil)

	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, errors.New("Invalid address (" + err.Error() + "). Must be: IP|Host:Port")
	}

	var cmd byte = connect
	var connPayload []byte
	ip := net.ParseIP(host)
	if ip == nil {
		// Hostname string
		cmd |= 0x80
		connPayload = []byte(host)
	} else {
		// IPv4
		connPayload = ip.To4()
	}
	portNum, _ := strconv.Atoi(port)
	connPayload = append(connPayload, byte(portNum&0x00FF), byte((portNum>>8)&0x00FF))

	c.txBuff <- Packet{command: cmd, payload: connPayload}
	select {
	case reply, ok := <-c.acknowledgeEvent:
		if ok && reply {
			return &c, nil
		}
	case <-time.After(time.Second * 5):
	}

	return nil, errors.New("Timed out while dialing server")
}

func (c *client) Read(b []byte) (n int, err error) {
	if c.Available() == 0 {
		return 0, nil
	}
	c.rxBufLock.Lock()
	rByte, err := c.rxBuffer.ReadByte()
	c.rxBufLock.Unlock()
	if err != nil {
		return 0, err
	}
	b[0] = rByte
	return 1, nil
}

func (c *client) Write(b []byte) (n int, err error) {
	if c.state != Connected {
		return 0, errors.New("Not connected")
	}

	c.txBuffer <- Packet{command: publish, payload: b}
	return len(b), nil
}

func (c *client) Close() error {
	c.Stop()
	return nil
}

// To satisfy net.Conn interface
func (c *client) LocalAddr() net.Addr {
	return nil
}

func (c *client) RemoteAddr() net.Addr {
	return nil
}

func (c *client) SetDeadline(t time.Time) error {
	return nil
}

func (c *client) SetReadDeadline(t time.Time) error {
	return nil
}

func (c *client) SetWriteDeadline(t time.Time) error {
	return nil
}

func (c *client) Available() int {
	c.rxBufLock.Lock()
	defer c.rxBufLock.Unlock()
	return c.rxBuffer.Len()
}

func (c *client) Connected() bool {
	return c.state == Connected
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
			c.rxBufLock.Lock()
			c.rxBuffer.Write(packet.payload)
			c.rxBufLock.Unlock()
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
			p, ok := <-c.txBuffer
			if ok {
				err = nil
			} else {
				err = errors.New("channel closed")
			}
			return
		}, c.Stop)
	case disconnect:
		if c.state == Connected {
			log.Println("Client wants to disconnect. Ending link session")
			c.Stop()
		}
	}
}
