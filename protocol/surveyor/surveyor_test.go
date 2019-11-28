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

package surveyor

import (
	"testing"
	"time"

	"nanomsg.org/go/mangos/v2"
	. "nanomsg.org/go/mangos/v2/internal/test"
	_ "nanomsg.org/go/mangos/v2/transport/inproc"
)

func TestSurveyorIdentity(t *testing.T) {
	s, err := NewSocket()
	MustSucceed(t, err)
	id := s.Info()
	MustBeTrue(t, id.Self == mangos.ProtoSurveyor)
	MustBeTrue(t, id.SelfName == "surveyor")
	MustBeTrue(t, id.Peer == mangos.ProtoRespondent)
	MustBeTrue(t, id.PeerName == "respondent")
	MustSucceed(t, s.Close())
}

func TestSurveyorCooked(t *testing.T) {
	VerifyCooked(t, NewSocket)
}

func TestSurveyorNonBlock(t *testing.T) {
	maxqlen := 2

	rp, err := NewSocket()
	MustSucceed(t, err)
	MustNotBeNil(t, rp)

	MustSucceed(t, rp.SetOption(mangos.OptionWriteQLen, maxqlen))
	MustSucceed(t, rp.Listen(AddrTestInp()))

	msg := []byte{'A', 'B', 'C'}
	start := time.Now()
	for i := 0; i < maxqlen*10; i++ {
		MustSucceed(t, rp.Send(msg))
	}
	end := time.Now()
	MustBeTrue(t, end.Sub(start) < time.Second/10)
	MustSucceed(t, rp.Close())
}

func TestSurveyorOptions(t *testing.T) {
	VerifyInvalidOption(t, NewSocket)
	VerifyOptionDuration(t, NewSocket, mangos.OptionRecvDeadline)
	VerifyOptionDuration(t, NewSocket, mangos.OptionSurveyTime)
	VerifyOptionQLen(t, NewSocket, mangos.OptionReadQLen)
	VerifyOptionQLen(t, NewSocket, mangos.OptionWriteQLen)
}
