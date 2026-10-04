//go:build !plan9 && !js

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

// Package ipc implements the platform's default local transport. On POSIX
// systems ipc:// is an alias for unix://. On Windows it is an alias for
// winpipe://. Importing this package also registers the explicit transport.
package ipc

import (
	"strings"

	"go.nanomsg.org/mangos/v3"
	"go.nanomsg.org/mangos/v3/transport"
)

// Transport implements the ipc alias.
const Transport = ipcTran(0)

func init() {
	transport.RegisterTransport(Transport)
}

type ipcTran int

func (ipcTran) Scheme() string { return "ipc" }

func (t ipcTran) NewDialer(addr string, sock mangos.Socket) (transport.Dialer, error) {
	path, err := transport.StripScheme(t, addr)
	if err != nil {
		return nil, err
	}
	return platformTransport.NewDialer(platformTransport.Scheme()+"://"+path, sock)
}

func (t ipcTran) NewListener(addr string, sock mangos.Socket) (transport.Listener, error) {
	path, err := transport.StripScheme(t, addr)
	if err != nil {
		return nil, err
	}
	l, err := platformTransport.NewListener(platformTransport.Scheme()+"://"+path, sock)
	if err != nil {
		return nil, err
	}
	return &listener{Listener: l}, nil
}

type listener struct {
	transport.Listener
}

func (l *listener) Address() string {
	return "ipc://" + strings.TrimPrefix(l.Listener.Address(), platformTransport.Scheme()+"://")
}
