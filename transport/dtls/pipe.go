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
	"encoding/binary"
	"io"
	"sync"
	"time"

	pion "github.com/pion/dtls/v4"
	"go.nanomsg.org/mangos/v3"
	"go.nanomsg.org/mangos/v3/transport"
)

const headerSize = 8

const (
	opData byte = iota
	opCReq
	opCAck
	opDisc
)

// The NNG datagram header is also used by the UDP transport. DATA records
// contain the SP message header followed by its body; controls have no body.
type header struct {
	op      byte
	proto   uint16
	param   uint16
	refresh uint16
}

func (h header) encode(buf []byte) {
	buf[0], buf[1] = 1, h.op
	binary.LittleEndian.PutUint16(buf[2:], h.proto)
	binary.LittleEndian.PutUint16(buf[4:], h.param)
	binary.LittleEndian.PutUint16(buf[6:], h.refresh)
}

func decodeHeader(buf []byte) (header, bool) {
	if len(buf) < headerSize || buf[0] != 1 || buf[1] > opDisc {
		return header{}, false
	}
	return header{op: buf[1], proto: binary.LittleEndian.Uint16(buf[2:]),
		param: binary.LittleEndian.Uint16(buf[4:]), refresh: binary.LittleEndian.Uint16(buf[6:])}, true
}

type pipe struct {
	conn    *pion.Conn
	proto   transport.ProtocolInfo
	maxRecv int
	mu      sync.Mutex
	sendMax int
	refresh time.Duration
	expires time.Time
	closed  bool
	closeq  chan struct{}
	done    chan struct{}
	wake    chan struct{}
	recvq   chan *transport.Message
	writeMu sync.Mutex
}

func newPipe(conn *pion.Conn, proto transport.ProtocolInfo, maxRecv int) *pipe {
	return &pipe{
		conn: conn, proto: proto, maxRecv: maxRecv, sendMax: maxPayload,
		refresh: 5 * time.Second, expires: time.Now().Add(25 * time.Second),
		closeq: make(chan struct{}), done: make(chan struct{}), wake: make(chan struct{}, 1),
		recvq: make(chan *transport.Message, 16),
	}
}

func (p *pipe) control(op byte) error {
	p.mu.Lock()
	h := header{op: op, proto: p.proto.Self, param: uint16(p.maxRecv), refresh: uint16(p.refresh / time.Second)}
	p.mu.Unlock()
	buf := make([]byte, headerSize)
	h.encode(buf)
	return p.write(buf)
}

func (p *pipe) write(buf []byte) error {
	p.writeMu.Lock()
	defer p.writeMu.Unlock()
	n, err := p.conn.Write(buf)
	if err == nil && n != len(buf) {
		err = io.ErrShortWrite
	}
	return err
}

func (p *pipe) negotiate(ctx context.Context, dial bool) error {
	limit, _ := ctx.Deadline()
	if err := p.conn.SetDeadline(limit); err != nil {
		return err
	}
	buf := make([]byte, 16384)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		if dial {
			if err := p.control(opCReq); err != nil {
				return err
			}
			readLimit := time.Now().Add(200 * time.Millisecond)
			if limit.Before(readLimit) {
				readLimit = limit
			}
			_ = p.conn.SetReadDeadline(readLimit)
		}
		n, err := p.conn.Read(buf)
		if err != nil {
			if timeout, ok := err.(interface{ Timeout() bool }); ok && timeout.Timeout() && dial && time.Now().Before(limit) {
				continue
			}
			return err
		}
		h, ok := decodeHeader(buf[:n])
		if !ok || n != headerSize ||
			(dial && h.op != opCAck) || (!dial && h.op != opCReq) {
			_ = p.closeWithReason(7) // NNG DISC_PROTO
			return mangos.ErrBadProto
		}
		if h.proto != p.proto.Peer {
			_ = p.closeWithReason(1) // NNG DISC_TYPE
			return mangos.ErrBadProto
		}
		p.update(h)
		if !dial {
			if err := p.control(opCAck); err != nil {
				return err
			}
		}
		return p.conn.SetDeadline(time.Time{})
	}
}

func (p *pipe) update(h header) {
	p.mu.Lock()
	if h.param > 0 && int(h.param) < maxPayload {
		p.sendMax = int(h.param)
	} else {
		p.sendMax = maxPayload
	}
	if h.refresh > 0 && time.Duration(h.refresh)*time.Second < p.refresh {
		p.refresh = time.Duration(h.refresh) * time.Second
		select {
		case p.wake <- struct{}{}:
		default:
		}
	}
	p.expires = time.Now().Add(5 * p.refresh)
	p.mu.Unlock()
}

func (p *pipe) start(dial bool) {
	go p.readLoop(dial)
	go p.keepAlive(dial)
}

func (p *pipe) readLoop(dial bool) {
	defer p.Close()
	buf := make([]byte, 16384)
	for {
		n, err := p.conn.Read(buf)
		if err != nil {
			return
		}
		h, ok := decodeHeader(buf[:n])
		if !ok {
			_ = p.closeWithReason(7)
			return
		}
		if h.proto != p.proto.Peer {
			_ = p.closeWithReason(1)
			return
		}
		if n > headerSize+p.maxRecv {
			_ = p.closeWithReason(4) // NNG DISC_MSGSIZE
			return
		}
		switch h.op {
		case opData:
			// DTLS record boundaries delimit DATA, as in NNG. The size
			// parameter is not a stream length or a reassembly request.
			p.mu.Lock()
			if p.closed {
				p.mu.Unlock()
				return
			}
			p.expires = time.Now().Add(5 * p.refresh)
			m := mangos.NewMessage(n - headerSize)
			m.Body = append(m.Body, buf[headerSize:n]...)
			select {
			case p.recvq <- m:
			default:
				m.Free()
			}
			p.mu.Unlock()
		case opCReq:
			if dial || n != headerSize {
				return
			}
			p.update(h)
			if p.control(opCAck) != nil {
				return
			}
		case opCAck:
			if !dial || n != headerSize {
				return
			}
			p.update(h)
		case opDisc:
			return
		}
	}
}

func (p *pipe) keepAlive(dial bool) {
	for {
		p.mu.Lock()
		wait := time.Until(p.expires)
		if dial && p.refresh < wait {
			wait = p.refresh
		}
		p.mu.Unlock()
		timer := time.NewTimer(max(0, wait))
		select {
		case <-p.closeq:
			timer.Stop()
			return
		case <-p.wake:
			timer.Stop()
			continue
		case <-timer.C:
		}
		p.mu.Lock()
		expired := !time.Now().Before(p.expires)
		p.mu.Unlock()
		if expired || (dial && p.control(opCReq) != nil) {
			_ = p.Close()
			return
		}
	}
}

func (p *pipe) Send(m *transport.Message) error {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return mangos.ErrClosed
	}
	limit := p.sendMax
	p.mu.Unlock()
	if len(m.Header) > limit || len(m.Body) > limit-len(m.Header) {
		m.Free()
		return nil
	}
	size := len(m.Header) + len(m.Body)
	buf := make([]byte, headerSize+size)
	header{op: opData, proto: p.proto.Self, param: uint16(size)}.encode(buf)
	copy(buf[headerSize:], m.Header)
	copy(buf[headerSize+len(m.Header):], m.Body)
	if err := p.write(buf); err != nil {
		return err
	}
	m.Free()
	return nil
}

func (p *pipe) Recv() (*transport.Message, error) {
	select {
	case <-p.closeq:
		return nil, mangos.ErrClosed
	case m := <-p.recvq:
		return m, nil
	}
}

func (p *pipe) Close() error { return p.closeWithReason(0) }

func (p *pipe) closeWithReason(reason uint16) error {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		<-p.done
		return nil
	}
	p.closed = true
	close(p.closeq)
	p.mu.Unlock()
	defer close(p.done)
	// A disconnect is advisory. Bound this write so a congested shared UDP
	// socket cannot prevent pipe shutdown or cancellation of pending I/O.
	_ = p.conn.SetWriteDeadline(time.Now().Add(100 * time.Millisecond))
	buf := make([]byte, headerSize)
	header{op: opDisc, proto: p.proto.Self, param: reason}.encode(buf)
	_ = p.write(buf)
	err := p.conn.Close()
	for {
		select {
		case m := <-p.recvq:
			m.Free()
		default:
			return err
		}
	}
}

func (p *pipe) GetOption(name string) (interface{}, error) {
	switch name {
	case mangos.OptionLocalAddr:
		return p.conn.LocalAddr(), nil
	case mangos.OptionRemoteAddr:
		return p.conn.RemoteAddr(), nil
	case mangos.OptionMaxRecvSize:
		return p.maxRecv, nil
	case mangos.OptionDTLSConnState:
		state, ok := p.conn.ConnectionState()
		if !ok {
			return nil, mangos.ErrBadProperty
		}
		return state, nil
	default:
		return nil, mangos.ErrBadProperty
	}
}
