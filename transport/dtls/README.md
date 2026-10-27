The `dtls` package implements NNG's message-oriented DTLS transport using
[Pion DTLS v4](https://github.com/pion/dtls). Import this package, or
`transport/all`, to register its transports.

| URI | Address family |
| --- | --- |
| `dtls://HOST:PORT` | IPv4 or IPv6 |
| `dtls4://HOST:PORT` | IPv4 |
| `dtls6://HOST:PORT` | IPv6 |

Listeners may use an empty host or `*` for wildcard binding, and port `0`
for an automatically assigned port. `Listener.Address()` returns the bound
address. Dialers require a destination host and nonzero port.
Bracket literal IPv6 addresses, for example `dtls6://[::1]:4433`.

Configure endpoints before starting them with `mangos.OptionDTLSConfig`.
The value is a `*dtls.Config`, with Pion options for each endpoint role.
It is separate from `mangos.OptionTLSConfig`, which continues to use
`crypto/tls.Config` for TLS over TCP and WebSocket.

For example, after loading a server certificate and private key with
`tls.LoadX509KeyPair`:

```go
import (
    pion "github.com/pion/dtls/v4"
    "go.nanomsg.org/mangos/v3"
    "go.nanomsg.org/mangos/v3/transport/dtls"
)

config := &dtls.Config{
    ServerOptions: []pion.ServerOption{
        pion.WithCertificates(serverCertificate),
    },
}
listener, err := serverSocket.NewListener("dtls4://127.0.0.1:4433",
    map[string]interface{}{mangos.OptionDTLSConfig: config})
if err != nil {
    return err
}
if err := listener.Listen(); err != nil {
    return err
}
```

Configure a client with a certificate pool containing the server's CA:

```go
config := &dtls.Config{
    ClientOptions: []pion.ClientOption{
        pion.WithRootCAs(roots),
    },
}
err := clientSocket.DialOptions("dtls4://localhost:4433",
    map[string]interface{}{mangos.OptionDTLSConfig: config})
```

Dialers verify the server's certificate using the URL host, including IP
subject alternative names for IP addresses. Pion's `WithServerName` option
overrides the expected identity. A dialer without an explicit configuration
uses system trust roots. As with standard TLS, listeners do not require a
client certificate by default. For mutual authentication, add
`pion.WithClientAuth(pion.RequireAndVerifyClientCert)` and
`pion.WithClientCAs(roots)` to the server options, and
`pion.WithCertificates(clientCertificate)` to the client options.

### Certificate limitations

The transport is pinned to Pion `v4.0.0-rc.7`, a prerelease. Its public client
and server option types are exposed through `Config`; applications configuring
DTLS depend on that upstream API. The pinned release has two certificate
limitations:

- For DTLS 1.3 use **ECDSA keys or PSK**. Its CertificateVerify encoder
  rejects RSA-PSS signature schemes. RSA keys work with DTLS 1.2; restrict
  RSA-key endpoints with `pion.WithMaxVersion(protocol.Version1_2)`.
- Ed25519-signed certificate chains fail its certificate-signature policy
  check because its certificate and allowed-scheme hash mappings disagree.
  Use ECDSA- or RSA-signed chains until this upstream issue is fixed.

Certificate verification stays enabled. These are Pion rc.7 implementation
limitations, not restrictions imposed by the DTLS protocol. The relevant
upstream code is [CertificateVerify encoding](https://github.com/pion/dtls/blob/v4.0.0-rc.7/pkg/protocol/handshake/message_certificate_verify.go)
and [signature/hash mappings](https://github.com/pion/dtls/blob/v4.0.0-rc.7/pkg/crypto/signaturehash/signaturehash.go).

Pion's PSK callbacks are also supported through `pion.WithPSK`. For DTLS 1.2
PSK-only endpoints, explicitly select a compatible PSK cipher suite on both
ends, such as `ciphersuite.TLS_PSK_WITH_AES_128_GCM_SHA256` from Pion's
`pkg/crypto/ciphersuite` package.

### Versions and connection information

Both DTLS 1.2 and 1.3 are enabled by default, and the highest shared version
is selected. To require DTLS 1.3, add these options to the relevant role:

```go
pion.WithMinVersion(protocol.Version1_3)
pion.WithMaxVersion(protocol.Version1_3)
```

The version constants come from `github.com/pion/dtls/v4/pkg/protocol`,
and differ from the `crypto/tls` constants. The default curves are X25519
and P-256. This keeps the initial ClientHello small enough to interoperate
with NNG engines that require an unfragmented ClientHello before validating
cookies. Pion options can override these defaults; enabling larger hybrid
key shares may require additional interoperability and path-MTU testing.

`pipe.GetOption(mangos.OptionDTLSConnState)` returns a
`dtls.ConnectionState` (an alias for Pion's `State`). It is available at
`mangos.PipeEventAttaching`, after both DTLS authentication and NNG
negotiation. `NegotiatedVersion()` returns the selected DTLS version;
`PeerCertificates` contains DER-encoded peer certificates when certificate
authentication was used. PSK peers need not have certificates.

Configuration option slices are copied when set and when retrieved.
Certificate pools and callback state captured by options must remain safe
for concurrent use. Endpoint configuration and receive limits cannot change
after startup; attempts return `mangos.ErrProtoState`.

### Message and resource limits

Messages can be lost or reordered. DTLS retransmits its handshake, and the
transport retries NNG connection negotiation, but application messages are
not retransmitted. Each message is one DTLS application record; there is no
transport-level message fragmentation or reassembly. The existing TCP
stream handshake and stream length prefix are not used.

The maximum message size is 16,320 bytes, including the SP protocol header.
`mangos.OptionMaxRecvSize` can reduce it; zero or values above the maximum
select the transport maximum. Messages exceeding the peer's advertised
limit are silently dropped, matching NNG. Datagram boundaries delimit
incoming messages. Malformed or oversized incoming records close the pipe.
Prefer payloads that fit in the path MTU after IP, UDP, DTLS, and SP headers.

UDP sockets request 1 MiB send and receive buffers, best effort, matching
NNG. The OS may clamp or reject the request. In-memory ciphertext and
message queues are bounded independently; overflow drops packets/messages
rather than blocking unrelated peers on a listener's shared UDP socket.

`Config.ConnectTimeout` bounds DTLS and NNG negotiation together. Its default
is 5 seconds, starting at admission. `Config.MaxPending` defaults to 1,024
pending inbound connections. `Config.HandshakeRate` defaults to 128 new
connections/second, with an initial burst of 128. Zero selects the default
for these fields; negative values are rejected. Only plausible initial
ClientHello fragments allocate a peer. Established peers bypass the
admission limits. Cookies and peer authentication are handled by Pion.

Logical connections use periodic CREQ/CACK control messages. The default
refresh interval is 5 seconds; the shorter advertised peer interval is
honored. A connection expires after five refresh intervals without valid
inbound traffic. Closing a listener cancels pending connections and stops
admission while allowing accepted pipes to continue until they close.

Connection IDs/path migration, session export, and 0-RTT transport
semantics are not provided by this package. Pion v4 is currently pinned to
`v4.0.0-rc.7`, a prerelease; applications should account for that dependency
when choosing a release policy.

### Interoperability tests

Ordinary tests use pure Go and include DTLS 1.2/1.3, IPv4/IPv6, mutual
certificate authentication, PSK, version fallback, full-sized and empty
messages, loss during negotiation, admission limits, expiry, and concurrent
shutdown:

```sh
go test -race ./transport/dtls
```

An optional test exercises real NNG pair0 sockets, in both roles, with
mutual certificate verification and messages up to 16,320 bytes. Compile
`testdata/nng-peer.c` against NNG 2.1 with DTLS and a suitable TLS engine
enabled, for example against an installed library:

```sh
cc testdata/nng-peer.c -o /tmp/nng-dtls-peer $(pkg-config --cflags --libs nng)
MANGOS_NNG_DTLS_PEER=/tmp/nng-dtls-peer go test -v -run TestNNGInterop .
```

Run these commands from this package directory. For an engine supporting
only DTLS 1.2, restrict the test with `-run 'TestNNGInterop/12/'`.
DTLS 1.3 interoperability requires an NNG TLS engine built with that feature,
such as wolfSSL 5.9.1+ or OpenSSL 4.1+ with DTLS 1.3 enabled.

`macat` also accepts DTLS URIs with its existing certificate, key, CA, and
insecure-verification flags; see the macat documentation for those flags.
