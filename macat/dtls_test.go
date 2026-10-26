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

package macat

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
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
	"go.nanomsg.org/mangos/v3"
	"go.nanomsg.org/mangos/v3/internal/test"
	"go.nanomsg.org/mangos/v3/protocol/pull"
	"go.nanomsg.org/mangos/v3/transport/dtls"
)

func TestAppDTLS(t *testing.T) {
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	test.MustSucceed(t, err)
	certificate := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "localhost"},
		DNSNames: []string{"localhost"}, IPAddresses: []net.IP{net.IPv4(127, 0, 0, 1)},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		IsCA: true, BasicConstraintsValid: true,
		KeyUsage:    x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, certificate, certificate, &privateKey.PublicKey, privateKey)
	test.MustSucceed(t, err)
	certificatePEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyDER, err := x509.MarshalECPrivateKey(privateKey)
	test.MustSucceed(t, err)
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	identity, err := tls.X509KeyPair(certificatePEM, keyPEM)
	test.MustSucceed(t, err)
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(certificatePEM) {
		t.Fatal("cannot load test CA")
	}
	dir := t.TempDir()
	ca, cert, key := filepath.Join(dir, "ca.pem"), filepath.Join(dir, "cert.pem"), filepath.Join(dir, "key.pem")
	test.MustSucceed(t, os.WriteFile(ca, certificatePEM, 0600))
	test.MustSucceed(t, os.WriteFile(cert, certificatePEM, 0600))
	test.MustSucceed(t, os.WriteFile(key, keyPEM, 0600))
	for _, listen := range []bool{false, true} {
		role := "dial"
		if listen {
			role = "listen"
		}
		t.Run(role, func(t *testing.T) {
			sock := test.GetSocket(t, pull.NewSocket)
			defer test.MustClose(t, sock)
			test.MustSucceed(t, sock.SetOption(mangos.OptionRecvDeadline, 3*time.Second))
			var address string
			if !listen {
				config := &dtls.Config{ServerOptions: []pion.ServerOption{pion.WithCertificates(identity)}}
				l, err := sock.NewListener("dtls4://127.0.0.1:0", map[string]interface{}{mangos.OptionDTLSConfig: config})
				test.MustSucceed(t, err)
				test.MustSucceed(t, l.Listen())
				address = l.Address()
			} else {
				reserve, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
				test.MustSucceed(t, err)
				address = "dtls4://" + reserve.LocalAddr().String()
				test.MustSucceed(t, reserve.Close())
			}
			app := &App{}
			app.Initialize()
			args := []string{"--push", "--data=ping", "--count=1", "--send-timeout=2s"}
			if listen {
				args = append(args, "--bind", address, "--cert", cert, "--key", key)
			} else {
				args = append(args, "--connect", address, "--cacert", ca)
			}
			result := make(chan error, 1)
			go func() { result <- app.Run(args...) }()
			if listen {
				config := &dtls.Config{ConnectTimeout: 200 * time.Millisecond, ClientOptions: []pion.ClientOption{pion.WithRootCAs(roots)}}
				test.MustSucceed(t, sock.DialOptions(address, map[string]interface{}{mangos.OptionDTLSConfig: config, mangos.OptionDialAsynch: true}))
			}
			type received struct {
				data []byte
				err  error
			}
			messages := make(chan received, 1)
			go func() { data, err := sock.Recv(); messages <- received{data, err} }()
			select {
			case message := <-messages:
				test.MustSucceed(t, message.err)
				if string(message.data) != "ping" {
					t.Fatalf("received %q", message.data)
				}
			case err := <-result:
				test.MustSucceed(t, err)
				message := <-messages
				test.MustSucceed(t, message.err)
				if string(message.data) != "ping" {
					t.Fatalf("received %q", message.data)
				}
				return
			case <-time.After(3 * time.Second):
				t.Fatal("timed out waiting for macat")
			}
			select {
			case err := <-result:
				test.MustSucceed(t, err)
			case <-time.After(3 * time.Second):
				t.Fatal("macat did not finish")
			}
		})
	}
}

func TestAppDTLSRequiresCredentials(t *testing.T) {
	for _, option := range []string{"--bind", "--connect"} {
		app := &App{}
		app.Initialize()
		err := app.Run("--push", option, "dtls4://127.0.0.1:9999", "--data=ping", "--count=1")
		if err == nil {
			t.Fatal("missing credentials accepted")
		}
	}
}
