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
	"bytes"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	pion "github.com/pion/dtls/v4"
	"github.com/pion/dtls/v4/pkg/protocol"
	"go.nanomsg.org/mangos/v3"
	"go.nanomsg.org/mangos/v3/internal/test"
	"go.nanomsg.org/mangos/v3/protocol/pair"
	"go.nanomsg.org/mangos/v3/protocol/rep"
	"go.nanomsg.org/mangos/v3/protocol/req"
	"go.nanomsg.org/mangos/v3/transport"
)

type identities struct {
	root   []byte
	roots  *x509.CertPool
	server tls.Certificate
	client tls.Certificate
}

func testIdentities(t *testing.T) identities {
	t.Helper()
	return testIdentitiesWithKey(t, func() (crypto.Signer, error) {
		return ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	})
}

func testIdentitiesWithKey(t *testing.T, generate func() (crypto.Signer, error)) identities {
	t.Helper()
	caKey, err := generate()
	test.MustSucceed(t, err)
	ca := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "DTLS test CA"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, ca, ca, caKey.Public(), caKey)
	test.MustSucceed(t, err)
	roots := x509.NewCertPool()
	root := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER})
	if !roots.AppendCertsFromPEM(root) {
		t.Fatal("could not add test CA")
	}
	issue := func(serial int64, usage x509.ExtKeyUsage) tls.Certificate {
		key, err := generate()
		test.MustSucceed(t, err)
		template := &x509.Certificate{
			SerialNumber: big.NewInt(serial), Subject: pkix.Name{CommonName: "localhost"},
			DNSNames: []string{"localhost"}, IPAddresses: []net.IP{net.IPv4(127, 0, 0, 1), net.IPv6loopback},
			NotBefore: ca.NotBefore, NotAfter: ca.NotAfter,
			KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{usage},
		}
		der, err := x509.CreateCertificate(rand.Reader, template, ca, key.Public(), caKey)
		test.MustSucceed(t, err)
		return tls.Certificate{Certificate: [][]byte{der, caDER}, PrivateKey: key}
	}
	return identities{root: root, roots: roots, server: issue(2, x509.ExtKeyUsageServerAuth), client: issue(3, x509.ExtKeyUsageClientAuth)}
}

func (ids identities) configs() (*Config, *Config) {
	server := &Config{ServerOptions: []pion.ServerOption{
		pion.WithCertificates(ids.server), pion.WithClientCAs(ids.roots),
		pion.WithClientAuth(pion.RequireAndVerifyClientCert),
	}}
	client := &Config{ClientOptions: []pion.ClientOption{
		pion.WithCertificates(ids.client), pion.WithRootCAs(ids.roots),
	}}
	return server, client
}

func (ids identities) writeIdentity(t *testing.T, cert tls.Certificate) (string, string) {
	t.Helper()
	dir := t.TempDir()
	rootPath, identityPath := filepath.Join(dir, "root.pem"), filepath.Join(dir, "identity.pem")
	var identity []byte
	for _, der := range cert.Certificate {
		identity = append(identity, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})...)
	}
	key, err := x509.MarshalPKCS8PrivateKey(cert.PrivateKey)
	test.MustSucceed(t, err)
	identity = append(identity, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: key})...)
	test.MustSucceed(t, os.WriteFile(rootPath, ids.root, 0600))
	test.MustSucceed(t, os.WriteFile(identityPath, identity, 0600))
	return rootPath, identityPath
}

func testPipes(t *testing.T, tran dtlsTran, serverConfig, clientConfig *Config, recvMax int) (*listener, *pipe, *pipe) {
	t.Helper()
	server, client := test.GetMockSocket(), test.GetMockSocket()
	t.Cleanup(func() { test.MustClose(t, server); test.MustClose(t, client) })
	host := "127.0.0.1"
	if tran == Transport6 {
		host = "[::1]"
	}
	li, err := tran.NewListener(string(tran)+"://"+host+":0", server)
	test.MustSucceed(t, err)
	l := li.(*listener)
	t.Cleanup(func() { test.MustSucceed(t, l.Close()) })
	test.MustSucceed(t, l.SetOption(mangos.OptionDTLSConfig, serverConfig))
	test.MustSucceed(t, l.SetOption(mangos.OptionMaxRecvSize, recvMax))
	test.MustSucceed(t, l.Listen())
	d, err := tran.NewDialer(l.Address(), client)
	test.MustSucceed(t, err)
	test.MustSucceed(t, d.SetOption(mangos.OptionDTLSConfig, clientConfig))
	cp, err := d.Dial()
	test.MustSucceed(t, err)
	t.Cleanup(func() { test.MustSucceed(t, cp.Close()) })
	sp, err := l.Accept()
	test.MustSucceed(t, err)
	t.Cleanup(func() { test.MustSucceed(t, sp.Close()) })
	return l, sp.(*pipe), cp.(*pipe)
}

func receive(t *testing.T, p transport.Pipe) *mangos.Message {
	t.Helper()
	type result struct {
		msg *mangos.Message
		err error
	}
	resultq := make(chan result)
	go func() { m, err := p.Recv(); resultq <- result{m, err} }()
	select {
	case result := <-resultq:
		test.MustSucceed(t, result.err)
		return result.msg
	case <-time.After(3 * time.Second):
		_ = p.Close()
		result := <-resultq
		if result.msg != nil {
			result.msg.Free()
		}
		t.Fatal("timed out receiving DTLS message")
		return nil
	}
}

func sendReceive(t *testing.T, sender, receiver transport.Pipe, size int) {
	t.Helper()
	want := bytes.Repeat([]byte{0x5a}, size)
	m := mangos.NewMessage(0)
	// The receive limit includes the SP header, not just the body.
	n := min(size, 7)
	m.Header = append(m.Header, want[:n]...)
	m.Body = append(m.Body, want[n:]...)
	test.MustSucceed(t, sender.Send(m))
	got := receive(t, receiver)
	defer got.Free()
	if !bytes.Equal(got.Body, want) {
		t.Fatalf("received %d bytes, want %d matching bytes", len(got.Body), len(want))
	}
}

func TestDTLSSendReceive(t *testing.T) {
	ids := testIdentities(t)
	for _, tran := range []dtlsTran{Transport, Transport4, Transport6} {
		for _, tc := range []struct {
			name    string
			version protocol.Version
		}{{"default", 0}, {"1.2", protocol.Version1_2}, {"1.3", protocol.Version1_3}} {
			t.Run(string(tran)+"/"+tc.name, func(t *testing.T) {
				version := tc.version
				if tran == Transport6 {
					probe, err := net.ListenUDP("udp6", &net.UDPAddr{IP: net.IPv6loopback})
					if err != nil {
						t.Skip("IPv6 loopback unavailable")
					}
					probe.Close()
				}
				server, client := ids.configs()
				if version != 0 {
					server.ServerOptions = append(server.ServerOptions, pion.WithMinVersion(version), pion.WithMaxVersion(version))
					client.ClientOptions = append(client.ClientOptions, pion.WithMinVersion(version), pion.WithMaxVersion(version))
				}
				_, sp, cp := testPipes(t, tran, server, client, 0)
				expected := version
				if expected == 0 {
					expected = protocol.Version1_3
				}
				for _, p := range []*pipe{sp, cp} {
					value, err := p.GetOption(mangos.OptionDTLSConnState)
					test.MustSucceed(t, err)
					state := value.(ConnectionState)
					if state.NegotiatedVersion() != expected || len(state.PeerCertificates) == 0 {
						t.Fatalf("unexpected connection state: version %v, certificates %d", state.NegotiatedVersion(), len(state.PeerCertificates))
					}
				}
				for _, size := range []int{0, 1200, 8000, maxPayload} {
					sendReceive(t, cp, sp, size)
					sendReceive(t, sp, cp, size)
				}
			})
		}
	}
}

func TestDTLSListenerCloseKeepsPipes(t *testing.T) {
	server, client := testIdentities(t).configs()
	l, sp, cp := testPipes(t, Transport4, server, client, 0)
	test.MustSucceed(t, l.Close())
	_, err := l.Accept()
	if err != mangos.ErrClosed {
		t.Fatalf("Accept after close: %v", err)
	}
	sendReceive(t, cp, sp, maxPayload)
	sendReceive(t, sp, cp, maxPayload)
	test.MustSucceed(t, cp.Close())
	test.MustSucceed(t, sp.Close())
	probe, err := net.ListenUDP("udp4", l.packets.Addr().(*net.UDPAddr))
	test.MustSucceed(t, err)
	test.MustSucceed(t, probe.Close())
}

func TestDTLSRSACertificates(t *testing.T) {
	ids := testIdentitiesWithKey(t, func() (crypto.Signer, error) { return rsa.GenerateKey(rand.Reader, 2048) })
	server, client := ids.configs()
	// Pion rc.7 cannot serialize RSA-PSS CertificateVerify in DTLS 1.3.
	// RSA certificates remain usable when the endpoint requires DTLS 1.2.
	server.ServerOptions = append(server.ServerOptions, pion.WithMinVersion(protocol.Version1_2), pion.WithMaxVersion(protocol.Version1_2))
	client.ClientOptions = append(client.ClientOptions, pion.WithMinVersion(protocol.Version1_2), pion.WithMaxVersion(protocol.Version1_2))
	_, sp, cp := testPipes(t, Transport4, server, client, 0)
	sendReceive(t, cp, sp, 1200)
}

func TestDTLSNegotiatedLimit(t *testing.T) {
	server, client := testIdentities(t).configs()
	_, sp, cp := testPipes(t, Transport4, server, client, 32)
	for _, size := range []int{33, maxPayload + 1} {
		m := mangos.NewMessage(0)
		m.Body = append(m.Body, bytes.Repeat([]byte("x"), size)...)
		test.MustSucceed(t, cp.Send(m))
	}
	sendReceive(t, cp, sp, 32)
	sendReceive(t, sp, cp, maxPayload)
}

func TestDTLSAuthFailures(t *testing.T) {
	ids := testIdentities(t)
	for _, name := range []string{"untrusted_server", "wrong_name", "missing_client_cert", "version_mismatch", "protocol_mismatch"} {
		t.Run(name, func(t *testing.T) {
			serverConfig, clientConfig := ids.configs()
			serverConfig.ConnectTimeout, clientConfig.ConnectTimeout = 500*time.Millisecond, 500*time.Millisecond
			switch name {
			case "untrusted_server":
				clientConfig.ClientOptions = append(clientConfig.ClientOptions, pion.WithRootCAs(x509.NewCertPool()))
			case "wrong_name":
				clientConfig.ClientOptions = append(clientConfig.ClientOptions, pion.WithServerName("wrong.example"))
			case "missing_client_cert":
				clientConfig.ClientOptions = []pion.ClientOption{pion.WithRootCAs(ids.roots)}
			case "version_mismatch":
				serverConfig.ServerOptions = append(serverConfig.ServerOptions, pion.WithMinVersion(protocol.Version1_3))
				clientConfig.ClientOptions = append(clientConfig.ClientOptions, pion.WithMaxVersion(protocol.Version1_2))
			}
			server := test.GetSocket(t, pair.NewSocket)
			client := test.GetSocket(t, pair.NewSocket)
			defer test.MustClose(t, server)
			if name == "protocol_mismatch" {
				test.MustClose(t, client)
				client = test.GetSocket(t, req.NewSocket)
			}
			defer test.MustClose(t, client)
			li, err := Transport4.NewListener("dtls4://127.0.0.1:0", server)
			test.MustSucceed(t, err)
			l := li.(*listener)
			defer l.Close()
			test.MustSucceed(t, l.SetOption(mangos.OptionDTLSConfig, serverConfig))
			test.MustSucceed(t, l.Listen())
			d, err := Transport4.NewDialer(l.Address(), client)
			test.MustSucceed(t, err)
			test.MustSucceed(t, d.SetOption(mangos.OptionDTLSConfig, clientConfig))
			p, err := d.Dial()
			if p != nil {
				p.Close()
			}
			if err == nil {
				t.Fatal("unauthenticated or incompatible peer was accepted")
			}
			if len(l.ready) != 0 {
				t.Fatal("failed negotiation published a server pipe")
			}
		})
	}
}

func TestDTLSPipeEventAndRequestReply(t *testing.T) {
	serverConfig, clientConfig := testIdentities(t).configs()
	server, client := test.GetSocket(t, rep.NewSocket), test.GetSocket(t, req.NewSocket)
	defer test.MustClose(t, server)
	defer test.MustClose(t, client)
	for _, sock := range []mangos.Socket{server, client} {
		test.MustSucceed(t, sock.SetOption(mangos.OptionRecvDeadline, 3*time.Second))
		test.MustSucceed(t, sock.SetOption(mangos.OptionSendDeadline, 3*time.Second))
	}
	states := make(chan ConnectionState, 1)
	server.SetPipeEventHook(func(ev mangos.PipeEvent, p mangos.Pipe) {
		if ev != mangos.PipeEventAttaching {
			return
		}
		value, err := p.GetOption(mangos.OptionDTLSConnState)
		if err == nil {
			states <- value.(ConnectionState)
		}
	})
	l, err := server.NewListener("dtls4://127.0.0.1:0", map[string]interface{}{mangos.OptionDTLSConfig: serverConfig})
	test.MustSucceed(t, err)
	test.MustSucceed(t, l.Listen())
	test.MustSucceed(t, client.DialOptions(l.Address(), map[string]interface{}{mangos.OptionDTLSConfig: clientConfig}))
	select {
	case state := <-states:
		if state.NegotiatedVersion() != protocol.Version1_3 || len(state.PeerCertificates) == 0 {
			t.Fatal("missing authenticated state at pipe event")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("missing pipe event")
	}
	test.MustSucceed(t, client.Send([]byte("request")))
	got, err := server.Recv()
	test.MustSucceed(t, err)
	if string(got) != "request" {
		t.Fatalf("request = %q", got)
	}
	test.MustSucceed(t, server.Send([]byte("reply")))
	got, err = client.Recv()
	test.MustSucceed(t, err)
	if string(got) != "reply" {
		t.Fatalf("reply = %q", got)
	}
}
