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

package test

import (
	"math/rand"
	"sync"
	"testing"

	"nanomsg.org/go/mangos/v2"
	"nanomsg.org/go/mangos/v2/protocol/pair"
	_ "nanomsg.org/go/mangos/v2/transport/tcp"
)

// This test case just tests that the simple Send/Recv (suboptimal) interfaces
// work as advertised.  This covers verification that was reported in GitHub
// issue 139: Race condition in simple Send()/Recv() code

func TestSimpleCorrect(t *testing.T) {

	tx, e := pair.NewSocket()
	MustSucceed(t, e)
	MustNotBeNil(t, tx)
	defer tx.Close()

	rx, e := pair.NewSocket()
	MustSucceed(t, e)
	MustNotBeNil(t, rx)
	defer rx.Close()

	a := AddrTestTCP()
	MustSucceed(t, rx.Listen(a))
	MustSucceed(t, tx.Dial(a))

	iter := 100000
	wg := &sync.WaitGroup{}
	wg.Add(2)
	goodtx := true
	goodrx := true
	go simpleSend(tx, wg, iter, &goodtx)
	go simpleRecv(rx, wg, iter, &goodrx)
	wg.Wait()
	MustBeTrue(t, goodtx)
	MustBeTrue(t, goodrx)
}

func simpleSend(tx mangos.Socket, wg *sync.WaitGroup, iter int, pass *bool) {
	defer wg.Done()
	var buf [256]byte
	good := true
	for i := 0; i < iter; i++ {
		l := rand.Intn(255) + 1
		buf[0] = uint8(l)
		if e := tx.Send(buf[:l]); e != nil {
			good = false
			break
		}
	}
	*pass = good
}

func simpleRecv(rx mangos.Socket, wg *sync.WaitGroup, iter int, pass *bool) {
	defer wg.Done()
	good := true
	for i := 0; i < iter; i++ {
		buf, e := rx.Recv()
		if buf == nil || e != nil || len(buf) < 1 || len(buf) != int(buf[0]) {
			good = false
			break
		}
	}
	*pass = good
}
