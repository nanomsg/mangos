# Changes in Version 3

## Version 3.6.0 (unreleased)

### New transports

* Added NNG-compatible UDP transports using `udp://`, `udp4://`, and `udp6://`.
  Messages may be lost or reordered and are limited to 65,000 bytes including
  the SP protocol header. Messages above the peer's advertised receive limit
  are silently dropped. UDP socket buffers request 1 MiB, subject to OS limits.
* Added NNG-compatible DTLS transports using `dtls://`, `dtls4://`, and
  `dtls6://`, with DTLS 1.2 and 1.3, certificate authentication, mutual
  authentication, and PSK support. Configure endpoints with
  `mangos.OptionDTLSConfig`; `OptionTLSConfig` remains for TLS over TCP and
  WebSocket. `macat` supports DTLS with its certificate, key, CA, and insecure
  verification flags.
* DTLS retains UDP's loss and reordering behavior. Its maximum message size
  is 16,320 bytes including the SP protocol header, with no transport-level
  message fragmentation or reassembly. Prefer messages that fit the path MTU.
* Added `socket://` listeners for connected stream descriptors, supplied with
  `mangos.OptionSocketFD`, including descriptors created by `socketpair`.
* Importing `transport/all` registers the new transports automatically.

### DTLS dependency and certificate limitations

DTLS is pinned to Pion `v4.0.0-rc.7`, a prerelease. The configuration API uses
Pion v4 client and server option types, so applications configuring DTLS also
depend on that upstream API.

* For DTLS 1.3, use ECDSA keys or PSK. The pinned Pion encoder rejects RSA-PSS
  CertificateVerify schemes. RSA-key endpoints must be restricted to DTLS 1.2.
* Ed25519-signed certificate chains fail the pinned Pion signature policy
  check. Use ECDSA- or RSA-signed chains.
* These limitations do not disable certificate verification. Dialers verify
  the server's identity using the URI host by default.

See [the DTLS documentation](transport/dtls/README.md) for configuration,
resource limits, and optional interoperability tests against real NNG peers.

### Connection management

* Added socket activation for TCP, TLS-over-TCP, and Unix domain listeners
  through `mangos.OptionListenFD`. Unix socket permission and ownership options
  apply to activated listeners as well.
* TLS connection state is available to pipe hooks at `PipeEventAttaching`,
  after authentication completes. DTLS exposes its authenticated state with
  `mangos.OptionDTLSConnState`.
* TCP connection establishment after DNS resolution, TLS negotiation, and SP
  stream negotiation each have a ten-second timeout. These are separate phase
  limits, not a single end-to-end dialing deadline. Stalled TLS handshakes no
  longer prevent a listener from accepting other connections.

### WebSocket transport

* Replaced Gorilla WebSocket with `github.com/coder/websocket`, preserving the
  SP wire protocol and existing transport options.
* Added `ws.OptionWebSocketFollowRedirects` to opt in to HTTP redirects during
  dialing. Redirect following remains disabled by default.

### Fixes and validation

* Negative send and receive deadlines now perform nonblocking operations on
  protocols that support those options, including PAIRv1 and protocol contexts.
  Already available data or queue space takes precedence over the timeout.
  Zero deadlines continue to mean no timeout.
* Fixed concurrent slice appends in the REQ and SURVEYOR multicontext tests.
  Linux CI now runs the full race suite and `go vet`.
* Transport tests use ephemeral ports where supported to reduce collisions.

### Go compatibility

Go 1.26 or later is required, as in 3.5.0. Use a current patched Go toolchain.
See [the support policy](README.md#compatibility) for supported releases.

## Version 3.5.0

* Added explicit `unix://` and `winpipe://` transports. Unix domain sockets
  are available on POSIX systems and Windows versions with AF_UNIX support;
  Windows named pipes use `winpipe://`. The existing `ipc://` scheme remains
  a platform alias for Unix sockets on POSIX and named pipes on Windows.
  Existing IPC options remain compatible with the corresponding transports.
* Added `mangos.OptionReconnectBackoffOnClose`. When enabled, a dialer applies
  reconnect backoff after a pipe closes, including when an attaching pipe
  hook rejects a connection. It is disabled by default.
* Fixed a deadlock and pipe cleanup when an attaching hook closes a pipe.
* TLS-over-TCP listeners now accept configurations that obtain certificates
  dynamically through `tls.Config.GetCertificate`.
* Fixed Windows named-pipe listener startup/shutdown races and updated
  `go-winio` to include its connection shutdown fix.
* Updated dependencies, build constraints, examples, and CI. Formalized the
  Go support policy around the stable and preceding Go releases. Go 1.26
  or later is required by the Windows named-pipe dependency.

[Full changes](https://github.com/nanomsg/mangos/compare/v3.4.2...v3.5.0)

## Version 3.4.2

* Fixed a REQ panic caused by retransmission callbacks attempting to resend
  a request that no longer has a message. Also captured the request ID before
  scheduling a resend after a pipe disconnects (issue #264).

[Full changes](https://github.com/nanomsg/mangos/compare/v3.4.1...v3.4.2)

## Version 3.4.0

* Added cooked `protocol/pair1` and raw `protocol/xpair1` implementations of
  NNG's PAIRv1 protocol. A Mangos PAIRv1 socket supports one partner, and can
  connect to NNG peers using either monogamous or polyamorous mode. Mangos
  does not itself provide polyamorous PAIRv1 sockets.
* Fixed a PUSH sender goroutine leak during socket shutdown (issue #245).
* Stopped the REQ receive-deadline timer when a reply arrives, avoiding
  lingering timers when many requests complete before their deadlines.
* Updated the Windows named-pipe and WebSocket dependencies.

[Full changes](https://github.com/nanomsg/mangos/compare/v3.3.0...v3.4.0)

## Version 3.3.0

* Fixed Unix IPC socket permissions and stale-path cleanup, including error
  detection for wrapped system errors (issue #240).
* Updated dependencies, documentation links, and CI, and added Dependabot
  configuration. This was a maintenance release with no new public API.

[Full changes](https://github.com/nanomsg/mangos/compare/v3.2.1...v3.3.0)

## Version 3.2.1

* Fixed REQ contexts created with `OpenContext()` to inherit
  `mangos.OptionFailNoPeers` from the socket. The default socket context
  already supported the option introduced in 3.2.0.

[Full changes](https://github.com/nanomsg/mangos/compare/v3.2.0...v3.2.1)

## Version 3.2.0

* Added `mangos.OptionFailNoPeers` and `mangos.ErrNoPeers` for REQ and PUSH.
  Applications can opt in to immediate failure when no peers are connected,
  instead of waiting for a send or receive timeout. This helps distinguish
  an absent peer from a connected peer exerting backpressure; applications
  using it must handle temporary disconnects (issue #224).
* Corrected the documented type of `mangos.OptionMaxRecvSize` to `int`,
  matching the implementation (issue #221).

[Full changes](https://github.com/nanomsg/mangos/compare/v3.1.3...v3.2.0)

## Version 3.1.3

* Fixed REQ handling when `mangos.OptionRetryTime` is zero and the peer
  disconnects with a request outstanding. The request is canceled and a
  waiting receive returns `mangos.ErrCanceled`, rather than retransmitting
  an operation that may not be safe to repeat (issue #218).

[Full changes](https://github.com/nanomsg/mangos/compare/v3.1.2...v3.1.3)

## Version 3.1.2

* Fixed the Windows compilation failure introduced in 3.1.1. Use this
  release or a later version in place of 3.1.1.

[Full changes](https://github.com/nanomsg/mangos/compare/v3.1.1...v3.1.2)

## Version 3.1.1

* Made the IPC permission and ownership option constants available on all
  platforms, allowing portable applications to handle unsupported options
  at runtime instead of using platform-specific source files.

[Full changes](https://github.com/nanomsg/mangos/compare/v3.1.0...v3.1.1)

## Version 3.1.0

* Added read-only IPC pipe options for peer process, user, and group IDs:
  `mangos.OptionPeerPID`, `mangos.OptionPeerUID`, and `mangos.OptionPeerGID`.
  These are supported on Linux and illumos/Solaris; the latter also exposes
  `mangos.OptionPeerZone` and requires cgo (issue #211).
* Added Unix IPC listener options `ipc.OptionIpcSocketPermissions`,
  `ipc.OptionIpcSocketOwner`, and `ipc.OptionIpcSocketGroup` to set socket
  permissions and ownership (issue #213). Enforcement and the ability to
  change ownership depend on the platform and process privileges.

[Full changes](https://github.com/nanomsg/mangos/compare/v3.0.2...v3.1.0)

## Version 3.0.2

* Improved REQ scheduling so slow REP workers do not hold up faster workers
  (issue #195).
* Fixed a REQ context-close race that could remove a usable connection from
  the scheduling queue and cause later requests to time out sending
  (issue #201).

[Full changes](https://github.com/nanomsg/mangos/compare/v3.0.1...v3.0.2)

## Version 3.0.1

* Reduced REQ message copies by using reference-counted message cloning,
  improving performance for large requests (issue #181).
* Fixed a REQ panic during reconnection when a REP peer disconnects while
  holding a request (issue #192).

[Full changes](https://github.com/nanomsg/mangos/compare/v3.0.0...v3.0.1)

## Version 3.0.0

### Import path changed

Use `go.nanomsg.org/mangos/v3` as the base URL for imports.

### Go v1.12 Or Better Required at Introduction

Version 3.0 introduced modules and semantic versions, with Go 1.12 as its
supported minimum at the time. See the compatibility section above for the
current requirement.
