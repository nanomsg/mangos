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
	"testing"

	"nanomsg.org/go/mangos/v2"
)

// VerifyClosedSend verifies that Send on the socket created returns protocol.ErrClosed if it is closed.
func VerifyClosedSend(t *testing.T, f func() (mangos.Socket, error)) {
	s, err := f()
	MustSucceed(t, err)
	MustSucceed(t, s.Close())
	err = s.Send([]byte{})
	MustFail(t, err)
	MustBeTrue(t, err == protocol.ErrClosed)
}

// VerifyClosedRecv verifies that Recv on the socket created returns protocol.ErrClosed if it is closed.
func VerifyClosedRecv(t *testing.T, f func() (mangos.Socket, error)) {
	s, err := f()
	MustSucceed(t, err)
	MustSucceed(t, s.Close())
	_, err = s.Recv()
	MustFail(t, err)
	MustBeTrue(t, err == protocol.ErrClosed)
}

// VerifyClosedClose verifies that Close on an already closed socket returns protocol.ErrClosed.
func VerifyClosedClose(t *testing.T, f func() (mangos.Socket, error)) {
	s, err := f()
	MustSucceed(t, err)
	MustSucceed(t, s.Close())
	err = s.Close()
	MustFail(t, err)
	MustBeTrue(t, err == protocol.ErrClosed)
}

// VerifyClosedListen verifies that Listen returns protocol.ErrClosed on a closed socket.
func VerifyClosedListen(t *testing.T, f func() (mangos.Socket, error)) {
	s, err := f()
	MustSucceed(t, err)
	MustSucceed(t, s.Close())
	err = s.Listen(AddrTestInp())
	MustFail(t, err)
	MustBeTrue(t, err == protocol.ErrClosed)
}

// VerifyClosedDial verifies that Dial returns protocol.ErrClosed on a closed socket.
func VerifyClosedDial(t *testing.T, f func() (mangos.Socket, error)) {
	s, err := f()
	MustSucceed(t, err)
	MustSucceed(t, s.Close())
	err = s.DialOptions(AddrTestInp(), map[string]interface{}{
		mangos.OptionDialAsynch: true,
	})
	MustFail(t, err)
	MustBeTrue(t, err == protocol.ErrClosed)
}

type nullPipe struct {
	p interface{}
}

func (*nullPipe) ID() uint32 {
	return 100
}
func (*nullPipe) Address() string {
	return "null"
}
func (*nullPipe) Close() error {
	return protocol.ErrProtoOp
}
func (*nullPipe) RecvMsg() *protocol.Message {
	return nil
}
func (*nullPipe) SendMsg(*protocol.Message) error {
	return protocol.ErrProtoOp
}
func (n *nullPipe) GetPrivate() interface{} {
	return n.p
}
func (n *nullPipe) SetPrivate(p interface{}) {
	n.p = p
}

func VerifyClosedAddPipe(t *testing.T, f func() protocol.Protocol) {
	p := f()
	MustSucceed(t, p.Close())
	MustBeError(t, p.AddPipe(&nullPipe{}), mangos.ErrClosed)
}
