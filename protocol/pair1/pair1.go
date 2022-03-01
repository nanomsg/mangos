// Copyright 2022 The Mangos Authors
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

// Package pair1 implements the PAIRv1 protocol.  This protocol is a 1:1
// peering protocol.  (Only monogamous mode is supported, but we can connect
// to poly peers.)
package pair1

import (
	"go.nanomsg.org/mangos/v3/protocol"
	"go.nanomsg.org/mangos/v3/protocol/xpair1"
)

// Protocol identity information.
const (
	Self     = protocol.ProtoPair1
	Peer     = protocol.ProtoPair1
	SelfName = "pair1"
	PeerName = "pair1"
)

type socket struct {
	protocol.Protocol
}

func (s *socket) GetOption(name string) (interface{}, error) {
	switch name {
	case protocol.OptionRaw:
		return false, nil
	}
	return s.Protocol.GetOption(name)
}

func (s *socket) SendMsg(m *protocol.Message) error {
	m.Header = make([]byte, 4)
	err := s.Protocol.SendMsg(m)
	if err != nil {
		m.Header = m.Header[:0]
	}
	return err
}

func (s *socket) RecvMsg() (*protocol.Message, error) {
	m, err := s.Protocol.RecvMsg()
	if err == nil && m != nil {
		m.Header = m.Header[:0]
	}
	return m, err
}

// NewProtocol returns a new protocol implementation.
func NewProtocol() protocol.Protocol {
	s := &socket{
		Protocol: xpair1.NewProtocol(),
	}
	return s
}

// NewSocket allocates a raw Socket using the PAIR protocol.
func NewSocket() (protocol.Socket, error) {
	return protocol.MakeSocket(NewProtocol()), nil
}
