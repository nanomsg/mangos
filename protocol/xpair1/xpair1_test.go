// Copyright 2022 The Mangos Authors
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

package xpair1

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.nanomsg.org/mangos/v3"
	. "go.nanomsg.org/mangos/v3/internal/test"
	_ "go.nanomsg.org/mangos/v3/transport/inproc"
)

func TestXPair1Identity(t *testing.T) {
	s, err := NewSocket()
	MustSucceed(t, err)
	id := s.Info()
	MustBeTrue(t, id.Self == mangos.ProtoPair1)
	MustBeTrue(t, id.SelfName == "pair1")
	MustBeTrue(t, id.Peer == mangos.ProtoPair1)
	MustBeTrue(t, id.PeerName == "pair1")
	MustSucceed(t, s.Close())
}

func TestXPair1Raw(t *testing.T) {
	VerifyRaw(t, NewSocket)
}

func TestXPair1Closed(t *testing.T) {
	VerifyClosedRecv(t, NewSocket)
	//VerifyClosedSend(t, NewSocket)
	VerifyClosedClose(t, NewSocket)
	VerifyClosedDial(t, NewSocket)
	VerifyClosedListen(t, NewSocket)
	VerifyClosedAddPipe(t, NewSocket)
}

func TestXPair1Options(t *testing.T) {
	VerifyInvalidOption(t, NewSocket)
	VerifyOptionDuration(t, NewSocket, mangos.OptionRecvDeadline)
	VerifyOptionDuration(t, NewSocket, mangos.OptionSendDeadline)
	VerifyOptionInt(t, NewSocket, mangos.OptionReadQLen)
	VerifyOptionInt(t, NewSocket, mangos.OptionWriteQLen)
	VerifyOptionInt(t, NewSocket, mangos.OptionTTL)
	VerifyOptionBool(t, NewSocket, mangos.OptionBestEffort)
}

func TestXPair1ReceiveDeadline(t *testing.T) {
	self := GetSocket(t, NewSocket)
	MustSucceed(t, self.SetOption(mangos.OptionRecvDeadline, time.Millisecond*10))
	MustNotRecv(t, self, mangos.ErrRecvTimeout)
	MustSucceed(t, self.Close())
}

func TestXPair1SendMissingHeader(t *testing.T) {
	self := GetSocket(t, NewSocket)
	peer := GetSocket(t, NewSocket)
	ConnectPair(t, self, peer)
	MustSucceed(t, peer.SetOption(mangos.OptionRecvDeadline, time.Millisecond*10))
	MustSend(t, self, []byte{}) // empty message (no header)
	MustNotRecv(t, peer, mangos.ErrRecvTimeout)
	MustSucceed(t, self.Close())
	MustSucceed(t, peer.Close())
}

func TestXPair1SendMalformedHeader(t *testing.T) {
	self := GetSocket(t, NewSocket)
	peer := GetSocket(t, NewSocket)
	ConnectPair(t, self, peer)
	MustSucceed(t, peer.SetOption(mangos.OptionRecvDeadline, time.Millisecond*10))
	m := mangos.NewMessage(0)
	m.Header = append(m.Header, 1, 0, 0, 0)
	MustSendMsg(t, self, m) // malformed header
	MustNotRecv(t, peer, mangos.ErrRecvTimeout)
	MustSucceed(t, self.Close())
	MustSucceed(t, peer.Close())
}

func TestXPair1SendClosed(t *testing.T) {
	self := GetSocket(t, NewSocket)
	peer := GetSocket(t, NewSocket)
	ConnectPair(t, self, peer)
	MustSucceed(t, peer.SetOption(mangos.OptionRecvDeadline, time.Millisecond*10))
	MustSucceed(t, self.Close())
	m := mangos.NewMessage(0)
	m.Header = append(m.Header, 0, 0, 0, 0)
	err := self.SendMsg(m)
	MustBeError(t, err, mangos.ErrClosed)
	MustNotRecv(t, peer, mangos.ErrRecvTimeout)
	MustSucceed(t, peer.Close())
}

func TestXPair1SendClosedBestEffort(t *testing.T) {
	self := GetSocket(t, NewSocket)
	peer := GetSocket(t, NewSocket)
	ConnectPair(t, self, peer)
	MustSucceed(t, peer.SetOption(mangos.OptionRecvDeadline, time.Millisecond*10))
	MustSucceed(t, self.SetOption(mangos.OptionBestEffort, true))
	MustSucceed(t, self.Close())
	m := mangos.NewMessage(0)
	m.Header = append(m.Header, 0, 0, 0, 0)
	MustBeError(t, self.SendMsg(m), mangos.ErrClosed)
	MustNotRecv(t, peer, mangos.ErrRecvTimeout)
	MustSucceed(t, peer.Close())
}

func TestXPair1SendDeadline(t *testing.T) {
	self := GetSocket(t, NewSocket)
	MustSucceed(t, self.SetOption(mangos.OptionWriteQLen, 0))
	MustSucceed(t, self.SetOption(mangos.OptionSendDeadline, time.Millisecond*10))
	m := mangos.NewMessage(0)
	m.Header = append(m.Header, 0, 0, 0, 1)
	MustBeError(t, self.SendMsg(m), mangos.ErrSendTimeout)
	MustSucceed(t, self.Close())
}

func TestXPair1SendBestEffort(t *testing.T) {
	self := GetSocket(t, NewSocket)
	MustSucceed(t, self.SetOption(mangos.OptionBestEffort, true))
	MustSucceed(t, self.SetOption(mangos.OptionWriteQLen, 0))
	MustSucceed(t, self.SetOption(mangos.OptionSendDeadline, time.Millisecond*10))
	for i := 0; i < 100; i++ {
		m := mangos.NewMessage(0)
		m.Header = append(m.Header, 0, 0, 0, 1)
		m.Body = append(m.Body, []byte("yep")...)
		MustSendMsg(t, self, m)
	}
	time.Sleep(time.Millisecond * 20)
	MustSucceed(t, self.Close())
}

func TestXPair1RejectSecondPipe(t *testing.T) {
	self := GetSocket(t, NewSocket)
	peer1 := GetSocket(t, NewSocket)
	peer2 := GetSocket(t, NewSocket)

	ConnectPair(t, self, peer1)
	a := AddrTestInp()
	MustSucceed(t, self.Listen(a))

	con := int32(0)
	add := int32(0)
	dis := int32(0)
	peer2.SetPipeEventHook(func(ev mangos.PipeEvent, p mangos.Pipe) {
		switch ev {
		case mangos.PipeEventAttaching:
			atomic.AddInt32(&con, 1)
		case mangos.PipeEventAttached:
			atomic.AddInt32(&add, 1)
		case mangos.PipeEventDetached:
			atomic.AddInt32(&dis, 1)
		}
	})
	MustSucceed(t, peer2.Dial(a))
	time.Sleep(time.Millisecond * 10)
	MustBeTrue(t, atomic.LoadInt32(&con) > 0)
	MustBeTrue(t, atomic.LoadInt32(&add) > 0)
	MustBeTrue(t, atomic.LoadInt32(&dis) > 0)
	MustSucceed(t, peer2.Close())
	MustSucceed(t, peer1.Close())
	MustSucceed(t, self.Close())
}

func TestXPair1CloseAbort(t *testing.T) {
	self := GetSocket(t, NewSocket)
	MustSucceed(t, self.SetOption(mangos.OptionSendDeadline, time.Minute))
	MustSucceed(t, self.SetOption(mangos.OptionWriteQLen, 1))
	pass := false
	time.AfterFunc(time.Millisecond*10, func() {
		MustSucceed(t, self.Close())
	})
	for i := 0; i < 20; i++ {
		m := mangos.NewMessage(0)
		m.Header = append(m.Header, 0, 0, 0, 1)
		if e := self.SendMsg(m); e != nil {
			MustBeError(t, e, mangos.ErrClosed)
			pass = true
			break
		}
	}
	MustBeTrue(t, pass)
}

func TestXPair1ClosePipe(t *testing.T) {
	s := GetSocket(t, NewSocket)
	p := GetSocket(t, NewSocket)
	MustSucceed(t, p.SetOption(mangos.OptionWriteQLen, 1))
	MustSucceed(t, s.SetOption(mangos.OptionReadQLen, 3))
	MustSucceed(t, s.SetOption(mangos.OptionRecvDeadline, time.Minute))
	ConnectPair(t, s, p)
	m := mangos.NewMessage(0)
	m.Header = append(m.Header, 0, 0, 0, 1)
	MustSucceed(t, p.SendMsg(m))
	m, e := s.RecvMsg()
	MustSucceed(t, e)
	MustSucceed(t, p.SetOption(mangos.OptionSendDeadline, time.Millisecond))

	// Fill the pipe
	for i := 0; i < 20; i++ {
		// These all will work, but the back-pressure will go all the
		// way to the sender.
		m := mangos.NewMessage(0)
		m.Header = append(m.Header, 0, 0, 0, 1)
		if e := p.SendMsg(m); e != nil {
			MustBeError(t, e, mangos.ErrSendTimeout)
			break
		}
	}

	time.Sleep(time.Millisecond * 10)
	MustSucceed(t, m.Pipe.Close())

	time.Sleep(time.Millisecond * 10)
	MustSucceed(t, s.Close())
}

func TestXPair1ResizeReceive(t *testing.T) {
	self := GetSocket(t, NewSocket)
	peer := GetSocket(t, NewSocket)
	MustSucceed(t, peer.SetOption(mangos.OptionWriteQLen, 1))
	MustSucceed(t, self.SetOption(mangos.OptionReadQLen, 0))
	MustSucceed(t, self.SetOption(mangos.OptionRecvDeadline, time.Millisecond))
	ConnectPair(t, self, peer)

	m := mangos.NewMessage(0)
	m.Header = append(m.Header, 0, 0, 0, 1)
	m.Body = append(m.Body, 'o', 'n', 'e')
	MustSendMsg(t, peer, m)
	time.Sleep(time.Millisecond * 10)
	MustSucceed(t, self.SetOption(mangos.OptionReadQLen, 2))
	MustNotRecv(t, self, mangos.ErrRecvTimeout)
	MustSucceed(t, self.Close())
	MustSucceed(t, peer.Close())
}

func TestXPair1ResizeReceive1(t *testing.T) {
	self := GetSocket(t, NewSocket)
	peer := GetSocket(t, NewSocket)
	MustSucceed(t, peer.SetOption(mangos.OptionWriteQLen, 1))
	MustSucceed(t, self.SetOption(mangos.OptionReadQLen, 0))
	MustSucceed(t, self.SetOption(mangos.OptionRecvDeadline, time.Second))
	ConnectPair(t, self, peer)

	time.AfterFunc(time.Millisecond*20, func() {
		MustSucceed(t, self.SetOption(mangos.OptionReadQLen, 2))
		m := mangos.NewMessage(0)
		m.Header = append(m.Header, 0, 0, 0, 1)
		m.Body = append(m.Body, []byte("one")...)
		MustSendMsg(t, peer, m)
	})
	MustRecvString(t, self, "one")
	MustSucceed(t, self.Close())
	MustSucceed(t, peer.Close())
}

func TestXPair1ResizeReceive2(t *testing.T) {
	self := GetSocket(t, NewSocket)
	peer := GetSocket(t, NewSocket)
	MustSucceed(t, peer.SetOption(mangos.OptionWriteQLen, 0))
	MustSucceed(t, self.SetOption(mangos.OptionReadQLen, 20))
	MustSucceed(t, self.SetOption(mangos.OptionRecvDeadline, time.Millisecond))
	ConnectPair(t, self, peer)

	// Fill the pipe
	for i := 0; i < 20; i++ {
		m := mangos.NewMessage(0)
		m.Header = append(m.Header, 0, 0, 0, 1)
		MustSendMsg(t, peer, m)
	}

	time.Sleep(time.Millisecond * 10)
	MustSucceed(t, self.SetOption(mangos.OptionReadQLen, 1))
	// Sleep so the resize filler finishes
	time.Sleep(time.Millisecond * 20)

	MustNotRecv(t, self, mangos.ErrRecvTimeout)
	MustSucceed(t, self.Close())
	MustSucceed(t, peer.Close())
}

func TestXPair1ResizeSend(t *testing.T) {
	self := GetSocket(t, NewSocket)
	_, _ = MockConnect(t, self)
	MustSucceed(t, self.SetOption(mangos.OptionWriteQLen, 0))
	MustSucceed(t, self.SetOption(mangos.OptionSendDeadline, time.Second))

	var wg sync.WaitGroup
	wg.Add(1)
	time.AfterFunc(time.Millisecond*50, func() {
		defer wg.Done()
		MustSucceed(t, self.SetOption(mangos.OptionWriteQLen, 2))
	})
	m1 := mangos.NewMessage(0)
	m1.Header = append(m1.Header, 0, 0, 0, 1)
	m1.Body = append(m1.Body, '1')
	MustSendMsg(t, self, m1)

	m2 := mangos.NewMessage(0)
	m2.Header = append(m1.Header, 0, 0, 0, 1)
	m2.Body = append(m1.Body, '2')
	MustSendMsg(t, self, m2)

	wg.Wait()
	MustSucceed(t, self.Close())
}

func TestXPair1TTL(t *testing.T) {
	SetTTLZero(t, NewSocket)
	SetTTLNegative(t, NewSocket)
	SetTTLTooBig(t, NewSocket)
	SetTTLNotInt(t, NewSocket)
	SetTTL(t, NewSocket)
}

func TestXPair1EnforceTTL(t *testing.T) {
	self := GetSocket(t, NewSocket)
	mock, _ := MockConnect(t, self)

	MustSucceed(t, self.SetOption(mangos.OptionTTL, 4))
	MustSucceed(t, self.SetOption(mangos.OptionRecvDeadline, time.Millisecond*50))

	// First byte is non-zero
	m := mangos.NewMessage(0)
	m.Body = append(m.Body, 0, 0, 0, 1)
	m.Body = append(m.Body, []byte("one")...)

	MockMustSendMsg(t, mock, m, time.Second)
	MustRecvString(t, self, "one")

	m = mangos.NewMessage(0)
	m.Body = append(m.Body, 0, 0, 0, 5)
	m.Body = append(m.Body, []byte("drop")...)

	MockMustSendMsg(t, mock, m, time.Second)
	MustNotRecv(t, self, mangos.ErrRecvTimeout)

	m = mangos.NewMessage(0)
	m.Body = append(m.Body, 0, 0, 0, 2)
	m.Body = append(m.Body, []byte("two")...)

	MockMustSendMsg(t, mock, m, time.Second)
	m = MustRecvMsg(t, self)

	MustBeTrue(t, string(m.Body) == "two")
	MustBeTrue(t, len(m.Header) == 4)
	MustBeTrue(t, m.Header[0] == 0)
	MustBeTrue(t, m.Header[0] == 0)
	MustBeTrue(t, m.Header[0] == 0)
	MustBeTrue(t, m.Header[3] == 3) // incremented on receive

	MustClose(t, self)
}

func TestXPair1DropMissingTTL(t *testing.T) {
	self := GetSocket(t, NewSocket)
	mock, _ := MockConnect(t, self)

	MustSucceed(t, self.SetOption(mangos.OptionTTL, 4))
	MustSucceed(t, self.SetOption(mangos.OptionRecvDeadline, time.Millisecond*50))

	// First byte is non-zero
	m := mangos.NewMessage(0)
	m.Body = append(m.Body, 0, 0, 0)

	MockMustSendMsg(t, mock, m, time.Second)
	MustNotRecv(t, self, mangos.ErrRecvTimeout)

	MustClose(t, self)
}
