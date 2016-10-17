package protocol

import (
	"bytes"
	"github.com/RoanBrand/goBuffers"
	"io"
	"net"
	"strconv"
	"testing"
	"time"
)

const PORT = 3541

// This test setups the following:
// [TCP Echo Server] <--> [Protocol Gateway] <--> [Fake Serial Wire] <--> [Protocol Client]
// The Client then writes a single message to the Gateway, waits 1s, then reads any incoming data.
// The test compares the sent and received messages, expecting them to be equivalent.
func TestEcho(t *testing.T) {
	// start tcp server
	go startTCPServer(t)
	time.Sleep(time.Millisecond * 100)

	// start protocol gateway server
	serialTransport := NewFakeTransport()
	gateway := gateway{}
	go gateway.Listen(&fakeTransportServerInterface{fakeTransportInterface{serialTransport}})
	t.Log("Protocol Gateway started")

	// start protocol client
	endClient := client{}
	endClient.com = &fakeTransportClientInterface{fakeTransportInterface{serialTransport}}
	if res := endClient.Connect(&[4]byte{127, 0, 0, 1}, PORT); res != 1 {
		t.Fatalf("Protocol client unable to connect to gateway: %d", res)
	}
	t.Log("Protocol Client connected to Gateway")

	message := []byte(
		`This message will be sent from the client to the gateway,
		which in turn forwards it to the TCP server we created.
		The TCP server echoes back what is sent to it, so in the end
		we expext the gateway to send the same message back to the client.`)
	messageLength := len(message)

	nWritten := endClient.Write(message, messageLength)
	if nWritten != messageLength {
		t.Fatalf("Client write fail: Expected to send %v but sent %v instead\n", messageLength, nWritten)
	}
	t.Log("Client sent message to Gateway")

	time.Sleep(time.Second)

	t.Log("Client reading response")
	in := []byte{}
	for {
		inByte := endClient.Read()
		if inByte == -1 { // -1 if nothing to read
			break
		}
		in = append(in, byte(inByte))
	}

	if !bytes.Equal(in, message) {
		t.Fatalf("Sent message:(%v) is not equal to received one:(%v)\n", string(message), string(in))
	}
	t.Log("Client RX message equivalent to TX message. Successful.")
}

func startTCPServer(t *testing.T) {
	server, err := net.Listen("tcp", "127.0.0.1:"+strconv.Itoa(PORT))
	if server == nil {
		t.Fatal("TCP Server couldn't start listening: " + err.Error())
	}
	defer server.Close()
	conns := clientConns(t, server)
	t.Log("TCP Server started")
	for {
		go handleConn(t, <-conns)
	}
}

func clientConns(t *testing.T, listener net.Listener) chan net.Conn {
	ch := make(chan net.Conn)
	i := 0
	go func() {
		for {
			client, err := listener.Accept()
			if client == nil {
				t.Errorf("TCP Server couldn't accept: %v\n", err)
				continue
			}
			i++
			t.Logf("TCP Server accepted conn: #%d %v <-> %v\n", i, client.LocalAddr(), client.RemoteAddr())
			ch <- client
		}
	}()
	return ch
}

func handleConn(t *testing.T, client net.Conn) {
	defer client.Close()
	msg := make([]byte, 1024)
	for {
		n, err := client.Read(msg)
		if err == io.EOF {
			t.Logf("TCP Server: Received EOF (%d bytes ignored)\n", n)
			return
		} else if err != nil {
			t.Fatalf("TCP Server: Error reading from client: %v\n", err)
		}
		n, err = client.Write(msg[:n])
		if err != nil {
			t.Fatalf("TCP Server: Error writing to client: %v\n", err)
		}
	}
}

// Fake 2 way network connection.
type fakeTransport struct {
	Buf1 *goBuffers.BlockingReadWriter
	Buf2 *goBuffers.BlockingReadWriter
}

func NewFakeTransport() *fakeTransport {
	t := &fakeTransport{Buf1: goBuffers.NewBlockingReadWriter(), Buf2: goBuffers.NewBlockingReadWriter()}
	return t
}

// Interface for Protocol Server/Gateway/Client to the fake transport.
type fakeTransportInterface struct {
	transport *fakeTransport
}

func (ci fakeTransportInterface) Close() error {
	return nil
}

func (ci fakeTransportInterface) Flush() error {
	return nil
}

// Interface for Client to the fake transport. (On the one side)
type fakeTransportClientInterface struct {
	fakeTransportInterface
}

func (ci *fakeTransportClientInterface) Read(p []byte) (n int, err error) {
	n, err = ci.transport.Buf2.Read(p)
	return
}

func (ci *fakeTransportClientInterface) Write(p []byte) (n int, err error) {
	n, err = ci.transport.Buf1.Write(p)
	return
}

// Interface for Server/Gateway to the fake transport. (On the other side)
type fakeTransportServerInterface struct {
	fakeTransportInterface
}

func (si *fakeTransportServerInterface) Read(p []byte) (n int, err error) {
	n, err = si.transport.Buf1.Read(p)
	return
}

func (si *fakeTransportServerInterface) Write(p []byte) (n int, err error) {
	n, err = si.transport.Buf2.Write(p)
	return
}
