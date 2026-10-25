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

package unix

import (
	"net"
	"os"
	"syscall"
	"testing"

	"go.nanomsg.org/mangos/v3"
	. "go.nanomsg.org/mangos/v3/internal/test"
)

func TestUnixSocketActivation(t *testing.T) {
	sock := GetMockSocket()
	defer MustClose(t, sock)

	name := AddrTestUnix()[len("unix://"):]
	_ = os.Remove(name)
	defer func() { _ = os.Remove(name) }()
	base, err := net.Listen("unix", name)
	MustSucceed(t, err)
	base.(*net.UnixListener).SetUnlinkOnClose(false)
	file, err := base.(*net.UnixListener).File()
	MustSucceed(t, err)
	fd, err := syscall.Dup(int(file.Fd()))
	MustSucceed(t, err)
	MustSucceed(t, file.Close())
	// SetOption consumes fd.
	l, err := sock.NewListener("unix://ignored", map[string]interface{}{
		mangos.OptionListenFD:   fd,
		OptionSocketPermissions: uint32(0600),
	})
	MustSucceed(t, err)
	MustSucceed(t, base.Close())
	MustSucceed(t, l.Listen())
	info, err := os.Stat(name)
	MustSucceed(t, err)
	MustBeTrue(t, info.Mode()&os.ModePerm == 0600)

	conn, err := net.Dial("unix", name)
	MustSucceed(t, err)
	defer func() { _ = conn.Close() }()
	TranConnHandshake(t, conn, sock.Info().Peer)
}
