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

package star

import (
	"math/rand"
	"testing"
	"time"

	"nanomsg.org/go/mangos/v2"
	. "nanomsg.org/go/mangos/v2/internal/test"
	_ "nanomsg.org/go/mangos/v2/transport/inproc"
)

type starTester struct {
	id     int
	sock   mangos.Socket
	rdoneq chan bool
	sdoneq chan bool
}

func starTestSender(t *testing.T, bt *starTester, cnt int) {
	defer close(bt.sdoneq)
	for i := 0; i < cnt; i++ {
		// Inject a small delay to give receivers a chance to catch up
		// Maximum is 10 msec.
		d := time.Duration(rand.Uint32() % 10000)
		time.Sleep(d * time.Microsecond)
		start := time.Now()
		tstr := start.Format(time.StampMilli)
		msg := mangos.NewMessage(2)
		msg.Body = append(msg.Body, byte(bt.id), byte(i))
		if err := bt.sock.SendMsg(msg); err != nil {
			tstr = time.Now().Format(time.StampMilli)
			t.Errorf("%s: Peer %d send %d fail: %v", tstr, bt.id, i, err)
			return
		}
		tstr = time.Now().Format(time.StampMilli)
	}
}

func starTestReceiver(t *testing.T, bt *starTester, cnt int, numID int) {
	var rcpt = make([]int, numID)
	defer close(bt.rdoneq)

	for tot := 0; tot < (numID-1)*cnt; {
		msg, err := bt.sock.RecvMsg()
		now := time.Now().Format(time.StampMilli)
		if err != nil {
			t.Errorf("%s: Peer %d: Recv fail: %v", now, bt.id, err)
			return
		}

		if len(msg.Body) != 2 {
			t.Errorf("%s: Peer %d: Received wrong length", now, bt.id)
			return
		}
		peer := int(msg.Body[0])
		if peer == bt.id {
			t.Errorf("%s: Peer %d: Got its own message!", now, bt.id)
			return
		}
		if int(msg.Body[1]) != rcpt[peer] {
			t.Errorf("%s: Peer %d: Bad message from peer %d: %d s/b %d",
				now, bt.id, peer, msg.Body[1], rcpt[peer])
			return
		}
		if int(msg.Body[1]) >= cnt {
			t.Errorf("%s: Peer %d: Too many from peer %d", now, bt.id,
				peer)
			return
		}
		rcpt[peer]++
		tot++
		msg.Free()
	}
}

func starTestNewServer(t *testing.T, addr string, id int) *starTester {
	var err error
	bt := &starTester{id: id, rdoneq: make(chan bool), sdoneq: make(chan bool)}

	if bt.sock, err = NewSocket(); err != nil {
		t.Errorf("Failed getting server %d socket: %v", id, err)
		return nil
	}
	if err = bt.sock.Listen(addr); err != nil {
		t.Errorf("Failed server %d listening: %v", id, err)
		bt.sock.Close()
		return nil
	}
	return bt
}

func starTestNewClient(t *testing.T, addr string, id int) *starTester {
	var err error
	bt := &starTester{id: id, rdoneq: make(chan bool), sdoneq: make(chan bool)}

	if bt.sock, err = NewSocket(); err != nil {
		t.Errorf("Failed getting client %d socket: %v", id, err)
		return nil
	}
	if err = bt.sock.Dial(addr); err != nil {
		t.Errorf("Failed client %d dialing: %v", id, err)
		bt.sock.Close()
		return nil
	}
	return bt
}

func starTestCleanup(t *testing.T, bts []*starTester) {
	time.Sleep(time.Second / 2)
	for id := 0; id < len(bts); id++ {
		if bts[id].sock != nil {
			bts[id].sock.Close()
		}
	}
}

func TestStar(t *testing.T) {
	addr := AddrTestInp()
	num := 5
	pkts := 7
	bts := make([]*starTester, num)
	defer starTestCleanup(t, bts)

	for id := 0; id < num; id++ {
		if id == 0 {
			bts[id] = starTestNewServer(t, addr, id)
		} else {
			bts[id] = starTestNewClient(t, addr, id)
		}
		if bts[id] == nil {
			t.Errorf("Failed creating %d", id)
			return
		}
	}

	// start receivers first... avoids first missed dropped packet
	for id := 0; id < num; id++ {
		go starTestReceiver(t, bts[id], pkts, num)
	}

	// wait a little just to be sure go routines are all running
	time.Sleep(time.Second / 7)

	// then start senders
	for id := 0; id < num; id++ {
		go starTestSender(t, bts[id], pkts)
	}

	tmout := time.After(5 * time.Second)

	for id := 0; id < num; id++ {
		select {
		case <-bts[id].sdoneq:
			continue
		case <-tmout:
			t.Errorf("%s: Timeout waiting for sender id %d",
				time.Now().Format(time.StampMilli), id)
			return
		}
	}

	for id := 0; id < num; id++ {
		select {
		case <-bts[id].rdoneq:
			continue
		case <-tmout:
			t.Errorf("%s: Timeout waiting for receiver id %d",
				time.Now().Format(time.StampMilli), id)
			return
		}
	}
}
