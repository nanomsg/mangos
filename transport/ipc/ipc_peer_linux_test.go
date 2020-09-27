// Copyright 2020 The Mangos Authors
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

// +build linux

package ipc

import (
	"os"
	"testing"
	"time"

	"go.nanomsg.org/mangos/v3"
	. "go.nanomsg.org/mangos/v3/internal/test"
)

func TestIpcPeerIdLinux(t *testing.T) {
	sock1 := GetMockSocket()
	sock2 := GetMockSocket()
	defer MustClose(t, sock1)
	defer MustClose(t, sock2)
	addr := AddrTestIPC()
	l, e := sock1.NewListener(addr, nil)
	MustSucceed(t, e)
	MustSucceed(t, l.Listen())
	d, e := sock2.NewDialer(addr, nil)
	MustSucceed(t, d.Dial())
	time.Sleep(time.Millisecond * 20)

	MustSend(t, sock1, make([]byte, 1))
	m := MustRecvMsg(t, sock2)
	p := m.Pipe

	v, err := p.GetOption(mangos.OptionPeerPID)
	MustSucceed(t, err)
	pid, ok := v.(int)
	MustBeTrue(t, ok)
	MustBeTrue(t, pid == os.Getpid())

	v, err = p.GetOption(mangos.OptionPeerUID)
	MustSucceed(t, err)
	uid, ok := v.(int)
	MustBeTrue(t, ok)
	MustBeTrue(t, uid == os.Getuid())

	v, err = p.GetOption(mangos.OptionPeerGID)
	MustSucceed(t, err)
	gid, ok := v.(int)
	MustBeTrue(t, ok)
	MustBeTrue(t, gid == os.Getgid())
}
