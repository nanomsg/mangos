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

package ws

import (
	"testing"

	"nanomsg.org/go/mangos/v2"
	. "nanomsg.org/go/mangos/v2/internal/test"
)

var tran = Transport

func TestWsOptions(t *testing.T) {
	TranVerifyInvalidOption(t, tran)
	TranVerifyIntOption(t, tran, mangos.OptionMaxRecvSize)
	TranVerifyNoDelayOption(t, tran)
	TranVerifyBoolOption(t, tran, OptionWebSocketCheckOrigin)
}

func TestWsScheme(t *testing.T) {
	TranVerifyScheme(t, tran)
}
func TestWsRecvMax(t *testing.T) {
	TranVerifyMaxRecvSize(t, tran, nil, nil)
}
func TestWsAcceptWithoutListen(t *testing.T) {
	TranVerifyAcceptWithoutListen(t, tran)
}
func TestWsListenAndAccept(t *testing.T) {
	TranVerifyListenAndAccept(t, tran, nil, nil)
}
func TestWsDuplicateListen(t *testing.T) {
	TranVerifyDuplicateListen(t, tran, nil)
}
func TestWsConnectionRefused(t *testing.T) {
	TranVerifyConnectionRefused(t, tran, nil)
}
func TestTcpHandshake(t *testing.T) {
	TranVerifyHandshakeFail(t, tran, nil, nil)
}
func TestWsSendRecv(t *testing.T) {
	TranVerifySendRecv(t, tran, nil, nil)
}
func TestWsAnonymousPort(t *testing.T) {
	TranVerifyAnonymousPort(t, "ws://127.0.0.1:0/", nil, nil)
}
func TestWsInvalidDomain(t *testing.T) {
	TranVerifyBadAddress(t, "ws://invalid.invalid/", nil, nil)
}
func TestWsInvalidURI(t *testing.T) {
	TranVerifyBadAddress(t, "ws://127.0.0.1:80/\x01", nil, nil)
}

func TestWsInvalidLocalIP(t *testing.T) {
	TranVerifyBadLocalAddress(t, "ws://1.1.1.1:80", nil)
}
func TestWsBroadcastIP(t *testing.T) {
	TranVerifyBadAddress(t, "ws://255.255.255.255:80", nil, nil)
}

func TestWsListenerClosed(t *testing.T) {
	TranVerifyListenerClosed(t, tran, nil)
}

func TestWsResolverChange(t *testing.T) {
	sock := GetMockSocket()
	defer MustClose(t, sock)

	addr := AddrTestWS()
	MustSucceed(t, sock.Listen(addr))

	d, e := tran.NewDialer(addr, sock)
	MustSucceed(t, e)
	td := d.(*dialer)
	addr = td.addr
	td.addr = "ws://invalid.invalid:80"
	p, e := d.Dial()
	MustFail(t, e)
	MustBeTrue(t, p == nil)

	td.addr = addr
	p, e = d.Dial()
	MustSucceed(t, e)
	MustSucceed(t, p.Close())
}

func TestWsPipeOptions(t *testing.T) {
	TranVerifyPipeOptions(t, tran, nil, nil)
}

func TestWsMessageSize(t *testing.T) {
	TranVerifyMessageSizes(t, tran, nil, nil)
}

func TestWsMessageHeader(t *testing.T) {
	TranVerifyMessageHeader(t, tran, nil, nil)
}
