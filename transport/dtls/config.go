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
	"slices"
	"time"

	pion "github.com/pion/dtls/v4"
	"github.com/pion/dtls/v4/pkg/crypto/elliptic"
	"github.com/pion/dtls/v4/pkg/protocol"
	"go.nanomsg.org/mangos/v3"
)

// Config configures DTLS endpoints through mangos.OptionDTLSConfig.
// Options are applied after the transport's defaults. Certificates, trust
// roots, authentication policy, version restrictions, and PSKs are configured
// with Pion's options. ClientOptions are used only by dialers; ServerOptions
// are used only by listeners.
//
// By default both DTLS 1.2 and 1.3 are enabled. The default curves are X25519
// and P-256, keeping the initial ClientHello small enough for NNG's TLS
// engines to validate cookies without reassembling a fragmented ClientHello.
// Connection ID routing and path migration are not supported.
// The pinned Pion release requires ECDSA keys or PSK for DTLS 1.3; restrict
// RSA-key endpoints to DTLS 1.2. Ed25519-signed certificate chains are not
// currently usable. See the package README for the upstream limitations.
//
// The option slices are copied when the configuration is set. Values captured
// by options, such as certificate pools and callback state, must not be
// modified concurrently with their use.
type Config struct {
	ClientOptions []pion.ClientOption
	ServerOptions []pion.ServerOption

	// ConnectTimeout bounds DTLS and NNG negotiation together, starting
	// when a connection is admitted. Zero selects 5 seconds.
	ConnectTimeout time.Duration

	// MaxPending bounds inbound connections that have not completed both
	// negotiations, including queued connections. Zero selects 1024.
	MaxPending int

	// HandshakeRate limits new inbound connections per second, with a
	// burst of the same size. Zero selects 128. Established peers bypass it.
	HandshakeRate int
}

// ConnectionState describes the authenticated DTLS connection. Its
// NegotiatedVersion method returns Pion's DTLS version, not a crypto/tls
// version constant. PeerCertificates contains DER-encoded certificates.
type ConnectionState = pion.State

func cloneConfig(c *Config) *Config {
	if c == nil {
		return nil
	}
	copy := *c
	copy.ClientOptions = slices.Clone(c.ClientOptions)
	copy.ServerOptions = slices.Clone(c.ServerOptions)
	return &copy
}

func (c *Config) validate() error {
	if c.ConnectTimeout < 0 || c.MaxPending < 0 || c.HandshakeRate < 0 {
		return mangos.ErrBadValue
	}
	for _, option := range c.ClientOptions {
		if option == nil {
			return mangos.ErrBadValue
		}
	}
	for _, option := range c.ServerOptions {
		if option == nil {
			return mangos.ErrBadValue
		}
	}
	return nil
}

func (c *Config) timeout() time.Duration {
	if c.ConnectTimeout != 0 {
		return c.ConnectTimeout
	}
	return 5 * time.Second
}

func (c *Config) maxPending() int {
	if c.MaxPending != 0 {
		return c.MaxPending
	}
	return 1024
}

func (c *Config) handshakeRate() int {
	if c.HandshakeRate != 0 {
		return c.HandshakeRate
	}
	return 128
}

func defaultOptions() []pion.Option {
	return []pion.Option{
		pion.WithMinVersion(protocol.Version1_2),
		pion.WithMaxVersion(protocol.Version1_3),
		pion.WithEllipticCurves(elliptic.X25519, elliptic.P256),
		pion.WithReceiveBufferSize(65535),
	}
}

func (c *Config) clientOptions(host string) []pion.ClientOption {
	opts := []pion.ClientOption{pion.WithServerName(host)}
	for _, option := range defaultOptions() {
		opts = append(opts, option)
	}
	return append(opts, c.ClientOptions...)
}

func (c *Config) serverOptions() []pion.ServerOption {
	var opts []pion.ServerOption
	for _, option := range defaultOptions() {
		opts = append(opts, option)
	}
	return append(opts, c.ServerOptions...)
}
