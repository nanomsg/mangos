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

package xsub

import (
	"nanomsg.org/go/mangos/v2"
	"nanomsg.org/go/mangos/v2/protocol/pub"
	"testing"
	"time"

	. "nanomsg.org/go/mangos/v2/internal/test"
	_ "nanomsg.org/go/mangos/v2/transport/inproc"
)

func TestXSubIdentity(t *testing.T) {
	s, err := NewSocket()
	defer s.Close()
	MustSucceed(t, err)
	id := s.Info()
	MustBeTrue(t, id.Self == mangos.ProtoSub)
	MustBeTrue(t, id.SelfName == "sub")
	MustBeTrue(t, id.Peer == mangos.ProtoPub)
	MustBeTrue(t, id.PeerName == "pub")
}


func TestXSubRaw(t *testing.T) {
	VerifyRaw(t, NewSocket)
}

func TestXSubClosedRecv(t *testing.T) {
	VerifyClosedRecv(t, NewSocket)
}

func TestXSubDoubleClose(t *testing.T) {
	VerifyClosedClose(t, NewSocket)
}

func TestXSubClosedDial(t *testing.T) {
	VerifyClosedDial(t, NewSocket)
}

func TestXSubClosedListen(t *testing.T) {
	VerifyClosedListen(t, NewSocket)
}

func TestXSubCannotSend(t *testing.T) {
	CannotSend(t, NewSocket)
}

func TestXSubCannotSubscribe(t *testing.T) {
	// Raw sockets cannot subscribe or unsubscribe.
	s, e := NewSocket()
	MustSucceed(t, e)
	e = s.SetOption(mangos.OptionSubscribe, []byte("topic"))
	MustFail(t, e)
	MustBeTrue(t, e == mangos.ErrBadOption)
	_ = s.Close()
}

func TestXSubCannotUnsubscribe(t *testing.T) {
	// Raw sockets cannot subscribe or unsubscribe.
	s, e := NewSocket()
	MustSucceed(t, e)
	e = s.SetOption(mangos.OptionUnsubscribe, []byte("topic"))
	MustFail(t, e)
	MustBeTrue(t, e == mangos.ErrBadOption)
	_ = s.Close()
}

func TestXSubRecvDeadline(t *testing.T) {
	s, e := NewSocket()
	MustSucceed(t, e)
	e = s.SetOption(mangos.OptionRecvDeadline, time.Millisecond)
	MustSucceed(t, e)
	m, e := s.RecvMsg()
	MustFail(t, e)
	MustBeTrue(t, e == mangos.ErrRecvTimeout)
	MustBeNil(t, m)
	_ = s.Close()
}

func TestXSubRecvClean(t *testing.T) {
	s, e := NewSocket()
	MustSucceed(t, e)
	p, e := pub.NewSocket()
	MustSucceed(t, e)
	addr := AddrTestInp()
	MustSucceed(t, s.Listen(addr))
	MustSucceed(t, p.Dial(addr))
	MustSucceed(t, s.SetOption(mangos.OptionRecvDeadline, time.Second))
	m := mangos.NewMessage(0)
	m.Body = append(m.Body, []byte("Hello world")...)
	e = p.SendMsg(m)
	MustSucceed(t, e)
	m, e = s.RecvMsg()
	MustSucceed(t, e)
	MustNotBeNil(t, m)
	MustBeTrue(t, string(m.Body) == "Hello world")
	_ = p.Close()
	_ = s.Close()
}

func TestXSubRecvQLen(t *testing.T) {
	s, e := NewSocket()
	MustSucceed(t, e)
	p, e := pub.NewSocket()
	MustSucceed(t, e)
	addr := AddrTestInp()
	MustSucceed(t, s.SetOption(mangos.OptionRecvDeadline, time.Millisecond*10))
	MustSucceed(t, s.SetOption(mangos.OptionReadQLen, 2))
	MustSucceed(t, s.Listen(addr))
	MustSucceed(t, p.Dial(addr))
	MustSucceed(t, p.Send([]byte("one")))
	MustSucceed(t, p.Send([]byte("two")))
	MustSucceed(t, p.Send([]byte("three")))
	MustSucceed(t, e)
	time.Sleep(time.Millisecond*50)
	m, e := s.RecvMsg()
	MustSucceed(t, e)
	MustNotBeNil(t, m)
	m, e = s.RecvMsg()
	MustSucceed(t, e)
	MustNotBeNil(t, m)
	// this verifies we discarded the oldest first
	MustBeTrue(t, string(m.Body) == "three")
	m, e = s.RecvMsg()
	MustFail(t, e)
	MustBeTrue(t, e == mangos.ErrRecvTimeout)
	_ = p.Close()
	_ = s.Close()
}

func TestXSubRecvQLenResizeDiscard(t *testing.T) {
	s, e := NewSocket()
	MustSucceed(t, e)
	p, e := pub.NewSocket()
	MustSucceed(t, e)
	addr := AddrTestInp()
	MustSucceed(t, s.SetOption(mangos.OptionRecvDeadline, time.Millisecond*10))
	MustSucceed(t, s.SetOption(mangos.OptionReadQLen, 3))
	MustSucceed(t, s.Listen(addr))
	MustSucceed(t, p.Dial(addr))
	MustSucceed(t, p.Send([]byte("one")))
	MustSucceed(t, p.Send([]byte("two")))
	MustSucceed(t, p.Send([]byte("three")))
	MustSucceed(t, e)
	time.Sleep(time.Millisecond*50)
	// Shrink it
	MustSucceed(t, s.SetOption(mangos.OptionReadQLen, 2))
	m, e := s.RecvMsg()
	MustSucceed(t, e)
	MustNotBeNil(t, m)
	m, e = s.RecvMsg()
	MustSucceed(t, e)
	MustNotBeNil(t, m)
	// this verifies we discarded the oldest first
	MustBeTrue(t, string(m.Body) == "three")
	m, e = s.RecvMsg()
	MustFail(t, e)
	MustBeTrue(t, e == mangos.ErrRecvTimeout)
	_ = p.Close()
	_ = s.Close()
}

func TestXSubOptions(t *testing.T) {
	VerifyInvalidOption(t, NewSocket)
	VerifyOptionDuration(t, NewSocket, mangos.OptionRecvDeadline)
	VerifyOptionInt(t, NewSocket, mangos.OptionReadQLen)
}