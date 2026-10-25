//go:build !windows && !plan9 && !js

// Copyright 2026 The Mangos Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package tcp

import (
	"net"
	"os"
	"syscall"
	"testing"

	"go.nanomsg.org/mangos/v3"
	. "go.nanomsg.org/mangos/v3/internal/test"
)

func TestTcpSocketActivation(t *testing.T) {
	sock := GetMockSocket()
	defer MustClose(t, sock)

	base, err := net.Listen("tcp", "127.0.0.1:0")
	MustSucceed(t, err)
	file, err := base.(*net.TCPListener).File()
	MustSucceed(t, err)
	fd, err := syscall.Dup(int(file.Fd()))
	MustSucceed(t, err)
	MustSucceed(t, file.Close())
	// SetOption consumes fd.
	l, err := sock.NewListener("tcp://ignored:0", map[string]interface{}{mangos.OptionListenFD: fd})
	MustSucceed(t, err)
	MustSucceed(t, base.Close())
	MustSucceed(t, l.Listen())

	conn, err := net.Dial("tcp", l.Address()[len("tcp://"):])
	MustSucceed(t, err)
	defer func() { _ = conn.Close() }()
	TranConnHandshake(t, conn, sock.Info().Peer)
}

func TestTcpSocketActivationRejectsOtherListener(t *testing.T) {
	sock := GetMockSocket()
	defer MustClose(t, sock)

	name := AddrTestUnix()[len("unix://"):]
	_ = os.Remove(name)
	base, err := net.Listen("unix", name)
	MustSucceed(t, err)
	defer func() { _ = os.Remove(name) }()
	file, err := base.(*net.UnixListener).File()
	MustSucceed(t, err)
	fd, err := syscall.Dup(int(file.Fd()))
	MustSucceed(t, err)
	MustSucceed(t, file.Close())
	l, err := sock.NewListener("tcp://127.0.0.1:0", nil)
	MustSucceed(t, err)
	MustBeError(t, l.SetOption(mangos.OptionListenFD, fd), mangos.ErrBadValue)
	_ = base.Close()
}
