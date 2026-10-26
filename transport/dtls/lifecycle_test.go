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
	"context"
	"net"
	"sync"
	"testing"
	"time"

	pion "github.com/pion/dtls/v4"
	"github.com/pion/dtls/v4/pkg/protocol"
	"go.nanomsg.org/mangos/v3"
	"go.nanomsg.org/mangos/v3/internal/test"
)

func waitClosed(t *testing.T, p *pipe) {
	t.Helper()
	select {
	case <-p.done:
	case <-time.After(time.Second):
		t.Fatal("pipe did not close")
	}
}

func TestDTLSRejectsMalformedRecords(t *testing.T) {
	ids := testIdentities(t)
	for _, name := range []string{"short", "version", "opcode", "protocol", "size"} {
		t.Run(name, func(t *testing.T) {
			server, client := ids.configs()
			_, sp, cp := testPipes(t, Transport4, server, client, 16)
			buf := make([]byte, headerSize)
			header{op: opData, proto: cp.proto.Self}.encode(buf)
			switch name {
			case "short":
				buf = buf[:3]
			case "version":
				buf[0] = 2
			case "opcode":
				buf[1] = 255
			case "protocol":
				buf[2] ^= 1
			case "size":
				buf = append(buf, make([]byte, 17)...)
			}
			_, err := cp.conn.Write(buf)
			test.MustSucceed(t, err)
			waitClosed(t, sp)
		})
	}
}

func TestDTLSRefreshAndExpiry(t *testing.T) {
	server, client := testIdentities(t).configs()
	_, sp, cp := testPipes(t, Transport4, server, client, 0)
	buf := make([]byte, headerSize)
	header{op: opCReq, proto: cp.proto.Self, param: maxPayload, refresh: 1}.encode(buf)
	_, err := cp.conn.Write(buf)
	test.MustSucceed(t, err)
	limit := time.Now().Add(time.Second)
	for {
		sp.mu.Lock()
		serverRefresh := sp.refresh
		sp.mu.Unlock()
		cp.mu.Lock()
		clientRefresh := cp.refresh
		cp.mu.Unlock()
		if serverRefresh == time.Second && clientRefresh == time.Second {
			break
		}
		if time.Now().After(limit) {
			t.Fatal("refresh interval was not negotiated")
		}
		time.Sleep(time.Millisecond)
	}
	sp.mu.Lock()
	sp.expires = time.Now().Add(10 * time.Millisecond)
	sp.mu.Unlock()
	select {
	case sp.wake <- struct{}{}:
	default:
	}
	waitClosed(t, sp)
	waitClosed(t, cp)
}

func TestDTLSConcurrentClose(t *testing.T) {
	server, client := testIdentities(t).configs()
	l, sp, cp := testPipes(t, Transport4, server, client, 0)
	started := make(chan struct{}, 2)
	var wg sync.WaitGroup
	for _, p := range []*pipe{sp, cp} {
		wg.Add(2)
		go func() {
			defer wg.Done()
			started <- struct{}{}
			for {
				m := mangos.NewMessage(0)
				m.Body = append(m.Body, "message"...)
				if err := p.Send(m); err != nil {
					m.Free()
					return
				}
			}
		}()
		go func() {
			defer wg.Done()
			for {
				m, err := p.Recv()
				if err != nil {
					return
				}
				m.Free()
			}
		}()
	}
	<-started
	<-started
	// Exercise concurrent Close callers and shared listener teardown.
	for _, p := range []*pipe{sp, cp, sp, cp} {
		wg.Add(1)
		go func() { defer wg.Done(); _ = p.Close() }()
	}
	_ = l.Close()
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("concurrent shutdown stalled")
	}
}

func captureClientHello(t *testing.T, client *Config) []byte {
	t.Helper()
	capture, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	test.MustSucceed(t, err)
	defer capture.Close()
	raw, err := net.ListenUDP("udp4", nil)
	test.MustSucceed(t, err)
	conn, err := pion.Client(raw, capture.LocalAddr(), client.clientOptions("localhost")...)
	test.MustSucceed(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- conn.HandshakeContext(ctx) }()
	test.MustSucceed(t, capture.SetReadDeadline(time.Now().Add(time.Second)))
	buf := make([]byte, 65535)
	n, _, err := capture.ReadFromUDP(buf)
	cancel()
	_ = raw.Close()
	_ = conn.Close()
	<-done
	test.MustSucceed(t, err)
	if !clientHello(buf[:n]) {
		t.Fatal("captured datagram was not a ClientHello")
	}
	return buf[:n]
}

func TestDTLSStalledHandshakeAndClose(t *testing.T) {
	serverConfig, clientConfig := testIdentities(t).configs()
	hello := captureClientHello(t, clientConfig)
	serverConfig.ConnectTimeout = time.Second
	server, client := test.GetMockSocket(), test.GetMockSocket()
	defer test.MustClose(t, server)
	defer test.MustClose(t, client)
	li, err := Transport4.NewListener("dtls4://127.0.0.1:0", server)
	test.MustSucceed(t, err)
	l := li.(*listener)
	defer l.Close()
	test.MustSucceed(t, l.SetOption(mangos.OptionDTLSConfig, serverConfig))
	test.MustSucceed(t, l.Listen())
	stalled, err := net.DialUDP("udp4", nil, l.packets.Addr().(*net.UDPAddr))
	test.MustSucceed(t, err)
	defer stalled.Close()
	_, err = stalled.Write(hello)
	test.MustSucceed(t, err)
	test.MustSucceed(t, stalled.SetReadDeadline(time.Now().Add(time.Second)))
	_, err = stalled.Read(make([]byte, 65535)) // cookie challenge; never answer it
	test.MustSucceed(t, err)
	d, err := Transport4.NewDialer(l.Address(), client)
	test.MustSucceed(t, err)
	test.MustSucceed(t, d.SetOption(mangos.OptionDTLSConfig, clientConfig))
	cp, err := d.Dial()
	test.MustSucceed(t, err)
	defer cp.Close()
	sp, err := l.Accept()
	test.MustSucceed(t, err)
	defer sp.Close()
	sendReceive(t, cp, sp, 1200)
	l.packets.mu.Lock()
	pending := l.packets.pending
	l.packets.mu.Unlock()
	if pending != 1 {
		t.Fatalf("expected one stalled handshake, got %d", pending)
	}
	test.MustSucceed(t, l.Close())
	sendReceive(t, cp, sp, 1200)
	l.packets.mu.Lock()
	pending = l.packets.pending
	l.packets.mu.Unlock()
	if pending != 0 {
		t.Fatalf("Close retained %d pending connections", pending)
	}
}

func TestDTLSNegotiationSurvivesLoss(t *testing.T) {
	serverConfig, clientConfig := testIdentities(t).configs()
	serverConfig.ServerOptions = append(serverConfig.ServerOptions, pion.WithMaxVersion(protocol.Version1_2), pion.WithFlightInterval(20*time.Millisecond))
	clientConfig.ClientOptions = append(clientConfig.ClientOptions, pion.WithMaxVersion(protocol.Version1_2), pion.WithFlightInterval(20*time.Millisecond))
	server, client := test.GetMockSocket(), test.GetMockSocket()
	defer test.MustClose(t, server)
	defer test.MustClose(t, client)
	l, err := Transport4.NewListener("dtls4://127.0.0.1:0", server)
	test.MustSucceed(t, err)
	defer l.Close()
	test.MustSucceed(t, l.SetOption(mangos.OptionDTLSConfig, serverConfig))
	test.MustSucceed(t, l.Listen())
	serverAddr, err := net.ResolveUDPAddr("udp4", l.Address()[len("dtls4://"):])
	test.MustSucceed(t, err)
	relay, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	test.MustSucceed(t, err)
	done := make(chan struct{})
	var droppedHello, droppedRequest, droppedAck bool
	go func() {
		defer close(done)
		buf := make([]byte, 65535)
		var clientAddr *net.UDPAddr
		for {
			n, from, err := relay.ReadFromUDP(buf)
			if err != nil {
				return
			}
			if from.String() == serverAddr.String() {
				if n > 0 && buf[0] == 23 && !droppedAck {
					droppedAck = true
					continue
				}
				if clientAddr != nil {
					_, _ = relay.WriteToUDP(buf[:n], clientAddr)
				}
			} else {
				clientAddr = from
				if n > 0 && buf[0] == 22 && !droppedHello {
					droppedHello = true
					continue
				}
				if n > 0 && buf[0] == 23 && !droppedRequest {
					droppedRequest = true
					continue
				}
				_, _ = relay.WriteToUDP(buf[:n], serverAddr)
			}
		}
	}()
	defer func() { _ = relay.Close(); <-done }()
	d, err := Transport4.NewDialer("dtls4://"+relay.LocalAddr().String(), client)
	test.MustSucceed(t, err)
	test.MustSucceed(t, d.SetOption(mangos.OptionDTLSConfig, clientConfig))
	cp, err := d.Dial()
	test.MustSucceed(t, err)
	defer cp.Close()
	sp, err := l.Accept()
	test.MustSucceed(t, err)
	defer sp.Close()
	sendReceive(t, cp, sp, 1200)
	_ = relay.Close()
	<-done
	if !droppedHello || !droppedRequest || !droppedAck {
		t.Fatal("did not exercise both DTLS and NNG retransmission")
	}
}
