/*
 * Copyright 2022 The Mangos Authors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use file except in compliance with the License.
 *  You may obtain a copy of the license at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 * limitations under the License.
 *
 */

package pair1

import (
	"testing"

	"go.nanomsg.org/mangos/v3"

	. "go.nanomsg.org/mangos/v3/internal/test"
	_ "go.nanomsg.org/mangos/v3/transport/inproc"
)

func TestPair1Identity(t *testing.T) {
	s := GetSocket(t, NewSocket)
	id := s.Info()
	MustBeTrue(t, id.Self == mangos.ProtoPair1)
	MustBeTrue(t, id.Peer == mangos.ProtoPair1)
	MustBeTrue(t, id.SelfName == "pair1")
	MustBeTrue(t, id.PeerName == "pair1")
	MustClose(t, s)
}

func TestPairCooked(t *testing.T) {
	VerifyCooked(t, NewSocket)
}

func TestPair1SendReceive(t *testing.T) {
	self := GetSocket(t, NewSocket)
	peer := GetSocket(t, NewSocket)
	ConnectPair(t, self, peer)
	MustSendString(t, self, "ping")
	m := MustRecvMsg(t, peer)
	MustBeTrue(t, len(m.Header) == 0)
	MustClose(t, self)
	MustClose(t, peer)
}
func TestPairClosed(t *testing.T) {
	VerifyClosedSend(t, NewSocket)
	VerifyClosedRecv(t, NewSocket)
	VerifyClosedClose(t, NewSocket)
	VerifyClosedDial(t, NewSocket)
	VerifyClosedListen(t, NewSocket)
	VerifyClosedAddPipe(t, NewSocket)
}

func TestPairOptions(t *testing.T) {
	VerifyInvalidOption(t, NewSocket)
	VerifyOptionDuration(t, NewSocket, mangos.OptionRecvDeadline)
	VerifyOptionDuration(t, NewSocket, mangos.OptionSendDeadline)
	VerifyOptionInt(t, NewSocket, mangos.OptionReadQLen)
	VerifyOptionInt(t, NewSocket, mangos.OptionWriteQLen)
	VerifyOptionBool(t, NewSocket, mangos.OptionBestEffort)
}
