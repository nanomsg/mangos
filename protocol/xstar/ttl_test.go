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

package xstar

import (
	"testing"

	. "nanomsg.org/go/mangos/v2/internal/test"
	_ "nanomsg.org/go/mangos/v2/transport/all"
)

func TestXStarTTLZero(t *testing.T) {
	SetTTLZero(t, NewSocket)
}

func TestXStarTTLNegative(t *testing.T) {
	SetTTLNegative(t, NewSocket)
}

func TestXStarTTLTooBig(t *testing.T) {
	SetTTLTooBig(t, NewSocket)
}

func TestXStarTTLNotInt(t *testing.T) {
	SetTTLNotInt(t, NewSocket)
}

func TestXStarTTLSet(t *testing.T) {
	SetTTL(t, NewSocket)
}
