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
	"sync"
	"testing"
	"time"

	"nanomsg.org/go/mangos/v2"
	"nanomsg.org/go/mangos/v2/protocol/rep"
	"nanomsg.org/go/mangos/v2/protocol/req"
	_ "nanomsg.org/go/mangos/v2/transport/tcp"
)

type phookinfo struct {
	action mangos.PipeEvent
	server bool
	addr   string
}

func (i phookinfo) String() string {
	var s string
	switch i.action {
	case mangos.PipeEventAttaching:
		s = "Attaching"
	case mangos.PipeEventAttached:
		s = "Attached "
	case mangos.PipeEventDetached:
		s = "Detached "
	default:
		s = "???????? "
	}
	if i.server {
		s += "SRV "
	} else {
		s += "CLI "
	}
	s += i.addr + " "
	return s
}

type phooktest struct {
	t      *testing.T
	calls  []phookinfo
	expect []phookinfo
	allow  bool
	sync.Mutex
}

func (h *phooktest) Hook(action mangos.PipeEvent, p mangos.Pipe) {
	h.t.Logf("Hook called - %v", action)
	i := phookinfo{
		action: action,
		addr:   p.Address(),
		server: p.Listener() != nil,
	}
	h.Lock()
	h.calls = append(h.calls, i)
	h.Unlock()
	if !h.allow {
		p.Close()
	}
}

func TestPipeHook(t *testing.T) {

	srvtest := &phooktest{allow: true, t: t}
	clitest := &phooktest{allow: true, t: t}

	addr := AddrTestTCP()

	srvtest.expect = []phookinfo{
		{
			action: mangos.PipeEventAttaching,
			addr:   addr,
			server: true,
		}, {
			action: mangos.PipeEventAttached,
			addr:   addr,
			server: true,
		}, {
			action: mangos.PipeEventDetached,
			addr:   addr,
			server: true,
		},
	}

	clitest.expect = []phookinfo{
		{
			action: mangos.PipeEventAttaching,
			addr:   addr,
			server: false,
		}, {
			action: mangos.PipeEventAttached,
			addr:   addr,
			server: false,
		}, {
			action: mangos.PipeEventDetached,
			addr:   addr,
			server: false,
		},
	}

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

	l, err := sockrep.NewListener(addr, nil)
	MustSucceed(t, err)
	MustNotBeNil(t, l)

	hook := sockreq.SetPipeEventHook(clitest.Hook)
	MustBeNil(t, hook) // no previous hook

	hook = sockrep.SetPipeEventHook(srvtest.Hook)
	MustBeNil(t, hook) // no previous hook

	MustSucceed(t, l.Listen())
	MustSucceed(t, d.Dial())

	// time for conn to establish
	time.Sleep(time.Millisecond * 100)

	// Shutdown the sockets
	d.Close()
	l.Close()

	sockrep.Close()
	sockreq.Close()

	time.Sleep(100 * time.Millisecond)

	clitest.Lock()
	defer clitest.Unlock()

	srvtest.Lock()
	defer srvtest.Unlock()

	MustBeTrue(t, len(srvtest.calls) == len(srvtest.expect))
	for i := range srvtest.calls {
		MustBeTrue(t, srvtest.calls[i].String() == srvtest.expect[i].String())
	}
	MustBeTrue(t, len(clitest.calls) == len(clitest.expect))
	for i := range clitest.calls {
		MustBeTrue(t, clitest.calls[i].String() == clitest.expect[i].String())
	}
}
