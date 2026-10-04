//go:build !plan9 && !js
// +build !plan9,!js

// Copyright 2026 The Mangos Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use file except in compliance with the License.
// You may obtain a copy of the license at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package unix implements the unix transport on top of AF_UNIX sockets.
// It is supported on POSIX systems and Windows systems with AF_UNIX support.
// To enable it simply import it.
package unix

import (
	"errors"
	"net"
	"os"
	"sync"
	"syscall"
	"time"

	"go.nanomsg.org/mangos/v3"
	"go.nanomsg.org/mangos/v3/transport"
)

const (
	// Transport implements the unix transport.
	Transport = unixTran(0)
)

func init() {
	transport.RegisterTransport(Transport)
}

type dialer struct {
	addr       *net.UnixAddr
	proto      transport.ProtocolInfo
	hs         transport.Handshaker
	maxRcvSize int
	lock       sync.Mutex
}

// Dial implements the Dialer Dial method
func (d *dialer) Dial() (transport.Pipe, error) {

	conn, err := net.DialUnix("unix", nil, d.addr)
	if err != nil {
		return nil, err
	}
	p := transport.NewConnPipeIPC(conn, d.proto)
	d.lock.Lock()
	p.SetOption(mangos.OptionMaxRecvSize, d.maxRcvSize)
	getPeer(conn, p)
	d.lock.Unlock()
	d.hs.Start(p)
	return d.hs.Wait()
}

// SetOption implements Dialer SetOption method.
func (d *dialer) SetOption(n string, v interface{}) error {
	d.lock.Lock()
	defer d.lock.Unlock()

	switch n {
	case mangos.OptionMaxRecvSize:
		if b, ok := v.(int); ok {
			d.maxRcvSize = b
			return nil
		}
		return mangos.ErrBadValue
	}
	return mangos.ErrBadOption
}

// GetOption implements Dialer GetOption method.
func (d *dialer) GetOption(n string) (interface{}, error) {
	d.lock.Lock()
	defer d.lock.Unlock()

	switch n {
	case mangos.OptionMaxRecvSize:
		return d.maxRcvSize, nil
	}
	return nil, mangos.ErrBadOption
}

type listener struct {
	addr       *net.UnixAddr
	proto      transport.ProtocolInfo
	listener   *net.UnixListener
	hs         transport.Handshaker
	closeQ     chan struct{}
	closed     bool
	maxRcvSize int
	owner      int
	group      int
	chown      bool
	mode       uint32
	chmod      bool
	once       sync.Once
	lock       sync.Mutex
}

// Listen implements the PipeListener Listen method.
func (l *listener) Listen() error {
	l.lock.Lock()
	defer l.lock.Unlock()

	select {
	case <-l.closeQ:
		return mangos.ErrClosed
	default:
	}
	listener, err := net.ListenUnix("unix", l.addr)

	if err != nil && isAddrInUse(err) {
		l.removeStaleSocket()
		listener, err = net.ListenUnix("unix", l.addr)
		if isAddrInUse(err) {
			err = mangos.ErrAddrInUse
		}
	}
	if err != nil {
		return err
	}

	l.setPermissions()

	l.listener = listener
	go func() {
		for {
			conn, err := l.listener.AcceptUnix()
			if err != nil {
				select {
				case <-l.closeQ:
					return
				default:
					continue
				}
			}
			p := transport.NewConnPipeIPC(conn, l.proto)
			l.lock.Lock()
			p.SetOption(mangos.OptionMaxRecvSize, l.maxRcvSize)
			getPeer(conn, p)
			l.lock.Unlock()
			l.hs.Start(p)
		}
	}()
	return nil
}

func (l *listener) Address() string {
	return "unix://" + l.addr.String()
}

// Accept implements the PipeListener Accept method.
func (l *listener) Accept() (transport.Pipe, error) {
	l.lock.Lock()
	if l.listener == nil {
		l.lock.Unlock()
		return nil, mangos.ErrClosed
	}
	l.lock.Unlock()
	return l.hs.Wait()
}

// Close implements the PipeListener Close method.
func (l *listener) Close() error {
	l.once.Do(func() {
		l.lock.Lock()
		l.closed = true
		if l.listener != nil {
			_ = l.listener.Close()
		}
		l.hs.Close()
		close(l.closeQ)
		l.lock.Unlock()
	})
	return nil
}

// SetOption implements a stub PipeListener SetOption method.
func (l *listener) SetOption(n string, v interface{}) error {
	l.lock.Lock()
	defer l.lock.Unlock()

	switch n {
	case mangos.OptionMaxRecvSize:
		if b, ok := v.(int); ok {
			l.maxRcvSize = b
			return nil
		}
		return mangos.ErrBadValue
	}

	return l.setOption(n, v)
}

// GetOption implements a stub PipeListener GetOption method.
func (l *listener) GetOption(n string) (interface{}, error) {
	l.lock.Lock()
	defer l.lock.Unlock()

	switch n {
	case mangos.OptionMaxRecvSize:
		return l.maxRcvSize, nil
	}
	return nil, mangos.ErrBadOption
}

func (l *listener) removeStaleSocket() {
	addr := l.addr.String()
	// if it's not a socket, then leave it alone!
	if !isSocket(addr) {
		return
	}
	conn, err := net.DialTimeout("unix", l.addr.String(), 100*time.Millisecond)
	if err != nil && isConnRefused(err) {
		_ = os.Remove(l.addr.String())
		return
	}
	if err == nil {
		_ = conn.Close()
	}
}

type unixTran int

// Scheme implements the Transport Scheme method.
func (unixTran) Scheme() string {
	return "unix"
}

// NewDialer implements the Transport NewDialer method.
func (t unixTran) NewDialer(addr string, sock mangos.Socket) (transport.Dialer, error) {
	var err error
	d := &dialer{
		proto: sock.Info(),
		hs:    transport.NewConnHandshaker(),
	}

	if addr, err = transport.StripScheme(t, addr); err != nil {
		return nil, err
	}

	// Resolving a Unix address only checks the network name.
	d.addr, _ = net.ResolveUnixAddr("unix", addr)
	return d, nil
}

// NewListener implements the Transport NewListener method.
func (t unixTran) NewListener(addr string, sock mangos.Socket) (transport.Listener, error) {
	var err error
	l := &listener{
		proto:  sock.Info(),
		closeQ: make(chan struct{}),
		hs:     transport.NewConnHandshaker(),
		// Both return -1 on Windows, where ownership options are unsupported
		// and setPermissions is a no-op.
		owner: os.Geteuid(),
		group: os.Getegid(),
	}

	if addr, err = transport.StripScheme(t, addr); err != nil {
		return nil, err
	}

	// Resolving a Unix address only checks the network name.
	l.addr, _ = net.ResolveUnixAddr("unix", addr)
	return l, nil
}

func isSyscallError(err error, code syscall.Errno) bool {

	var errno syscall.Errno
	if errors.As(err, &errno) {
		return errno == code
	}
	return false
}
