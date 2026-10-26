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

// This opt-in peer requires NNG 2.1 with DTLS enabled. The Go test generates
// identity and CA files; no credentials or native libraries are bundled.
#include <nng/nng.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

static void
check(int rv, const char *op)
{
	if (rv != 0) {
		fprintf(stderr, "%s: %s\n", op, nng_strerror(rv));
		exit(1);
	}
}

int
main(int argc, char **argv)
{
	if (argc != 6) {
		fprintf(stderr, "usage: nng-peer listen|dial 12|13 URL IDENTITY CA\n");
		return 2;
	}
	check(nng_init(NULL), "init");
	int server = strcmp(argv[1], "listen") == 0;
	nng_tls_version version = strcmp(argv[2], "13") == 0 ? NNG_TLS_1_3 : NNG_TLS_1_2;
	nng_socket sock;
	nng_tls_config *config;
	check(nng_pair0_open(&sock), "socket");
	check(nng_socket_set_ms(sock, NNG_OPT_RECVTIMEO, 8000), "receive timeout");
	check(nng_socket_set_ms(sock, NNG_OPT_SENDTIMEO, 8000), "send timeout");
	check(nng_tls_config_alloc(&config, server ? NNG_TLS_MODE_SERVER : NNG_TLS_MODE_CLIENT), "config");
	check(nng_tls_config_version(config, version, version), "version");
	check(nng_tls_config_ca_file(config, argv[5]), "CA");
	check(nng_tls_config_cert_key_file(config, argv[4], NULL), "identity");
	check(nng_tls_config_auth_mode(config, NNG_TLS_AUTH_MODE_REQUIRED), "authentication");
	if (server) {
		nng_listener listener;
		check(nng_listener_create(&listener, sock, argv[3]), "listener");
		check(nng_listener_set_tls(listener, config), "listener TLS");
		check(nng_listener_start(listener, 0), "listen");
	} else {
		nng_dialer dialer;
		check(nng_tls_config_server_name(config, "localhost"), "server name");
		check(nng_dialer_create(&dialer, sock, argv[3]), "dialer");
		check(nng_dialer_set_tls(dialer, config), "dialer TLS");
		check(nng_dialer_start(dialer, 0), "dial");
	}
	nng_tls_config_free(config);
	puts("READY");
	fflush(stdout);
	size_t sizes[] = {1, 1200, 16320};
	for (size_t i = 0; i < sizeof(sizes) / sizeof(sizes[0]); i++) {
		unsigned char *buf = malloc(sizes[i]);
		if (buf == NULL) return 3;
		memset(buf, 'A' + i, sizes[i]);
		if (!server) check(nng_send(sock, buf, sizes[i], 0), "send");
		size_t len = sizes[i];
		check(nng_recv(sock, buf, &len, 0), "receive");
		if (len != sizes[i]) return 4;
		for (size_t j = 0; j < len; j++) if (buf[j] != 'A' + i) return 5;
		if (server) check(nng_send(sock, buf, len, 0), "echo");
		free(buf);
	}
	// Let the final asynchronous send reach the peer before closing.
	nng_msleep(200);
	nng_socket_close(sock);
	nng_fini();
	return 0;
}
