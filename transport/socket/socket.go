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

// Package socket implements the socket:// transport.  It accepts connected
// stream sockets supplied through OptionSocketFD, typically from socketpair.
package socket

import (
	"net"
	"sync"

	"go.nanomsg.org/mangos/v3"
	"go.nanomsg.org/mangos/v3/transport"
)

// Transport is a transport.Transport for connected socket descriptors.
const Transport = socketTran(0)

func init() {
	transport.RegisterTransport(Transport)
}

type listener struct {
	proto   transport.ProtocolInfo
	hs      transport.Handshaker
	maxRecv int
	active  bool
	closed  bool
	pending []net.Conn
	mu      sync.Mutex
}

func (l *listener) Listen() error {
	l.mu.Lock()
	if l.closed {
		l.mu.Unlock()
		return mangos.ErrClosed
	}
	if l.active {
		l.mu.Unlock()
		return mangos.ErrAddrInUse
	}
	l.active = true
	pending := l.pending
	l.pending = nil
	l.mu.Unlock()
	for _, c := range pending {
		l.start(c)
	}
	return nil
}

func (l *listener) start(c net.Conn) {
	l.mu.Lock()
	defer l.mu.Unlock()
	closed := l.closed
	maxRecv := l.maxRecv
	if closed {
		_ = c.Close()
		return
	}
	p := transport.NewConnPipe(c, l.proto)
	p.SetOption(mangos.OptionMaxRecvSize, maxRecv)
	l.hs.Start(p)
}

func (l *listener) Accept() (transport.Pipe, error) {
	l.mu.Lock()
	active := l.active
	closed := l.closed
	l.mu.Unlock()
	if !active || closed {
		return nil, mangos.ErrClosed
	}
	return l.hs.Wait()
}

func (*listener) Address() string { return "socket://" }

func (l *listener) Close() error {
	l.mu.Lock()
	if l.closed {
		l.mu.Unlock()
		return nil
	}
	l.closed = true
	pending := l.pending
	l.pending = nil
	l.mu.Unlock()
	for _, c := range pending {
		_ = c.Close()
	}
	l.hs.Close()
	return nil
}

func (l *listener) SetOption(name string, value interface{}) error {
	switch name {
	case mangos.OptionMaxRecvSize:
		v, ok := value.(int)
		if !ok || v < 0 {
			return mangos.ErrBadValue
		}
		l.mu.Lock()
		l.maxRecv = v
		l.mu.Unlock()
		return nil
	case mangos.OptionSocketFD:
		fd, ok := value.(int)
		if !ok {
			return mangos.ErrBadValue
		}
		c, err := transport.FileConn(fd)
		if err != nil {
			return err
		}
		l.mu.Lock()
		if l.closed {
			l.mu.Unlock()
			_ = c.Close()
			return mangos.ErrClosed
		}
		active := l.active
		if !active {
			l.pending = append(l.pending, c)
		}
		l.mu.Unlock()
		if active {
			l.start(c)
		}
		return nil
	}
	return mangos.ErrBadOption
}

func (l *listener) GetOption(name string) (interface{}, error) {
	if name != mangos.OptionMaxRecvSize {
		return nil, mangos.ErrBadOption
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.maxRecv, nil
}

type socketTran int

func (socketTran) Scheme() string { return "socket" }

func (t socketTran) NewDialer(string, mangos.Socket) (transport.Dialer, error) {
	return nil, mangos.ErrBadTran
}

func (t socketTran) NewListener(addr string, sock mangos.Socket) (transport.Listener, error) {
	path, err := transport.StripScheme(t, addr)
	if err != nil {
		return nil, err
	}
	if path != "" {
		return nil, mangos.ErrBadAddr
	}
	return &listener{proto: sock.Info(), hs: transport.NewConnHandshaker()}, nil
}
