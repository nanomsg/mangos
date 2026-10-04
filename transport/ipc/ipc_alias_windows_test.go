//go:build windows

// Copyright 2026 The Mangos Authors
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

package ipc

import (
	"testing"
	"time"

	"go.nanomsg.org/mangos/v3"
	. "go.nanomsg.org/mangos/v3/internal/test"
	"go.nanomsg.org/mangos/v3/transport"
)

func TestIpcWindowsUnixIndependent(t *testing.T) {
	MustBeTrue(t, transport.GetTransport("unix") != nil)
	// All three schemes are registered together on Windows, but unix uses
	// the filesystem rather than the named pipe namespace used by ipc.
	servers := []mangos.Socket{GetMockSocket(), GetMockSocket()}
	clients := []mangos.Socket{GetMockSocket(), GetMockSocket()}
	for i, scheme := range []string{"unix", "winpipe"} {
		server, client := servers[i], clients[i]
		defer MustClose(t, server)
		defer MustClose(t, client)
		for _, s := range []mangos.Socket{server, client} {
			MustSucceed(t, s.SetOption(mangos.OptionSendDeadline, time.Second))
			MustSucceed(t, s.SetOption(mangos.OptionRecvDeadline, time.Second))
		}
		MustSucceed(t, server.Listen(scheme+"://independent"))
		dialScheme := scheme
		if scheme == "winpipe" {
			dialScheme = "ipc"
		}
		MustSucceed(t, client.Dial(dialScheme+"://independent"))
	}
	for i, msg := range []string{"unix socket", "named pipe"} {
		MustSendString(t, servers[i], msg)
		MustRecvString(t, clients[i], msg)
		MustSendString(t, clients[i], msg)
		MustRecvString(t, servers[i], msg)
	}
}
