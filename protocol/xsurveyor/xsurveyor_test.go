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

package xsurveyor

import (
	"nanomsg.org/go/mangos/v2"
	"nanomsg.org/go/mangos/v2/protocol"
	"nanomsg.org/go/mangos/v2/protocol/respondent"
	"nanomsg.org/go/mangos/v2/protocol/xrespondent"
	"sync"
	"testing"
	"time"

	. "nanomsg.org/go/mangos/v2/internal/test"
	_ "nanomsg.org/go/mangos/v2/transport/inproc"
)

func TestXSurveyorIdentity(t *testing.T) {
	s, err := NewSocket()
	MustSucceed(t, err)
	id := s.Info()
	MustBeTrue(t, id.Self == mangos.ProtoSurveyor)
	MustBeTrue(t, id.SelfName == "surveyor")
	MustBeTrue(t, id.Peer == mangos.ProtoRespondent)
	MustBeTrue(t, id.PeerName == "respondent")
	MustSucceed(t, s.Close())
}

func TestXSurveyorRaw(t *testing.T) {
	VerifyRaw(t, NewSocket)
}

func TestXSurveyorClosed(t *testing.T) {
	VerifyClosedRecv(t, NewSocket)
	VerifyClosedSend(t, NewSocket)
	VerifyClosedClose(t, NewSocket)
	VerifyClosedDial(t, NewSocket)
	VerifyClosedListen(t, NewSocket)
}

func TestXSurveyorNoHeader(t *testing.T) {
	s, err := NewSocket()
	MustSucceed(t, err)

	m := mangos.NewMessage(0)

	MustSucceed(t, s.SendMsg(m))
	MustSucceed(t, s.Close())
}

func TestXSurveyorNonBlock(t *testing.T) {
	s, err := NewSocket()
	MustSucceed(t, err)

	m := mangos.NewMessage(0)
	m.Header = append(m.Header, 0x80, 0, 0, 1)

	MustSucceed(t, s.SendMsg(m))
	MustSucceed(t, s.Close())
}

func TestXSurveyorRecvTimeout(t *testing.T) {
	s, err := NewSocket()
	MustSucceed(t, err)

	MustSucceed(t, s.SetOption(mangos.OptionRecvDeadline, time.Millisecond))
	m, err := s.RecvMsg()
	MustBeNil(t, m)
	MustBeError(t, err, protocol.ErrRecvTimeout)
	MustSucceed(t, s.Close())
}

func TestXSurveyorOptions(t *testing.T) {
	VerifyInvalidOption(t, NewSocket)
	VerifyOptionDuration(t, NewSocket, mangos.OptionRecvDeadline)
	VerifyOptionQLen(t, NewSocket, mangos.OptionReadQLen)
	VerifyOptionQLen(t, NewSocket, mangos.OptionWriteQLen)
}

func TestXSurveyorRecvQLenResizeDiscard(t *testing.T) {
	s, e := NewSocket()
	MustSucceed(t, e)
	p, e := xrespondent.NewSocket()
	MustSucceed(t, e)
	addr := AddrTestInp()
	MustSucceed(t, s.SetOption(mangos.OptionRecvDeadline, time.Millisecond*10))
	MustSucceed(t, s.SetOption(mangos.OptionReadQLen, 3))
	MustSucceed(t, s.Listen(addr))
	MustSucceed(t, p.Dial(addr))
	time.Sleep(time.Millisecond * 50)

	// We need to do a send from the surveyor to get the pipe ID so we can
	// issue a working reply
	m := mangos.NewMessage(0)
	m.Header = append(m.Header, 0x80, 0, 0, 1)
	m.Body = append(m.Body, 'A', 'B', 'C')
	MustSucceed(t, s.SendMsg(m.Dup()))
	m, e = p.RecvMsg()
	MustSucceed(t, e)
	MustSucceed(t, p.SendMsg(m.Dup()))
	MustSucceed(t, p.SendMsg(m.Dup()))
	MustSucceed(t, p.SendMsg(m.Dup()))
	MustSucceed(t, e)
	time.Sleep(time.Millisecond * 50)
	// Shrink it
	MustSucceed(t, s.SetOption(mangos.OptionReadQLen, 2))
	m, e = s.RecvMsg()
	MustSucceed(t, e)
	MustNotBeNil(t, m)
	m, e = s.RecvMsg()
	MustSucceed(t, e)
	MustNotBeNil(t, m)
	// this verifies we discarded the oldest first
	MustBeTrue(t, string(m.Body) == "ABC")
	m, e = s.RecvMsg()
	MustBeError(t, e, mangos.ErrRecvTimeout)
	MustSucceed(t, p.Close())
	MustSucceed(t, s.Close())
}

func TestXSurveyorBroadcast(t *testing.T) {
	s, err := NewSocket()
	MustSucceed(t, err)
	MustNotBeNil(t, s)

	s1, err := respondent.NewSocket()
	MustSucceed(t, err)
	MustNotBeNil(t, s1)

	s2, err := respondent.NewSocket()
	MustSucceed(t, err)
	MustNotBeNil(t, s2)

	a := AddrTestInp()
	MustSucceed(t, s.Listen(a))
	MustSucceed(t, s1.Dial(a))
	MustSucceed(t, s2.Dial(a))

	time.Sleep(time.Millisecond * 50)

	MustSucceed(t, s1.SetOption(mangos.OptionRecvDeadline, time.Millisecond*50))
	MustSucceed(t, s2.SetOption(mangos.OptionRecvDeadline, time.Millisecond*50))

	m := mangos.NewMessage(0)
	m.Header = append(m.Header, 0x80, 1, 2 ,3)
	m.Body = append(m.Body, []byte("one")...)
	MustSucceed(t, s.SendMsg(m))
	m = mangos.NewMessage(0)
	m.Header = append(m.Header, 0x80, 1, 2 ,4)
	m.Body = append(m.Body, []byte("two")...)
	MustSucceed(t, s.SendMsg(m))
	m = mangos.NewMessage(0)
	m.Header = append(m.Header, 0x80, 1, 2 ,5)
	m.Body = append(m.Body, []byte("three")...)
	MustSucceed(t, s.SendMsg(m))

	var wg sync.WaitGroup
	wg.Add(2)
	pass1 := false
	pass2 := false

	f := func(s mangos.Socket, pass *bool) { // Subscriber one
		defer wg.Done()
		v, e := s.Recv()
		MustSucceed(t, e)
		println(string(v))
		MustBeTrue(t, string(v) == "one")

		v, e = s.Recv()
		MustSucceed(t, e)
		MustBeTrue(t, string(v) == "two")

		v, e = s.Recv()
		MustSucceed(t, e)
		MustBeTrue(t, string(v) == "three")

		v, e = s.Recv()
		MustBeError(t, e, mangos.ErrRecvTimeout)
		MustBeNil(t, v)
		*pass = true
	}

	go f(s1, &pass1)
	go f(s2, &pass2)

	wg.Wait()

	MustBeTrue(t, pass1)
	MustBeTrue(t, pass2)

	MustSucceed(t, s.Close())
	MustSucceed(t, s1.Close())
	MustSucceed(t, s2.Close())
}
