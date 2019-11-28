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

package xrespondent

import (
	"nanomsg.org/go/mangos/v2"
	"nanomsg.org/go/mangos/v2/protocol"
	"nanomsg.org/go/mangos/v2/protocol/surveyor"
	"nanomsg.org/go/mangos/v2/protocol/xsurveyor"
	"testing"
	"time"

	. "nanomsg.org/go/mangos/v2/internal/test"
	_ "nanomsg.org/go/mangos/v2/transport/inproc"
)

func TestXRespondentIdentity(t *testing.T) {
	s, err := NewSocket()
	MustSucceed(t, err)
	id := s.Info()
	MustBeTrue(t, id.Peer == mangos.ProtoSurveyor)
	MustBeTrue(t, id.PeerName == "surveyor")
	MustBeTrue(t, id.Self == mangos.ProtoRespondent)
	MustBeTrue(t, id.SelfName == "respondent")
	MustSucceed(t, s.Close())
}

func TestXRespondentRaw(t *testing.T) {
	VerifyRaw(t, NewSocket)
}

func TestXRespondentClosed(t *testing.T) {
	VerifyClosedRecv(t, NewSocket)
	VerifyClosedSend(t, NewSocket)
	VerifyClosedClose(t, NewSocket)
	VerifyClosedDial(t, NewSocket)
	VerifyClosedListen(t, NewSocket)
}

func TestXRespondentNoHeader(t *testing.T) {
	s, err := NewSocket()
	MustSucceed(t, err)
	m := mangos.NewMessage(0)

	MustSucceed(t, s.SendMsg(m))
	MustSucceed(t, s.Close())
}

func TestXRespondentMismatchHeader(t *testing.T) {
	s, err := NewSocket()
	MustSucceed(t, err)

	m := mangos.NewMessage(0)
	m.Header = append(m.Header, []byte{1, 1, 1, 1}...)

	MustSucceed(t, s.SendMsg(m))
	MustSucceed(t, s.Close())
}

func TestXRespondentRecvTimeout(t *testing.T) {
	s, err := NewSocket()
	MustSucceed(t, err)

	MustSucceed(t, s.SetOption(mangos.OptionRecvDeadline, time.Millisecond))
	m, err := s.RecvMsg()
	MustBeNil(t, m)
	MustFail(t, err)
	MustBeTrue(t, err == protocol.ErrRecvTimeout)
	MustSucceed(t, s.Close())
}

func TestXRespondentTTLZero(t *testing.T) {
	SetTTLZero(t, NewSocket)
}

func TestXRespondentTTLNegative(t *testing.T) {
	SetTTLNegative(t, NewSocket)
}

func TestXRespondentTTLTooBig(t *testing.T) {
	SetTTLTooBig(t, NewSocket)
}

func TestXRespondentTTLNotInt(t *testing.T) {
	SetTTLNotInt(t, NewSocket)
}

func TestXRespondentTTLSet(t *testing.T) {
	SetTTL(t, NewSocket)
}

func TestXRespondentTTLDrop(t *testing.T) {
	TTLDropTest(t, surveyor.NewSocket, NewSocket, xsurveyor.NewSocket, NewSocket)
}

func TestXRespondentOptions(t *testing.T) {
	VerifyInvalidOption(t, NewSocket)
	VerifyOptionDuration(t, NewSocket, mangos.OptionRecvDeadline)
	VerifyOptionDuration(t, NewSocket, mangos.OptionSendDeadline)
	VerifyOptionQLen(t, NewSocket, mangos.OptionReadQLen)
	VerifyOptionQLen(t, NewSocket, mangos.OptionWriteQLen)
	VerifyOptionInt(t, NewSocket, mangos.OptionTTL)
	VerifyOptionBool(t, NewSocket, mangos.OptionBestEffort)
}

func TestXRespondentSendTimeout(t *testing.T) {
	timeout := time.Millisecond * 10

	s, err := NewSocket()
	MustSucceed(t, err)
	MustNotBeNil(t, s)

	r, err := xsurveyor.NewSocket()
	MustSucceed(t, err)
	MustNotBeNil(t, r)

	MustSucceed(t, s.SetOption(mangos.OptionWriteQLen, 1))
	MustSucceed(t, r.SetOption(mangos.OptionReadQLen, 0))
	MustSucceed(t, s.SetOption(mangos.OptionRecvDeadline, timeout))
	MustSucceed(t, s.SetOption(mangos.OptionSendDeadline, timeout))
	MustSucceed(t, r.SetOption(mangos.OptionRecvDeadline, timeout))

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
	MustSucceed(t, s.Close())
	MustSucceed(t, r.Close())
}

func TestXRespondentBestEffort(t *testing.T) {
	timeout := time.Millisecond * 10

	s, err := NewSocket()
	MustSucceed(t, err)
	MustNotBeNil(t, s)

	r, err := xsurveyor.NewSocket()
	MustSucceed(t, err)
	MustNotBeNil(t, r)

	MustSucceed(t, s.SetOption(mangos.OptionWriteQLen, 1))
	MustSucceed(t, r.SetOption(mangos.OptionReadQLen, 0))
	MustSucceed(t, s.SetOption(mangos.OptionRecvDeadline, timeout))
	MustSucceed(t, s.SetOption(mangos.OptionSendDeadline, timeout))
	MustSucceed(t, r.SetOption(mangos.OptionRecvDeadline, timeout))

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
	MustSucceed(t, s.Close())
	MustSucceed(t, r.Close())
}
