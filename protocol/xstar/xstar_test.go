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

package xstar

import (
	"nanomsg.org/go/mangos/v2"
	"testing"
	"time"

	. "nanomsg.org/go/mangos/v2/internal/test"
	_ "nanomsg.org/go/mangos/v2/transport/inproc"
)

func TestXStarIdentity(t *testing.T) {
	id := MustGetInfo(t, NewSocket)
	MustBeTrue(t, id.Self == mangos.ProtoStar)
	MustBeTrue(t, id.SelfName == "star")
	MustBeTrue(t, id.Peer == mangos.ProtoStar)
	MustBeTrue(t, id.PeerName == "star")
}

func TestXStarRaw(t *testing.T) {
	VerifyRaw(t, NewSocket)
}

func TestXStarClosed(t *testing.T) {
	VerifyClosedRecv(t, NewSocket)
	VerifyClosedSend(t, NewSocket)
	VerifyClosedClose(t, NewSocket)
	VerifyClosedDial(t, NewSocket)
	VerifyClosedListen(t, NewSocket)
}

func TestXStarOptions(t *testing.T) {
	VerifyInvalidOption(t, NewSocket)
	VerifyOptionDuration(t, NewSocket, mangos.OptionRecvDeadline)
	VerifyOptionInt(t, NewSocket, mangos.OptionReadQLen)
	VerifyOptionInt(t, NewSocket, mangos.OptionWriteQLen)
	VerifyOptionInt(t, NewSocket, mangos.OptionTTL)
}

func TestXStarRecvDeadline(t *testing.T) {
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

func TestXStarNoHeader(t *testing.T) {
	s, err := NewSocket()
	MustSucceed(t, err)
	defer MustClose(t, s)

	m := mangos.NewMessage(0)

	MustSucceed(t, s.SendMsg(m))
}

func TestXStarTTLZero(t *testing.T) {
	SetTTLZero(t, NewSocket)
}

func TestXStarTTLNegative(t *testing.T) {
	SetTTLNegative(t, NewSocket)
}

func TestXStarTTLTooBig(t *testing.T) {
	SetTTLTooBig(t, NewSocket)
}

func TestXStarTTLNotInt(t *testing.T) {
	SetTTLNotInt(t, NewSocket)
}

func TestXStarTTLSet(t *testing.T) {
	SetTTL(t, NewSocket)
}
