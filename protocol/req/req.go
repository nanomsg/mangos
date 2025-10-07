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

// Package req implements the REQ protocol, which is the request side of
// the request/response pattern.  (REP is the response.)
package req

import (
	"context"
	"encoding/binary"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"go.nanomsg.org/mangos/v3/protocol"
)

// Protocol identity information.
const (
	Self     = protocol.ProtoReq
	Peer     = protocol.ProtoRep
	SelfName = "req"
	PeerName = "rep"
)

type pipe struct {
	p      protocol.Pipe
	s      *socket
	sendQ  chan *protocol.Message
	closeQ chan struct{}
	closed bool
}

type reqContext struct {
	s             *socket
	notifyCh      chan struct{}     // channel for notifications (replaces cond)
	resendTime    time.Duration     // tunable resend time
	sendExpire    time.Duration     // how long to wait in send
	receiveExpire time.Duration     // how long to wait in receive
	sendTimer     *time.Timer       // send timer
	receiveTimer  *time.Timer       // receive timer
	resendTimer   *time.Timer       // resend timeout
	reqMsg        *protocol.Message // message for transmit
	repMsg        *protocol.Message // received reply
	sendMsg       *protocol.Message // messaging waiting for send
	lastPipe      *pipe             // last pipe used for transmit
	reqID         uint32            // request ID
	receiveWait   bool              // true if a thread is blocked receiving
	bestEffort    bool              // if true, don't block waiting in send
	failNoPeers   bool              // fast fail if no peers present
	queued        bool              // true if we need to send a message
	closed        bool              // true if we are closed
}

type socket struct {
	sync.Mutex
	defCtx   *reqContext              // default context
	contexts map[*reqContext]struct{} // all contexts (set)
	ctxByID  map[uint32]*reqContext   // contexts by request ID
	nextID   uint32                   // next request ID
	closed   bool                     // true if we are closed
	sendQ    []*reqContext            // contexts waiting to send
	readyQ   []*pipe                  // pipes available for sending
	pipes    map[uint32]*pipe         // all pipes
}

// notify sends a non-blocking notification to the context
func (c *reqContext) notify() {
	select {
	case c.notifyCh <- struct{}{}:
	default:
		// Channel full, notification already pending
	}
}

func (s *socket) send() {
	for len(s.sendQ) != 0 && len(s.readyQ) != 0 {
		c := s.sendQ[0]
		s.sendQ = s.sendQ[1:]
		c.queued = false

		var m *protocol.Message
		if m = c.sendMsg; m != nil {
			c.reqMsg = m
			c.sendMsg = nil
			s.ctxByID[c.reqID] = c
			c.notify()
		} else {
			m = c.reqMsg
		}
		m.Clone()
		p := s.readyQ[0]
		s.readyQ = s.readyQ[1:]

		// Schedule retransmission for the future.
		c.lastPipe = p
		if c.resendTime > 0 {
			id := c.reqID
			c.resendTimer = time.AfterFunc(c.resendTime, func() {
				c.resendMessage(id)
			})
		}
		p.sendQ <- m
	}
}

func (p *pipe) sender() {
	s := p.s
	for {
		select {
		case m := <-p.sendQ:
			if err := p.p.SendMsg(m); err != nil {
				m.Free()
				if err == protocol.ErrClosed {
					return
				}
			}
			s.Lock()
			if !s.closed && !p.closed {
				s.readyQ = append(s.readyQ, p)
				s.send()
			}
			s.Unlock()
		case <-p.closeQ:
			return
		}
	}
}

func (p *pipe) receiver() {
	s := p.s
	for {
		m := p.p.RecvMsg()
		if m == nil {
			break
		}
		if len(m.Body) < 4 {
			m.Free()
			continue
		}
		m.Header = append(m.Header, m.Body[:4]...)
		m.Body = m.Body[4:]

		id := binary.BigEndian.Uint32(m.Header)

		s.Lock()
		// Since we just received a reply, stick our send at the
		// head of the list, since that's a good indication that
		// we're ready for another request.
		for i, rp := range s.readyQ {
			if p == rp {
				s.readyQ[0], s.readyQ[i] = s.readyQ[i], s.readyQ[0]
				break
			}
		}

		if c, ok := s.ctxByID[id]; ok {
			c.cancelSend()
			c.reqMsg.Free()
			c.reqMsg = nil
			c.repMsg = m
			delete(s.ctxByID, id)
			if c.resendTimer != nil {
				c.resendTimer.Stop()
				c.resendTimer = nil
			}
			if c.receiveTimer != nil {
				c.receiveTimer.Stop()
				c.receiveTimer = nil
			}
			c.notify()
		} else {
			// No matching receiver so just drop it.
			m.Free()
		}
		s.Unlock()
	}

	go p.Close()
}

func (p *pipe) Close() {
	_ = p.p.Close()
}

func (c *reqContext) resendMessage(id uint32) {
	s := c.s
	s.Lock()
	defer s.Unlock()
	if c.reqID == id && c.reqMsg != nil {
		if !c.queued {
			c.queued = true
			s.sendQ = append(s.sendQ, c)
			s.send()
		}
	}
}

func (c *reqContext) cancelSend() {
	s := c.s
	if c.queued {
		c.queued = false
		for i, c2 := range s.sendQ {
			if c2 == c {
				s.sendQ = append(s.sendQ[:i], s.sendQ[i+1:]...)
				return
			}
		}
	}
}

func (c *reqContext) cancel() {
	s := c.s
	c.cancelSend()
	if c.reqID != 0 {
		delete(s.ctxByID, c.reqID)
		c.reqID = 0
	}
	if c.repMsg != nil {
		c.repMsg.Free()
		c.repMsg = nil
	}
	if c.reqMsg != nil {
		c.reqMsg.Free()
		c.reqMsg = nil
	}
	if c.sendMsg != nil {
		c.sendMsg = nil
	}
	if c.resendTimer != nil {
		c.resendTimer.Stop()
		c.resendTimer = nil
	}
	if c.sendTimer != nil {
		c.sendTimer.Stop()
		c.sendTimer = nil
	}
	if c.receiveTimer != nil {
		c.receiveTimer.Stop()
		c.receiveTimer = nil
	}
	c.notify()
}

// SendMsgContext implements context-aware send
func (c *reqContext) SendMsgContext(ctx context.Context, m *protocol.Message) error {
	s := c.s

	id := atomic.AddUint32(&s.nextID, 1)
	id |= 0x80000000

	// cooked mode, we stash the header
	m.Header = append([]byte{},
		byte(id>>24), byte(id>>16), byte(id>>8), byte(id))

	s.Lock()
	if s.closed || c.closed {
		s.Unlock()
		return protocol.ErrClosed
	}

	if c.failNoPeers && len(s.pipes) == 0 {
		s.Unlock()
		return protocol.ErrNoPeers
	}
	c.cancel() // this cancels any pending send or receive calls
	c.cancelSend()

	c.reqID = id
	c.queued = true
	c.sendMsg = m

	s.sendQ = append(s.sendQ, c)

	if c.bestEffort {
		// for best effort case, we just immediately go the
		// reqMsg, and schedule sending.  No waiting.
		// This means that if the message cannot be delivered
		// immediately, it will still get a chance later.
		s.send()
		s.Unlock()
		return nil
	}

	s.send()

	// Wait for the message to be sent, context cancellation, or other events
	for c.sendMsg == m && ctx.Err() == nil && !c.closed && !(c.failNoPeers && len(s.pipes) == 0) {
		s.Unlock()
		select {
		case <-c.notifyCh:
			// State changed, re-check conditions
		case <-ctx.Done():
			// Context cancelled
		}
		s.Lock()
	}

	// Check why we exited the wait loop - order matters!
	// Check c.closed first because Close() calls cancel() which clears c.sendMsg
	if c.closed {
		s.Unlock()
		return protocol.ErrClosed
	}

	if c.sendMsg == m {
		// Message was not sent - clean up and determine error
		c.cancelSend()
		c.sendMsg = nil
		c.reqID = 0
		s.Unlock()

		if c.failNoPeers && len(s.pipes) == 0 {
			return protocol.ErrNoPeers
		}
		// Check if context was cancelled
		if ctx.Err() != nil {
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				return protocol.ErrSendTimeout
			}
			return ctx.Err()
		}
		return protocol.ErrSendTimeout
	}

	s.Unlock()
	return nil
}

func (c *reqContext) SendMsg(m *protocol.Message) error {
	ctx := context.Background()
	if c.sendExpire > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.sendExpire)
		defer cancel()
	}
	return c.SendMsgContext(ctx, m)
}

// RecvMsgContext implements context-aware receive using a goroutine to monitor ctx.Done()
func (c *reqContext) RecvMsgContext(ctx context.Context) (*protocol.Message, error) {
	s := c.s
	s.Lock()
	if s.closed || c.closed {
		s.Unlock()
		return nil, protocol.ErrClosed
	}
	if c.failNoPeers && len(s.pipes) == 0 {
		s.Unlock()
		return nil, protocol.ErrNoPeers
	}
	if c.receiveWait || c.reqID == 0 {
		s.Unlock()
		return nil, protocol.ErrProtoState
	}
	c.receiveWait = true
	id := c.reqID

	for id == c.reqID && c.repMsg == nil && ctx.Err() == nil {
		s.Unlock()
		select {
		case <-c.notifyCh:
			// State changed, re-check conditions
		case <-ctx.Done():
			// Context cancelled
		}
		s.Lock()
	}

	m := c.repMsg
	c.reqID = 0
	c.repMsg = nil
	c.receiveWait = false
	c.notify()
	s.Unlock()

	if m == nil {
		if c.closed {
			return nil, protocol.ErrClosed
		}
		// Check if context was cancelled
		if ctx.Err() != nil {
			// Return ErrRecvTimeout for deadline exceeded, otherwise return context error
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				return nil, protocol.ErrRecvTimeout
			}
			return nil, ctx.Err()
		}
		if c.failNoPeers && len(s.pipes) == 0 {
			return nil, protocol.ErrNoPeers
		}
		return nil, protocol.ErrCanceled
	}
	return m, nil
}

// RecvMsg wraps RecvMsgContext
func (c *reqContext) RecvMsg() (*protocol.Message, error) {
	ctx := context.Background()
	if c.receiveExpire > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.receiveExpire)
		defer cancel()
	}
	return c.RecvMsgContext(ctx)
}

func (c *reqContext) SetOption(name string, value interface{}) error {
	switch name {
	case protocol.OptionRetryTime:
		if v, ok := value.(time.Duration); ok {
			c.s.Lock()
			c.resendTime = v
			c.s.Unlock()
			return nil
		}
		return protocol.ErrBadValue

	case protocol.OptionRecvDeadline:
		if v, ok := value.(time.Duration); ok {
			c.s.Lock()
			c.receiveExpire = v
			c.s.Unlock()
			return nil
		}
		return protocol.ErrBadValue

	case protocol.OptionSendDeadline:
		if v, ok := value.(time.Duration); ok {
			c.s.Lock()
			c.sendExpire = v
			c.s.Unlock()
			return nil
		}
		return protocol.ErrBadValue

	case protocol.OptionBestEffort:
		if v, ok := value.(bool); ok {
			c.s.Lock()
			c.bestEffort = v
			c.s.Unlock()
			return nil
		}
		return protocol.ErrBadValue

	case protocol.OptionFailNoPeers:
		if v, ok := value.(bool); ok {
			c.s.Lock()
			c.failNoPeers = v
			c.s.Unlock()
			return nil
		}
		return protocol.ErrBadValue

	}

	return protocol.ErrBadOption
}

func (c *reqContext) GetOption(option string) (interface{}, error) {
	switch option {
	case protocol.OptionRetryTime:
		c.s.Lock()
		v := c.resendTime
		c.s.Unlock()
		return v, nil
	case protocol.OptionRecvDeadline:
		c.s.Lock()
		v := c.receiveExpire
		c.s.Unlock()
		return v, nil
	case protocol.OptionSendDeadline:
		c.s.Lock()
		v := c.sendExpire
		c.s.Unlock()
		return v, nil
	case protocol.OptionBestEffort:
		c.s.Lock()
		v := c.bestEffort
		c.s.Unlock()
		return v, nil
	case protocol.OptionFailNoPeers:
		c.s.Lock()
		v := c.failNoPeers
		c.s.Unlock()
		return v, nil
	}

	return nil, protocol.ErrBadOption
}

func (c *reqContext) Close() error {
	s := c.s
	c.s.Lock()
	defer c.s.Unlock()
	if c.closed {
		return protocol.ErrClosed
	}
	c.closed = true
	c.cancel()
	delete(s.contexts, c)
	return nil
}

func (s *socket) GetOption(option string) (interface{}, error) {
	switch option {
	case protocol.OptionRaw:
		return false, nil
	default:
		return s.defCtx.GetOption(option)
	}
}
func (s *socket) SetOption(option string, value interface{}) error {
	return s.defCtx.SetOption(option, value)
}

func (s *socket) SendMsg(m *protocol.Message) error {
	return s.defCtx.SendMsg(m)
}

func (s *socket) SendMsgContext(ctx context.Context, m *protocol.Message) error {
	return s.defCtx.SendMsgContext(ctx, m)
}

func (s *socket) RecvMsg() (*protocol.Message, error) {
	return s.defCtx.RecvMsg()
}

func (s *socket) RecvMsgContext(ctx context.Context) (*protocol.Message, error) {
	return s.defCtx.RecvMsgContext(ctx)
}

func (s *socket) Close() error {
	s.Lock()

	if s.closed {
		s.Unlock()
		return protocol.ErrClosed
	}
	s.closed = true
	for c := range s.contexts {
		c.closed = true
		c.cancel()
		delete(s.contexts, c)
	}
	s.Unlock()
	return nil
}

func (s *socket) OpenContext() (protocol.Context, error) {
	s.Lock()
	defer s.Unlock()
	if s.closed {
		return nil, protocol.ErrClosed
	}
	c := &reqContext{
		s:             s,
		notifyCh:      make(chan struct{}, 1),
		bestEffort:    s.defCtx.bestEffort,
		resendTime:    s.defCtx.resendTime,
		sendExpire:    s.defCtx.sendExpire,
		receiveExpire: s.defCtx.receiveExpire,
		failNoPeers:   s.defCtx.failNoPeers,
	}
	s.contexts[c] = struct{}{}
	return c, nil
}

func (s *socket) AddPipe(pp protocol.Pipe) error {
	p := &pipe{
		p:      pp,
		s:      s,
		sendQ:  make(chan *protocol.Message, 1),
		closeQ: make(chan struct{}),
	}
	pp.SetPrivate(p)
	s.Lock()
	defer s.Unlock()
	if s.closed {
		return protocol.ErrClosed
	}
	s.readyQ = append(s.readyQ, p)
	s.send()
	s.pipes[pp.ID()] = p
	go p.sender()
	go p.receiver()
	return nil
}

func (s *socket) RemovePipe(pp protocol.Pipe) {
	p := pp.GetPrivate().(*pipe)
	close(p.closeQ)
	s.Lock()
	p.closed = true
	for i, rp := range s.readyQ {
		if p == rp {
			s.readyQ = append(s.readyQ[:i], s.readyQ[i+1:]...)
		}
	}
	delete(s.pipes, pp.ID())
	for c := range s.contexts {
		if c.failNoPeers && len(s.pipes) == 0 {
			c.cancel()
		} else if c.lastPipe == p && c.reqMsg != nil {
			// We are closing this pipe, so we need to
			// immediately reschedule it.
			c.lastPipe = nil
			id := c.reqID
			// If there is no resend time, then we need to simply
			// discard the message, because it's not necessarily idempotent.
			if c.resendTime == 0 {
				c.cancel()
			} else {
				c.cancelSend()
				go c.resendMessage(id)
			}
		}
	}
	s.Unlock()
}

func (*socket) Info() protocol.Info {
	return protocol.Info{
		Self:     Self,
		Peer:     Peer,
		SelfName: SelfName,
		PeerName: PeerName,
	}
}

// NewProtocol allocates a new protocol implementation.
func NewProtocol() protocol.Protocol {
	s := &socket{
		nextID:   uint32(time.Now().UnixNano()), // quasi-random
		contexts: make(map[*reqContext]struct{}),
		ctxByID:  make(map[uint32]*reqContext),
		pipes:    make(map[uint32]*pipe),
	}
	s.defCtx = &reqContext{
		s:          s,
		notifyCh:   make(chan struct{}, 1),
		resendTime: time.Minute,
	}
	s.contexts[s.defCtx] = struct{}{}
	return s
}

// NewSocket allocates a new Socket using the REQ protocol.
func NewSocket() (protocol.Socket, error) {
	return protocol.MakeSocket(NewProtocol()), nil
}
