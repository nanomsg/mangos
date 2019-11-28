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

package xrep

import (
	"nanomsg.org/go/mangos/v2"
	"nanomsg.org/go/mangos/v2/protocol/req"
	"nanomsg.org/go/mangos/v2/protocol/xreq"
	"testing"
	"time"

	. "nanomsg.org/go/mangos/v2/internal/test"
	_ "nanomsg.org/go/mangos/v2/transport/inproc"
)

func TestXRepRaw(t *testing.T) {
	VerifyRaw(t, NewSocket)
}

func TestXRepIdentity(t *testing.T) {
	s, err := NewSocket()
	MustSucceed(t, err)
	id := s.Info()
	MustBeTrue(t, id.Self == mangos.ProtoRep)
	MustBeTrue(t, id.SelfName == "rep")
	MustBeTrue(t, id.Peer == mangos.ProtoReq)
	MustBeTrue(t, id.PeerName == "req")
	_ = s.Close()
}

func TestXRepClosed(t *testing.T) {
	VerifyClosedRecv(t, NewSocket)
	// Checking send closed is harder, because we have extra checks
	// in place.
	//VerifyClosedSend(t, NewSocket)
	VerifyClosedClose(t, NewSocket)
	VerifyClosedDial(t, NewSocket)
	VerifyClosedListen(t, NewSocket)
}

func TestXRepOptions(t *testing.T) {
	VerifyInvalidOption(t, NewSocket)
	VerifyOptionDuration(t, NewSocket, mangos.OptionRecvDeadline)
	VerifyOptionDuration(t, NewSocket, mangos.OptionSendDeadline)
	VerifyOptionInt(t, NewSocket, mangos.OptionReadQLen)
	VerifyOptionInt(t, NewSocket, mangos.OptionWriteQLen)
	VerifyOptionBool(t, NewSocket, mangos.OptionBestEffort)
	VerifyOptionInt(t, NewSocket, mangos.OptionTTL)
}

func TestXRepNoHeader(t *testing.T) {
	s, err := NewSocket()
	MustSucceed(t, err)
	m := mangos.NewMessage(0)
	MustSucceed(t, s.SendMsg(m))
	_ = s.Close()
}

func TestXRepMismatchHeader(t *testing.T) {
	s, err := NewSocket()
	MustSucceed(t, err)

	m := mangos.NewMessage(0)
	m.Header = append(m.Header, []byte{1, 1, 1, 1, 0x80, 0, 0, 1}...)

	MustSucceed(t, s.SendMsg(m))
	_ = s.Close()
}

func TestXRepRecvDeadline(t *testing.T) {
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

func TestXRepTTLZero(t *testing.T) {
	SetTTLZero(t, NewSocket)
}

func TestXRepTTLNegative(t *testing.T) {
	SetTTLNegative(t, NewSocket)
}

func TestXRepTTLTooBig(t *testing.T) {
	SetTTLTooBig(t, NewSocket)
}

func TestXRepTTLSet(t *testing.T) {
	SetTTL(t, NewSocket)
}

func TestXRepTTLDrop(t *testing.T) {
	TTLDropTest(t, req.NewSocket, NewSocket, xreq.NewSocket, NewSocket)
}

func TestXRepSendTimeout(t *testing.T) {
	timeout := time.Millisecond * 10

	s, err := NewSocket()
	MustSucceed(t, err)
	MustNotBeNil(t, s)

	r, err := xreq.NewSocket()
	MustSucceed(t, err)
	MustNotBeNil(t, r)

	MustSucceed(t, s.SetOption(mangos.OptionWriteQLen, 1))
	MustSucceed(t, r.SetOption(mangos.OptionReadQLen, 0))
	MustSucceed(t, s.SetOption(mangos.OptionRecvDeadline, timeout))
	MustSucceed(t, s.SetOption(mangos.OptionSendDeadline, timeout))
	MustSucceed(t, r.SetOption(mangos.OptionRecvDeadline, timeout))
	MustSucceed(t, r.SetOption(mangos.OptionSendDeadline, timeout))

	// We need to setup a connection so that we can get a meaningful
	// pipe ID.  We get this by receiving a message.
	a := AddrTestInp()
	MustSucceed(t, s.Listen(a))
	MustSucceed(t, r.Dial(a))

	time.Sleep(time.Millisecond * 20)
	MustSucceed(t, r.Send([]byte{0x80, 0, 0, 1})) // Request ID #1
	m, e := s.RecvMsg()
	MustSucceed(t, e)
	MustBeTrue(t, len(m.Header) >= 8) // request ID and pipe ID

	// Because of vagaries in the queuing, we slam messages until we
	// hit a timeout.  We expect to do so after only a modest number
	// of messages, as we have no reader on the other side.
	for i := 0; i < 100; i++ {
		e = s.SendMsg(m.Dup())
		if e != nil {
			break
		}
	}
	MustBeError(t, s.SendMsg(m.Dup()), mangos.ErrSendTimeout)
	_ = s.Close()
	_ = r.Close()
}

func TestXRepBestEffort(t *testing.T) {
	timeout := time.Millisecond * 10

	s, err := NewSocket()
	MustSucceed(t, err)
	MustNotBeNil(t, s)

	r, err := xreq.NewSocket()
	MustSucceed(t, err)
	MustNotBeNil(t, r)

	MustSucceed(t, s.SetOption(mangos.OptionWriteQLen, 1))
	MustSucceed(t, r.SetOption(mangos.OptionReadQLen, 0))
	MustSucceed(t, s.SetOption(mangos.OptionRecvDeadline, timeout))
	MustSucceed(t, s.SetOption(mangos.OptionSendDeadline, timeout))
	MustSucceed(t, r.SetOption(mangos.OptionRecvDeadline, timeout))
	MustSucceed(t, r.SetOption(mangos.OptionSendDeadline, timeout))

	// We need to setup a connection so that we can get a meaningful
	// pipe ID.  We get this by receiving a message.
	a := AddrTestInp()
	MustSucceed(t, s.Listen(a))
	MustSucceed(t, r.Dial(a))

	time.Sleep(time.Millisecond * 20)
	MustSucceed(t, r.Send([]byte{0x80, 0, 0, 1})) // Request ID #1
	m, e := s.RecvMsg()
	MustSucceed(t, e)
	MustBeTrue(t, len(m.Header) >= 8) // request ID and pipe ID
	MustSucceed(t, s.SetOption(mangos.OptionBestEffort, true))

	// Because of vagaries in the queuing, we slam messages until we
	// hit a timeout.  We expect to do so after only a modest number
	// of messages, as we have no reader on the other side.
	for i := 0; i < 100; i++ {
		e = s.SendMsg(m.Dup())
		if e != nil {
			break
		}
	}
	MustSucceed(t, e)
	_ = s.Close()
	_ = r.Close()
}
