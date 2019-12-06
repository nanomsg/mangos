// Copyright 2019 The Mangos Authors
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

// +build windows

package ipc

import (
	"nanomsg.org/go/mangos/v2"
	"testing"

	. "nanomsg.org/go/mangos/v2/internal/test"
)

func TestIpcListenerOptions(t *testing.T) {
	sock := GetMockSocket()
	l, e := tran.NewListener(AddrTestIPC(), sock)
	MustSucceed(t, e)

	// Security Descriptor
	sd := "O:AOG:DAD:(A;;RPWPCCDCLCSWRCWDWOGA;;;S-1-0-0)"
	MustBeError(t, l.SetOption(OptionSecurityDescriptor, 0), mangos.ErrBadValue)
	MustBeError(t, l.SetOption(OptionSecurityDescriptor, true), mangos.ErrBadValue)
	MustSucceed(t, l.SetOption(OptionSecurityDescriptor, sd)) // SDDL not validated
	v, e := l.GetOption(OptionSecurityDescriptor)
	MustSucceed(t, e)
	sd2, ok := v.(string)
	MustBeTrue(t, ok)
	MustBeTrue(t, sd2 == sd)

	for _, opt := range []string{OptionInputBufferSize, OptionOutputBufferSize} {
		MustBeError(t, l.SetOption(opt, "string"), mangos.ErrBadValue)
		MustBeError(t, l.SetOption(opt, true), mangos.ErrBadValue)
		MustSucceed(t, l.SetOption(opt, int32(16384)))
		v, e = l.GetOption(opt)
		MustSucceed(t, e)
		v2, ok := v.(int32)
		MustBeTrue(t, ok)
		MustBeTrue(t, v2 == 16384)
	}
}
