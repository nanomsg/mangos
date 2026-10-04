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

package unix

import (
	"os"
	"testing"

	"golang.org/x/sys/windows"

	"go.nanomsg.org/mangos/v3"
	. "go.nanomsg.org/mangos/v3/internal/test"
)

func TestUnixWindowsErrors(t *testing.T) {
	MustBeTrue(t, isAddrInUse(os.NewSyscallError("bind", windows.WSAEADDRINUSE)))
	MustBeFalse(t, isAddrInUse(os.NewSyscallError("bind", windows.WSAEACCES)))
	MustBeTrue(t, isConnRefused(os.NewSyscallError("connect", windows.WSAECONNREFUSED)))
	MustBeFalse(t, isConnRefused(os.NewSyscallError("connect", windows.WSAEACCES)))
}

func TestUnixWindowsOptions(t *testing.T) {
	sock := GetMockSocket()
	defer MustClose(t, sock)
	l, err := Transport.NewListener(AddrTestUnix(), sock)
	MustSucceed(t, err)
	defer func() { MustSucceed(t, l.Close()) }()
	for _, opt := range []string{OptionSocketOwner, OptionSocketGroup, OptionSocketPermissions} {
		MustBeError(t, l.SetOption(opt, 0), mangos.ErrBadOption)
	}
}
