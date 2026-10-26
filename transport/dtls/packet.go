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
	"encoding/binary"
	"io"
	"net"
	"os"
	"sync"
	"time"

	"github.com/pion/transport/v5/deadline"
)

// packetListener gives Pion one bounded packet queue per source address.
// Admission occurs before allocating a Pion connection. Closing the listener
// stops admission while leaving accepted connections on the shared socket.
type packetListener struct {
	mu         sync.Mutex
	conn       *net.UDPConn
	peers      map[string]*packetConn
	acceptq    chan *packetConn
	done       chan struct{}
	closed     bool
	pending    int
	maxPending int
	timeout    time.Duration
	rate       float64
	credit     float64
	updated    time.Time
	writers    chan struct{}
}

func newPacketListener(conn *net.UDPConn, config *Config) *packetListener {
	rate := float64(config.handshakeRate())
	return &packetListener{
		conn: conn, peers: make(map[string]*packetConn),
		acceptq: make(chan *packetConn, min(128, config.maxPending())),
		done:    make(chan struct{}), writers: make(chan struct{}, 1),
		maxPending: config.maxPending(), timeout: config.timeout(),
		rate: rate, credit: rate, updated: time.Now(),
	}
}

func (l *packetListener) Accept() (net.PacketConn, net.Addr, error) {
	for {
		select {
		case <-l.done:
			return nil, nil, net.ErrClosed
		case c := <-l.acceptq:
			l.mu.Lock()
			select {
			case <-c.done:
				l.mu.Unlock()
				continue
			default:
			}
			if l.closed {
				l.mu.Unlock()
				return nil, nil, net.ErrClosed
			}
			c.accepted = true
			l.mu.Unlock()
			return c, c.remote, nil
		}
	}
}

func (l *packetListener) Addr() net.Addr { return l.conn.LocalAddr() }

func (l *packetListener) holdPeer(addr net.Addr) *packetConn {
	l.mu.Lock()
	defer l.mu.Unlock()
	c := l.peers[addr.String()]
	// Pion preserves the address supplied by Accept. Match that object as
	// well as its string: an expired connection's address may already have
	// been reused by a new ClientHello while Pion was constructing the old
	// connection. It must not acquire the replacement's admission slot.
	if c != nil && c.remote == addr {
		// Keep its admission slot until TLS resources have been reclaimed,
		// even if its packet connection expires before cleanup completes.
		c.held = true
		return c
	}
	return nil
}

func (l *packetListener) Close() error {
	l.mu.Lock()
	if l.closed {
		l.mu.Unlock()
		return nil
	}
	l.closed = true
	close(l.done)
	var pending []*packetConn
	for _, c := range l.peers {
		if !c.accepted {
			pending = append(pending, c)
		}
	}
	empty := len(l.peers) == 0
	l.mu.Unlock()
	for _, c := range pending {
		_ = c.Close()
	}
	if empty {
		return l.conn.Close()
	}
	return nil
}

func (l *packetListener) readLoop() {
	buf := make([]byte, 65535)
	for {
		n, remote, err := l.conn.ReadFromUDP(buf)
		if err != nil {
			_ = l.Close()
			l.mu.Lock()
			peers := make([]*packetConn, 0, len(l.peers))
			for _, c := range l.peers {
				peers = append(peers, c)
			}
			l.mu.Unlock()
			for _, c := range peers {
				_ = c.Close()
			}
			return
		}
		l.dispatch(buf[:n], remote)
	}
}

func (l *packetListener) dispatch(buf []byte, remote *net.UDPAddr) {
	l.mu.Lock()
	defer l.mu.Unlock()
	c := l.peers[remote.String()]
	if c == nil {
		if l.closed || !clientHello(buf) || l.pending >= l.maxPending || len(l.acceptq) == cap(l.acceptq) {
			return
		}
		now := time.Now()
		l.credit = min(l.rate, l.credit+now.Sub(l.updated).Seconds()*l.rate)
		l.updated = now
		if l.credit < 1 {
			return
		}
		l.credit--
		c = &packetConn{
			listener: l, remote: remote, expires: now.Add(l.timeout),
			packets: make(chan []byte, 16), done: make(chan struct{}), finished: make(chan struct{}),
			pending: true, readDeadline: deadline.New(), writeDeadline: deadline.New(),
		}
		l.peers[remote.String()] = c
		l.pending++
		c.timer = time.AfterFunc(l.timeout, func() { _ = c.close(true) })
		l.acceptq <- c
	}
	// Drop on overflow rather than stalling all peers on the shared socket.
	if len(c.packets) < cap(c.packets) {
		c.packets <- append([]byte(nil), buf...)
	}
}

// Screen the epoch-zero record and first ClientHello fragment, as NNG does.
// The TLS implementation validates the body and authenticates the peer.
func clientHello(buf []byte) bool {
	if len(buf) < 25 || buf[0] != 22 || buf[1] != 0xfe ||
		(buf[2] != 0xff && buf[2] != 0xfd) || buf[3] != 0 || buf[4] != 0 || buf[13] != 1 {
		return false
	}
	record := int(binary.BigEndian.Uint16(buf[11:]))
	total := int(buf[14])<<16 | int(buf[15])<<8 | int(buf[16])
	offset := int(buf[19])<<16 | int(buf[20])<<8 | int(buf[21])
	fragment := int(buf[22])<<16 | int(buf[23])<<8 | int(buf[24])
	return record >= 12 && record <= len(buf)-13 && offset == 0 &&
		total <= 16384 && fragment > 0 && fragment <= total && fragment <= record-12
}

type packetConn struct {
	listener *packetListener
	remote   *net.UDPAddr
	expires  time.Time
	packets  chan []byte
	done     chan struct{}
	finished chan struct{}
	// Protected by listener.mu.
	accepted      bool
	pending       bool
	held          bool
	timer         *time.Timer
	readDeadline  *deadline.Deadline
	writeDeadline *deadline.Deadline
	writeMu       sync.Mutex
	writing       bool
	writeTime     time.Time
}

func (c *packetConn) establish() bool {
	l := c.listener
	l.mu.Lock()
	defer l.mu.Unlock()
	select {
	case <-c.done:
		return false
	default:
	}
	if c.pending {
		c.pending = false
		l.pending--
		c.timer.Stop()
	}
	return true
}

func (c *packetConn) release() {
	l := c.listener
	l.mu.Lock()
	if c.pending {
		c.pending = false
		l.pending--
	}
	c.held = false
	l.mu.Unlock()
}

func (c *packetConn) LocalAddr() net.Addr { return c.listener.Addr() }

func (c *packetConn) ReadFrom(buf []byte) (int, net.Addr, error) {
	select {
	case <-c.done:
		return 0, nil, net.ErrClosed
	case <-c.readDeadline.Done():
		return 0, nil, os.ErrDeadlineExceeded
	default:
	}
	select {
	case <-c.done:
		return 0, nil, net.ErrClosed
	case <-c.readDeadline.Done():
		return 0, nil, os.ErrDeadlineExceeded
	case packet := <-c.packets:
		if len(packet) > len(buf) {
			return 0, nil, io.ErrShortBuffer
		}
		return copy(buf, packet), c.remote, nil
	}
}

func (c *packetConn) WriteTo(buf []byte, addr net.Addr) (int, error) {
	if addr == nil || addr.String() != c.remote.String() {
		return 0, net.InvalidAddrError("DTLS peer address changed")
	}
	l := c.listener
	select {
	case <-c.done:
		return 0, net.ErrClosed
	case <-c.writeDeadline.Done():
		return 0, os.ErrDeadlineExceeded
	case l.writers <- struct{}{}:
	}
	defer func() { <-l.writers }()
	c.writeMu.Lock()
	select {
	case <-c.done:
		c.writeMu.Unlock()
		return 0, net.ErrClosed
	default:
	}
	c.writing = true
	err := l.conn.SetWriteDeadline(c.writeTime)
	c.writeMu.Unlock()
	var n int
	if err == nil {
		n, err = l.conn.WriteToUDP(buf, c.remote)
	}
	c.writeMu.Lock()
	c.writing = false
	c.writeMu.Unlock()
	return n, err
}

func (c *packetConn) SetDeadline(t time.Time) error {
	_ = c.SetReadDeadline(t)
	return c.SetWriteDeadline(t)
}

func (c *packetConn) SetReadDeadline(t time.Time) error {
	c.readDeadline.Set(t)
	return nil
}

func (c *packetConn) SetWriteDeadline(t time.Time) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	c.writeTime = t
	c.writeDeadline.Set(t)
	if c.writing {
		return c.listener.conn.SetWriteDeadline(t)
	}
	return nil
}

func (c *packetConn) Close() error { return c.close(false) }

func (c *packetConn) close(pendingOnly bool) error {
	l := c.listener
	l.mu.Lock()
	select {
	case <-c.done:
		l.mu.Unlock()
		<-c.finished
		return nil
	default:
	}
	if pendingOnly && !c.pending {
		l.mu.Unlock()
		return nil
	}
	close(c.done)
	delete(l.peers, c.remote.String())
	if c.pending && !c.held {
		c.pending = false
		l.pending--
	}
	c.timer.Stop()
	last := l.closed && len(l.peers) == 0
	l.mu.Unlock()
	defer close(c.finished)
	c.readDeadline.Set(time.Time{})
	_ = c.SetWriteDeadline(time.Now())
	if last {
		return l.conn.Close()
	}
	return nil
}
