//go:build !windows && !plan9 && !js

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
	"go.nanomsg.org/mangos/v3"
	. "go.nanomsg.org/mangos/v3/internal/test"
	"os"
	"strings"
	"testing"
)

func TestUnixListenerOptions(t *testing.T) {
	sock := GetMockSocket()
	defer MustClose(t, sock)
	addr := AddrTestUnix()
	l, e := tran.NewListener(addr, sock)
	MustSucceed(t, e)

	MustBeError(t, l.SetOption(OptionSocketOwner, true), mangos.ErrBadValue)
	MustBeError(t, l.SetOption(OptionSocketGroup, true), mangos.ErrBadValue)
	MustBeError(t, l.SetOption(OptionSocketPermissions, true), mangos.ErrBadValue)
	MustBeError(t, l.SetOption(OptionSocketPermissions, os.ModeDir), mangos.ErrBadValue)
	MustSucceed(t, l.SetOption(OptionSocketPermissions, uint32(0642)))
	MustSucceed(t, l.SetOption(OptionSocketPermissions, os.FileMode(0642)))
	MustSucceed(t, l.SetOption(OptionSocketOwner, 0))
	MustSucceed(t, l.SetOption(OptionSocketGroup, 0))

	MustSucceed(t, l.Listen())
	i, e := os.Stat(strings.TrimPrefix(addr, "unix://"))
	MustSucceed(t, e)
	MustBeTrue(t, i.Mode()&os.ModePerm == 0642)
}
