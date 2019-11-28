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
	"math/rand"
	"nanomsg.org/go/mangos/v2"
	"nanomsg.org/go/mangos/v2/protocol/pub"
	"sync"
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
	MustSucceed(t, s.Close())
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
	MustSucceed(t, s.Close())
}

func TestSubSubscribe(t *testing.T) {
	s, e := NewSocket()
	MustSucceed(t, e)
	MustNotBeNil(t, s)
	MustSucceed(t, s.SetOption(mangos.OptionSubscribe, "topic"))
	MustSucceed(t, s.SetOption(mangos.OptionSubscribe, "topic"))
	MustSucceed(t, s.SetOption(mangos.OptionSubscribe, []byte{0, 1}))
	MustBeError(t, s.SetOption(mangos.OptionSubscribe, 1), mangos.ErrBadValue)

	MustBeError(t, s.SetOption(mangos.OptionUnsubscribe, "nope"), mangos.ErrBadValue)
	MustSucceed(t, s.SetOption(mangos.OptionUnsubscribe, "topic"))
	MustSucceed(t, s.SetOption(mangos.OptionUnsubscribe, []byte{0, 1}))
	MustBeError(t, s.SetOption(mangos.OptionUnsubscribe, false), mangos.ErrBadValue)
	MustSucceed(t, s.Close())
}

func TestSubUnsubscribeDrops(t *testing.T) {
	s, e := NewSocket()
	MustSucceed(t, e)
	p, e := pub.NewSocket()
	MustSucceed(t, e)
	a := AddrTestInp()

	MustSucceed(t, s.SetOption(mangos.OptionReadQLen, 50))
	MustSucceed(t, p.Listen(a))
	MustSucceed(t, s.Dial(a))

	time.Sleep(time.Millisecond * 20)

	MustSucceed(t, s.SetOption(mangos.OptionSubscribe, "1"))
	MustSucceed(t, s.SetOption(mangos.OptionSubscribe, "2"))

	for i := 0; i < 10; i++ {
		MustSucceed(t, p.Send([]byte("1")))
		MustSucceed(t, p.Send([]byte("2")))
	}

	time.Sleep(time.Millisecond * 10)
	MustSucceed(t, s.SetOption(mangos.OptionUnsubscribe, "1"))
	for i := 0; i < 10; i++ {
		v, e := s.Recv()
		MustSucceed(t, e)
		MustBeTrue(t, string(v) == "2")
	}

	MustSucceed(t, p.Close())
	MustSucceed(t, s.Close())
}

func TestSubRecvQLen(t *testing.T) {
	s, e := NewSocket()
	MustSucceed(t, e)
	p, e := pub.NewSocket()
	MustSucceed(t, e)
	addr := AddrTestInp()
	MustSucceed(t, s.SetOption(mangos.OptionRecvDeadline, time.Millisecond*10))
	MustSucceed(t, s.SetOption(mangos.OptionReadQLen, 2))
	MustSucceed(t, s.SetOption(mangos.OptionSubscribe, []byte{}))

	MustSucceed(t, s.Listen(addr))
	MustSucceed(t, p.Dial(addr))
	time.Sleep(time.Millisecond * 50)

	MustSucceed(t, p.Send([]byte("one")))
	MustSucceed(t, p.Send([]byte("two")))
	MustSucceed(t, p.Send([]byte("three")))
	time.Sleep(time.Millisecond * 50)

	MustSucceed(t, e)
	m, e := s.RecvMsg()
	MustSucceed(t, e)
	MustNotBeNil(t, m)
	m, e = s.RecvMsg()
	MustSucceed(t, e)
	MustNotBeNil(t, m)
	// this verifies we discarded the oldest first
	MustBeTrue(t, string(m.Body) == "three")
	m, e = s.RecvMsg()
	MustFail(t, e)
	MustBeTrue(t, e == mangos.ErrRecvTimeout)
	MustSucceed(t, p.Close())
	MustSucceed(t, s.Close())
}

func TestSubRecvQLenResizeDiscard(t *testing.T) {
	s, e := NewSocket()
	MustSucceed(t, e)
	p, e := pub.NewSocket()
	MustSucceed(t, e)
	addr := AddrTestInp()
	MustSucceed(t, s.SetOption(mangos.OptionRecvDeadline, time.Millisecond*100))
	MustSucceed(t, s.SetOption(mangos.OptionReadQLen, 10))
	MustSucceed(t, s.SetOption(mangos.OptionSubscribe, []byte{}))
	MustSucceed(t, s.Listen(addr))
	MustSucceed(t, p.Dial(addr))
	time.Sleep(time.Millisecond * 50)

	MustSucceed(t, p.Send([]byte("one")))
	MustSucceed(t, p.Send([]byte("two")))
	MustSucceed(t, p.Send([]byte("three")))

	// Sleep allows the messages to arrive in the recvq before we resize.
	time.Sleep(time.Millisecond * 50)

	// Shrink it
	MustSucceed(t, s.SetOption(mangos.OptionReadQLen, 2))

	// We should be able to get the first message, which should be "two"
	m, e := s.RecvMsg()
	MustSucceed(t, e)
	MustNotBeNil(t, m)

	m, e = s.RecvMsg()
	MustSucceed(t, e)
	MustNotBeNil(t, m)
	// this verifies we discarded the oldest first
	MustBeTrue(t, string(m.Body) == "three")
	m, e = s.RecvMsg()
	MustFail(t, e)
	MustBeTrue(t, e == mangos.ErrRecvTimeout)
	MustSucceed(t, p.Close())
	MustSucceed(t, s.Close())
}

func TestSubContextOpen(t *testing.T) {
	s, e := NewSocket()
	MustSucceed(t, e)
	c, e := s.OpenContext()
	MustSucceed(t, e)

	// Also test that we can't send on this.
	MustBeError(t, c.Send([]byte{}), mangos.ErrProtoOp)
	MustSucceed(t, c.Close())
	MustSucceed(t, s.Close())

	MustBeError(t, c.Close(), mangos.ErrClosed)
}

func TestSubSocketCloseContext(t *testing.T) {
	s, e := NewSocket()
	MustSucceed(t, e)
	c, e := s.OpenContext()
	MustSucceed(t, e)

	MustSucceed(t, s.Close())

	// Verify that the context is already closed (closing the socket
	// closes the context.)
	MustBeError(t, c.Close(), mangos.ErrClosed)
}

func TestSubMultiContexts(t *testing.T) {
	s, e := NewSocket()
	MustSucceed(t, e)
	MustNotBeNil(t, s)

	c1, e := s.OpenContext()
	MustSucceed(t, e)
	MustNotBeNil(t, c1)
	c2, e := s.OpenContext()
	MustSucceed(t, e)
	MustNotBeNil(t, c2)
	MustBeTrue(t, c1 != c2)

	MustSucceed(t, c1.SetOption(mangos.OptionSubscribe, "1"))
	MustSucceed(t, c2.SetOption(mangos.OptionSubscribe, "2"))

	p, e := pub.NewSocket()
	MustSucceed(t, e)
	MustNotBeNil(t, p)

	a := AddrTestInp()

	MustSucceed(t, p.Listen(a))
	MustSucceed(t, s.Dial(a))

	// Make sure we have dialed properly
	time.Sleep(time.Millisecond * 10)

	sent := []int{0, 0}
	recv := []int{0, 0}
	mesg := []string{"1", "2"}
	var wg sync.WaitGroup
	wg.Add(2)
	fn := func(c mangos.Context, index int) {
		for {
			m, e := c.RecvMsg()
			if e == nil {
				MustBeTrue(t, string(m.Body) == mesg[index])
				recv[index]++
				continue
			}
			MustBeError(t, e, mangos.ErrClosed)
			wg.Done()
			return
		}
	}

	go fn(c1, 0)
	go fn(c2, 1)

	rng := rand.NewSource(32)

	// Choose an odd number so that it does not divide evenly, ensuring
	// that there will be a non-equal distribution.  Note that with our
	// fixed seed above, it works out to 41 & 60.
	for i := 0; i < 101; i++ {
		index := int(rng.Int63() & 1)
		MustSucceed(t, p.Send([]byte(mesg[index])))
		sent[index]++
	}

	// Give time for everything to be delivered.
	time.Sleep(time.Millisecond * 50)
	MustSucceed(t, c1.Close())
	MustSucceed(t, c2.Close())
	wg.Wait()

	MustBeTrue(t, sent[0] != sent[1])
	MustBeTrue(t, sent[0] == recv[0])
	MustBeTrue(t, sent[1] == recv[1])

	MustSucceed(t, s.Close())
	MustSucceed(t, p.Close())
}
