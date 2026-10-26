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
	"bufio"
	"bytes"
	"context"
	"net"
	"os"
	"os/exec"
	"testing"
	"time"

	pion "github.com/pion/dtls/v4"
	"github.com/pion/dtls/v4/pkg/protocol"
	"go.nanomsg.org/mangos/v3"
	"go.nanomsg.org/mangos/v3/internal/test"
	"go.nanomsg.org/mangos/v3/protocol/pair"
)

// TestNNGInterop uses testdata/nng-peer.c compiled against an actual NNG
// build. Ordinary Go tests do not need a C toolchain or an NNG installation.
func TestNNGInterop(t *testing.T) {
	peer := os.Getenv("MANGOS_NNG_DTLS_PEER")
	if peer == "" {
		t.Skip("MANGOS_NNG_DTLS_PEER is not set")
	}
	ids := testIdentities(t)
	for _, version := range []struct {
		name  string
		value protocol.Version
	}{{"12", protocol.Version1_2}, {"13", protocol.Version1_3}} {
		for _, listen := range []bool{true, false} {
			role := "dial"
			if listen {
				role = "listen"
			}
			t.Run(version.name+"/mangos_"+role, func(t *testing.T) {
				ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
				defer cancel()
				serverConfig, clientConfig := ids.configs()
				serverConfig.ServerOptions = append(serverConfig.ServerOptions, pion.WithMinVersion(version.value), pion.WithMaxVersion(version.value))
				clientConfig.ClientOptions = append(clientConfig.ClientOptions, pion.WithMinVersion(version.value), pion.WithMaxVersion(version.value))
				sock := test.GetSocket(t, pair.NewSocket)
				defer test.MustClose(t, sock)
				test.MustSucceed(t, sock.SetOption(mangos.OptionRecvDeadline, 3*time.Second))
				test.MustSucceed(t, sock.SetOption(mangos.OptionSendDeadline, 3*time.Second))
				var address string
				nngRole := "listen"
				identity := ids.server
				if listen {
					nngRole = "dial"
					identity = ids.client
					l, err := sock.NewListener("dtls4://127.0.0.1:0", map[string]interface{}{mangos.OptionDTLSConfig: serverConfig})
					test.MustSucceed(t, err)
					test.MustSucceed(t, l.Listen())
					address = l.Address()
				} else {
					reserve, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
					test.MustSucceed(t, err)
					address = "dtls4://" + reserve.LocalAddr().String()
					test.MustSucceed(t, reserve.Close())
				}
				caPath, identityPath := ids.writeIdentity(t, identity)
				cmd := exec.CommandContext(ctx, peer, nngRole, version.name, address, identityPath, caPath)
				var stderr bytes.Buffer
				cmd.Stderr = &stderr
				stdout, err := cmd.StdoutPipe()
				test.MustSucceed(t, err)
				test.MustSucceed(t, cmd.Start())
				defer func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }()
				scan := bufio.NewScanner(stdout)
				if !listen {
					if !scan.Scan() || scan.Text() != "READY" {
						_ = cmd.Wait()
						t.Fatalf("NNG startup failed: %s", stderr.String())
					}
					test.MustSucceed(t, sock.DialOptions(address, map[string]interface{}{mangos.OptionDTLSConfig: clientConfig}))
				}
				for i, size := range []int{1, 1200, maxPayload} {
					expected := bytes.Repeat([]byte{byte('A' + i)}, size)
					if !listen {
						test.MustSucceed(t, sock.Send(expected))
					}
					message, err := sock.Recv()
					test.MustSucceed(t, err)
					if !bytes.Equal(message, expected) {
						t.Fatalf("NNG payload=%d bytes, want %d matching bytes", len(message), size)
					}
					if listen {
						test.MustSucceed(t, sock.Send(expected))
					}
				}
				for scan.Scan() {
				}
				if err := cmd.Wait(); err != nil {
					t.Fatalf("NNG peer: %v: %s", err, stderr.String())
				}
			})
		}
	}
}
