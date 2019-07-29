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
	"testing"
	"time"

	"nanomsg.org/go/mangos/v2"
	"nanomsg.org/go/mangos/v2/protocol/pair"
	_ "nanomsg.org/go/mangos/v2/transport/tcp"
)

func testBestEffortDrop(t *testing.T, addr string) {
	timeout := time.Millisecond * 10
	msg := []byte{'A', 'B', 'C'}

	rp, err := pair.NewSocket()
	MustSucceed(t, err)
	MustBeTrue(t, rp != nil)

	defer rp.Close()

	MustSucceed(t, rp.SetOption(mangos.OptionWriteQLen, 0))
	MustSucceed(t, rp.SetOption(mangos.OptionSendDeadline, timeout))
	MustSucceed(t, rp.Listen(addr))
	MustSucceed(t, rp.SetOption(mangos.OptionBestEffort, true))
	for i := 0; i < 2; i++ {
		MustSucceed(t, rp.Send(msg))
	}
}

func testBestEffortTimeout(t *testing.T, addr string) {
	timeout := time.Millisecond * 10
	msg := []byte{'A', 'B', 'C'}

	rp, err := pair.NewSocket()
	MustSucceed(t, err)
	MustBeTrue(t, rp != nil)

	defer rp.Close()

	MustSucceed(t, rp.SetOption(mangos.OptionWriteQLen, 0))
	MustSucceed(t, rp.SetOption(mangos.OptionSendDeadline, timeout))
	MustSucceed(t, err)
	MustSucceed(t, rp.Listen(addr))
	MustSucceed(t, rp.SetOption(mangos.OptionBestEffort, false))

	for i := 0; i < 2; i++ {
		err = rp.Send(msg)
		MustFail(t, err)
		MustBeTrue(t, err.Error() == mangos.ErrSendTimeout.Error())
	}

}

func testBestEffort(t *testing.T, addr string) {
	testBestEffortDrop(t, addr)
	testBestEffortTimeout(t, addr)
}

func TestBestEffortTCP(t *testing.T) {
	testBestEffort(t, AddrTestTCP())
}
