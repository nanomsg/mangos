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

package dtls

import (
	"crypto/tls"
	"testing"
	"time"

	pion "github.com/pion/dtls/v4"
	"github.com/pion/dtls/v4/pkg/crypto/ciphersuite"
	"github.com/pion/dtls/v4/pkg/protocol"
	"go.nanomsg.org/mangos/v3"
	"go.nanomsg.org/mangos/v3/internal/test"
	"go.nanomsg.org/mangos/v3/transport"
)

func TestDTLSOptions(t *testing.T) {
	sock := test.GetMockSocket()
	defer test.MustClose(t, sock)
	for _, tran := range []dtlsTran{Transport, Transport4, Transport6} {
		if transport.GetTransport(tran.Scheme()) != tran {
			t.Fatal("transport not registered")
		}
		l, err := tran.NewListener(string(tran)+"://:0", sock)
		test.MustSucceed(t, err)
		defer l.Close()
		d, err := tran.NewDialer(string(tran)+"://localhost:12345", sock)
		test.MustSucceed(t, err)
		for _, ep := range []interface {
			SetOption(string, interface{}) error
			GetOption(string) (interface{}, error)
		}{l, d} {
			for _, value := range []interface{}{-1, "bad", nil} {
				if err := ep.SetOption(mangos.OptionMaxRecvSize, value); err != mangos.ErrBadValue {
					t.Fatalf("max receive %v: %v", value, err)
				}
			}
			for _, size := range []int{0, 32, maxPayload + 1} {
				test.MustSucceed(t, ep.SetOption(mangos.OptionMaxRecvSize, size))
				got, err := ep.GetOption(mangos.OptionMaxRecvSize)
				test.MustSucceed(t, err)
				want := size
				if size == 0 || size > maxPayload {
					want = maxPayload
				}
				if got != want {
					t.Fatalf("max receive %v, want %d", got, want)
				}
			}
			for _, value := range []interface{}{nil, (*Config)(nil), &tls.Config{}, &Config{ConnectTimeout: -1}, &Config{MaxPending: -1}, &Config{HandshakeRate: -1}, &Config{ClientOptions: []pion.ClientOption{nil}}} {
				if err := ep.SetOption(mangos.OptionDTLSConfig, value); err != mangos.ErrBadValue {
					t.Fatalf("DTLS config %v: %v", value, err)
				}
			}
			if err := ep.SetOption(mangos.OptionTLSConfig, &tls.Config{}); err != mangos.ErrBadOption {
				t.Fatalf("TLS config accepted: %v", err)
			}
			config := &Config{ConnectTimeout: time.Second, ClientOptions: []pion.ClientOption{pion.WithMaxVersion(protocol.Version1_3)}}
			test.MustSucceed(t, ep.SetOption(mangos.OptionDTLSConfig, config))
			config.ConnectTimeout = 2 * time.Second
			config.ClientOptions[0] = nil
			value, err := ep.GetOption(mangos.OptionDTLSConfig)
			test.MustSucceed(t, err)
			copy := value.(*Config)
			if copy.ConnectTimeout != time.Second || copy.ClientOptions[0] == nil {
				t.Fatal("configuration was not copied")
			}
			copy.ClientOptions[0] = nil
			value, err = ep.GetOption(mangos.OptionDTLSConfig)
			test.MustSucceed(t, err)
			if value.(*Config).ClientOptions[0] == nil {
				t.Fatal("GetOption exposed option slice")
			}
		}
	}
}

func TestDTLSAddresses(t *testing.T) {
	sock := test.GetMockSocket()
	defer test.MustClose(t, sock)
	for _, addr := range []string{"tcp://127.0.0.1:1", "dtls4://*:1", "dtls4://:1", "dtls4://127.0.0.1:0", "dtls4://127.0.0.1:1/path", "dtls4://127.0.0.1:1?query", "dtls4://127.0.0.1:1#fragment", "dtls4://0.0.0.0:1", "dtls4://224.0.0.1:12345", "dtls4://255.255.255.255:12345"} {
		if _, err := Transport4.NewDialer(addr, sock); err == nil {
			t.Fatalf("accepted dial address %q", addr)
		}
	}
	for _, addr := range []string{"tcp://:0", "dtls4://:0/path", "dtls4://bad:port", "dtls4://:0?query"} {
		if _, err := Transport4.NewListener(addr, sock); err == nil {
			t.Fatalf("accepted listen address %q", addr)
		}
	}
	l, err := Transport4.NewListener("dtls4://*:0", sock)
	test.MustSucceed(t, err)
	defer l.Close()
	if err := l.Listen(); err != mangos.ErrTLSNoConfig {
		t.Fatalf("missing config: %v", err)
	}
	test.MustSucceed(t, l.SetOption(mangos.OptionDTLSConfig, &Config{}))
	if err := l.Listen(); err != mangos.ErrTLSNoCert {
		t.Fatalf("missing certificate: %v", err)
	}
}

func TestDTLSStartedOptions(t *testing.T) {
	server, client := testIdentities(t).configs()
	l, _, _ := testPipes(t, Transport4, server, client, 0)
	if err := l.Listen(); err != mangos.ErrAddrInUse {
		t.Fatalf("duplicate Listen: %v", err)
	}
	if err := l.SetOption(mangos.OptionMaxRecvSize, 32); err != mangos.ErrProtoState {
		t.Fatalf("changed running receive limit: %v", err)
	}
	if err := l.SetOption(mangos.OptionDTLSConfig, server); err != mangos.ErrProtoState {
		t.Fatalf("changed running config: %v", err)
	}
}

func TestDTLSVersionFallback(t *testing.T) {
	server, client := testIdentities(t).configs()
	client.ClientOptions = append(client.ClientOptions, pion.WithMaxVersion(protocol.Version1_2))
	_, sp, cp := testPipes(t, Transport4, server, client, 0)
	state, err := cp.GetOption(mangos.OptionDTLSConnState)
	test.MustSucceed(t, err)
	value := state.(ConnectionState)
	if value.NegotiatedVersion() != protocol.Version1_2 {
		t.Fatal("did not negotiate DTLS 1.2 fallback")
	}
	sendReceive(t, cp, sp, 1200)
}

func TestDTLSPSK(t *testing.T) {
	key := []byte("0123456789abcdef")
	identity := []byte("mangos-test")
	for _, tc := range []struct {
		name    string
		version protocol.Version
	}{{"1.2", protocol.Version1_2}, {"1.3", protocol.Version1_3}} {
		t.Run(tc.name, func(t *testing.T) {
			server := &Config{ServerOptions: []pion.ServerOption{
				pion.WithMinVersion(tc.version), pion.WithMaxVersion(tc.version),
				pion.WithPSK(nil, func(identities [][]byte) (*pion.PSK, error) {
					for _, id := range identities {
						if string(id) == string(identity) {
							return &pion.PSK{Identity: identity, Key: key}, nil
						}
					}
					return nil, nil
				}),
			}}
			client := &Config{ClientOptions: []pion.ClientOption{
				pion.WithMinVersion(tc.version), pion.WithMaxVersion(tc.version),
				pion.WithPSK(func() ([]pion.PSK, error) { return []pion.PSK{{Identity: identity, Key: key}}, nil }, nil),
			}}
			if tc.version == protocol.Version1_2 {
				server.ServerOptions = append(server.ServerOptions, pion.WithCipherSuites(ciphersuite.TLS_PSK_WITH_AES_128_GCM_SHA256))
				client.ClientOptions = append(client.ClientOptions, pion.WithCipherSuites(ciphersuite.TLS_PSK_WITH_AES_128_GCM_SHA256))
			}
			_, sp, cp := testPipes(t, Transport4, server, client, 0)
			sendReceive(t, cp, sp, 1200)
		})
	}
}
