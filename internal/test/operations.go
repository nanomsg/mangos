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

package test

import (
	"testing"
	"time"

	"nanomsg.org/go/mangos/v2"
)

// CannotSend verifies that the socket cannot send.
func CannotSend(t *testing.T, f func() (mangos.Socket, error)) {
	s := GetSocket(t, f)

	// Not all protocols support this option, but try.
	_ = s.SetOption(mangos.OptionSendDeadline, time.Millisecond)

	MustBeError(t, s.Send([]byte{0, 1, 2, 3}), mangos.ErrProtoOp)
	MustSucceed(t, s.Close())
}

// CannotRecv verifies that the socket cannot recv.
func CannotRecv(t *testing.T, f func() (mangos.Socket, error)) {
	s := GetSocket(t, f)
	_ = s.SetOption(mangos.OptionRecvDeadline, time.Millisecond)

	v, err := s.Recv()
	MustBeError(t, err, mangos.ErrProtoOp)
	MustBeNil(t, v)
	MustSucceed(t, s.Close())
}

func GetSocket(t *testing.T, f func() (mangos.Socket, error)) mangos.Socket {
	s, err := f()
	MustSucceed(t, err)
	MustNotBeNil(t, s)
	return s
}