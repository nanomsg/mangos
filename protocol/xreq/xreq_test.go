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

package xreq

import (
	"nanomsg.org/go/mangos/v2"
	"nanomsg.org/go/mangos/v2/protocol/rep"
	"nanomsg.org/go/mangos/v2/protocol/req"
	"nanomsg.org/go/mangos/v2/protocol/xrep"
	"testing"
	"time"

	. "nanomsg.org/go/mangos/v2/internal/test"
	_ "nanomsg.org/go/mangos/v2/transport/inproc"
)

func TestXReqIdentity(t *testing.T) {
	s, err := NewSocket()
	defer s.Close()
	MustSucceed(t, err)
	id := s.Info()
	MustBeTrue(t, id.Self == mangos.ProtoReq)
	MustBeTrue(t, id.SelfName == "req")
	MustBeTrue(t, id.Peer == mangos.ProtoRep)
	MustBeTrue(t, id.PeerName == "rep")
}

func TestXReqRaw(t *testing.T) {
	VerifyRaw(t, NewSocket)
}

func TestXReqClosed(t *testing.T) {
	VerifyClosedRecv(t, NewSocket)
	VerifyClosedSend(t, NewSocket)
	VerifyClosedClose(t, NewSocket)
	VerifyClosedDial(t, NewSocket)
	VerifyClosedListen(t, NewSocket)
}

func TestXReqOptions(t *testing.T) {
	VerifyInvalidOption(t, NewSocket)
	VerifyOptionDuration(t, NewSocket, mangos.OptionRecvDeadline)
	VerifyOptionDuration(t, NewSocket, mangos.OptionSendDeadline)
	VerifyOptionInt(t, NewSocket, mangos.OptionReadQLen)
	VerifyOptionInt(t, NewSocket, mangos.OptionWriteQLen)
	VerifyOptionBool(t, NewSocket, mangos.OptionBestEffort)
}

func TestXReqBestEffort(t *testing.T) {
	timeout := time.Millisecond
	msg := []byte{'0', '1', '2', '3'}

	s, err := NewSocket()
	MustSucceed(t, err)
	MustNotBeNil(t, s)

	defer s.Close()

	MustSucceed(t, s.SetOption(mangos.OptionWriteQLen, 0))
	MustSucceed(t, s.SetOption(mangos.OptionSendDeadline, timeout))
	MustSucceed(t, s.Listen(AddrTestInp()))
	MustSucceed(t, s.SetOption(mangos.OptionBestEffort, true))
	MustSucceed(t, s.Send(msg))
	MustSucceed(t, s.Send(msg))
	MustSucceed(t, s.SetOption(mangos.OptionBestEffort, false))
	MustBeError(t, s.Send(msg), mangos.ErrSendTimeout)
	MustBeError(t, s.Send(msg), mangos.ErrSendTimeout)
}

func TestXReqDevice(t *testing.T) {
	r1, e := req.NewSocket()
	MustSucceed(t, e)
	r2, e := rep.NewSocket()
	MustSucceed(t, e)

	r3, e := xrep.NewSocket()
	MustSucceed(t, e)
	r4, e := NewSocket()
	MustSucceed(t, e)

	a1 := AddrTestInp()
	a2 := AddrTestInp()

	MustSucceed(t, mangos.Device(r3, r4))

	// r1 -> r3 / r4 -> r2
	MustSucceed(t, r3.Listen(a1))
	MustSucceed(t, r4.Listen(a2))

	MustSucceed(t, r1.Dial(a1))
	MustSucceed(t, r2.Dial(a2))

	MustSucceed(t, r1.SetOption(mangos.OptionRecvDeadline, time.Second))
	MustSucceed(t, r1.SetOption(mangos.OptionSendDeadline, time.Second))
	MustSucceed(t, r2.SetOption(mangos.OptionRecvDeadline, time.Second))
	MustSucceed(t, r2.SetOption(mangos.OptionSendDeadline, time.Second))
	MustSucceed(t, r1.Send([]byte("PING")))
	ping, e := r2.Recv()
	MustSucceed(t, e)
	MustBeTrue(t, string(ping) == "PING")
	MustSucceed(t, r2.Send([]byte("PONG")))
	pong, e := r1.Recv()
	MustSucceed(t, e)
	MustBeTrue(t, string(pong) == "PONG")
	_ = r1.Close()
	_ = r2.Close()
	_ = r3.Close()
	_ = r4.Close()
}

func TestXReqRecvDeadline(t *testing.T) {
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
