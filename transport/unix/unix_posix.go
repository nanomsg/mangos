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
	"os"
	"syscall"

	"go.nanomsg.org/mangos/v3"
)

func isAddrInUse(err error) bool {
	return isSyscallError(err, syscall.EADDRINUSE) || isSyscallError(err, syscall.EEXIST)
}

func isConnRefused(err error) bool {
	return isSyscallError(err, syscall.ECONNREFUSED)
}

func isSocket(path string) bool {
	st, err := os.Lstat(path)
	return err == nil && st.Mode()&os.ModeType == os.ModeSocket
}

func (l *listener) setPermissions() {
	// Best effort: socket permissions are not enforced on all POSIX systems.
	if l.chown {
		_ = os.Chown(l.addr.String(), l.owner, l.group)
	}
	if l.chmod {
		_ = os.Chmod(l.addr.String(), os.FileMode(l.mode))
	}
}

// Called with l.lock held.
func (l *listener) setOption(n string, v interface{}) error {
	switch n {
	case OptionSocketPermissions:
		if b, ok := v.(uint32); ok && b&uint32(os.ModePerm) == b {
			l.mode, l.chmod = b, true
			return nil
		}
		if b, ok := v.(os.FileMode); ok && b&os.ModePerm == b {
			l.mode, l.chmod = uint32(b), true
			return nil
		}
		return mangos.ErrBadValue
	case OptionSocketOwner:
		if b, ok := v.(int); ok {
			l.owner, l.chown = b, true
			return nil
		}
		return mangos.ErrBadValue
	case OptionSocketGroup:
		if b, ok := v.(int); ok {
			l.group, l.chown = b, true
			return nil
		}
		return mangos.ErrBadValue
	}
	return mangos.ErrBadOption
}
