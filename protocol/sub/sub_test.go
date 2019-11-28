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

package sub

import (
	"nanomsg.org/go/mangos/v2"
	"testing"
	"time"

	. "nanomsg.org/go/mangos/v2/internal/test"
	_ "nanomsg.org/go/mangos/v2/transport/inproc"
)

func TestSubIdentity(t *testing.T) {
	s, e := NewSocket()
	MustSucceed(t, e)
	id := s.Info()
	MustBeTrue(t, id.Self == mangos.ProtoSub)
	MustBeTrue(t, id.Peer == mangos.ProtoPub)
	MustBeTrue(t, id.SelfName == "sub")
	MustBeTrue(t, id.PeerName == "pub")
}

func TestSubCooked(t *testing.T) {
	VerifyCooked(t, NewSocket)
}

func TestSubNoSend(t *testing.T) {
	CannotSend(t, NewSocket)
}

func TestSubClosed(t *testing.T) {
	VerifyClosedRecv(t, NewSocket)
	VerifyClosedClose(t, NewSocket)
	VerifyClosedDial(t, NewSocket)
	VerifyClosedListen(t, NewSocket)
}

func TestSubOptions(t *testing.T) {
	VerifyInvalidOption(t, NewSocket)
	VerifyOptionDuration(t, NewSocket, mangos.OptionRecvDeadline)
	VerifyOptionInt(t, NewSocket, mangos.OptionReadQLen)
}

func TestSubRecvDeadline(t *testing.T) {
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

func TestSubSubscribe(t *testing.T) {
	s, e := NewSocket()
	MustSucceed(t, e)
	MustNotBeNil(t, s)
	MustSucceed(t, s.SetOption(mangos.OptionSubscribe, "topic"))
	MustSucceed(t, s.SetOption(mangos.OptionSubscribe, "topic"))
	MustSucceed(t, s.SetOption(mangos.OptionSubscribe, []byte{0, 1}))
	MustBeError(t, s.SetOption(mangos.OptionSubscribe, 1), mangos.ErrBadValue)

	MustSucceed(t, s.SetOption(mangos.OptionUnsubscribe, "topic"))
	MustSucceed(t, s.SetOption(mangos.OptionUnsubscribe, []byte{0, 1}))
	MustBeError(t, s.SetOption(mangos.OptionUnsubscribe, "nope"), mangos.ErrBadValue)
	MustBeError(t, s.SetOption(mangos.OptionUnsubscribe, false), mangos.ErrBadValue)
	_ = s.Close()
}
