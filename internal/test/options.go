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
	"reflect"
	"testing"
	"time"

	"nanomsg.org/go/mangos/v2"
)

func VerifyInvalidOption(t *testing.T, f func() (mangos.Socket, error)) {
	s, err := f()
	MustSucceed(t, err)
	_, err = s.GetOption("nosuchoption")
	MustBeError(t, err, mangos.ErrBadOption)

	MustBeError(t, s.SetOption("nosuchoption", 0), mangos.ErrBadOption)
	MustSucceed(t, s.Close())
}

// VerifyOptionDuration validates time.Duration options
func VerifyOptionDuration(t *testing.T, f func() (mangos.Socket, error), option string) {
	s, err := f()
	MustSucceed(t, err)
	val, err := s.GetOption(option)
	MustSucceed(t, err)
	MustBeTrue(t, reflect.TypeOf(val) == reflect.TypeOf(time.Duration(0)))

	MustSucceed(t, s.SetOption(option, time.Second))
	val, err = s.GetOption(option)
	MustSucceed(t, err)
	MustBeTrue(t, val.(time.Duration) == time.Second)

	MustBeError(t, s.SetOption(option, time.Now()), mangos.ErrBadValue)
	MustBeError(t, s.SetOption(option, "junk"), mangos.ErrBadValue)
	MustSucceed(t, s.Close())
}

func VerifyOptionInt(t *testing.T, f func() (mangos.Socket, error), option string) {
	s, err := f()
	MustSucceed(t, err)
	val, err := s.GetOption(option)
	MustSucceed(t, err)
	MustBeTrue(t, reflect.TypeOf(val) == reflect.TypeOf(1))

	MustSucceed(t, s.SetOption(option, 2))
	val, err = s.GetOption(option)
	MustSucceed(t, err)
	MustBeTrue(t, val.(int) == 2)

	MustBeError(t, s.SetOption(option, time.Now()), mangos.ErrBadValue)
	MustBeError(t, s.SetOption(option, "junk"), mangos.ErrBadValue)
	MustSucceed(t, s.Close())
}

func VerifyOptionQLen(t *testing.T, f func() (mangos.Socket, error), option string) {
	s, err := f()
	MustSucceed(t, err)
	val, err := s.GetOption(option)
	MustSucceed(t, err)
	MustBeTrue(t, reflect.TypeOf(val) == reflect.TypeOf(1))

	MustSucceed(t, s.SetOption(option, 2))
	val, err = s.GetOption(option)
	MustSucceed(t, err)
	MustBeTrue(t, val.(int) == 2)

	// Queue lengths are not permitted to be negative.
	MustBeError(t, s.SetOption(option, -1), mangos.ErrBadValue)

	MustBeError(t, s.SetOption(option, time.Now()), mangos.ErrBadValue)
	MustBeError(t, s.SetOption(option, "junk"), mangos.ErrBadValue)
	MustSucceed(t, s.Close())
}

func VerifyOptionBool(t *testing.T, f func() (mangos.Socket, error), option string) {
	s, err := f()
	MustSucceed(t, err)
	val, err := s.GetOption(option)
	MustSucceed(t, err)
	MustBeTrue(t, reflect.TypeOf(val) == reflect.TypeOf(true))

	MustSucceed(t, s.SetOption(option, true))
	val, err = s.GetOption(option)
	MustSucceed(t, err)
	MustBeTrue(t, val.(bool) == true)

	MustSucceed(t, s.SetOption(option, false))
	val, err = s.GetOption(option)
	MustSucceed(t, err)
	MustBeTrue(t, val.(bool) == false)

	MustBeError(t, s.SetOption(option, time.Now()), mangos.ErrBadValue)
	MustBeError(t, s.SetOption(option, "junk"), mangos.ErrBadValue)
	MustSucceed(t, s.Close())
}

func VerifyOptionTTL(t *testing.T, f func() (mangos.Socket, error)) {
	VerifyOptionInt(t, f, mangos.OptionTTL)
	SetTTLZero(t, f)
	SetTTLNegative(t, f)
	SetTTLTooBig(t, f)
	SetTTL(t, f)
}

func VerifyOptionMaxRecvSize(t *testing.T, f func() (mangos.Socket, error)) {
	VerifyOptionInt(t, f, mangos.OptionMaxRecvSize)
	// Max Receive size must not be negative.
	s := GetSocket(t, f)
	MustBeError(t, s.SetOption(mangos.OptionMaxRecvSize, -1), mangos.ErrBadValue)
	// Can set it to zero.
	MustSucceed(t, s.SetOption(mangos.OptionMaxRecvSize, 0))
	// Can set it to some other values
	MustSucceed(t, s.SetOption(mangos.OptionMaxRecvSize, 1024))
}
