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

package test

import (
	"bytes"
	"encoding/binary"
	"testing"
	"testing/synctest"
	"time"

	"go.nanomsg.org/mangos/v3"
	itest "go.nanomsg.org/mangos/v3/internal/test"
	"go.nanomsg.org/mangos/v3/protocol/bus"
	"go.nanomsg.org/mangos/v3/protocol/pair"
	"go.nanomsg.org/mangos/v3/protocol/pair1"
	"go.nanomsg.org/mangos/v3/protocol/pull"
	"go.nanomsg.org/mangos/v3/protocol/push"
	"go.nanomsg.org/mangos/v3/protocol/rep"
	"go.nanomsg.org/mangos/v3/protocol/req"
	"go.nanomsg.org/mangos/v3/protocol/respondent"
	"go.nanomsg.org/mangos/v3/protocol/star"
	"go.nanomsg.org/mangos/v3/protocol/sub"
	"go.nanomsg.org/mangos/v3/protocol/surveyor"
	"go.nanomsg.org/mangos/v3/protocol/xbus"
	"go.nanomsg.org/mangos/v3/protocol/xpair"
	"go.nanomsg.org/mangos/v3/protocol/xpair1"
	"go.nanomsg.org/mangos/v3/protocol/xpull"
	"go.nanomsg.org/mangos/v3/protocol/xpush"
	"go.nanomsg.org/mangos/v3/protocol/xrep"
	"go.nanomsg.org/mangos/v3/protocol/xreq"
	"go.nanomsg.org/mangos/v3/protocol/xrespondent"
	"go.nanomsg.org/mangos/v3/protocol/xstar"
	"go.nanomsg.org/mangos/v3/protocol/xsub"
	"go.nanomsg.org/mangos/v3/protocol/xsurveyor"
)

type deadlineEndpoint interface {
	SetOption(string, interface{}) error
	SendMsg(*mangos.Message) error
	RecvMsg() (*mangos.Message, error)
}

// Bound a broken nonblocking operation so failures cannot hang the test suite.
func deadlineRecv(t *testing.T, endpoint deadlineEndpoint) (*mangos.Message, error) {
	t.Helper()
	type result struct {
		m   *mangos.Message
		err error
	}
	done := make(chan result, 1)
	go func() { m, err := endpoint.RecvMsg(); done <- result{m, err} }()
	select {
	case r := <-done:
		return r.m, r.err
	case <-time.After(time.Second):
		t.Fatal("negative receive deadline blocked")
		return nil, nil
	}
}

func TestNegativeRecvDeadline(t *testing.T) {
	for _, tc := range []struct {
		name      string
		newSocket func() (mangos.Socket, error)
		header    string
	}{
		{"bus", bus.NewSocket, ""},
		{"pair", pair.NewSocket, ""},
		{"pair1", pair1.NewSocket, "hop"},
		{"pull", pull.NewSocket, ""},
		{"rep", rep.NewSocket, "route"},
		{"req", req.NewSocket, "request"},
		{"respondent", respondent.NewSocket, "route"},
		{"star", star.NewSocket, "hop"},
		{"sub", sub.NewSocket, ""},
		{"surveyor", surveyor.NewSocket, "request"},
		{"xbus", xbus.NewSocket, ""},
		{"xpair", xpair.NewSocket, ""},
		{"xpair1", xpair1.NewSocket, "hop"},
		{"xpull", xpull.NewSocket, ""},
		{"xrep", xrep.NewSocket, "route"},
		{"xreq", xreq.NewSocket, "route"},
		{"xrespondent", xrespondent.NewSocket, "route"},
		{"xstar", xstar.NewSocket, "hop"},
		{"xsub", xsub.NewSocket, ""},
		{"xsurveyor", xsurveyor.NewSocket, "route"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, useContext := range []bool{false, true} {
				name := "socket"
				if useContext {
					name = "context"
				}
				t.Run(name, func(t *testing.T) {
					s := itest.GetSocket(t, tc.newSocket)
					t.Cleanup(func() { _ = s.Close() })
					var endpoint deadlineEndpoint = s
					if useContext {
						c, err := s.OpenContext()
						if err == mangos.ErrProtoOp {
							t.Skip("protocol has no contexts")
						}
						itest.MustSucceed(t, err)
						defer c.Close()
						endpoint = c
					}
					if tc.name == "sub" {
						itest.MustSucceed(t, endpoint.SetOption(mangos.OptionSubscribe, []byte{}))
					}
					itest.MustSucceed(t, endpoint.SetOption(mangos.OptionRecvDeadline, -time.Nanosecond))
					mock, _ := itest.MockConnect(t, s)
					prepare := func() []byte {
						switch tc.header {
						case "route":
							return []byte{0x80, 0, 0, 1}
						case "hop":
							return []byte{0, 0, 0, 0}
						case "request":
							if tc.name == "req" {
								itest.MustSucceed(t, endpoint.SetOption(mangos.OptionSendDeadline, time.Second))
							}
							itest.MustSucceed(t, endpoint.SendMsg(mangos.NewMessage(0)))
							request, err := mock.MockRecvMsg(time.Second)
							itest.MustSucceed(t, err)
							header := append([]byte(nil), request.Header...)
							request.Free()
							return header
						}
						return nil
					}
					prepare()
					m, err := deadlineRecv(t, endpoint)
					itest.MustBeNil(t, m)
					itest.MustBeError(t, err, mangos.ErrRecvTimeout)

					if tc.name == "rep" {
						// REP has an unbuffered receive queue; readiness is tested
						// inside its package with a sender known to be waiting.
						return
					}

					// Repeat after a timeout to verify stateful contexts remain usable.
					// Receiving a second wire message proves that the protocol has finished
					// processing the first, so its data is ready before the nonblocking call.
					for i := 0; i < 20; i++ {
						header := prepare()
						for j := 0; j < 2; j++ {
							message := mangos.NewMessage(0)
							message.Body = append(message.Body, header...)
							message.Body = append(message.Body, 'o', 'k')
							itest.MustSucceed(t, mock.MockSendMsg(message, time.Second))
						}
						m, err := deadlineRecv(t, endpoint)
						itest.MustSucceed(t, err)
						if !bytes.Equal(m.Body, []byte("ok")) {
							t.Fatalf("body = %q, want ok", m.Body)
						}
						m.Free()
						// REQ stores a single reply; other protocols may queue both messages.
						if tc.name != "req" {
							// The second message may still be in flight; drain using a positive
							// deadline so no stale response can mask a later readiness failure.
							itest.MustSucceed(t, endpoint.SetOption(mangos.OptionRecvDeadline, time.Second))
							m, err = endpoint.RecvMsg()
							itest.MustSucceed(t, err)
							m.Free()
							itest.MustSucceed(t, endpoint.SetOption(mangos.OptionRecvDeadline, -time.Nanosecond))
						}
					}
				})
			}
		})
	}
}

// A stalled mock transport makes backpressure deterministic without sleeps.
// The first send must succeed; once the bounded queue fills, the caller must
// get its message back with ErrSendTimeout, including its routing header.
func TestNegativeSendDeadline(t *testing.T) {
	for _, tc := range []struct {
		name      string
		newSocket func() (mangos.Socket, error)
	}{
		{"pair", pair.NewSocket},
		{"pair1", pair1.NewSocket},
		{"push", push.NewSocket},
		{"req", req.NewSocket},
		{"rep", rep.NewSocket},
		{"respondent", respondent.NewSocket},
		{"xpair", xpair.NewSocket},
		{"xpair1", xpair1.NewSocket},
		{"xpush", xpush.NewSocket},
		{"xreq", xreq.NewSocket},
		{"xrep", xrep.NewSocket},
		{"xrespondent", xrespondent.NewSocket},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, useContext := range []bool{false, true} {
				name := "socket"
				if useContext {
					name = "context"
				}
				t.Run(name, func(t *testing.T) {
					synctest.Test(t, func(t *testing.T) {
						s := itest.GetSocket(t, tc.newSocket)
						defer s.Close()
						if tc.name != "req" {
							itest.MustSucceed(t, s.SetOption(mangos.OptionWriteQLen, 1))
						}
						var endpoint deadlineEndpoint = s
						if useContext {
							c, err := s.OpenContext()
							if err == mangos.ErrProtoOp {
								t.Skip("protocol has no contexts")
							}
							itest.MustSucceed(t, err)
							defer c.Close()
							endpoint = c
						}
						itest.MustSucceed(t, endpoint.SetOption(mangos.OptionSendDeadline, -time.Nanosecond))
						mock, pipe := itest.MockConnect(t, s)
						for i := 0; i < 10; i++ {
							m := mangos.NewMessage(0)
							m.Body = append(m.Body, "ok"...)
							switch tc.name {
							case "rep", "respondent":
								itest.MustSucceed(t, endpoint.SetOption(mangos.OptionRecvDeadline, time.Second))
								incoming := mangos.NewMessage(0)
								incoming.Body = []byte{0x80, 0, 0, 1}
								itest.MustSucceed(t, mock.MockSendMsg(incoming, time.Second))
								request, err := endpoint.RecvMsg()
								itest.MustSucceed(t, err)
								request.Free()
							case "xrep", "xrespondent":
								m.Header = make([]byte, 8)
								binary.BigEndian.PutUint32(m.Header, pipe.ID())
								m.Header[4] = 0x80
							case "xpair1":
								m.Header = []byte{0, 0, 0, 0}
							case "xreq":
								m.Header = []byte{0x80, 0, 0, 1}
							}
							header := append([]byte(nil), m.Header...)
							before := time.Now()
							err := endpoint.SendMsg(m)
							if time.Since(before) != 0 {
								t.Fatal("negative send deadline waited")
							}
							if err != nil {
								itest.MustBeError(t, err, mangos.ErrSendTimeout)
								if i == 0 {
									t.Fatal("send timed out with queue space available")
								}
								// Raw routed protocols must restore the caller's routing header.
								if tc.name == "xrep" || tc.name == "xrespondent" {
									if !bytes.Equal(m.Header, header) {
										t.Fatalf("header = %x, want %x", m.Header, header)
									}
								}
								m.Free()
								return
							}
							synctest.Wait()
						}
						t.Fatal("send never reported backpressure")
					})
				})
			}
		})
	}
}

// REP and RESPONDENT used to reject zero as well as negative deadlines.
// Resetting to zero must restore blocking behavior on both sockets and contexts.
func TestDeadlineResetToBlocking(t *testing.T) {
	for _, tc := range []struct {
		name      string
		newSocket func() (mangos.Socket, error)
	}{
		{"rep", rep.NewSocket},
		{"respondent", respondent.NewSocket},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, useContext := range []bool{false, true} {
				name := "socket"
				if useContext {
					name = "context"
				}
				t.Run(name, func(t *testing.T) {
					synctest.Test(t, func(t *testing.T) {
						s := itest.GetSocket(t, tc.newSocket)
						defer s.Close()
						var endpoint deadlineEndpoint = s
						if useContext {
							c, err := s.OpenContext()
							itest.MustSucceed(t, err)
							defer c.Close()
							endpoint = c
						}
						for _, option := range []string{mangos.OptionRecvDeadline, mangos.OptionSendDeadline} {
							itest.MustSucceed(t, endpoint.SetOption(option, -time.Nanosecond))
							itest.MustSucceed(t, endpoint.SetOption(option, time.Duration(0)))
						}
						mock, _ := itest.MockConnect(t, s)
						done := make(chan error, 1)
						go func() {
							time.Sleep(time.Second)
							m := mangos.NewMessage(0)
							m.Body = []byte{0x80, 0, 0, 1}
							done <- mock.MockSendMsg(m, time.Second)
						}()
						before := time.Now()
						m, err := endpoint.RecvMsg()
						itest.MustSucceed(t, err)
						m.Free()
						itest.MustSucceed(t, <-done)
						if time.Since(before) != time.Second {
							t.Fatal("zero receive deadline did not wait for the request")
						}
					})
				})
			}
		})
	}
}
