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

package tcp

import (
	"testing"
	"time"

	"nanomsg.org/go/mangos/v2"
	"nanomsg.org/go/mangos/v2/internal/test"
)

var tran = Transport

func TestTcpRecvMax(t *testing.T) {
	test.TranVerifyMaxRecvSize(t, test.AddrTestTCP(), nil, nil)
}

func TestTcpOptions(t *testing.T) {
	test.TranVerifyInvalidOption(t, tran)
	test.TranVerifyIntOption(t, tran, mangos.OptionMaxRecvSize)
	test.TranVerifyNoDelayOption(t, tran)
	test.TranVerifyKeepAliveOption(t, tran)
}

func TestTcpScheme(t *testing.T) {
	test.TranVerifyScheme(t, tran)
}
func TestTcpAcceptWithoutListen(t *testing.T) {
	test.TranVerifyAcceptWithoutListen(t, tran)
}
func TestTcpListenAndAccept(t *testing.T) {
	test.TranVerifyListenAndAccept(t, tran, nil, nil)
}
func TestTcpDuplicateListen(t *testing.T) {
	test.TranVerifyDuplicateListen(t, tran, nil)
}
func TestTcpConnectionRefused(t *testing.T) {
	test.TranVerifyConnectionRefused(t, tran, nil)
}
func TestTcpHandshake(t *testing.T) {
	test.TranVerifyHandshakeFail(t, tran, nil, nil)
}
func TestTcpSendRecv(t *testing.T) {
	test.TranVerifySendRecv(t, tran, nil, nil)
}
func TestTcpAnonymousPort(t *testing.T) {
	test.TranVerifyAnonymousPort(t, "tcp://127.0.0.1:0", nil, nil)
}
func TestTcpInvalidDomain(t *testing.T) {
	test.TranVerifyBadAddress(t, "tcp://invalid.invalid", nil, nil)
}
func TestTcpInvalidLocalIP(t *testing.T) {
	test.TranVerifyBadLocalAddress(t, "tcp://1.1.1.1:80", nil)
}
func TestTcpBroadcastIP(t *testing.T) {
	test.TranVerifyBadAddress(t, "tcp://255.255.255.255:80", nil, nil)
}

func TestTcpListenerClosed(t *testing.T) {
	test.TranVerifyListenerClosed(t, tran, nil)
}

func TestTcpResolverChange(t *testing.T) {
	sock := test.GetMockSocket()
	defer test.MustClose(t, sock)

	addr := test.AddrTestTCP()
	test.MustSucceed(t, sock.Listen(addr))

	d, e := tran.NewDialer(addr, sock)
	test.MustSucceed(t, e)
	td := d.(*dialer)
	addr = td.addr
	td.addr = "tcp://invalid.invalid:80"
	p, e := d.Dial()
	test.MustFail(t, e)
	test.MustBeTrue(t, p == nil)

	td.addr = addr
	p, e = d.Dial()
	test.MustSucceed(t, e)
	test.MustSucceed(t, p.Close())
}

func TestTcpAcceptAbort(t *testing.T) {
	sock := test.GetMockSocket()
	defer test.MustClose(t, sock)

	addr := test.AddrTestTCP()
	l, e := tran.NewListener(addr, sock)
	test.MustSucceed(t, e)
	test.MustSucceed(t, l.Listen())
	_ = l.(*listener).l.Close()
	// This will make the accept loop spin hard, but nothing much
	// we can do about it.
	time.Sleep(time.Millisecond * 50)
}
