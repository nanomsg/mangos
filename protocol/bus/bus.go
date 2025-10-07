// Copyright 2018 The Mangos Authors
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

// Package bus implements the BUS protocol.  In this protocol, participants
// send a message to each of their peers.
package bus

import (
	"context"

	"go.nanomsg.org/mangos/v3/protocol"
	"go.nanomsg.org/mangos/v3/protocol/xbus"
)

type socket struct {
	protocol.Protocol
}

// Protocol identity information.
const (
	Self     = protocol.ProtoBus
	Peer     = protocol.ProtoBus
	SelfName = "bus"
	PeerName = "bus"
)

func (s *socket) GetOption(name string) (interface{}, error) {
	switch name {
	case protocol.OptionRaw:
		return false, nil
	}
	return s.Protocol.GetOption(name)
}

func (s *socket) SendMsg(m *protocol.Message) error {
	return s.SendMsgContext(context.Background(), m)
}

func (s *socket) SendMsgContext(ctx context.Context, m *protocol.Message) error {
	if len(m.Header) > 0 {
		m.Header = m.Header[:0]
	}
	return s.Protocol.SendMsgContext(ctx, m)
}

func (s *socket) RecvMsg() (*protocol.Message, error) {
	return s.RecvMsgContext(context.Background())
}

func (s *socket) RecvMsgContext(ctx context.Context) (*protocol.Message, error) {
	m, e := s.Protocol.RecvMsgContext(ctx)
	if m != nil {
		// Strip the raw mode header, as we don't use it in cooked mode
		m.Header = m.Header[:0]
	}
	return m, e
}

// NewProtocol returns a new protocol implementation.
func NewProtocol() protocol.Protocol {

	s := &socket{
		Protocol: xbus.NewProtocol(),
	}
	return s
}

// NewSocket allocates a raw Socket using the BUS protocol.
func NewSocket() (protocol.Socket, error) {
	return protocol.MakeSocket(NewProtocol()), nil
}
