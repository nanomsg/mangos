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
	"nanomsg.org/go/mangos/v2/protocol/rep"
	"nanomsg.org/go/mangos/v2/protocol/req"
	_ "nanomsg.org/go/mangos/v2/transport/inproc"
)

func TestReqRetryLateConnect(t *testing.T) {
	addr := AddrTestInp()

	sockreq, err := req.NewSocket()
	MustSucceed(t, err)
	MustNotBeNil(t, sockreq)
	defer sockreq.Close()

	sockrep, err := rep.NewSocket()
	MustSucceed(t, err)
	MustNotBeNil(t, sockrep)
	defer sockrep.Close()

	d, err := sockreq.NewDialer(addr, nil)
	MustSucceed(t, err)
	MustNotBeNil(t, d)

	MustSucceed(t, sockreq.SetOption(mangos.OptionReconnectTime,
		time.Millisecond*100))
	MustSucceed(t, sockreq.SetOption(mangos.OptionDialAsynch, true))

	l, err := sockrep.NewListener(addr, nil)
	MustSucceed(t, err)
	MustNotBeNil(t, l)

	MustSucceed(t, d.Dial())

	m := mangos.NewMessage(0)
	m.Body = append(m.Body, []byte("hello")...)
	MustSucceed(t, sockreq.SetOption(mangos.OptionBestEffort, true))
	MustSucceed(t, sockreq.SendMsg(m))

	MustSucceed(t, l.Listen())

	m, err = sockrep.RecvMsg()
	MustSucceed(t, err)
	MustNotBeNil(t, m)

	m.Body = append(m.Body, []byte(" there")...)
	MustSucceed(t, sockrep.SendMsg(m))

	m, err = sockreq.RecvMsg()
	MustSucceed(t, err)
	MustNotBeNil(t, m)

	m.Free()
}

func TestReqRetryReconnect(t *testing.T) {
	addr := AddrTestInp()

	sockreq, err := req.NewSocket()
	MustSucceed(t, err)
	MustNotBeNil(t, sockreq)
	defer sockreq.Close()

	sockrep, err := rep.NewSocket()
	MustSucceed(t, err)
	MustNotBeNil(t, sockrep)
	defer sockrep.Close()

	d, err := sockreq.NewDialer(addr, nil)
	MustSucceed(t, err)
	MustNotBeNil(t, d)

	MustSucceed(t, sockreq.SetOption(mangos.OptionReconnectTime,
		time.Millisecond*100))
	MustSucceed(t, sockreq.SetOption(mangos.OptionDialAsynch, true))

	l, err := sockrep.NewListener(addr, nil)
	MustSucceed(t, err)
	MustNotBeNil(t, l)

	MustSucceed(t, d.Dial())

	rep2, err := rep.NewSocket()
	MustSucceed(t, err)
	MustNotBeNil(t, rep2)
	defer rep2.Close()

	l2, err := rep2.NewListener(addr, nil)
	MustSucceed(t, err)
	MustNotBeNil(t, l2)

	MustSucceed(t, l.Listen())
	time.Sleep(time.Millisecond * 50)

	m := mangos.NewMessage(0)
	m.Body = append(m.Body, []byte("hello")...)
	MustSucceed(t, sockreq.SendMsg(m))

	m, err = sockrep.RecvMsg()
	MustSucceed(t, err)
	MustNotBeNil(t, m)

	// Now close the connection -- no reply!
	l.Close()
	sockrep.Close()

	// Open the new one on the other socket
	MustSucceed(t, l2.Listen())
	m, err = rep2.RecvMsg()
	MustSucceed(t, err)
	MustNotBeNil(t, m)

	m.Body = append(m.Body, []byte(" again")...)
	MustSucceed(t, rep2.SendMsg(m))

	m, err = sockreq.RecvMsg()
	MustSucceed(t, err)
	MustNotBeNil(t, m)

	m.Free()
}
