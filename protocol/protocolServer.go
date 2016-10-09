package protocol

import (
	"bytes"
	"encoding/binary"
	"log"
	"net"
	"strconv"
	"time"
)

// Implementation of the Protocol Server.
type server struct {
	uStream net.Conn          // Upstream connection to tcp Server.
	dStream protocolTransport // Downstream connection to protocol Client.
	protocolSession           // Connection session between protocol Server/Gateway & Client.
}

// Initialize downstream RX and listen for a protocol Client.
func (s *server) Listen(ds protocolTransport) {
	s.dStream = ds
	s.rxBuff = make(chan byte, 512)
	s.acknowledgeEvent = make(chan bool)
	s.state = Disconnected

	s.session.Add(2)
	go s.rxDownstream()
	go s.packetReader()
	s.session.Wait()
}

// Receive from downstream and write to RX buffer.
func (s *server) rxDownstream() {
	defer s.session.Done()
	rx := make([]byte, 128)
	s.dStream.Flush()
	for {
		nRx, err := s.dStream.Read(rx)
		if err != nil {
			log.Printf("Error receiving downstream: %v\n", err)
			s.dropServer()
			return
		}
		for _, v := range rx[:nRx] {
			s.rxBuff <- v
		}
	}
}

// Parse RX buffer for legitimate packets.
func (s *server) packetReader() {
	defer s.session.Done()
	timeouts := 0
PACKET_RX_LOOP:
	for {
		if timeouts >= 5 {
			if s.state == Connected {
				log.Println("Downstream RX timeout. Disconnecting client")
				s.txBuff <- Packet{command: disconnect}
				s.dropLink()
				return
			}
			timeouts = 0
		}

		p := Packet{}
		var ok bool

		// Length byte
		p.length, ok = <-s.rxBuff
		if !ok {
			return
		}

		// Command byte
		select {
		case p.command, ok = <-s.rxBuff:
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
			case payloadByte, ok := <-s.rxBuff:
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
			case crcByte, ok := <-s.rxBuff:
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
			log.Println("Downstream packet RX CRCFAIL")
			timeouts++
			continue PACKET_RX_LOOP
		}
		timeouts = 0
		s.handleRxPacket(&p)
	}
}

// Packet RX done. Handle it.
func (s *server) handleRxPacket(packet *Packet) {
	var rxSeqFlag bool = (packet.command & 0x80) > 0
	switch packet.command & 0x7F {
	case publish:
		// Payload from serial client
		if s.state == Connected {
			s.txBuff <- Packet{command: acknowledge | (packet.command & 0x80)}
			if rxSeqFlag == s.expectedRxSeqFlag {
				s.expectedRxSeqFlag = !s.expectedRxSeqFlag
				_, err := s.uStream.Write(packet.payload)
				if err != nil {
					log.Printf("Error sending upstream: %v Disconnecting client\n", err)
					s.txBuff <- Packet{command: disconnect}
					s.dropLink()
				}
			}
		}
	case acknowledge:
		if s.state == Connected {
			s.acknowledgeEvent <- rxSeqFlag
		}
	case connect:
		if s.state != Disconnected {
			return
		}
		if len(packet.payload) != 6 {
			return
		}

		port := binary.LittleEndian.Uint16(packet.payload[4:])

		var dst bytes.Buffer
		for i := 0; i < 3; i++ {
			dst.WriteString(strconv.Itoa(int(packet.payload[i])))
			dst.WriteByte('.')
		}
		dst.WriteString(strconv.Itoa(int(packet.payload[3])))
		dst.WriteByte(':')
		dst.WriteString(strconv.Itoa(int(port)))
		dstStr := dst.String()

		s.txBuff = make(chan Packet, 2)
		s.expectedRxSeqFlag = false

		// Start downstream TX
		s.session.Add(1)
		go s.txDownstream()

		// Open connection to upstream server on behalf of client
		log.Printf("Dialing to: %v\n", dstStr)
		var err error
		if s.uStream, err = net.Dial("tcp", dstStr); err != nil { // TODO: add timeout
			log.Printf("Failed to connect to: %v\n", dstStr)
			s.txBuff <- Packet{command: disconnect} // TODO: payload to contain error or timeout
			s.dropLink()
			return
		}

		// Start link session
		s.session.Add(1)
		go s.packetSender()
		log.Printf("Connected to %v\n", dstStr)
		s.state = Connected
		s.txBuff <- Packet{command: connack}
	case disconnect:
		if s.state == Connected {
			log.Println("Client wants to disconnect. Ending link session")
			s.dropLink()
		}
	}
}

// Publish data downstream received from upstream tcp server.
// We need to get an Ack before sending the next publish packet.
// Resend same publish packet after timeout, and kill link after 5 retries.
func (s *server) packetSender() {
	defer s.session.Done()
	sequenceTxFlag := false
	retries := 0
	tx := make([]byte, 512)
	for {
		nRx, err := s.uStream.Read(tx)
		if err != nil {
			if s.state == Connected {
				log.Printf("Error receiving upstream: %v. Disconnecting client\n", err)
				s.txBuff <- Packet{command: disconnect}
				s.dropLink()
			}
			return
		}
		p := Packet{command: publish, payload: tx[:nRx]}
		if sequenceTxFlag {
			p.command |= 0x80
		}
	PUB_LOOP:
		for {
			s.txBuff <- p
			select {
			case ack, ok := <-s.acknowledgeEvent:
				if ok && ack == sequenceTxFlag {
					retries = 0
					sequenceTxFlag = !sequenceTxFlag
					break PUB_LOOP // success
				}
			case <-time.After(time.Millisecond * 500):
				retries++
				if retries >= 5 {
					log.Println("Too many downstream send retries. Disconnecting client")
					s.txBuff <- Packet{command: disconnect}
					s.dropLink()
					return
				}
			}
		}
	}
}

// Read from TX buffer and write out downstream.
func (s *server) txDownstream() {
	defer s.session.Done()
	for txPacket := range s.txBuff {
		txPacket.length = byte(len(txPacket.payload) + 5)
		txPacket.crc = txPacket.calcCrc()
		serialPacket := txPacket.serialize()

		nTx, err := s.dStream.Write(serialPacket)
		if err != nil {
			log.Printf("Error writing downstream: %v\n", err)
			s.dropServer()
			return
		}

		if nTx != len(serialPacket) {
			log.Printf("TX mismatch. Want to send %v bytes. Sent: %v bytes.", len(serialPacket), nTx)
		}
	}
}

// End link session between upstream server and downstream client.
func (s *server) dropLink() {
	if s.uStream != nil {
		s.uStream.Close()
	}
	if s.txBuff != nil {
		close(s.txBuff)
		s.txBuff = nil
	}
	s.state = Disconnected
}

// Stop activity and release downstream interface.
func (s *server) dropServer() {
	s.dropLink()
	s.dStream.Close()
	close(s.rxBuff)
	s.state = TransportNotReady
}
