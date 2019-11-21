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

package respondent

import (
	"nanomsg.org/go/mangos/v2/protocol/surveyor"
	"nanomsg.org/go/mangos/v2/protocol/xrespondent"
	"nanomsg.org/go/mangos/v2/protocol/xsurveyor"
	"testing"

	. "nanomsg.org/go/mangos/v2/internal/test"
	_ "nanomsg.org/go/mangos/v2/transport/all"
)

func TestRespondentTTLZero(t *testing.T) {
	SetTTLZero(t, NewSocket)
}

func TestRespondentTTLNegative(t *testing.T) {
	SetTTLNegative(t, NewSocket)
}

func TestRespondentTTLTooBig(t *testing.T) {
	SetTTLTooBig(t, NewSocket)
}

func TestRespondentTTLNotInt(t *testing.T) {
	SetTTLNotInt(t, NewSocket)
}

func TestRespondentTTLSet(t *testing.T) {
	SetTTL(t, NewSocket)
}

func TestRespondentTTLDrop(t *testing.T) {
	TTLDropTest(t, surveyor.NewSocket, NewSocket, xsurveyor.NewSocket, xrespondent.NewSocket)
}
