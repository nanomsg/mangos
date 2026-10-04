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

const (
	// OptionSocketPermissions is used to set the permissions on the
	// UNIX domain socket via chmod.  The argument is a uint32, and
	// represents the mode passed to chmod().  This is
	// done on the server side on POSIX systems. Windows does not support
	// this option. Be aware that relying on
	// socket permissions for enforcement is not portable.
	OptionSocketPermissions = "UNIX-IPC-CHMOD"

	// OptionSocketOwner is used to set the socket owner by
	// using chown on the server socket on POSIX systems.  This will only work if
	// the process has permission.   The argument is an int.
	// If this fails to set at socket creation time,
	// no error is reported.
	OptionSocketOwner = "UNIX-IPC-OWNER"

	// OptionSocketGroup is used to set the socket group by
	// using chown on the server socket on POSIX systems.  This will only work if
	// the process has permission.   The argument is an int.
	// If this fails to set at socket creation time,
	// no error is reported.
	OptionSocketGroup = "UNIX-IPC-GROUP"
)
