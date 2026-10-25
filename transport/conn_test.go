// Copyright 2026 The Mangos Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the license at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package transport

import (
	"encoding/binary"
	"io"
	"net"
	"testing"
	"time"
)

const testHandshakeTimeout = 100 * time.Millisecond

func newTestConn(c net.Conn, timeout time.Duration) *conn {
	return &conn{
		c:                c,
		handshakeTimeout: timeout,
		options:          make(map[string]interface{}),
	}
}

func TestConnHandshakerCloseAbortsPendingHandshake(t *testing.T) {
	server, client := net.Pipe()
	defer client.Close()

	h := NewConnHandshaker()
	h.Start(newTestConn(server, 5*time.Second))

	// Consume the local header so the handshaker is blocked waiting for the
	// peer's header when it is closed.
	header := make([]byte, 8)
	if _, err := io.ReadFull(client, header); err != nil {
		t.Fatalf("reading handshake header: %v", err)
	}

	h.Close()
	errQ := make(chan error, 1)
	go func() {
		_, err := client.Read(make([]byte, 1))
		errQ <- err
	}()
	select {
	case err := <-errQ:
		if err == nil {
			t.Fatal("pending handshake connection remained open")
		}
	case <-time.After(time.Second):
		t.Fatal("pending handshake connection was not closed")
	}
}

func TestConnHandshakerTimesOut(t *testing.T) {
	server, client := net.Pipe()
	defer client.Close()

	h := NewConnHandshaker()
	defer h.Close()
	h.Start(newTestConn(server, testHandshakeTimeout))

	header := make([]byte, 8)
	if _, err := io.ReadFull(client, header); err != nil {
		t.Fatalf("reading handshake header: %v", err)
	}

	errQ := make(chan error, 1)
	go func() {
		_, err := h.Wait()
		errQ <- err
	}()
	select {
	case err := <-errQ:
		if err == nil {
			t.Fatal("silent peer did not cause the handshake to time out")
		} else if ne, ok := err.(net.Error); !ok || !ne.Timeout() {
			t.Fatalf("handshake error = %v, want timeout", err)
		}
	case <-time.After(time.Second):
		t.Fatal("silent peer did not cause the handshake to time out")
	}
}

func TestConnHandshakeClearsDeadline(t *testing.T) {
	server, client := net.Pipe()
	defer client.Close()

	h := NewConnHandshaker()
	h.Start(newTestConn(server, testHandshakeTimeout))

	header := make([]byte, 8)
	if _, err := io.ReadFull(client, header); err != nil {
		t.Fatalf("reading handshake header: %v", err)
	}
	if _, err := client.Write([]byte{0, 'S', 'P', 0, 0, 0, 0, 0}); err != nil {
		t.Fatalf("writing handshake header: %v", err)
	}
	p, err := h.Wait()
	if err != nil {
		t.Fatalf("waiting for handshake: %v", err)
	}

	// The deadline only applies to the handshake.  A message that arrives
	// after it expires must still be received.
	time.Sleep(2 * testHandshakeTimeout)
	errQ := make(chan error, 1)
	go func() {
		m, err := p.Recv()
		if err == nil {
			defer m.Free()
			if string(m.Body) != "ok" {
				err = io.ErrUnexpectedEOF
			}
		}
		errQ <- err
	}()
	msg := make([]byte, 10)
	binary.BigEndian.PutUint64(msg, 2)
	copy(msg[8:], "ok")
	if err := client.SetWriteDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatalf("setting write deadline: %v", err)
	}
	if _, err := client.Write(msg); err != nil {
		t.Fatalf("writing message: %v", err)
	}
	if err := <-errQ; err != nil {
		t.Fatalf("receiving message after handshake deadline: %v", err)
	}
}
