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

package socket

import (
	"syscall"
	"testing"

	"go.nanomsg.org/mangos/v3"
	. "go.nanomsg.org/mangos/v3/internal/test"
	"go.nanomsg.org/mangos/v3/protocol/pair"
)

func TestSocketPair(t *testing.T) {
	s0 := GetSocket(t, pair.NewSocket)
	s1 := GetSocket(t, pair.NewSocket)
	defer MustClose(t, s0)
	defer MustClose(t, s1)

	l0, err := s0.NewListener("socket://", nil)
	MustSucceed(t, err)
	l1, err := s1.NewListener("socket://", nil)
	MustSucceed(t, err)
	MustSucceed(t, l0.Listen())
	MustSucceed(t, l1.Listen())

	fds, err := syscall.Socketpair(syscall.AF_UNIX, syscall.SOCK_STREAM, 0)
	MustSucceed(t, err)
	MustSucceed(t, l0.SetOption(mangos.OptionSocketFD, fds[0]))
	MustSucceed(t, l1.SetOption(mangos.OptionSocketFD, fds[1]))

	MustSend(t, s0, []byte("hello"))
	MustRecvString(t, s1, "hello")
	MustSend(t, s1, []byte("world"))
	MustRecvString(t, s0, "world")
}

func TestSocketOptions(t *testing.T) {
	s := GetSocket(t, pair.NewSocket)
	defer MustClose(t, s)
	l, err := s.NewListener("socket://", nil)
	MustSucceed(t, err)
	MustBeError(t, l.SetOption(mangos.OptionSocketFD, "invalid"), mangos.ErrBadValue)
	MustBeError(t, l.SetOption(mangos.OptionSocketFD, -1), mangos.ErrBadValue)
	MustBeError(t, l.SetOption(mangos.OptionListenFD, 3), mangos.ErrBadOption)
	_, err = s.NewListener("socket://path", nil)
	MustBeError(t, err, mangos.ErrBadAddr)
	_, err = s.NewDialer("socket://", nil)
	MustBeError(t, err, mangos.ErrBadTran)
}
