package protocol_test

import (
	"bytes"
	"github.com/RoanBrand/SerialToTCPBridgeProtocol/protocol"
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
	gateway := protocol.Gateway{}
	go gateway.Listen(&fakeTransportServerInterface{serialTransport})
	t.Log("Protocol Gateway started")

	// start protocol client
	endClient, err := protocol.Dial(&fakeTransportClientInterface{serialTransport}, "127.0.0.1:"+strconv.Itoa(PORT))
	if err != nil {
		t.Fatalf("Protocol client unable to connect to gateway: %v", err)
	}
	t.Log("Protocol Client connected to Gateway")

	message := []byte(
		`This message will be sent from the client to the gateway,
		which in turn forwards it to the TCP server we created.
		The TCP server echoes back what is sent to it, so in the end
		we expext the gateway to send the same message back to the client.`)
	messageLength := len(message)

	for i := 1; i <= 5; i++ {
		nWritten, err := endClient.Write(message)
		if err != nil {
			t.Fatalf("Client write fail: %v\n", err)
		}
		if nWritten != messageLength {
			t.Fatalf("Client write fail: Expected to send %v but sent %v instead\n", messageLength, nWritten)
		}
		t.Logf("Client sent message #%d to Gateway. Now waiting for response.\n", i)

		startTime := time.Now()
		in := []byte{}
		for {
			if time.Now().Sub(startTime) > time.Second {
				if !bytes.Equal(in, message) {
					t.Fatalf("Client timed out waiting for correct response. Received so far:\n%s\n%v", string(in), in)
				}
			}
			inByte := make([]byte, 1)
			nRx, err := endClient.Read(inByte)
			if err != nil {
				t.Fatalf("Client read fail: %v\n", err)
			}
			if nRx == 0 {
				continue
			}
			in = append(in, inByte...)
			if bytes.Equal(in, message) {
				break
			}
		}
		t.Logf("Client received message #%d. Successful.\n", i)
	}
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

func (ci fakeTransport) Close() error {
	return nil
}

func (ci fakeTransport) Flush() error {
	return nil
}

// Interface for Client to the fake transport. (On the one side)
type fakeTransportClientInterface struct {
	*fakeTransport
}

func (ci *fakeTransportClientInterface) Read(p []byte) (n int, err error) {
	n, err = ci.Buf2.Read(p)
	return
}

func (ci *fakeTransportClientInterface) Write(p []byte) (n int, err error) {
	n, err = ci.Buf1.Write(p)
	return
}

// Interface for Server/Gateway to the fake transport. (On the other side)
type fakeTransportServerInterface struct {
	*fakeTransport
}

func (si *fakeTransportServerInterface) Read(p []byte) (n int, err error) {
	n, err = si.Buf1.Read(p)
	return
}

func (si *fakeTransportServerInterface) Write(p []byte) (n int, err error) {
	n, err = si.Buf2.Write(p)
	return
}
