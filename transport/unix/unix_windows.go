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
	"path/filepath"

	"golang.org/x/sys/windows"

	"go.nanomsg.org/mangos/v3"
)

func isAddrInUse(err error) bool {
	return isSyscallError(err, windows.WSAEADDRINUSE)
}

func isConnRefused(err error) bool {
	return isSyscallError(err, windows.WSAECONNREFUSED)
}

func isSocket(path string) bool {
	name, err := windows.UTF16PtrFromString(filepath.Clean(path))
	if err != nil {
		return false
	}
	var data windows.Win32finddata
	h, err := windows.FindFirstFile(name, &data)
	if err != nil {
		return false
	}
	defer windows.FindClose(h)
	// IO_REPARSE_TAG_AF_UNIX. Inspect the tag directly so this also works
	// with Go 1.22, whose os.FileMode does not identify Windows sockets.
	const afUnixTag = 0x80000023
	return data.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 && data.Reserved0 == afUnixTag
}

func (l *listener) setPermissions() {}

func (l *listener) setOption(string, interface{}) error {
	// POSIX ownership and permissions do not apply to Windows AF_UNIX sockets.
	return mangos.ErrBadOption
}
