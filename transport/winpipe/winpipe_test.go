// Copyright 2026 The Mangos Authors
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

//go:build windows
// +build windows

package winpipe

import (
	"io/ioutil"
	"os"
	"testing"

	"go.nanomsg.org/mangos/v3"
	"go.nanomsg.org/mangos/v3/internal/test"
)

var tran = Transport

func TestMain(m *testing.M) {
	cwd, err := os.Getwd()
	if err != nil {
		panic("Failed to determine working directory")
	}

	dir, err := ioutil.TempDir("", "winpipetest")
	if err != nil {
		panic("Failed to create directory")
	}
	if err = os.Chdir(dir); err != nil {
		panic("Failed to chdir: " + err.Error())
	}
	v := m.Run()
	if err = os.Chdir(cwd); err != nil {
		panic("Failed to chdir: " + err.Error())
	}
	if err = os.RemoveAll(dir); err != nil {
		panic("Failed to clean up directory: " + err.Error())
	}
	os.Exit(v)
}

func TestWinpipeRecvMax(t *testing.T) {
	test.TranVerifyMaxRecvSize(t, tran, nil, nil)
}

func TestWinpipeOptions(t *testing.T) {
	test.TranVerifyInvalidOption(t, tran)
	test.TranVerifyIntOption(t, tran, mangos.OptionMaxRecvSize)
}

func TestWinpipeScheme(t *testing.T) {
	test.TranVerifyScheme(t, tran)
}
func TestWinpipeAcceptWithoutListen(t *testing.T) {
	test.TranVerifyAcceptWithoutListen(t, tran)
}
func TestWinpipeListenAndAccept(t *testing.T) {
	test.TranVerifyListenAndAccept(t, tran, nil, nil)
}
func TestWinpipeDuplicateListen(t *testing.T) {
	test.TranVerifyDuplicateListen(t, tran, nil)
}
func TestWinpipeConnectionRefused(t *testing.T) {
	test.TranVerifyConnectionRefused(t, tran, nil)
}
func TestWinpipeHandshake(t *testing.T) {
	test.TranVerifyHandshakeFail(t, tran, nil, nil)
}
func TestWinpipeSendRecv(t *testing.T) {
	test.TranVerifySendRecv(t, tran, nil, nil)
}
func TestWinpipeListenerClosed(t *testing.T) {
	test.TranVerifyListenerClosed(t, tran, nil)
}
func TestWinpipeMessageSize(t *testing.T) {
	test.TranVerifyMessageSizes(t, tran, nil, nil)
}
func TestWinpipeMessageHeader(t *testing.T) {
	test.TranVerifyMessageHeader(t, tran, nil, nil)
}
func TestWinpipeVerifyPipeAddresses(t *testing.T) {
	test.TranVerifyPipeAddresses(t, tran, nil, nil)
}
func TestWinpipeVerifyPipeOptions(t *testing.T) {
	test.TranVerifyPipeOptions2(t, tran, nil, nil)
}
