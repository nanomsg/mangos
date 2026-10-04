//go:build !plan9 && !js

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
	"strings"
	"testing"
	"time"

	"go.nanomsg.org/mangos/v3"
	. "go.nanomsg.org/mangos/v3/internal/test"
	"go.nanomsg.org/mangos/v3/transport"
)

func TestIpcAlias(t *testing.T) {
	native := platformTransport.Scheme()
	MustBeTrue(t, transport.GetTransport(native) != nil)
	for _, listenScheme := range []string{"ipc", native} {
		for _, dialScheme := range []string{"ipc", native} {
			t.Run(listenScheme+"-"+dialScheme, func(t *testing.T) {
				s1, s2 := GetMockSocket(), GetMockSocket()
				defer MustClose(t, s1)
				defer MustClose(t, s2)
				for _, s := range []mangos.Socket{s1, s2} {
					MustSucceed(t, s.SetOption(mangos.OptionSendDeadline, time.Second))
					MustSucceed(t, s.SetOption(mangos.OptionRecvDeadline, time.Second))
				}
				path := strings.TrimPrefix(AddrTestIPC(), "ipc://")
				addr := listenScheme + "://" + path
				l, err := s1.NewListener(addr, nil)
				MustSucceed(t, err)
				MustBeTrue(t, l.Address() == addr)
				MustSucceed(t, l.Listen())
				MustBeTrue(t, l.Address() == addr)
				MustSucceed(t, s2.Dial(dialScheme+"://"+path))
				MustSendString(t, s1, "alias to native")
				MustRecvString(t, s2, "alias to native")
				MustSendString(t, s2, "native to alias")
				MustRecvString(t, s1, "native to alias")
			})
		}
	}
}

func TestIpcAliasDuplicateListen(t *testing.T) {
	native := platformTransport.Scheme()
	for _, scheme := range []string{"ipc", native} {
		t.Run(scheme, func(t *testing.T) {
			s1, s2 := GetMockSocket(), GetMockSocket()
			defer MustClose(t, s1)
			defer MustClose(t, s2)
			path := strings.TrimPrefix(AddrTestIPC(), "ipc://")
			other := native
			if scheme == native {
				other = "ipc"
			}
			MustSucceed(t, s1.Listen(scheme+"://"+path))
			MustFail(t, s2.Listen(other+"://"+path))
		})
	}
}
