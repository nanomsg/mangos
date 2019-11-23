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
	"testing"
	"time"

	. "nanomsg.org/go/mangos/v2/internal/test"
	_ "nanomsg.org/go/mangos/v2/transport/inproc"
)

func TestXRespondentIdentity(t *testing.T) {
	s, err := NewSocket()
	defer s.Close()
	MustSucceed(t, err)
	id := s.Info()
	MustBeTrue(t, id.Peer == mangos.ProtoSurveyor)
	MustBeTrue(t, id.PeerName == "surveyor")
	MustBeTrue(t, id.Self == mangos.ProtoRespondent)
	MustBeTrue(t, id.SelfName == "respondent")
}

func TestXRespondentNoHeader(t *testing.T) {
	s, err := NewSocket()
	MustSucceed(t, err)
	defer s.Close()

	m := mangos.NewMessage(0)

	MustSucceed(t, s.SendMsg(m))
}

func TestXRespondentMismatchHeader(t *testing.T) {
	s, err := NewSocket()
	MustSucceed(t, err)
	defer s.Close()

	m := mangos.NewMessage(0)
	m.Header = append(m.Header, []byte{1, 1, 1, 1}...)

	MustSucceed(t, s.SendMsg(m))
}

func TestXRespondentRecvTimeout(t *testing.T) {
	s, err := NewSocket()
	MustSucceed(t, err)
	defer s.Close()

	MustSucceed(t, s.SetOption(mangos.OptionRecvDeadline, time.Millisecond))
	m, err := s.RecvMsg()
	MustBeNil(t, m)
	MustFail(t, err)
	MustBeTrue(t, err == protocol.ErrRecvTimeout)
}
