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
	"nanomsg.org/go/mangos/v2/protocol/bus"
	"nanomsg.org/go/mangos/v2/protocol/xbus"

	_ "nanomsg.org/go/mangos/v2/transport/inproc"
)

func TestBusDeviceXbus(t *testing.T) {
	s1, err := xbus.NewSocket()
	MustSucceed(t, err)
	MustNotBeNil(t, s1)
	defer s1.Close()
	MustSucceed(t, mangos.Device(s1, s1))
}

func TestBusDeviceCooked(t *testing.T) {
	s1, err := bus.NewSocket()
	MustSucceed(t, err)
	MustNotBeNil(t, s1)
	defer s1.Close()
	MustFail(t, mangos.Device(s1, s1))
}

func TestBusDevice(t *testing.T) {

	s1, err := xbus.NewSocket()
	MustSucceed(t, err)
	MustNotBeNil(t, s1)

	defer s1.Close()
	MustSucceed(t, s1.Listen("inproc://busdevicetest"))
	MustSucceed(t, mangos.Device(s1, s1))

	c1, err := bus.NewSocket()
	MustSucceed(t, err)
	MustNotBeNil(t, c1)
	defer c1.Close()

	c2, err := bus.NewSocket()
	MustSucceed(t, err)
	MustNotBeNil(t, c2)
	defer c2.Close()

	MustSucceed(t, c1.SetOption(mangos.OptionRecvDeadline, time.Millisecond*100))
	MustSucceed(t, c2.SetOption(mangos.OptionRecvDeadline, time.Millisecond*100))

	MustSucceed(t, c1.Dial("inproc://busdevicetest"))
	MustSucceed(t, c2.Dial("inproc://busdevicetest"))

	m := mangos.NewMessage(0)
	m.Body = append(m.Body, []byte{1, 2, 3, 4}...)

	// Because dial is not synchronous...
	time.Sleep(time.Millisecond * 100)

	MustSucceed(t, c1.SendMsg(m))
	m2, e := c2.RecvMsg()
	MustSucceed(t, e)
	MustNotBeNil(t, m2)
	MustBeTrue(t, len(m2.Body) == 4)

	// But not to ourself!
	m3, e := c1.RecvMsg()
	MustFail(t, e)
	MustBeNil(t, m3)
}
