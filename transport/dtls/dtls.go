// Copyright 2026 The Mangos Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package dtls implements NNG's authenticated datagram transport using Pion
// DTLS 1.2 and 1.3. Import it to register dtls, dtls4, and dtls6.
//
// Messages retain UDP's loss and reordering behavior. The maximum message
// size is 16320 bytes including the SP protocol header. Larger messages than
// the peer's receive limit are silently dropped, as in NNG. Applications
// should prefer messages that fit within the path MTU after DTLS overhead.
//
// Listeners require mangos.OptionDTLSConfig. Dialers verify the server using
// the URL hostname by default; use pion.WithServerName to override it.
// Configuration and receive limits must be set before an endpoint starts.
package dtls

import (
	"context"
	"net"
	"strings"
	"sync"

	pion "github.com/pion/dtls/v4"
	"go.nanomsg.org/mangos/v3"
	"go.nanomsg.org/mangos/v3/transport"
)

const (
	// Transport registers the dtls scheme, allowing either address family.
	Transport = dtlsTran("dtls")
	// Transport4 registers the IPv4-only dtls4 scheme.
	Transport4 = dtlsTran("dtls4")
	// Transport6 registers the IPv6-only dtls6 scheme.
	Transport6 = dtlsTran("dtls6")

	maxPayload       = 16320
	socketBufferSize = 1 << 20
)

func init() {
	transport.RegisterTransport(Transport)
	transport.RegisterTransport(Transport4)
	transport.RegisterTransport(Transport6)
}

type dtlsTran string

func (t dtlsTran) Scheme() string { return string(t) }

func (t dtlsTran) network() string { return strings.Replace(string(t), "dtls", "udp", 1) }

func (t dtlsTran) NewDialer(addr string, sock mangos.Socket) (transport.Dialer, error) {
	address, err := transport.StripScheme(t, addr)
	if err != nil {
		return nil, err
	}
	if strings.ContainsAny(address, "* /?#") {
		return nil, mangos.ErrBadAddr
	}
	host, _, err := net.SplitHostPort(address)
	if err != nil || host == "" {
		return nil, mangos.ErrBadAddr
	}
	remote, err := net.ResolveUDPAddr(t.network(), address)
	if err != nil || remote.Port == 0 || remote.IP == nil || remote.IP.IsUnspecified() ||
		remote.IP.IsMulticast() || remote.IP.Equal(net.IPv4bcast) {
		return nil, mangos.ErrBadAddr
	}
	return &dialer{network: t.network(), remote: remote, host: host, proto: sock.Info(), maxRecv: maxPayload}, nil
}

func (t dtlsTran) NewListener(addr string, sock mangos.Socket) (transport.Listener, error) {
	address, err := transport.StripScheme(t, addr)
	if err != nil {
		return nil, err
	}
	if strings.ContainsAny(address, " /?#") {
		return nil, mangos.ErrBadAddr
	}
	address = strings.TrimPrefix(address, "*")
	local, err := net.ResolveUDPAddr(t.network(), address)
	if err != nil || local.IP.IsMulticast() || local.IP.Equal(net.IPv4bcast) {
		return nil, mangos.ErrBadAddr
	}
	return &listener{
		scheme: string(t), network: t.network(), local: local,
		proto: sock.Info(), maxRecv: maxPayload,
		ready: make(chan *pipe, 16), closed: make(chan struct{}),
		pending: make(map[*pion.Conn]*packetConn),
	}, nil
}

type dialer struct {
	mu      sync.Mutex
	network string
	remote  *net.UDPAddr
	host    string
	proto   transport.ProtocolInfo
	config  *Config
	maxRecv int
	started bool
}

func (d *dialer) Dial() (transport.Pipe, error) {
	d.mu.Lock()
	d.started = true
	config, maxRecv := d.config, d.maxRecv
	if config == nil {
		config = &Config{}
	}
	d.mu.Unlock()

	network := d.network
	if network == "udp" {
		if d.remote.IP.To4() != nil {
			network = "udp4"
		} else {
			network = "udp6"
		}
	}
	raw, err := net.ListenUDP(network, nil)
	if err != nil {
		return nil, err
	}
	setSocketBuffers(raw)
	conn, err := pion.Client(raw, d.remote, config.clientOptions(d.host)...)
	if err != nil {
		_ = raw.Close()
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), config.timeout())
	defer cancel()
	if err = conn.HandshakeContext(ctx); err != nil {
		_ = raw.Close()
		_ = conn.Close()
		return nil, err
	}
	p := newPipe(conn, d.proto, maxRecv)
	if err = p.negotiate(ctx, true); err != nil {
		_ = p.Close()
		return nil, err
	}
	p.start(true)
	return p, nil
}

func (d *dialer) SetOption(name string, value interface{}) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	return setOption(&d.config, &d.maxRecv, d.started, name, value)
}

func (d *dialer) GetOption(name string) (interface{}, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return getOption(d.config, d.maxRecv, name)
}

type listener struct {
	mu      sync.Mutex
	scheme  string
	network string
	local   *net.UDPAddr
	proto   transport.ProtocolInfo
	config  *Config
	maxRecv int
	started bool
	closing bool
	packets *packetListener
	inner   net.Listener
	ready   chan *pipe
	closed  chan struct{}
	pending map[*pion.Conn]*packetConn
}

func (l *listener) Listen() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closing {
		return mangos.ErrClosed
	}
	if l.started {
		return mangos.ErrAddrInUse
	}
	if l.config == nil {
		return mangos.ErrTLSNoConfig
	}
	if len(l.config.ServerOptions) == 0 {
		return mangos.ErrTLSNoCert
	}
	raw, err := net.ListenUDP(l.network, l.local)
	if err != nil {
		return err
	}
	setSocketBuffers(raw)
	packets := newPacketListener(raw, l.config)
	inner, err := pion.NewListener(packets, l.config.serverOptions()...)
	if err != nil {
		_ = raw.Close()
		return err
	}
	l.inner, l.packets, l.started = inner, packets, true
	go packets.readLoop()
	go l.acceptLoop()
	return nil
}

func (l *listener) acceptLoop() {
	for {
		accepted, err := l.inner.Accept()
		if err != nil {
			select {
			case <-l.closed:
				return
			default:
			}
			if err == net.ErrClosed {
				_ = l.Close()
				return
			}
			continue
		}
		conn := accepted.(*pion.Conn)
		l.mu.Lock()
		if l.closing {
			l.mu.Unlock()
			_ = conn.Close()
			continue
		}
		raw := l.packets.holdPeer(conn.RemoteAddr())
		if raw == nil {
			l.mu.Unlock()
			_ = conn.Close()
			continue
		}
		l.pending[conn] = raw
		l.mu.Unlock()
		go l.handshake(conn, raw)
	}
}

func (l *listener) handshake(conn *pion.Conn, raw *packetConn) {
	defer raw.release()
	ctx, cancel := context.WithDeadline(context.Background(), raw.expires)
	defer cancel()
	p := newPipe(conn, l.proto, l.maxRecv)
	err := conn.HandshakeContext(ctx)
	if err == nil {
		err = p.negotiate(ctx, false)
	}
	l.mu.Lock()
	if err == nil && !l.closing && raw.establish() {
		select {
		case l.ready <- p:
			delete(l.pending, conn)
			p.start(false)
			l.mu.Unlock()
			return
		default:
		}
	}
	l.mu.Unlock()
	// Closing the packet connection first also interrupts unfinished TLS I/O.
	_ = raw.Close()
	_ = p.Close()
	l.mu.Lock()
	delete(l.pending, conn)
	l.mu.Unlock()
}

func (l *listener) Accept() (transport.Pipe, error) {
	select {
	case <-l.closed:
		return nil, mangos.ErrClosed
	case p := <-l.ready:
		l.mu.Lock()
		closing := l.closing
		l.mu.Unlock()
		if closing {
			_ = p.Close()
			return nil, mangos.ErrClosed
		}
		return p, nil
	}
}

func (l *listener) Close() error {
	l.mu.Lock()
	if l.closing {
		l.mu.Unlock()
		return nil
	}
	l.closing = true
	close(l.closed)
	var queued []*pipe
	for draining := true; draining; {
		select {
		case p := <-l.ready:
			queued = append(queued, p)
		default:
			draining = false
		}
	}
	pending := make(map[*pion.Conn]*packetConn, len(l.pending))
	for conn, raw := range l.pending {
		pending[conn] = raw
	}
	inner := l.inner
	l.mu.Unlock()
	if inner != nil {
		_ = inner.Close()
	}
	for conn, raw := range pending {
		_ = raw.Close()
		_ = conn.Close()
		raw.release()
	}
	for _, p := range queued {
		_ = p.Close()
	}
	return nil
}

func (l *listener) Address() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.packets != nil {
		return l.scheme + "://" + l.packets.Addr().String()
	}
	return l.scheme + "://" + l.local.String()
}

func (l *listener) SetOption(name string, value interface{}) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closing {
		return mangos.ErrClosed
	}
	return setOption(&l.config, &l.maxRecv, l.started, name, value)
}

func (l *listener) GetOption(name string) (interface{}, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return getOption(l.config, l.maxRecv, name)
}

func setOption(config **Config, maxRecv *int, started bool, name string, value interface{}) error {
	if name != mangos.OptionDTLSConfig && name != mangos.OptionMaxRecvSize {
		return mangos.ErrBadOption
	}
	if started {
		return mangos.ErrProtoState
	}
	if name == mangos.OptionDTLSConfig {
		c, ok := value.(*Config)
		if !ok || c == nil {
			return mangos.ErrBadValue
		}
		if err := c.validate(); err != nil {
			return err
		}
		*config = cloneConfig(c)
		return nil
	}
	size, ok := value.(int)
	if !ok || size < 0 {
		return mangos.ErrBadValue
	}
	if size == 0 || size > maxPayload {
		size = maxPayload
	}
	*maxRecv = size
	return nil
}

func getOption(config *Config, maxRecv int, name string) (interface{}, error) {
	switch name {
	case mangos.OptionDTLSConfig:
		return cloneConfig(config), nil
	case mangos.OptionMaxRecvSize:
		return maxRecv, nil
	default:
		return nil, mangos.ErrBadOption
	}
}

func setSocketBuffers(c *net.UDPConn) {
	_ = c.SetWriteBuffer(socketBufferSize)
	_ = c.SetReadBuffer(socketBufferSize)
}
