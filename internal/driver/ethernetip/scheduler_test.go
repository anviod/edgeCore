package ethernetip

import (
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"net"
	"testing"

	"github.com/anviod/edgeCore/internal/model"

	go_ethernet_ip "github.com/anviod/ethernet-ip"
	"github.com/anviod/ethernet-ip/messages/packet"
)

func TestResolveLogixTagNameAndClass2AttrID(t *testing.T) {
	scheduler := &ENIPScheduler{}

	testCases := []struct {
		name      string
		input     string
		wantName  string
		wantAttr  int
		wantFound bool
	}{
		{"simple", "IntTag", "IntTag", 3, true},
		{"program scoped", "Program:Main.IntTag", "IntTag", 3, true},
		{"controller scoped", "Controller.BoolTag", "BoolTag", 1, true},
		{"no match", "Program:Main.UnknownTag", "UnknownTag", 0, false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			gotName := scheduler.resolveLogixTagName(tc.input)
			if gotName != tc.wantName {
				t.Fatalf("resolved name mismatch: got %q, want %q", gotName, tc.wantName)
			}

			attrID, ok := scheduler.getLogixClass2AttrID(gotName)
			if ok != tc.wantFound {
				t.Fatalf("expected found=%v for %q, got %v", tc.wantFound, gotName, ok)
			}
			if ok && attrID != tc.wantAttr {
				t.Fatalf("expected attrID=%d for %q, got %d", tc.wantAttr, gotName, attrID)
			}
		})
	}
}

func TestWriteClass2AttributeWithFakeServer(t *testing.T) {
	listener, host, port := startFakeENIPServer(t)
	defer listener.Close()

	conn, err := go_ethernet_ip.NewTCP(host, &go_ethernet_ip.Config{TCPPort: uint16(port)})
	if err != nil {
		t.Fatalf("failed to create ENIP client: %v", err)
	}
	defer conn.Close()

	if err := conn.Connect(); err != nil {
		t.Fatalf("failed to connect to fake ENIP server: %v", err)
	}

	scheduler := &ENIPScheduler{}
	err = scheduler.writeClass2Attribute(conn, 3, []byte{0x01, 0x00})
	if err != nil {
		t.Fatalf("writeClass2Attribute failed: %v", err)
	}
}

func TestWritePointUsesLogixClass2Branch(t *testing.T) {
	listener, host, port := startFakeENIPServer(t)
	defer listener.Close()

	transport := &ENIPTransport{
		ip:             host,
		port:           port,
		connectionType: "logix",
	}

	scheduler := &ENIPScheduler{
		transport: transport,
		decoder:   NewENIPDecoder(),
	}

	point := model.Point{
		Name:     "Program:Main.IntTag",
		DataType: "INT",
	}

	if err := scheduler.WritePoint(context.Background(), point, int16(12345)); err != nil {
		t.Fatalf("WritePoint failed: %v", err)
	}
}

func startFakeENIPServer(t *testing.T) (net.Listener, string, int) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start fake ENIP server: %v", err)
	}

	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		for {
			header := make([]byte, 24)
			if _, err := io.ReadFull(conn, header); err != nil {
				return
			}

			cmd := binary.LittleEndian.Uint16(header[0:2])
			length := binary.LittleEndian.Uint16(header[2:4])
			senderContext := make([]byte, 8)
			copy(senderContext, header[8:16])
			body := make([]byte, length)
			if length > 0 {
				if _, err := io.ReadFull(conn, body); err != nil {
					return
				}
			}

			var specificData []byte
			switch cmd {
			case 0x65: // RegisterSession
				specificData = make([]byte, 4)
				binary.LittleEndian.PutUint16(specificData[0:2], 1)
				binary.LittleEndian.PutUint16(specificData[2:4], 0)
			case 0x66: // UnRegisterSession
				specificData = []byte{}
			case 0x6F: // SendRRData
				if !bytes.Contains(body, []byte{0x20, 0x02, 0x24, 0x01, 0x30, 0x03}) {
					t.Errorf("expected Class 2 attribute path in SendRRData request, got body: %x", body)
					return
				}

				responseData := make([]byte, 8)
				binary.LittleEndian.PutUint16(responseData[0:2], 0x90)
				// reserved = 0
				// general status = 0
				// size of additional status = 0

				cpf := packet.NewCommonPacketFormat([]packet.CommonPacketFormatItem{
					{TypeID: packet.ItemIDUCMM},
					{TypeID: packet.ItemIDUnconnectedMessage, Data: responseData},
				})
				sd := packet.SpecificData{InterfaceHandle: 0, TimeOut: 10, Packet: cpf}
				specificData = sd.Encode()
			default:
				t.Errorf("unexpected command: 0x%02x", cmd)
				return
			}

			response := make([]byte, 24+len(specificData))
			binary.LittleEndian.PutUint16(response[0:2], cmd)
			binary.LittleEndian.PutUint16(response[2:4], uint16(len(specificData)))
			binary.LittleEndian.PutUint32(response[4:8], 1)
			binary.LittleEndian.PutUint32(response[8:12], 0)
			copy(response[12:20], senderContext)
			binary.LittleEndian.PutUint32(response[20:24], 0)
			copy(response[24:], specificData)

			if _, err := conn.Write(response); err != nil {
				return
			}
		}
	}()

	addr := listener.Addr().(*net.TCPAddr)
	return listener, addr.IP.String(), addr.Port
}
