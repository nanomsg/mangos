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

package dtls

import (
	"bytes"
	"encoding/binary"
	"errors"
	"net"
	"os"
	"testing"
	"time"

	"go.nanomsg.org/mangos/v3/internal/test"
)

// A plausible initial fragment. Only Pion may decide if its body is valid;
// the admission screen should allocate nothing for other record types.
func helloFragment() []byte {
	b := make([]byte, 26)
	b[0], b[1], b[2], b[12], b[13], b[16], b[24] = 22, 0xfe, 0xfd, 13, 1, 1, 1
	return b
}

func TestClientHelloScreen(t *testing.T) {
	if !clientHello(helloFragment()) {
		t.Fatal("initial fragment rejected")
	}
	for _, mutate := range []func([]byte){
		func(b []byte) { b[0] = 23 },
		func(b []byte) { b[2] = 0xfc },
		func(b []byte) { b[4] = 1 },
		func(b []byte) { b[13] = 2 },
		func(b []byte) { b[14] = 1 },
		func(b []byte) { b[21] = 1 },
		func(b []byte) { b[24] = 0 },
		func(b []byte) { b[24] = 2 },
		func(b []byte) { b[12] = 255 },
	} {
		b := helloFragment()
		mutate(b)
		if clientHello(b) {
			t.Fatalf("invalid ClientHello admitted: %x", b)
		}
	}
	for n := 0; n < 25; n++ {
		if clientHello(make([]byte, n)) {
			t.Fatal("short ClientHello admitted")
		}
	}
}

func packetListenerForTest(t *testing.T, config *Config) *packetListener {
	t.Helper()
	raw, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	test.MustSucceed(t, err)
	l := newPacketListener(raw, config)
	t.Cleanup(func() {
		_ = l.Close()
		l.mu.Lock()
		var peers []*packetConn
		for _, c := range l.peers {
			peers = append(peers, c)
		}
		l.mu.Unlock()
		for _, c := range peers {
			_ = c.Close()
		}
	})
	return l
}

func peerAddress(port int) *net.UDPAddr { return &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: port} }

func TestPacketAdmissionLimits(t *testing.T) {
	l := packetListenerForTest(t, &Config{MaxPending: 1, HandshakeRate: 2})
	one, two := peerAddress(10001), peerAddress(10002)
	l.dispatch([]byte("garbage"), one)
	if len(l.peers) != 0 {
		t.Fatal("garbage allocated a peer")
	}
	l.dispatch(helloFragment(), one)
	l.dispatch(helloFragment(), two)
	if len(l.peers) != 1 || l.pending != 1 {
		t.Fatal("pending limit not enforced")
	}
	c, _, err := l.Accept()
	test.MustSucceed(t, err)
	if !c.(*packetConn).establish() {
		t.Fatal("failed to establish peer")
	}
	l.dispatch(helloFragment(), two)
	if len(l.peers) != 2 || l.pending != 1 {
		t.Fatal("established peer consumed a pending slot")
	}
	second, _, err := l.Accept()
	test.MustSucceed(t, err)
	test.MustSucceed(t, second.Close())
	l.dispatch(helloFragment(), peerAddress(10003))
	if len(l.peers) != 1 {
		t.Fatal("handshake rate limit not enforced")
	}
	// Existing authenticated sources bypass new-connection admission limits.
	l.dispatch([]byte("existing peer"), one)
	if len(c.(*packetConn).packets) != 2 {
		t.Fatal("existing peer was rate limited")
	}
	// Simulate replenished token credit without a wall-clock sleep.
	l.updated = time.Now().Add(-time.Second)
	l.dispatch(helloFragment(), peerAddress(10003))
	if len(l.peers) != 2 {
		t.Fatal("handshake credit did not replenish")
	}
}

func TestPacketPendingExpires(t *testing.T) {
	l := packetListenerForTest(t, &Config{ConnectTimeout: 20 * time.Millisecond})
	l.dispatch(helloFragment(), peerAddress(10001))
	conn, _, err := l.Accept()
	test.MustSucceed(t, err)
	c := conn.(*packetConn)
	select {
	case <-c.done:
	case <-time.After(time.Second):
		t.Fatal("pending peer did not expire")
	}
	l.mu.Lock()
	pending, peers := l.pending, len(l.peers)
	l.mu.Unlock()
	if pending != 0 || peers != 0 {
		t.Fatal("expired handshake retained resources")
	}
}

func TestPacketAdmissionHoldsSlotDuringCleanup(t *testing.T) {
	l := packetListenerForTest(t, &Config{MaxPending: 1})
	one, two := peerAddress(10001), peerAddress(10002)
	l.dispatch(helloFragment(), one)
	conn, _, err := l.Accept()
	test.MustSucceed(t, err)
	c := l.holdPeer(one)
	if c != conn {
		t.Fatal("missing admitted connection")
	}
	test.MustSucceed(t, c.Close())
	l.dispatch(helloFragment(), two)
	if l.pending != 1 || len(l.peers) != 0 {
		t.Fatal("cleanup released admission slot early")
	}
	c.release()
	l.dispatch(helloFragment(), two)
	if l.pending != 1 || len(l.peers) != 1 {
		t.Fatal("cleanup retained admission slot")
	}
}

func TestPacketPeerAddressReuse(t *testing.T) {
	l := packetListenerForTest(t, &Config{})
	oldAddr := peerAddress(10001)
	l.dispatch(helloFragment(), oldAddr)
	old, _, err := l.Accept()
	test.MustSucceed(t, err)
	test.MustSucceed(t, old.Close())
	newAddr := peerAddress(10001)
	l.dispatch(helloFragment(), newAddr)
	if l.holdPeer(oldAddr) != nil {
		t.Fatal("expired peer acquired a replacement connection")
	}
	if l.holdPeer(newAddr) == nil {
		t.Fatal("replacement connection lost its admission slot")
	}
	// No TLS connection was allocated in this packet-layer test.
	l.peers[newAddr.String()].release()
}

func TestPacketQueuesAndDeadlines(t *testing.T) {
	l := packetListenerForTest(t, &Config{})
	remote, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	test.MustSucceed(t, err)
	defer remote.Close()
	addr := remote.LocalAddr().(*net.UDPAddr)
	l.dispatch(helloFragment(), addr)
	conn, _, err := l.Accept()
	test.MustSucceed(t, err)
	c := conn.(*packetConn)
	for i := 0; i < 100; i++ {
		l.dispatch([]byte("packet"), addr)
	}
	if len(c.packets) != cap(c.packets) {
		t.Fatal("packet queue was not bounded")
	}
	for len(c.packets) != 0 {
		<-c.packets
	}
	test.MustSucceed(t, c.SetReadDeadline(time.Now().Add(10*time.Millisecond)))
	_, _, err = c.ReadFrom(make([]byte, 100))
	if !errors.Is(err, os.ErrDeadlineExceeded) {
		t.Fatalf("read deadline: %v", err)
	}
	test.MustSucceed(t, c.SetReadDeadline(time.Time{}))
	l.dispatch([]byte("after deadline"), addr)
	buf := make([]byte, 100)
	n, _, err := c.ReadFrom(buf)
	test.MustSucceed(t, err)
	if string(buf[:n]) != "after deadline" {
		t.Fatal("deadline reset lost packet")
	}
	test.MustSucceed(t, c.SetWriteDeadline(time.Now().Add(-time.Second)))
	_, err = c.WriteTo([]byte("expired"), addr)
	if !errors.Is(err, os.ErrDeadlineExceeded) {
		t.Fatalf("write deadline: %v", err)
	}
	test.MustSucceed(t, c.SetWriteDeadline(time.Time{}))
	_, err = c.WriteTo([]byte("reply"), addr)
	test.MustSucceed(t, err)
	test.MustSucceed(t, remote.SetReadDeadline(time.Now().Add(time.Second)))
	n, _, err = remote.ReadFromUDP(buf)
	test.MustSucceed(t, err)
	if string(buf[:n]) != "reply" {
		t.Fatal("write deadline reset lost packet")
	}
	_, err = c.WriteTo(buf, peerAddress(addr.Port+1))
	if err == nil {
		t.Fatal("packet routed to a different peer")
	}
}

func TestNNGHeader(t *testing.T) {
	buf := make([]byte, headerSize)
	header{op: opCAck, proto: 0x1234, param: 0x5678, refresh: 0x9abc}.encode(buf)
	want := []byte{1, 2, 0x34, 0x12, 0x78, 0x56, 0xbc, 0x9a}
	if !bytes.Equal(buf, want) {
		t.Fatalf("header=%x, want %x", buf, want)
	}
	h, ok := decodeHeader(buf)
	if !ok || h.proto != 0x1234 || h.param != 0x5678 || h.refresh != 0x9abc {
		t.Fatal("bad header decode")
	}
	// DATA's size field is little-endian and includes the SP header.
	header{op: opData, param: maxPayload}.encode(buf)
	if binary.LittleEndian.Uint16(buf[4:]) != maxPayload {
		t.Fatal("incorrect message size")
	}
}
