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

package xbus

import (
	"bytes"
	"encoding/binary"
	"sync"
	"testing"
	"time"

	"nanomsg.org/go/mangos/v2"

	. "nanomsg.org/go/mangos/v2/internal/test"
	_ "nanomsg.org/go/mangos/v2/transport/inproc"
)

func TestXBusIdentity(t *testing.T) {
	s, err := NewSocket()
	MustSucceed(t, err)
	id := s.Info()
	MustBeTrue(t, id.Self == mangos.ProtoBus)
	MustBeTrue(t, id.SelfName == "bus")
	MustBeTrue(t, id.Peer == mangos.ProtoBus)
	MustBeTrue(t, id.PeerName == "bus")
	MustSucceed(t, s.Close())
}

func TestXBusRaw(t *testing.T) {
	VerifyRaw(t, NewSocket)
}

func TestXBusClosed(t *testing.T) {
	VerifyClosedRecv(t, NewSocket)
	VerifyClosedSend(t, NewSocket)
	VerifyClosedClose(t, NewSocket)
	VerifyClosedDial(t, NewSocket)
	VerifyClosedListen(t, NewSocket)
	VerifyClosedAddPipe(t, NewSocket)
}

func TestXBusOptions(t *testing.T) {
	VerifyInvalidOption(t, NewSocket)
	VerifyOptionDuration(t, NewSocket, mangos.OptionRecvDeadline)
	VerifyOptionInt(t, NewSocket, mangos.OptionReadQLen)
	VerifyOptionInt(t, NewSocket, mangos.OptionWriteQLen)
}

func TestXBusRecvDeadline(t *testing.T) {
	s := GetSocket(t, NewSocket)
	MustSucceed(t, s.SetOption(mangos.OptionRecvDeadline, time.Millisecond))
	m, e := s.RecvMsg()
	MustBeError(t, e, mangos.ErrRecvTimeout)
	MustBeNil(t, m)
	MustSucceed(t, s.Close())
}

// This ensures we get our pipe ID on receive as the sole header.
func TestXBusRecvHeader(t *testing.T) {
	self := GetSocket(t, NewSocket)
	peer := GetSocket(t, NewSocket)

	data := []byte{'a', 'b', 'c'}
	ConnectPair(t, self, peer)
	MustSucceed(t, peer.Send(data))
	recv := MustRecvMsg(t, self)
	MustBeTrue(t, bytes.Equal(data, recv.Body))
	MustBeTrue(t, len(recv.Header) == 4)
	MustBeTrue(t, recv.Pipe.ID() == binary.BigEndian.Uint32(recv.Header))
	MustSucceed(t, self.Close())
	MustSucceed(t, peer.Close())
}

// This ensures that on send the header is discarded.
func TestXBusSendRecvHeader(t *testing.T) {
	self := GetSocket(t, NewSocket)
	peer := GetSocket(t, NewSocket)

	data := []byte{'a', 'b', 'c'}
	send := mangos.NewMessage(0)
	send.Body = append(send.Body, data...)
	send.Header = append(send.Header, 1, 2, 3, 4)
	ConnectPair(t, self, peer)
	MustSucceed(t, peer.SendMsg(send))
	recv := MustRecvMsg(t, self)
	MustBeTrue(t, bytes.Equal(data, recv.Body))
	MustBeTrue(t, len(recv.Header) == 4)
	MustBeTrue(t, recv.Pipe.ID() == binary.BigEndian.Uint32(recv.Header))
	MustSucceed(t, self.Close())
	MustSucceed(t, peer.Close())
}

// This tests that if we send down a message with a header matching a peer,
// it won't go back out to where it came from.
func TestXBusNoLoop(t *testing.T) {
	self := GetSocket(t, NewSocket)
	peer := GetSocket(t, NewSocket)

	data := []byte{'a', 'b', 'c'}
	send := mangos.NewMessage(0)
	send.Body = append(send.Body, data...)
	send.Header = append(send.Header, 1, 2, 3, 4)
	MustSucceed(t, self.SetOption(mangos.OptionRecvDeadline, time.Millisecond*5))
	MustSucceed(t, peer.SetOption(mangos.OptionRecvDeadline, time.Millisecond*5))
	ConnectPair(t, self, peer)
	MustSucceed(t, peer.SendMsg(send))
	recv := MustRecvMsg(t, self)
	MustBeTrue(t, bytes.Equal(data, recv.Body))
	MustBeTrue(t, len(recv.Header) == 4)
	MustBeTrue(t, recv.Pipe.ID() == binary.BigEndian.Uint32(recv.Header))

	MustSucceed(t, self.SendMsg(recv))
	_, e := peer.RecvMsg()
	MustBeError(t, e, mangos.ErrRecvTimeout)
	MustSucceed(t, self.Close())
	MustSucceed(t, peer.Close())
}

func TestXBusNonBlock(t *testing.T) {

	self := GetSocket(t, NewSocket)
	MustSucceed(t, self.SetOption(mangos.OptionWriteQLen, 1))

	msg := []byte{'A', 'B', 'C'}
	for i := 0; i < 100; i++ {
		MustSucceed(t, self.Send(msg))
	}
	MustSucceed(t, self.Close())
}

func TestXBusSendDrop(t *testing.T) {
	self := GetSocket(t, NewSocket)
	MustSucceed(t, self.SetOption(mangos.OptionWriteQLen, 1))
	l, ml := GetMockListener(t, self)
	MustSucceed(t, l.Listen())
	mp := ml.NewPipe(self.Info().Peer)
	MustSucceed(t, ml.AddPipe(mp))

	for i := 0; i < 100; i++ {
		MustSucceed(t, self.Send([]byte{byte(i)}))
	}
	time.Sleep(time.Millisecond * 10)
	MustSucceed(t, self.Close())
}

func TestXBusResizeRecv(t *testing.T) {
	self := GetSocket(t, NewSocket)
	peer := GetSocket(t, NewSocket)
	MustSucceed(t, peer.SetOption(mangos.OptionWriteQLen, 20))
	MustSucceed(t, self.SetOption(mangos.OptionReadQLen, 20))
	MustSucceed(t, self.SetOption(mangos.OptionRecvDeadline, time.Minute))
	ConnectPair(t, self, peer)

	time.AfterFunc(time.Millisecond*20, func() {
		MustSucceed(t, self.SetOption(mangos.OptionReadQLen, 10))
		time.Sleep(time.Millisecond)
		MustSucceed(t, peer.Send([]byte{1}))
	})

	data := MustRecv(t, self)
	MustBeTrue(t, len(data) == 1)
	MustBeTrue(t, data[0] == 1)

	MustSucceed(t, self.Close())
}

func TestXBusResizeShrink(t *testing.T) {
	self := GetSocket(t, NewSocket)
	peer := GetSocket(t, NewSocket)
	MustSucceed(t, peer.SetOption(mangos.OptionWriteQLen, 20))
	MustSucceed(t, self.SetOption(mangos.OptionReadQLen, 20))
	MustSucceed(t, self.SetOption(mangos.OptionRecvDeadline, time.Millisecond))
	ConnectPair(t, self, peer)

	// Fill the pipe
	for i := 0; i < 20; i++ {
		// These all will work, but the back-pressure will go all the
		// way to the sender.
		MustSucceed(t, peer.Send([]byte{byte(i)}))
	}

	time.Sleep(time.Millisecond * 20)
	MustSucceed(t, self.SetOption(mangos.OptionReadQLen, 1))
	// Sleep so the resize filler finishes
	time.Sleep(time.Millisecond * 20)

	pass := false
	for i := 0; i < 20; i++ {
		if _, e := self.Recv(); e != nil {
			pass = true
			MustBeError(t, e, mangos.ErrRecvTimeout)
			break
		}
	}
	MustBeTrue(t, pass)
	MustSucceed(t, self.Close())
}

func TestXBusResizeGrow(t *testing.T) {
	self := GetSocket(t, NewSocket)
	peer := GetSocket(t, NewSocket)
	MustSucceed(t, peer.SetOption(mangos.OptionWriteQLen, 20))
	MustSucceed(t, self.SetOption(mangos.OptionReadQLen, 5))
	MustSucceed(t, self.SetOption(mangos.OptionRecvDeadline, time.Millisecond))
	ConnectPair(t, self, peer)

	// Fill the pipe
	for i := 0; i < 5; i++ {
		// These all will work, but the back-pressure will go all the
		// way to the sender.
		MustSucceed(t, peer.Send([]byte{byte(i)}))
	}

	time.Sleep(time.Millisecond * 20)
	MustSucceed(t, self.SetOption(mangos.OptionReadQLen, 10))
	// Sleep so the resize filler finishes
	time.Sleep(time.Millisecond * 20)

	pass := true
	for i := 0; i < 5; i++ {
		if _, e := self.Recv(); e != nil {
			pass = false
			MustBeError(t, e, mangos.ErrRecvTimeout)
			break
		}
	}
	_, e := self.Recv()
	MustBeError(t, e, mangos.ErrRecvTimeout)
	MustBeTrue(t, pass)
	MustSucceed(t, self.Close())
}

func TestXBusBackPressure(t *testing.T) {
	self := GetSocket(t, NewSocket)
	l, mc := GetMockListener(t, self)
	MustSucceed(t, self.SetOption(mangos.OptionWriteQLen, 2))
	wg := sync.WaitGroup{}
	wg.Add(1)
	old := self.SetPipeEventHook(func(ev mangos.PipeEvent, p mangos.Pipe) {
		if ev == mangos.PipeEventAttached {
			wg.Done()
		}
	})
	MustSucceed(t, l.Listen())
	mp := mc.NewPipe(self.Info().Peer)
	MustSucceed(t, mc.AddPipe(mp))
	wg.Wait()
	self.SetPipeEventHook(old)

	// We don't read at all from it.
	for i := 0; i < 100; i++ {
		MustSucceed(t, self.Send([]byte{}))
	}
	time.Sleep(time.Millisecond * 10)
	MustSucceed(t, self.Close())
}

func TestXBusRecvPipeAbort(t *testing.T) {
	self := GetSocket(t, NewSocket)
	l, mc := GetMockListener(t, self)

	MustSucceed(t, l.Listen())
	mp := mc.NewPipe(mangos.ProtoBus)
	p := MockAddPipe(t, self, mc, mp)

	wg := sync.WaitGroup{}
	wg.Add(1)

	closeQ := make(chan struct{})

	go func() {
		defer wg.Done()
		for {
			m := mangos.NewMessage(0)
			select {
			case mp.RecvQ() <- m:
			case <-closeQ:
				return
			}
		}
	}()

	time.Sleep(time.Millisecond * 20)
	//MustSucceed(t, self.Close())
	MustSucceed(t, p.Close())
	time.Sleep(time.Millisecond * 20)
	MustSucceed(t, self.Close())

	close(closeQ)
	wg.Wait()
}

func TestXBusRecvSockAbort(t *testing.T) {
	self := GetSocket(t, NewSocket)
	l, mc := GetMockListener(t, self)

	MustSucceed(t, l.Listen())
	mp := mc.NewPipe(self.Info().Peer)
	mp.DeferClose(true)
	MockAddPipe(t, self, mc, mp)

	wg := sync.WaitGroup{}
	wg.Add(1)

	closeQ := make(chan struct{})

	go func() {
		defer wg.Done()
		for {
			m := mangos.NewMessage(0)
			select {
			case mp.RecvQ() <- m:
			case <-closeQ:
				return
			}
		}
	}()

	time.Sleep(time.Millisecond * 20)
	MustSucceed(t, self.Close())
	mp.DeferClose(true)
	close(closeQ)
	wg.Wait()
}
