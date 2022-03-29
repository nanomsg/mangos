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
// WITHOUT WARRANTIES O R CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package test

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"sync"
	"testing"
	"time"
)

// KeyPair is a single public key pair
type KeyPair struct {
	cert    *x509.Certificate
	pair    tls.Certificate
	certDER []byte
	pubKey  ed25519.PublicKey
	prvKey  ed25519.PrivateKey
}

// Keys is a set of the Root, Server, and Client keys for a test config.
type Keys struct {
	Root   KeyPair // Root CA key pair
	Server KeyPair // Server key pair
	Client KeyPair // Client key pair
}

func (k *KeyPair) CertPEM() []byte {
	return pem.EncodeToMemory(&pem.Block{
		Type:    "CERTIFICATE",
		Headers: nil,
		Bytes:   k.certDER,
	})
}

func (k *KeyPair) KeyPEM() []byte {

	b, err := x509.MarshalPKCS8PrivateKey(k.prvKey)
	if err != nil {
		return nil
	}

	return pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: b,
	})
}

func (k *KeyPair) PubKeyPEM() []byte {
	b, err := x509.MarshalPKIXPublicKey(k.pubKey)
	if err != nil {
		return nil
	}

	return pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: b,
	})
}

func (k *KeyPair) genKey() (err error) {
	if k.pubKey, k.prvKey, err = ed25519.GenerateKey(rand.Reader); err != nil {
		return
	}
	return
}

func (k *KeyPair) genCert(tmpl *x509.Certificate, parent *KeyPair) (err error) {
	k.cert = tmpl // for self-signed, we pass ourselves as parent, this makes it work
	k.certDER, err = x509.CreateCertificate(rand.Reader, tmpl, parent.cert, k.pubKey, parent.prvKey)
	if err != nil {
		return
	}
	if k.cert, err = x509.ParseCertificate(k.certDER); err != nil {
		return
	}
	k.pair = tls.Certificate{
		Certificate: [][]byte{k.certDER},
		PrivateKey:  k.prvKey,
		Leaf:        k.cert,
	}
	return
}

func newKeys() (k *Keys, err error) {
	k = &Keys{}
	if err = k.Root.genKey(); err != nil {
		return nil, err
	}
	if err = k.Server.genKey(); err != nil {
		return nil, err
	}
	if err = k.Client.genKey(); err != nil {
		return nil, err
	}

	if err = k.Root.genCert(rootTmpl, &k.Root); err != nil {
		return nil, err
	}
	if err = k.Server.genCert(serverTmpl, &k.Root); err != nil {
		return nil, err
	}
	if err = k.Client.genCert(clientTmpl, &k.Root); err != nil {
		return nil, err
	}

	return k, nil
}

var rootTmpl = &x509.Certificate{
	SerialNumber: big.NewInt(1),

	Issuer: pkix.Name{
		CommonName:   "issuer.mangos.example.com",
		Organization: []string{"Mangos Issuer Org"},
	},
	Subject: pkix.Name{
		CommonName:   "Root.mangos.example.com",
		Organization: []string{"Mangos Root Org"},
	},
	NotBefore:             time.Unix(1000, 0),
	NotAfter:              time.Now().Add(time.Hour),
	IsCA:                  true,
	BasicConstraintsValid: true,
	OCSPServer:            []string{"ocsp.mangos.example.com"},
	DNSNames:              []string{"Root.mangos.example.com"},
	IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
	SignatureAlgorithm:    x509.PureEd25519,
	KeyUsage:              x509.KeyUsageCertSign,
	ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
}

var serverTmpl = &x509.Certificate{
	SerialNumber: big.NewInt(2),

	Issuer: pkix.Name{
		CommonName:   "issuer.mangos.example.com",
		Organization: []string{"Mangos Issuer Org"},
	},
	Subject: pkix.Name{
		CommonName:   "Server.mangos.example.com",
		Organization: []string{"Mangos Server Org"},
	},
	NotBefore:          time.Unix(1000, 0),
	NotAfter:           time.Now().Add(time.Hour),
	IsCA:               false,
	OCSPServer:         []string{"ocsp.mangos.example.com"},
	DNSNames:           []string{"Server.mangos.example.com"},
	IPAddresses:        []net.IP{net.ParseIP("127.0.0.1")},
	SignatureAlgorithm: x509.PureEd25519,
	KeyUsage:           x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
	ExtKeyUsage:        []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
}

var clientTmpl = &x509.Certificate{
	SerialNumber: big.NewInt(3),

	Issuer: pkix.Name{
		CommonName:   "issuer.mangos.example.com",
		Organization: []string{"Mangos Issuer Org"},
	},
	Subject: pkix.Name{
		CommonName:   "Client.mangos.example.com",
		Organization: []string{"Mangos Client Org"},
	},
	NotBefore:          time.Unix(1000, 0),
	NotAfter:           time.Now().Add(time.Hour),
	IsCA:               false,
	OCSPServer:         []string{"ocsp.mangos.example.com"},
	DNSNames:           []string{"Client.mangos.example.com"},
	IPAddresses:        []net.IP{net.ParseIP("127.0.0.1")},
	SignatureAlgorithm: x509.PureEd25519,
	KeyUsage:           x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
	ExtKeyUsage:        []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
}

// NewTLSConfig creates a suitable TLS configuration, using
// either a Server or Client.
func NewTLSConfig() (*tls.Config, *tls.Config, *Keys, error) {
	srvCfg := &tls.Config{}
	cliCfg := &tls.Config{}

	keys, err := newKeys()
	if err != nil {
		return nil, nil, nil, err
	}

	// Server side config.
	srvCfg.Certificates = append(srvCfg.Certificates, keys.Server.pair)

	// Client side config.
	cliCfg.Certificates = append(cliCfg.Certificates, keys.Client.pair)

	// Now configure the things the Client needs to know -- the self-signed
	// Root CA, and also the Server's identity.
	cliCfg.ServerName = "127.0.0.1"
	cliCfg.RootCAs = x509.NewCertPool()
	cliCfg.RootCAs.AddCert(keys.Root.cert)
	return srvCfg, cliCfg, keys, nil
}

var lock sync.Mutex
var clientConfig *tls.Config
var serverConfig *tls.Config
var keys *Keys

// GetTLSConfig is like NewTLSConfig, but it caches to avoid regenerating
// key material pointlessly.
func GetTLSConfig(t *testing.T, server bool) *tls.Config {
	var err error
	lock.Lock()
	defer lock.Unlock()

	if serverConfig == nil || clientConfig == nil || keys == nil {
		serverConfig, clientConfig, keys, err = NewTLSConfig()
		MustSucceed(t, err)
	}
	if server {
		return serverConfig
	}
	return clientConfig
}

// GetTLSConfigKeys is like NewTLSConfig, but it caches to avoid regenerating
// key material pointlessly.  It also returns the Keys.
func GetTLSConfigKeys(t *testing.T) (*tls.Config, *tls.Config, *Keys) {
	var err error
	lock.Lock()
	defer lock.Unlock()

	if serverConfig == nil || clientConfig == nil || keys == nil {
		serverConfig, clientConfig, keys, err = NewTLSConfig()
		MustSucceed(t, err)
	}
	return serverConfig, clientConfig, keys
}
