// Copyright 2019 The Mangos Authors
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

package test

import (
	"nanomsg.org/go/mangos/v2/protocol"
	"nanomsg.org/go/mangos/v2/transport"
	"sync"
	"testing"

	"nanomsg.org/go/mangos/v2"
)

// This file implements a mock transport, useful for testing.

// ListenerDialer does both Listen and Dial.  (That is, it is both
// a dialer and a listener at once.)
type mockCreator struct {
	pipeQ      chan MockPipe
	closeQ     chan struct{}
	errorQ     chan error
	proto      uint16
	deferClose bool // sometimes we don't want close to really work yet
	closed     bool
	addr       string
	lock       sync.Mutex
}

// mockPipe implements a mocked transport.Pipe
type mockPipe struct {
	lProto    uint16
	rProto    uint16
	closeQ    chan struct{}
	recvQ     chan *mangos.Message
	sendQ     chan *mangos.Message
	recvErrQ  chan error
	sendErrQ  chan error
	addr      string
	closeOnce sync.Once
	initOnce  sync.Once
}

func (mp *mockPipe) init() {
	mp.initOnce.Do(func() {
		mp.recvQ = make(chan *mangos.Message)
		mp.sendQ = make(chan *mangos.Message)
		mp.closeQ = make(chan struct{})
		mp.recvErrQ = make(chan error, 1)
		mp.sendErrQ = make(chan error, 1)
	})
}

func (mp *mockPipe) SendQ() <-chan *protocol.Message {
	mp.init()
	return mp.sendQ
}

func (mp *mockPipe) RecvQ() chan<- *protocol.Message {
	mp.init()
	return mp.recvQ
}

func (mp *mockPipe) InjectSendError(e error) {
	mp.init()
	select {
	case mp.sendErrQ <- e:
	default:
	}
}

func (mp *mockPipe) InjectRecvError(e error) {
	mp.init()
	select {
	case mp.recvErrQ <- e:
	default:
	}
}

func (mp *mockPipe) Send(m *mangos.Message) error {
	mp.init()
	select {
	case <-mp.closeQ:
		return mangos.ErrClosed
	case e := <-mp.sendErrQ:
		return e
	case mp.sendQ <- m:
		return nil
	}
}

func (mp *mockPipe) Recv() (*mangos.Message, error) {
	mp.init()
	select {
	case <-mp.closeQ:
		return nil, mangos.ErrClosed
	case e := <-mp.recvErrQ:
		return nil, e
	case m := <-mp.recvQ:
		return m, nil
	}
}

func (mp *mockPipe) LocalProtocol() uint16 {
	return mp.lProto
}

func (mp *mockPipe) RemoteProtocol() uint16 {
	return mp.rProto
}

func (mp *mockPipe) GetOption(name string) (interface{}, error) {
	switch name {
	case mangos.OptionRemoteAddr, mangos.OptionLocalAddr:
		return "mock://mock", nil
	}
	return nil, mangos.ErrBadOption
}

func (mp *mockPipe) Close() error {
	mp.closeOnce.Do(func() {
		close(mp.closeQ)
	})
	return nil
}

func NewMockPipe(lProto, rProto uint16) MockPipe {
	mp := &mockPipe{
		lProto: lProto,
		rProto: rProto,
	}
	mp.init()
	return mp
}

type MockPipe interface {
	// SendQ obtains the send queue.  Test code can read from this
	// to get messages sent by the socket.
	SendQ() <-chan *protocol.Message

	// RecvQ obtains the recv queue.  Test code can write to this
	// to send message to the socket.
	RecvQ() chan<- *protocol.Message

	// InjectSendError is used to inject an error that will be seen
	// by the next Send() operation.
	InjectSendError(error)

	// InjectRecvError is used to inject an error that will be seen
	// by the next Recv() operation.
	InjectRecvError(error)

	transport.Pipe
}

type MockCreator interface {
	// NewPipe creates a Pipe, but does not add it.  The pipe will
	// use the assigned peer protocol.
	NewPipe(peer uint16) MockPipe

	// AddPipe adds the given pipe, returning an error if there is
	// no room to do so in the pipeQ.
	AddPipe(pipe MockPipe) error

	// DeferClose is used to defer close operations.  If Close()
	// is called, and deferring is false, then the close
	// will happen immediately.
	DeferClose(deferring bool)

	// Close is used to close the creator.
	Close() error

	// These are methods from transport, for Dialer and Listener.

	// Dial simulates dialing
	Dial() (transport.Pipe, error)

	// Listen simulates listening
	Listen() error

	// Accept simulates accepting.
	Accept() (transport.Pipe, error)

	// GetOption simulates getting an option.
	GetOption(string) (interface{}, error)

	// SetOption simulates setting an option.
	SetOption(string, interface{}) error

	// Address returns the address.
	Address() string
}

func (mc *mockCreator) getPipe() (transport.Pipe, error) {
	select {
	case mp := <-mc.pipeQ:
		return mp, nil
	case <-mc.closeQ:
		return nil, mangos.ErrClosed
	case e := <-mc.errorQ:
		return nil, e
	}
}

func (mc *mockCreator) Dial() (transport.Pipe, error) {
	return mc.getPipe()
}

func (mc *mockCreator) Accept() (transport.Pipe, error) {
	return mc.getPipe()
}

func (mc *mockCreator) Listen() error {
	select {
	case e := <-mc.errorQ:
		return e
	default:
		return nil
	}
}

func (mc *mockCreator) SetOption(s string, i interface{}) error {
	return mangos.ErrBadOption
}

func (mc *mockCreator) GetOption(name string) (interface{}, error) {
	switch name {
	case "mock":
		return mc, nil
	}
	return nil, mangos.ErrBadOption
}

// NewPipe just returns a ready pipe with the local peer set up.
func (mc *mockCreator) NewPipe(peer uint16) MockPipe {
	return NewMockPipe(mc.proto, peer)
}

// AddPipe adds a pipe.
func (mc *mockCreator) AddPipe(mp MockPipe) error {
	select {
	case mc.pipeQ <- mp:
		return nil
	default:
	}
	return mangos.ErrConnRefused
}

// DeferClose is used to hold off the close to simulate a the endpoint
// still creating pipes even after close.  It doesn't actually do a close,
// but if this is disabled, and Close() was called previously, then the
// close will happen immediately.
func (md *mockCreator) DeferClose(b bool) {
	md.lock.Lock()
	defer md.lock.Unlock()
	md.deferClose = b
	if md.closed && !md.deferClose {
		select {
		case <-md.closeQ:
		default:
			close(md.closeQ)
		}
	}
}

// Close closes the endpoint, but only if SkipClose is false.
func (mc *mockCreator) Close() error {
	mc.lock.Lock()
	defer mc.lock.Unlock()
	mc.closed = true
	if !mc.deferClose {
		select {
		case <-mc.closeQ:
		default:
			close(mc.closeQ)
		}
	}
	return nil
}

func (mc *mockCreator) Address() string {
	return mc.addr
}

type mockTransport struct{}

func (mockTransport) Scheme() string {
	return "mock"
}

func (mt mockTransport) newCreator(addr string, sock mangos.Socket) (MockCreator, error) {
	if _, err := transport.StripScheme(mt, addr); err != nil {
		return nil, err
	}
	mc := &mockCreator{
		proto:  sock.Info().Self,
		pipeQ:  make(chan MockPipe, 1),
		closeQ: make(chan struct{}),
		errorQ: make(chan error, 1),
		addr:   addr,
	}
	return mc, nil
}

func (mt mockTransport) NewListener(addr string, sock mangos.Socket) (transport.Listener, error) {
	return mt.newCreator(addr, sock)
}

func (mt mockTransport) NewDialer(addr string, sock mangos.Socket) (transport.Dialer, error) {
	return mt.newCreator(addr, sock)
}

func AddMockTransport() {
	transport.RegisterTransport(mockTransport{})
}

func GetMockListener(t *testing.T, s mangos.Socket) (mangos.Listener, MockCreator) {
	AddMockTransport()
	l, e := s.NewListener("mock://mock", nil)
	MustSucceed(t, e)
	v, e := l.GetOption("mock")
	MustSucceed(t, e)
	ml, ok := v.(MockCreator)
	MustBeTrue(t, ok)
	return l, ml
}

func GetMockDialer(t *testing.T, s mangos.Socket) (mangos.Dialer, MockCreator) {
	AddMockTransport()
	d, e := s.NewDialer("mock://mock", nil)
	MustSucceed(t, e)
	v, e := d.GetOption("mock")
	MustSucceed(t, e)
	ml, ok := v.(MockCreator)
	MustBeTrue(t, ok)
	return d, ml
}
