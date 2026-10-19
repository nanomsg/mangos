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
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"testing"

	"go.nanomsg.org/mangos/v3/protocol/rep"
	"go.nanomsg.org/mangos/v3/protocol/req"
)

func TestWebsockPath(t *testing.T) {
	sockReq, _ := req.NewSocket()
	sockRep, _ := rep.NewSocket()
	tran := Transport
	l, e := tran.NewListener("ws://127.0.0.1:0/mysock", sockReq)
	if e != nil {
		t.Errorf("Failed new Listener: %v", e)
		return
	}
	if e = l.Listen(); e != nil {
		t.Errorf("Listen failed")
		return
	}
	defer func() {
		_ = l.Close()
	}()
	d, e := tran.NewDialer(l.Address()[:len(l.Address())-len("mysock")]+"boguspath", sockRep)
	if e != nil {
		t.Errorf("Failed new Dialer: %v", e)
		return
	}
	p, e := d.Dial()
	if p != nil {
		defer func() {
			_ = p.Close()
		}()
	}
	if e == nil {
		t.Errorf("Dial passed, when should not have!")
		return
	}
}

var bogusstr = "THIS IS BOGUS"

func bogusHandler(w http.ResponseWriter, _ *http.Request) {
	_, _ = fmt.Fprint(w, bogusstr)
}

func TestWebsockMux(t *testing.T) {
	sockReq, _ := req.NewSocket()
	sockRep, _ := rep.NewSocket()
	tran := Transport
	l, e := tran.NewListener("ws://127.0.0.1:0/mysock", sockReq)
	if e != nil {
		t.Errorf("Failed new Listener: %v", e)
		return
	}
	muxi, e := l.GetOption(OptionWebSocketMux)
	if e != nil {
		t.Errorf("Failed get mux: %v", e)
	}
	mux := muxi.(*http.ServeMux)
	mux.HandleFunc("/bogus", bogusHandler)
	if e = l.Listen(); e != nil {
		t.Errorf("Listen failed")
		return
	}
	defer func() {
		_ = l.Close()
	}()
	addr := l.Address()
	d, e := tran.NewDialer(addr[:len(addr)-len("mysock")]+"bogus", sockRep)
	if e != nil {
		t.Errorf("Failed new Dialer: %v", e)
		return
	}

	p, e := d.Dial()
	if p != nil {
		defer func() {
			_ = p.Close()
		}()
	}
	if e == nil {
		t.Errorf("Dial passed, when should not have!")
		return
	}

	// Now let's try to use http client.
	resp, err := http.Get("http://" + addr[len("ws://"):len(addr)-len("mysock")] + "bogus")

	if err != nil {
		t.Errorf("Get of boguspath failed: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Errorf("Response code wrong: %d", resp.StatusCode)
		return
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Errorf("ReadAll Failed: %v", err)
		return
	}
	if string(body) != bogusstr {
		t.Errorf("Results mismatch: %s != %s", string(body), bogusstr)
	}
}

// This test verifies that we can use stock http server instances with
// our own websocket handler.
func TestWebsockHandler(t *testing.T) {
	sockReq, _ := req.NewSocket()
	sockRep, _ := rep.NewSocket()
	tran := Transport
	l, e := tran.NewListener("ws://127.0.0.1:0/mysock", sockReq)
	if e != nil {
		t.Errorf("Failed new Listener: %v", e)
		return
	}
	hi, e := l.GetOption(OptionWebSocketHandler)
	if e != nil {
		t.Errorf("Failed get WebSocketHandler: %v", e)
	}
	handler := hi.(http.Handler)

	mux := http.NewServeMux()
	mux.HandleFunc("/bogus", bogusHandler)
	mux.Handle("/mysock", handler)

	server, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Errorf("Failed to create HTTP listener: %v", e)
		return
	}
	defer func() {
		_ = server.Close()
	}()
	go func() {
		_ = http.Serve(server, mux)
	}()
	addr := "ws://" + server.Addr().String()
	d, e := tran.NewDialer(addr+"/bogus", sockRep)
	if e != nil {
		t.Errorf("Failed new Dialer: %v", e)
		return
	}

	defer func() {
		_ = l.Close()
	}()

	p, e := d.Dial()
	if p != nil {
		defer func() {
			_ = p.Close()
		}()
	}
	if e == nil {
		t.Errorf("Dial passed, when should not have!")
		return
	}

	// Now let's try to use http client.
	resp, err := http.Get("http://" + server.Addr().String() + "/bogus")

	if err != nil {
		t.Errorf("Get of boguspath failed: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Errorf("Response code wrong: %d", resp.StatusCode)
		return
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Errorf("ReadAll Failed: %v", err)
		return
	}
	if string(body) != bogusstr {
		t.Errorf("Results mismatch: %s != %s", string(body), bogusstr)
	}
}
