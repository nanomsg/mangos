// Copyright 2026 The Mangos Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package udp

import (
	"bytes"
	"fmt"
	"net"
	"os"
	"os/exec"
	"testing"
	"time"

	"go.nanomsg.org/mangos/v3"
	. "go.nanomsg.org/mangos/v3/internal/test"
	"go.nanomsg.org/mangos/v3/protocol/pair"
)

func TestPacketWireFormat(t *testing.T) {
	p := packet{opcode: opData, proto: 0x1234, length: 0xddee, refresh: 0x5566}
	b := make([]byte, udpHeaderSize)
	p.marshal(b)
	want := []byte{1, 0, 0x34, 0x12, 0xee, 0xdd, 0x66, 0x55}
	if !bytes.Equal(b, want) {
		t.Fatalf("wire packet = %x, want %x", b, want)
	}
	got, ok := unmarshalPacket(b)
	if !ok || got != p {
		t.Fatalf("unmarshal = %#v, %v; want %#v, true", got, ok, p)
	}
}

func TestUDP4SendReceive(t *testing.T) {
	testUDPSendReceive(t, Transport4, "udp4://127.0.0.1:0")
}

func TestUDPSendReceive(t *testing.T) {
	testUDPSendReceive(t, Transport, "udp://127.0.0.1:0")
}

func testUDPSendReceive(t *testing.T, tran udpTran, addr string) {
	t.Helper()
	server := GetMockSocket()
	client := GetMockSocket()
	defer MustClose(t, server)
	defer MustClose(t, client)

	l, err := tran.NewListener(addr, server)
	MustSucceed(t, err)
	MustSucceed(t, l.Listen())
	defer func() { MustSucceed(t, l.Close()) }()

	d, err := tran.NewDialer(l.Address(), client)
	MustSucceed(t, err)
	cp, err := d.Dial()
	MustSucceed(t, err)
	defer func() { MustSucceed(t, cp.Close()) }()
	sp, err := l.Accept()
	MustSucceed(t, err)
	defer func() { MustSucceed(t, sp.Close()) }()

	msg := mangos.NewMessage(0)
	msg.Header = append(msg.Header, "header:"...)
	msg.Body = append(msg.Body, "payload"...)
	MustSucceed(t, cp.Send(msg))
	got, err := sp.Recv()
	MustSucceed(t, err)
	if string(got.Body) != "header:payload" {
		t.Fatalf("received %q, want %q", got.Body, "header:payload")
	}
	got.Free()

	msg = mangos.NewMessage(0)
	msg.Body = append(msg.Body, "reply"...)
	MustSucceed(t, sp.Send(msg))
	got, err = cp.Recv()
	MustSucceed(t, err)
	if string(got.Body) != "reply" {
		t.Fatalf("received %q, want %q", got.Body, "reply")
	}
	got.Free()

	// Both endpoints must be able to send a maximum-sized message, even
	// on systems whose default UDP send buffer is smaller than a datagram.
	payload := bytes.Repeat([]byte("x"), udpMaxPayload-len("header:"))
	want := append([]byte("header:"), payload...)
	for _, direction := range [][2]mangos.TranPipe{{cp, sp}, {sp, cp}} {
		msg = mangos.NewMessage(0)
		msg.Header = append(msg.Header, "header:"...)
		msg.Body = append(msg.Body, payload...)
		MustSucceed(t, direction[0].Send(msg))
		got, err = direction[1].Recv()
		MustSucceed(t, err)
		if !bytes.Equal(got.Body, want) {
			t.Fatalf("maximum-sized message = %d bytes, want %d matching bytes", len(got.Body), len(want))
		}
		got.Free()
	}
}

func TestUDP6SendReceive(t *testing.T) {
	if !ipv6Available() {
		t.Skip("IPv6 loopback is unavailable")
	}
	testUDPSendReceive(t, Transport6, "udp6://[::1]:0")
}

func TestUDPSchemeAndOptions(t *testing.T) {
	for _, tran := range []udpTran{Transport, Transport4, Transport6} {
		sock := GetMockSocket()
		l, err := tran.NewListener(string(tran)+"://:0", sock)
		MustSucceed(t, err)
		v, err := l.GetOption(mangos.OptionMaxRecvSize)
		MustSucceed(t, err)
		if v.(int) != udpMaxPayload {
			t.Fatalf("%s default max receive = %d, want %d", tran, v, udpMaxPayload)
		}
		MustSucceed(t, l.SetOption(mangos.OptionMaxRecvSize, 123))
		v, err = l.GetOption(mangos.OptionMaxRecvSize)
		MustSucceed(t, err)
		if v.(int) != 123 {
			t.Fatalf("%s max receive = %d, want 123", tran, v)
		}
		MustClose(t, sock)
	}
}

func TestUDPRejectsWildcardDial(t *testing.T) {
	sock := GetMockSocket()
	defer MustClose(t, sock)
	if _, err := Transport4.NewDialer("udp4://*:9999", sock); err == nil {
		t.Fatal("wildcard UDP dial unexpectedly succeeded")
	}
}

func TestListenerCloseKeepsEstablishedPipes(t *testing.T) {
	server := GetMockSocket()
	client := GetMockSocket()
	defer MustClose(t, server)
	defer MustClose(t, client)

	l, err := Transport4.NewListener("udp4://127.0.0.1:0", server)
	MustSucceed(t, err)
	MustSucceed(t, l.Listen())
	d, err := Transport4.NewDialer(l.Address(), client)
	MustSucceed(t, err)
	cp, err := d.Dial()
	MustSucceed(t, err)
	defer func() { MustSucceed(t, cp.Close()) }()
	sp, err := l.Accept()
	MustSucceed(t, err)
	defer func() { MustSucceed(t, sp.Close()) }()

	MustSucceed(t, l.Close())
	if _, err := l.Accept(); err != mangos.ErrClosed {
		t.Fatalf("Accept after Close = %v, want %v", err, mangos.ErrClosed)
	}
	msg := mangos.NewMessage(0)
	msg.Body = append(msg.Body, "still-connected"...)
	MustSucceed(t, cp.Send(msg))
	got, err := sp.Recv()
	MustSucceed(t, err)
	if string(got.Body) != "still-connected" {
		t.Fatalf("received %q, want %q", got.Body, "still-connected")
	}
	got.Free()
}

func TestListenerCapsAndExpiresPeers(t *testing.T) {
	sock := GetMockSocket()
	defer MustClose(t, sock)
	li, err := Transport4.NewListener("udp4://127.0.0.1:0", sock)
	MustSucceed(t, err)
	l := li.(*listener)
	l.maxPeers = 1
	MustSucceed(t, l.Listen())
	defer func() { MustSucceed(t, l.Close()) }()

	remote, err := net.ResolveUDPAddr("udp4", l.Address()[len("udp4://"):])
	MustSucceed(t, err)
	first, err := net.DialUDP("udp4", nil, remote)
	MustSucceed(t, err)
	defer func() { MustSucceed(t, first.Close()) }()
	second, err := net.DialUDP("udp4", nil, remote)
	MustSucceed(t, err)
	defer func() { MustSucceed(t, second.Close()) }()

	creq := make([]byte, udpHeaderSize)
	packet{opcode: opCReq, proto: l.proto.Peer, length: 100, refresh: 5}.marshal(creq)
	_, err = first.Write(creq)
	MustSucceed(t, err)
	waitForPeers(t, l, 1)
	_, err = second.Write(creq)
	MustSucceed(t, err)
	time.Sleep(20 * time.Millisecond)
	l.mu.Lock()
	count := len(l.pipes)
	if count != 1 {
		l.mu.Unlock()
		t.Fatalf("peer count = %d, want 1", count)
	}
	var p *pipe
	for _, p = range l.pipes {
	}
	p.mu.Lock()
	p.expires = time.Now().Add(-time.Second)
	p.mu.Unlock()
	l.mu.Unlock()
	l.reapExpired(time.Now())
	waitForPeers(t, l, 0)
}

func TestDialerHonorsNegotiatedRefresh(t *testing.T) {
	server := GetMockSocket()
	defer MustClose(t, server)
	c, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	MustSucceed(t, err)
	defer func() { MustSucceed(t, c.Close()) }()
	go func() {
		b := make([]byte, udpHeaderSize)
		n, remote, err := c.ReadFromUDP(b)
		if err != nil || n != udpHeaderSize {
			return
		}
		reply := make([]byte, udpHeaderSize)
		packet{opcode: opCAck, proto: server.Info().Peer, length: udpMaxPayload, refresh: 1}.marshal(reply)
		_, _ = c.WriteToUDP(reply, remote)
	}()

	d, err := Transport4.NewDialer("udp4://"+c.LocalAddr().String(), server)
	MustSucceed(t, err)
	connected, err := d.Dial()
	MustSucceed(t, err)
	p := connected.(*pipe)
	defer func() { MustSucceed(t, p.Close()) }()
	if got := p.refreshInterval(); got != time.Second {
		t.Fatalf("refresh interval = %v, want %v", got, time.Second)
	}
}

func TestDialerExpiresWithoutPeerRefresh(t *testing.T) {
	sock := GetMockSocket()
	defer MustClose(t, sock)
	proto := sock.Info()
	p := newPipe(proto, udpMaxPayload, &net.UDPAddr{}, &net.UDPAddr{}, func(b []byte) (int, error) {
		return len(b), nil
	}, nil)
	p.mu.Lock()
	p.refresh = time.Millisecond
	p.expires = time.Now().Add(-time.Millisecond)
	p.mu.Unlock()
	go p.refreshDialer()
	select {
	case <-p.closeq:
	case <-time.After(time.Second):
		t.Fatal("dialer pipe did not expire")
	}
}

func TestOversizeInboundDataClosesPipe(t *testing.T) {
	sock := GetMockSocket()
	defer MustClose(t, sock)
	proto := sock.Info()
	sent := make(chan []byte, 1)
	p := newPipe(proto, 1, &net.UDPAddr{}, &net.UDPAddr{}, func(b []byte) (int, error) {
		sent <- append([]byte(nil), b...)
		return len(b), nil
	}, nil)
	p.handleData(packet{opcode: opData, proto: proto.Peer, length: 2}, []byte{1, 2})
	select {
	case <-p.closeq:
	case <-time.After(time.Second):
		t.Fatal("oversize packet did not close the pipe")
	}
	b := <-sent
	h, ok := unmarshalPacket(b)
	if !ok || h.opcode != opDisc || h.length != discMsgSize {
		t.Fatalf("disconnect packet = %#v, %v", h, ok)
	}
}

func waitForPeers(t *testing.T, l *listener, want int) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		l.mu.Lock()
		got := len(l.pipes)
		l.mu.Unlock()
		if got == want {
			return
		}
		time.Sleep(time.Millisecond)
	}
	l.mu.Lock()
	got := len(l.pipes)
	l.mu.Unlock()
	t.Fatalf("peer count = %d, want %d", got, want)
}

// TestNNGInterop exercises both connection directions against nngcat from an
// actual NNG build. It is opt-in so ordinary mangos test runs do not require
// an NNG installation. Set MANGOS_NNGCAT to the nngcat executable path.
func TestNNGInterop(t *testing.T) {
	nngcat := os.Getenv("MANGOS_NNGCAT")
	if nngcat == "" {
		t.Skip("MANGOS_NNGCAT is not set")
	}
	if _, err := os.Stat(nngcat); err != nil {
		t.Fatalf("NNG cat %q: %v", nngcat, err)
	}

	for _, tc := range []struct {
		name   string
		scheme string
		host   string
		nng    string
		goOp   string
	}{
		{name: "udp_nng_dials_mangos_listener", scheme: "udp", host: "127.0.0.1", nng: "--dial", goOp: "listen"},
		{name: "udp_mangos_dials_nng_listener", scheme: "udp", host: "127.0.0.1", nng: "--listen", goOp: "dial"},
		{name: "udp4_nng_dials_mangos_listener", scheme: "udp4", host: "127.0.0.1", nng: "--dial", goOp: "listen"},
		{name: "udp4_mangos_dials_nng_listener", scheme: "udp4", host: "127.0.0.1", nng: "--listen", goOp: "dial"},
		{name: "udp6_nng_dials_mangos_listener", scheme: "udp6", host: "[::1]", nng: "--dial", goOp: "listen"},
		{name: "udp6_mangos_dials_nng_listener", scheme: "udp6", host: "[::1]", nng: "--listen", goOp: "dial"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.scheme == "udp6" && !ipv6Available() {
				t.Skip("IPv6 loopback is unavailable")
			}
			sock, err := pair.NewSocket()
			MustSucceed(t, err)
			defer MustClose(t, sock)
			MustSucceed(t, sock.SetOption(mangos.OptionRecvDeadline, 5*time.Second))
			MustSucceed(t, sock.SetOption(mangos.OptionSendDeadline, 5*time.Second))

			addr := fmt.Sprintf("%s://%s:%d", tc.scheme, tc.host, NextPort())
			if tc.goOp == "listen" {
				MustSucceed(t, sock.Listen(addr))
			}

			cmd := exec.Command(nngcat, "--pair0", tc.nng, addr, "--data", "from-nng", "--recv-timeout", "5", "--send-timeout", "5", "--quoted")
			var output bytes.Buffer
			cmd.Stdout = &output
			cmd.Stderr = &output
			MustSucceed(t, cmd.Start())
			if tc.goOp == "dial" {
				time.Sleep(100 * time.Millisecond)
				MustSucceed(t, sock.Dial(addr))
			}

			msg, err := sock.RecvMsg()
			MustSucceed(t, err)
			if string(msg.Body) != "from-nng" {
				t.Fatalf("NNG message = %q, want %q", msg.Body, "from-nng")
			}
			msg.Free()
			MustSucceed(t, sock.Send([]byte("from-mangos")))
			MustSucceed(t, cmd.Wait())
			if !bytes.Contains(output.Bytes(), []byte("from-mangos")) {
				t.Fatalf("nngcat output %q does not contain mangos reply", output.String())
			}
		})
	}
}

func ipv6Available() bool {
	c, err := net.ListenUDP("udp6", &net.UDPAddr{IP: net.IPv6loopback, Port: 0})
	if err != nil {
		return false
	}
	_ = c.SetDeadline(time.Now())
	return c.Close() == nil
}
