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

package rep

import (
	"testing"
	"testing/synctest"
	"time"

	"go.nanomsg.org/mangos/v3"
)

func TestRepRecvNonBlockingReady(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s := NewProtocol().(*socket)
		defer s.Close()
		if err := s.SetOption(mangos.OptionRecvDeadline, -time.Nanosecond); err != nil {
			t.Fatal(err)
		}
		for i := 0; i < 100; i++ {
			message := mangos.NewMessage(0)
			message.Header = []byte{0x80, 0, 0, 1}
			go func() { s.recvQ <- recvQEntry{m: message} }()
			synctest.Wait()
			received, err := s.RecvMsg()
			if err != nil {
				t.Fatal(err)
			}
			if received != message {
				t.Fatal("did not receive the waiting request")
			}
			received.Free()
		}
	})
}
