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
	"testing"

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
}

func TestXSubCannotUnsubscribe(t *testing.T) {
	// Raw sockets cannot subscribe or unsubscribe.
	s, e := NewSocket()
	MustSucceed(t, e)
	e = s.SetOption(mangos.OptionUnsubscribe, []byte("topic"))
	MustFail(t, e)
	MustBeTrue(t, e == mangos.ErrBadOption)
}