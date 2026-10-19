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

// Package udp implements NNG's UDP transport for mangos. To enable it simply
// import it.
package udp

import (
	"encoding/binary"
	"errors"
	"net"
	"strings"
	"sync"
	"time"

	"go.nanomsg.org/mangos/v3"
	"go.nanomsg.org/mangos/v3/transport"
)

const (
	// Transport is a transport.Transport for the NNG udp scheme.
	Transport = udpTran("udp")
	// Transport4 is a transport.Transport for the NNG udp4 scheme.
	Transport4 = udpTran("udp4")
	// Transport6 is a transport.Transport for the NNG udp6 scheme.
	Transport6 = udpTran("udp6")

	udpHeaderSize   = 8
	udpMaxPayload   = 65000
	udpMaxPeers     = 1024
	udpRefresh      = 5 * time.Second
	udpConnectWait  = 5 * time.Second
	udpExpireFactor = 5
)

const (
	opData uint8 = iota
	opCReq
	opCAck
	opDisc
)

const (
	discMsgSize = 4
	discNoBuf   = 8
)

func init() {
	transport.RegisterTransport(Transport)
	transport.RegisterTransport(Transport4)
	transport.RegisterTransport(Transport6)
}

// packet is the fixed, little-endian NNG UDP transport header. The two
// parameters hold the payload size for data, or the receive limit and refresh
// interval for CREQ and CACK packets.
type packet struct {
	opcode  uint8
	proto   uint16
	length  uint16
	refresh uint16
}

func (p packet) marshal(dst []byte) {
	dst[0] = 1
	dst[1] = p.opcode
	binary.LittleEndian.PutUint16(dst[2:], p.proto)
	binary.LittleEndian.PutUint16(dst[4:], p.length)
	binary.LittleEndian.PutUint16(dst[6:], p.refresh)
}

func unmarshalPacket(src []byte) (packet, bool) {
	if len(src) < udpHeaderSize || src[0] != 1 {
		return packet{}, false
	}
	return packet{
		opcode:  src[1],
		proto:   binary.LittleEndian.Uint16(src[2:]),
		length:  binary.LittleEndian.Uint16(src[4:]),
		refresh: binary.LittleEndian.Uint16(src[6:]),
	}, true
}

type udpTran string

func (t udpTran) Scheme() string { return string(t) }

func (t udpTran) network() string {
	switch t {
	case Transport4:
		return "udp4"
	case Transport6:
		return "udp6"
	default:
		return "udp"
	}
}

func (t udpTran) NewDialer(addr string, sock mangos.Socket) (transport.Dialer, error) {
	addr, err := transport.StripScheme(t, addr)
	if err != nil {
		return nil, err
	}
	if strings.HasPrefix(addr, "*") || strings.ContainsAny(addr, "/?#") {
		return nil, mangos.ErrBadAddr
	}
	remote, err := net.ResolveUDPAddr(t.network(), addr)
	if err != nil || remote.Port == 0 || remote.IP == nil {
		return nil, mangos.ErrBadAddr
	}
	return &dialer{network: t.network(), remote: remote, proto: sock.Info(), maxRecv: udpMaxPayload}, nil
}

func (t udpTran) NewListener(addr string, sock mangos.Socket) (transport.Listener, error) {
	addr, err := transport.StripScheme(t, addr)
	if err != nil {
		return nil, err
	}
	if strings.ContainsAny(addr, "/?#") {
		return nil, mangos.ErrBadAddr
	}
	addr = strings.TrimPrefix(addr, "*")
	local, err := net.ResolveUDPAddr(t.network(), addr)
	if err != nil {
		return nil, mangos.ErrBadAddr
	}
	return &listener{
		network: t.network(), local: local, proto: sock.Info(), maxRecv: udpMaxPayload, maxPeers: udpMaxPeers,
		acceptq: make(chan *pipe, 16), closeq: make(chan struct{}), pipes: make(map[string]*pipe),
		doneq: make(chan struct{}),
	}, nil
}

type dialer struct {
	network string
	remote  *net.UDPAddr
	proto   transport.ProtocolInfo
	maxRecv int
	mu      sync.Mutex
	closed  bool
}

func (d *dialer) Dial() (transport.Pipe, error) {
	d.mu.Lock()
	if d.closed {
		d.mu.Unlock()
		return nil, mangos.ErrClosed
	}
	maxRecv := d.maxRecv
	d.mu.Unlock()

	c, err := net.DialUDP(d.network, nil, d.remote)
	if err != nil {
		return nil, err
	}
	p := newPipe(d.proto, maxRecv, c.LocalAddr(), c.RemoteAddr(), func(b []byte) (int, error) { return c.Write(b) }, func() error { return c.Close() })

	deadline := time.Now().Add(udpConnectWait)
	buf := make([]byte, udpHeaderSize+udpMaxPayload)
	for {
		if err := p.sendCReq(); err != nil {
			_ = p.Close()
			return nil, err
		}
		if err := c.SetReadDeadline(time.Now().Add(time.Second / 5)); err != nil {
			_ = p.Close()
			return nil, err
		}
		for {
			n, err := c.Read(buf)
			if err != nil {
				if ne, ok := err.(net.Error); ok && ne.Timeout() && time.Now().Before(deadline) {
					break
				}
				_ = p.Close()
				if errors.Is(err, net.ErrClosed) {
					return nil, mangos.ErrClosed
				}
				return nil, err
			}
			h, ok := unmarshalPacket(buf[:n])
			if !ok || h.opcode != opCAck {
				continue
			}
			if h.proto != d.proto.Peer {
				_ = p.Close()
				return nil, mangos.ErrBadProto
			}
			if h.refresh == 0 {
				_ = p.Close()
				return nil, mangos.ErrBadProto
			}
			p.mu.Lock()
			p.sendMax = int(h.length)
			p.mu.Unlock()
			p.setRefresh(h.refresh)
			_ = c.SetReadDeadline(time.Time{})
			go p.readConnected(c)
			go p.refreshDialer()
			return p, nil
		}
	}
}

func (d *dialer) SetOption(name string, value interface{}) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	return setMaxRecv(&d.maxRecv, name, value)
}

func (d *dialer) GetOption(name string) (interface{}, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if name == mangos.OptionMaxRecvSize {
		return d.maxRecv, nil
	}
	return nil, mangos.ErrBadOption
}

type listener struct {
	network string
	local   *net.UDPAddr
	proto   transport.ProtocolInfo
	maxRecv int

	mu       sync.Mutex
	conn     *net.UDPConn
	pipes    map[string]*pipe
	maxPeers int
	acceptq  chan *pipe
	closeq   chan struct{}
	doneq    chan struct{}
	closed   bool
}

func (l *listener) Listen() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return mangos.ErrClosed
	}
	if l.conn != nil {
		return mangos.ErrAddrInUse
	}
	c, err := net.ListenUDP(l.network, l.local)
	if err != nil {
		return err
	}
	l.conn = c
	go l.readLoop(c)
	go l.reapLoop()
	return nil
}

func (l *listener) Address() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.conn != nil {
		return stringScheme(l.network) + "://" + l.conn.LocalAddr().String()
	}
	return stringScheme(l.network) + "://" + l.local.String()
}

func stringScheme(network string) string { return network }

func (l *listener) Accept() (transport.Pipe, error) {
	l.mu.Lock()
	closed := l.closed
	l.mu.Unlock()
	if closed {
		return nil, mangos.ErrClosed
	}
	select {
	case p := <-l.acceptq:
		l.mu.Lock()
		closed = l.closed
		l.mu.Unlock()
		if closed {
			_ = p.Close()
			return nil, mangos.ErrClosed
		}
		return p, nil
	case <-l.closeq:
		return nil, mangos.ErrClosed
	}
}

func (l *listener) Close() error {
	l.mu.Lock()
	if l.closed {
		l.mu.Unlock()
		return nil
	}
	l.closed = true
	close(l.closeq)
	var pending []*pipe
	for {
		select {
		case p := <-l.acceptq:
			pending = append(pending, p)
		default:
			goto drained
		}
	}

drained:
	var c *net.UDPConn
	if len(l.pipes) == 0 {
		c = l.closeConnLocked()
	}
	l.mu.Unlock()
	for _, p := range pending {
		_ = p.Close()
	}
	if c != nil {
		_ = c.Close()
	}
	return nil
}

func (l *listener) SetOption(name string, value interface{}) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	return setMaxRecv(&l.maxRecv, name, value)
}

func (l *listener) GetOption(name string) (interface{}, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if name == mangos.OptionMaxRecvSize {
		return l.maxRecv, nil
	}
	return nil, mangos.ErrBadOption
}

func (l *listener) readLoop(c *net.UDPConn) {
	buf := make([]byte, udpHeaderSize+udpMaxPayload)
	for {
		n, remote, err := c.ReadFromUDP(buf)
		if err != nil {
			return
		}
		packetBytes := append([]byte(nil), buf[:n]...)
		l.handlePacket(packetBytes, remote)
	}
}

func (l *listener) reapLoop() {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case now := <-ticker.C:
			l.reapExpired(now)
		case <-l.doneq:
			return
		}
	}
}

func (l *listener) reapExpired(now time.Time) {
	l.mu.Lock()
	expired := make([]*pipe, 0)
	for _, p := range l.pipes {
		if p.expired(now) {
			expired = append(expired, p)
		}
	}
	l.mu.Unlock()
	for _, p := range expired {
		_ = p.Close()
	}
}

// closeConnLocked returns the shared socket to close after releasing l.mu.
// It is only called when this listener has stopped accepting and no active
// logical UDP pipes remain.
func (l *listener) closeConnLocked() *net.UDPConn {
	if l.conn == nil {
		return nil
	}
	c := l.conn
	l.conn = nil
	close(l.doneq)
	return c
}

func (l *listener) handlePacket(b []byte, remote *net.UDPAddr) {
	h, ok := unmarshalPacket(b)
	if !ok {
		return
	}
	l.mu.Lock()
	key := remote.String()
	p := l.pipes[key]
	if p == nil && l.closed {
		l.mu.Unlock()
		return
	}
	if h.opcode == opCReq && p == nil && h.proto == l.proto.Peer && h.refresh != 0 {
		if len(l.pipes) >= l.maxPeers {
			l.mu.Unlock()
			l.sendDisc(remote, discNoBuf)
			return
		}
		// Keep the socket alive for this pipe's send function.  The listener
		// clears l.conn once its final pipe is removed, so the closure must not
		// consult l.conn after dropping l.mu.
		conn := l.conn
		p = newPipe(l.proto, l.maxRecv, conn.LocalAddr(), remote, func(data []byte) (int, error) { return conn.WriteToUDP(data, remote) }, nil)
		p.sendMax = int(h.length)
		p.setRefresh(h.refresh)
		p.remove = l.remove
		l.pipes[key] = p
		l.mu.Unlock()
		_ = p.sendCAck()
		select {
		case l.acceptq <- p:
		case <-l.closeq:
			_ = p.Close()
		}
		return
	}
	l.mu.Unlock()
	if p == nil || !sameUDPAddr(p.remote, remote) {
		return
	}
	switch h.opcode {
	case opData:
		p.handleData(h, b[udpHeaderSize:])
	case opCReq:
		if h.proto == p.proto.Peer && h.refresh != 0 {
			p.setRefresh(h.refresh)
			_ = p.sendCAck()
		}
	case opDisc:
		_ = p.Close()
	}
}

func (l *listener) remove(p *pipe) {
	l.mu.Lock()
	if l.pipes[p.remote.String()] == p {
		delete(l.pipes, p.remote.String())
	}
	var c *net.UDPConn
	if l.closed && len(l.pipes) == 0 {
		c = l.closeConnLocked()
	}
	l.mu.Unlock()
	if c != nil {
		_ = c.Close()
	}
}

func (l *listener) sendDisc(remote *net.UDPAddr, reason uint16) {
	l.mu.Lock()
	c := l.conn
	l.mu.Unlock()
	if c == nil {
		return
	}
	b := make([]byte, udpHeaderSize)
	packet{opcode: opDisc, proto: l.proto.Self, length: reason}.marshal(b)
	_, _ = c.WriteToUDP(b, remote)
}

type pipe struct {
	proto   transport.ProtocolInfo
	local   net.Addr
	remote  net.Addr
	maxRecv int
	sendMax int

	mu      sync.Mutex
	closed  bool
	refresh time.Duration
	expires time.Time
	closeq  chan struct{}
	recvq   chan *transport.Message
	send    func([]byte) (int, error)
	closeFn func() error
	remove  func(*pipe)
}

func newPipe(proto transport.ProtocolInfo, maxRecv int, local, remote net.Addr, send func([]byte) (int, error), closeFn func() error) *pipe {
	return &pipe{proto: proto, local: local, remote: remote, maxRecv: maxRecv, sendMax: udpMaxPayload,
		refresh: udpRefresh, expires: time.Now().Add(udpRefresh * udpExpireFactor), closeq: make(chan struct{}), recvq: make(chan *transport.Message, 16), send: send, closeFn: closeFn}
}

func (p *pipe) setRefresh(seconds uint16) {
	refresh := time.Duration(seconds) * time.Second
	if refresh == 0 {
		return
	}
	p.mu.Lock()
	if refresh < p.refresh {
		p.refresh = refresh
	}
	p.expires = time.Now().Add(p.refresh * udpExpireFactor)
	p.mu.Unlock()
}

func (p *pipe) touch() {
	p.mu.Lock()
	p.expires = time.Now().Add(p.refresh * udpExpireFactor)
	p.mu.Unlock()
}

func (p *pipe) expired(now time.Time) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return !p.closed && now.After(p.expires)
}

func (p *pipe) refreshInterval() time.Duration {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.refresh
}

func (p *pipe) writePacket(h packet, payload []byte) error {
	b := make([]byte, udpHeaderSize+len(payload))
	h.marshal(b)
	copy(b[udpHeaderSize:], payload)
	_, err := p.send(b)
	return err
}

func (p *pipe) sendCReq() error {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return mangos.ErrClosed
	}
	h := packet{opcode: opCReq, proto: p.proto.Self, length: uint16(p.maxRecv), refresh: uint16(p.refresh / time.Second)}
	p.mu.Unlock()
	return p.writePacket(h, nil)
}

func (p *pipe) sendCAck() error {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return mangos.ErrClosed
	}
	h := packet{opcode: opCAck, proto: p.proto.Self, length: uint16(p.maxRecv), refresh: uint16(p.refresh / time.Second)}
	p.mu.Unlock()
	return p.writePacket(h, nil)
}

func (p *pipe) refreshDialer() {
	for {
		t := time.NewTimer(p.refreshInterval())
		select {
		case <-t.C:
			if p.expired(time.Now()) {
				_ = p.Close()
				return
			}
			if p.sendCReq() != nil {
				return
			}
		case <-p.closeq:
			if !t.Stop() {
				select {
				case <-t.C:
				default:
				}
			}
			return
		}
	}
}

func (p *pipe) readConnected(c *net.UDPConn) {
	buf := make([]byte, udpHeaderSize+udpMaxPayload)
	for {
		n, err := c.Read(buf)
		if err != nil {
			_ = p.Close()
			return
		}
		h, ok := unmarshalPacket(buf[:n])
		if !ok {
			continue
		}
		switch h.opcode {
		case opData:
			p.handleData(h, buf[udpHeaderSize:n])
		case opCAck:
			if h.proto == p.proto.Peer && h.refresh != 0 {
				p.mu.Lock()
				p.sendMax = int(h.length)
				refresh := time.Duration(h.refresh) * time.Second
				if refresh < p.refresh {
					p.refresh = refresh
				}
				p.expires = time.Now().Add(p.refresh * udpExpireFactor)
				p.mu.Unlock()
			}
		case opDisc:
			_ = p.Close()
			return
		}
	}
}

func (p *pipe) handleData(h packet, payload []byte) {
	p.mu.Lock()
	if p.closed || h.proto != p.proto.Peer {
		p.mu.Unlock()
		return
	}
	tooLong := int(h.length) > len(payload) || int(h.length) > p.maxRecv
	p.mu.Unlock()
	if tooLong {
		_ = p.Close()
		return
	}
	p.touch()
	payload = payload[:h.length]
	m := mangos.NewMessage(len(payload))
	m.Body = append(m.Body, payload...)
	select {
	case p.recvq <- m:
	case <-p.closeq:
		m.Free()
	default:
		m.Free()
	}
}

func (p *pipe) Send(m *transport.Message) error {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return mangos.ErrClosed
	}
	length := len(m.Header) + len(m.Body)
	if length > p.sendMax || length > udpMaxPayload {
		p.mu.Unlock()
		// NNG intentionally drops packets that exceed the peer's advertised
		// UDP receive limit. Reporting an error here would close a mangos
		// pipe, which is not how this unreliable transport behaves.
		m.Free()
		return nil
	}
	h := packet{opcode: opData, proto: p.proto.Self, length: uint16(length)}
	p.mu.Unlock()
	payload := make([]byte, 0, length)
	payload = append(payload, m.Header...)
	payload = append(payload, m.Body...)
	if err := p.writePacket(h, payload); err != nil {
		return err
	}
	m.Free()
	return nil
}

func (p *pipe) Recv() (*transport.Message, error) {
	select {
	case m := <-p.recvq:
		return m, nil
	case <-p.closeq:
		return nil, mangos.ErrClosed
	}
}

func (p *pipe) Close() error {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil
	}
	p.closed = true
	close(p.closeq)
	closeFn := p.closeFn
	remove := p.remove
	p.mu.Unlock()
	// Tell an NNG peer that this logical connection is gone. UDP is
	// unreliable, so this is advisory; the peer's keep-alive also reaps it.
	_ = p.writePacket(packet{opcode: opDisc, proto: p.proto.Self, length: discMsgSize}, nil)
	if remove != nil {
		remove(p)
	}
	if closeFn != nil {
		return closeFn()
	}
	return nil
}

func (p *pipe) GetOption(name string) (interface{}, error) {
	switch name {
	case mangos.OptionLocalAddr:
		return p.local, nil
	case mangos.OptionRemoteAddr:
		return p.remote, nil
	case mangos.OptionMaxRecvSize:
		return p.maxRecv, nil
	default:
		return nil, mangos.ErrBadProperty
	}
}

func setMaxRecv(dst *int, name string, value interface{}) error {
	if name != mangos.OptionMaxRecvSize {
		return mangos.ErrBadOption
	}
	v, ok := value.(int)
	if !ok || v < 0 {
		return mangos.ErrBadValue
	}
	if v == 0 || v > udpMaxPayload {
		v = udpMaxPayload
	}
	*dst = v
	return nil
}

func sameUDPAddr(a net.Addr, b *net.UDPAddr) bool {
	x, ok := a.(*net.UDPAddr)
	return ok && x.Port == b.Port && x.Zone == b.Zone && x.IP.Equal(b.IP)
}
