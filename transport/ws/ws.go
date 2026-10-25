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

// Package ws implements a simple WebSocket transport for mangos.
// To enable it simply import it.
package ws

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/http/httptrace"
	"net/url"
	"strings"
	"sync"

	"github.com/coder/websocket"

	"go.nanomsg.org/mangos/v3"
	"go.nanomsg.org/mangos/v3/transport"
)

// Some special options
const (
	// OptionWebSocketMux is a retrieve-only property used to obtain
	// the *http.ServeMux instance associated with the server.  This
	// can be used to subsequently register additional handlers for
	// different URIs.  This option is only valid on a Listener.
	// Generally you use this option when you want to use the standard
	// mangos Listen() method to start up the server.
	OptionWebSocketMux = "WEBSOCKET-MUX"

	// OptionWebSocketHandler is used to obtain the underlying
	// http.Handler object, so you can use this
	// on your own http.Server instances.  It is a gross error to use
	// the value returned by this method on an http server if the
	// server is also started with mangos Listen().  This means that you
	// will use at most either this option, or OptionWebSocketMux, but
	// never both.  This option is only valid on a listener.
	OptionWebSocketHandler = "WEBSOCKET-HANDLER"

	// OptionWebSocketCheckOrigin controls the check of the origin of the
	// underlying Listener.
	// Web browsers allow JavaScript applications to open a WebSocket
	// connection to any host. It is up to the server to enforce an origin
	// policy using the Origin request header sent by the browser. By default,
	// cross-origin requests are rejected. Set this option to false to permit
	// connections from any origin.
	OptionWebSocketCheckOrigin = "WEBSOCKET-CHECKORIGIN"

	// OptionWebSocketFollowRedirects controls whether the Dialer follows HTTP
	// redirects during the WebSocket handshake. By default, redirects are
	// rejected and reported as mangos.ErrBadProto.
	OptionWebSocketFollowRedirects = "WEBSOCKET-FOLLOWREDIRECTS"

	// Transport is a transport.Transport for WebSocket
	Transport = wsTran(0)
)

type options map[string]interface{}

func init() {
	transport.RegisterTransport(Transport)
}

// GetOption retrieves an option value.
func (o options) get(name string) (interface{}, error) {
	if name == mangos.OptionNoDelay {
		return true, nil
	}
	v, ok := o[name]
	if !ok {
		return nil, mangos.ErrBadOption
	}
	return v, nil
}

// SetOption sets an option.  We have none, so just ErrBadOption.
func (o options) set(name string, val interface{}) error {
	switch name {
	case mangos.OptionNoDelay:
		if _, ok := val.(bool); ok {
			return nil
		}
		return mangos.ErrBadValue
	case OptionWebSocketCheckOrigin, OptionWebSocketFollowRedirects:
		if v, ok := val.(bool); ok {
			o[name] = v
			return nil
		}
		return mangos.ErrBadValue
	case mangos.OptionTLSConfig:
		if v, ok := val.(*tls.Config); ok {
			o[name] = v
			return nil
		}
		return mangos.ErrBadValue
	case mangos.OptionMaxRecvSize:
		if v, ok := val.(int); ok {
			o[name] = v
			return nil
		}
		return mangos.ErrBadValue
	}
	return mangos.ErrBadOption
}

// wsPipe implements the Pipe interface on a websocket
type wsPipe struct {
	ws *websocket.Conn
	// ctx is canceled by Close to interrupt an in-flight WebSocket operation.
	ctx     context.Context
	cancel  context.CancelFunc
	proto   transport.ProtocolInfo
	addr    string
	open    bool
	wg      sync.WaitGroup
	options map[string]interface{}
	iswss   bool
	dtype   websocket.MessageType
	sync.Mutex
}

type wsTran int

func (w *wsPipe) Recv() (*mangos.Message, error) {

	// We ignore the message type for receive.
	_, body, err := w.ws.Read(w.ctx)
	if err != nil {
		return nil, err
	}
	msg := mangos.NewMessage(0)
	msg.Body = body
	return msg, nil
}

func (w *wsPipe) Send(m *mangos.Message) error {

	var buf []byte

	if len(m.Header) > 0 {
		buf = make([]byte, 0, len(m.Header)+len(m.Body))
		buf = append(buf, m.Header...)
		buf = append(buf, m.Body...)
	} else {
		buf = m.Body
	}
	if err := w.ws.Write(w.ctx, w.dtype, buf); err != nil {
		return err
	}
	m.Free()
	return nil
}

func (w *wsPipe) Close() error {
	w.Lock()
	defer w.Unlock()
	if w.open {
		w.open = false
		w.cancel()
		_ = w.ws.CloseNow()
		w.wg.Done()
	}
	return nil
}

func (w *wsPipe) GetOption(name string) (interface{}, error) {
	if v, ok := w.options[name]; ok {
		return v, nil
	}
	return nil, mangos.ErrBadOption
}

type dialer struct {
	addr  string // url
	proto mangos.ProtocolInfo
	opts  options
	iswss bool
}

func (d *dialer) Dial() (transport.Pipe, error) {
	var w *wsPipe

	var conn net.Conn
	ctx := httptrace.WithClientTrace(context.Background(), &httptrace.ClientTrace{
		GotConn: func(info httptrace.GotConnInfo) {
			conn = info.Conn
		},
	})
	httpTransport := &http.Transport{
		ForceAttemptHTTP2: false,
	}
	defer httpTransport.CloseIdleConnections()
	if v, ok := d.opts[mangos.OptionTLSConfig]; ok {
		httpTransport.TLSClientConfig = v.(*tls.Config)
	}
	httpClient := &http.Client{Transport: httpTransport}
	if follow, _ := d.opts.get(OptionWebSocketFollowRedirects); !follow.(bool) {
		httpClient.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		}
	}
	wd := &websocket.DialOptions{
		HTTPClient:   httpClient,
		Subprotocols: []string{d.proto.PeerName + ".sp.nanomsg.org"},
	}

	pipeCtx, cancel := context.WithCancel(context.Background())
	w = &wsPipe{
		addr:    d.addr,
		ctx:     pipeCtx,
		cancel:  cancel,
		proto:   d.proto,
		open:    true,
		dtype:   websocket.MessageBinary,
		options: make(map[string]interface{}),
	}

	maxrx := 0
	v, err := d.opts.get(mangos.OptionMaxRecvSize)
	if err == nil {
		maxrx, _ = v.(int)
	}
	var resp *http.Response
	if w.ws, resp, err = websocket.Dial(ctx, d.addr, wd); err != nil {
		if resp != nil {
			return nil, mangos.ErrBadProto
		}
		return nil, err
	}
	w.ws.SetReadLimit(wsReadLimit(maxrx))
	if conn == nil {
		_ = w.ws.CloseNow()
		return nil, fmt.Errorf("websocket dial did not expose network connection")
	}
	w.options[mangos.OptionLocalAddr] = conn.LocalAddr()
	w.options[mangos.OptionRemoteAddr] = conn.RemoteAddr()
	if tlsConn, ok := conn.(*tls.Conn); ok {
		w.options[mangos.OptionTLSConnState] = tlsConn.ConnectionState()
	}

	w.wg.Add(1)
	return w, nil
}

func (d *dialer) SetOption(n string, v interface{}) error {
	return d.opts.set(n, v)
}

func (d *dialer) GetOption(n string) (interface{}, error) {
	return d.opts.get(n)
}

type listener struct {
	pending    []*wsPipe
	lock       sync.Mutex
	cv         sync.Cond
	running    bool
	noserve    bool
	addr       string
	bound      *net.TCPAddr
	anon       bool // anonymous port selected
	closed     bool
	acceptOpts websocket.AcceptOptions
	htsvr      *http.Server
	mux        *http.ServeMux
	url        *url.URL
	listener   net.Listener
	proto      transport.ProtocolInfo
	opts       options
	iswss      bool
}

func (l *listener) SetOption(n string, v interface{}) error {
	switch n {
	case OptionWebSocketCheckOrigin:
		if v, ok := v.(bool); ok {
			l.acceptOpts.InsecureSkipVerify = !v
		}
	}
	return l.opts.set(n, v)
}

func (l *listener) GetOption(n string) (interface{}, error) {
	switch n {
	case OptionWebSocketMux:
		return l.mux, nil
	case OptionWebSocketHandler:
		// Caller intends to use use in his own server, so mark
		// us running.  If he didn't mean this, the side effect is
		// that Accept() will appear to hang, even though Listen()
		// is not called yet.
		l.running = true
		l.noserve = true
		return l, nil
	case OptionWebSocketCheckOrigin:
		if v, err := l.opts.get(n); err == nil {
			if v, ok := v.(bool); ok {
				return v, nil
			}
		}
		return true, nil

	}
	return l.opts.get(n)
}

func (l *listener) Listen() error {
	var taddr *net.TCPAddr
	var err error
	var tcfg *tls.Config

	if l.closed {
		return mangos.ErrClosed
	}
	if l.noserve {
		// The HTTP framework is going to call us, so we use that rather than
		// listening on our own.  We just fake this out.
		return nil
	}
	if l.iswss {
		v, ok := l.opts[mangos.OptionTLSConfig]
		if !ok || v == nil {
			return mangos.ErrTLSNoConfig
		}
		tcfg = v.(*tls.Config)
		if tcfg.Certificates == nil || len(tcfg.Certificates) == 0 {
			return mangos.ErrTLSNoCert
		}
	}

	// We listen separately, that way we can catch and deal with the
	// case of a port already in use.  This also lets us configure
	// properties of the underlying TCP connection.

	if taddr, err = transport.ResolveTCPAddr(l.url.Host); err != nil {
		return err
	}

	if taddr.Port == 0 {
		l.anon = true
	}
	if tlist, err := net.ListenTCP("tcp", taddr); err != nil {
		return err
	} else if l.iswss {
		l.listener = tls.NewListener(tlist, tcfg)
	} else {
		l.listener = tlist
	}
	l.pending = nil
	l.running = true
	l.bound = l.listener.Addr().(*net.TCPAddr)

	l.htsvr = &http.Server{Addr: l.url.Host, Handler: l.mux}

	go func() {
		_ = l.htsvr.Serve(l.listener)
	}()

	return nil
}

func (l *listener) Accept() (transport.Pipe, error) {
	var w *wsPipe

	l.lock.Lock()
	defer l.lock.Unlock()

	for {
		if !l.running {
			return nil, mangos.ErrClosed
		}
		if len(l.pending) == 0 {
			l.cv.Wait()
			continue
		}
		w = l.pending[len(l.pending)-1]
		l.pending = l.pending[:len(l.pending)-1]
		break
	}

	return w, nil
}

func (l *listener) handler(ws *websocket.Conn, localAddr, remoteAddr net.Addr, tlsState *tls.ConnectionState) {
	l.lock.Lock()

	pipeCtx, cancel := context.WithCancel(context.Background())
	w := &wsPipe{
		ws:      ws,
		addr:    l.addr,
		ctx:     pipeCtx,
		cancel:  cancel,
		proto:   l.proto,
		open:    true,
		dtype:   websocket.MessageBinary,
		iswss:   l.iswss,
		options: make(map[string]interface{}),
	}
	maxRx := 0
	v, err := l.opts.get(mangos.OptionMaxRecvSize)
	if err == nil {
		maxRx, _ = v.(int)
	}

	w.ws.SetReadLimit(wsReadLimit(maxRx))
	if localAddr != nil {
		w.options[mangos.OptionLocalAddr] = localAddr
	}
	if remoteAddr != nil {
		w.options[mangos.OptionRemoteAddr] = remoteAddr
	}

	if tlsState != nil {
		w.options[mangos.OptionTLSConnState] = *tlsState
	}

	w.wg.Add(1)
	l.pending = append(l.pending, w)
	l.cv.Broadcast()
	l.lock.Unlock()

	// We must not return before the socket is closed, because
	// our caller will close the websocket on our return.
	w.wg.Wait()
}

func (l *listener) Close() error {
	l.lock.Lock()
	defer l.lock.Unlock()
	if l.closed {
		return mangos.ErrClosed
	}
	if l.listener != nil {
		_ = l.listener.Close()
	}
	l.closed = true
	l.running = false
	l.cv.Broadcast()
	for _, ws := range l.pending {
		_ = ws.Close()
	}
	l.pending = nil
	return nil
}

func (l *listener) ServeHTTP(w http.ResponseWriter, r *http.Request) {

	matched := false
	for _, value := range r.Header.Values("Sec-WebSocket-Protocol") {
		for _, subProto := range strings.Split(value, ",") {
			if strings.TrimSpace(subProto) == l.proto.SelfName+".sp.nanomsg.org" {
				matched = true
			}
		}
	}
	if !matched {
		http.Error(w, "SP protocol mis-match", http.StatusBadRequest)
		return
	}
	// Coder WebSocket correctly returns 426 for this case, but retain the
	// transport's long-standing 400 response for invalid handshake versions.
	if r.Header.Get("Sec-WebSocket-Version") != "13" {
		http.Error(w, "unsupported WebSocket version", http.StatusBadRequest)
		return
	}
	l.lock.Lock()
	if !l.running {
		l.lock.Unlock()
		http.Error(w, "No handler at that address", http.StatusNotFound)
		return
	}
	l.lock.Unlock()
	localAddr, _ := r.Context().Value(http.LocalAddrContextKey).(net.Addr)
	remoteAddr, _ := net.ResolveTCPAddr("tcp", r.RemoteAddr)
	tlsState := r.TLS
	ws, err := websocket.Accept(w, r, &l.acceptOpts)
	if err != nil {
		return
	}
	l.handler(ws, localAddr, remoteAddr, tlsState)
}

func (l *listener) Address() string {
	if l.anon {
		u := l.url
		u.Host = fmt.Sprintf("%s:%d", u.Hostname(), l.bound.Port)
		return u.String()
	}
	return l.url.String()
}

func (wsTran) Scheme() string {
	return "ws"
}

func (wsTran) NewDialer(addr string, sock mangos.Socket) (transport.Dialer, error) {
	d := &dialer{
		addr:  addr,
		proto: sock.Info(),
		iswss: false,
		opts:  make(map[string]interface{}),
	}

	if strings.HasPrefix(addr, "wss://") {
		d.iswss = true
	} else if !strings.HasPrefix(addr, "ws://") {
		return nil, mangos.ErrBadTran
	}

	d.opts[mangos.OptionNoDelay] = true
	d.opts[mangos.OptionMaxRecvSize] = 0
	d.opts[OptionWebSocketFollowRedirects] = false

	return d, nil
}

func (t wsTran) NewListener(addr string, sock mangos.Socket) (transport.Listener, error) {
	l, e := t.listener(addr, sock)
	if e != nil {
		return nil, e
	}
	l.mux.Handle(l.url.Path, l)
	return l, nil
}

func (wsTran) listener(addr string, sock mangos.Socket) (*listener, error) {
	var err error
	l := &listener{
		addr:  addr,
		proto: sock.Info(),
		opts:  make(map[string]interface{}),
	}
	l.opts[mangos.OptionMaxRecvSize] = 0
	l.opts[OptionWebSocketFollowRedirects] = false
	l.cv.L = &l.lock
	l.acceptOpts.Subprotocols = []string{l.proto.SelfName + ".sp.nanomsg.org"}

	if strings.HasPrefix(addr, "wss://") {
		l.iswss = true
	} else if !strings.HasPrefix(addr, "ws://") {
		return nil, mangos.ErrBadTran
	}

	if l.url, err = url.ParseRequestURI(addr); err != nil {
		return nil, err
	}
	if len(l.url.Path) == 0 {
		l.url.Path = "/"
	}
	l.mux = http.NewServeMux()

	l.htsvr = &http.Server{Addr: l.url.Host, Handler: l.mux}

	return l, nil
}

func wsReadLimit(limit int) int64 {
	if limit <= 0 {
		return -1
	}
	return int64(limit)
}
