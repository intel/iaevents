// Copyright (c) 2021, Intel Corporation.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// +build linux
// +build amd64

package iaevents

import (
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

// Unit tests need mocks to work properly.
// Please generate mocks by `make mock` or `make test` command.

func Test_perfModifiersHelper_updateQualifiers(t *testing.T) {
	modifiersHelper := &perfModifiersHelper{}

	t.Run("Return error if attributes are nil", func(t *testing.T) {
		err := modifiersHelper.updateQualifiers(nil, []string{})
		require.Error(t, err)
	})
	t.Run("Do not return error for empty qualifiers, do not modify attributes", func(t *testing.T) {
		testAttr := &unix.PerfEventAttr{Config: uint64(0), Bits: uint64(0)}
		err := modifiersHelper.updateQualifiers(testAttr, []string{})
		require.NoError(t, err)
		err = modifiersHelper.updateQualifiers(testAttr, nil)
		require.NoError(t, err)
		err = modifiersHelper.updateQualifiers(testAttr, []string{""})
		require.NoError(t, err)
		err = modifiersHelper.updateQualifiers(testAttr, []string{"", ""})
		require.NoError(t, err)
		require.Equal(t, uint64(0), testAttr.Config)
		require.Equal(t, uint64(0), testAttr.Bits)
	})
	t.Run("Return error on invalid format", func(t *testing.T) {
		testAttr := &unix.PerfEventAttr{}
		err := modifiersHelper.updateQualifiers(testAttr, []string{"Z"})
		require.Error(t, err)
		err = modifiersHelper.updateQualifiers(testAttr, []string{"configk=0x1"})
		require.Error(t, err)
		err = modifiersHelper.updateQualifiers(testAttr, []string{"config=k"})
		require.Error(t, err)
		err = modifiersHelper.updateQualifiers(testAttr, []string{"config=0x1Z"})
		require.Error(t, err)
		err = modifiersHelper.updateQualifiers(testAttr, []string{"config=0x1", "Z"})
		require.Error(t, err)
		err = modifiersHelper.updateQualifiers(testAttr, []string{"config1"})
		require.Error(t, err)
	})
	t.Run("Update single u qualifier", func(t *testing.T) {
		testAttr := &unix.PerfEventAttr{}
		err := modifiersHelper.updateQualifiers(testAttr, []string{"u"})
		require.NoError(t, err)
		require.Equal(t, uint64(unix.PerfBitExcludeKernel), testAttr.Bits)
	})
	t.Run("Update extended config 1", func(t *testing.T) {
		testAttr := &unix.PerfEventAttr{}
		err := modifiersHelper.updateQualifiers(testAttr, []string{"config1=0x2Bu"})
		require.NoError(t, err)
		require.Equal(t, uint64(0x2b), testAttr.Ext1)
		require.Equal(t, uint64(unix.PerfBitExcludeKernel), testAttr.Bits)
	})
	t.Run("Update extended config 2", func(t *testing.T) {
		testAttr := &unix.PerfEventAttr{}
		err := modifiersHelper.updateQualifiers(testAttr, []string{"config2=0x2b"})
		require.NoError(t, err)
		require.Equal(t, uint64(0x2b), testAttr.Ext2)
	})
	t.Run("Parse multiple combined qualifiers", func(t *testing.T) {
		testAttr := &unix.PerfEventAttr{}
		qualifiers := []string{"config=0xF11k", "config1=0xf22", "config2=0xF33", "u"}
		err := modifiersHelper.updateQualifiers(testAttr, qualifiers)
		require.NoError(t, err)
		require.Equal(t, uint64(0xf11), testAttr.Config)
		require.Equal(t, uint64(0xf22), testAttr.Ext1)
		require.Equal(t, uint64(0xf33), testAttr.Ext2)
		require.Equal(t, uint64(unix.PerfBitExcludeUser|unix.PerfBitExcludeKernel), testAttr.Bits)
	})
}

func Test_perfModifiersHelper_updateModifiers(t *testing.T) {
	modifiersHelper := &perfModifiersHelper{}

	t.Run("Should not return error for empty string", func(t *testing.T) {
		testAttr := &unix.PerfEventAttr{}
		require.NoError(t, modifiersHelper.updateModifiers(testAttr, ""))
		require.Equal(t, uint64(0), testAttr.Bits)
	})
	t.Run("Update attribute bits for single modifier", func(t *testing.T) {
		testAttr := &unix.PerfEventAttr{}
		require.NoError(t, modifiersHelper.updateModifiers(testAttr, "p"))
		require.Equal(t, uint64(unix.PerfBitPreciseIPBit1), testAttr.Bits)
	})
	t.Run("Return error if one of modifiers is invalid", func(t *testing.T) {
		testAttr := &unix.PerfEventAttr{}
		require.Error(t, modifiersHelper.updateModifiers(testAttr, "pZ"))
	})
	t.Run("Should preserve existing bits in perf attributes", func(t *testing.T) {
		testAttr := &unix.PerfEventAttr{Bits: uint64(unix.PerfBitExcludeUser)}
		expected := uint64(unix.PerfBitExcludeUser | unix.PerfBitExcludeKernel)
		// Use `u` to set PerfBitExcludeKernel
		require.NoError(t, modifiersHelper.updateModifiers(testAttr, "u"))
		require.Equal(t, expected, testAttr.Bits)
	})
	t.Run("Update bits in perf attributes for multiple modifiers", func(t *testing.T) {
		testAttr := &unix.PerfEventAttr{}
		expected := uint64(unix.PerfBitExcludeUser | unix.PerfBitExcludeKernel | unix.PerfBitExcludeGuest |
			unix.PerfBitExcludeIdle | unix.PerfBitExcludeHv | unix.PerfBitPinned)
		require.NoError(t, modifiersHelper.updateModifiers(testAttr, "kuhHIGD"))
		require.Equal(t, expected, testAttr.Bits)
	})
}

func Test_incPreciseIPBits(t *testing.T) {
	type args struct {
		bits uint64
	}
	tests := []struct {
		name string
		args args
		want uint64
	}{
		{"No Bits", args{0}, unix.PerfBitPreciseIPBit1},
		{"First Bit Set", args{unix.PerfBitPreciseIPBit1}, unix.PerfBitPreciseIPBit2},
		{"Second Bit Set", args{unix.PerfBitPreciseIPBit2}, unix.PerfBitPreciseIPBit1 | unix.PerfBitPreciseIPBit2},
		{"Both Bits Set", args{unix.PerfBitPreciseIPBit1 | unix.PerfBitPreciseIPBit2}, unix.PerfBitPreciseIPBit1 | unix.PerfBitPreciseIPBit2},
		{"All Bits Except First", args{^uint64(unix.PerfBitPreciseIPBit1)}, ^uint64(0)},
		{"All Bits Except Second", args{^uint64(unix.PerfBitPreciseIPBit2)}, ^uint64(unix.PerfBitPreciseIPBit1)},
		{"All Bits Except PreciseIP Bits", args{^uint64(unix.PerfBitPreciseIPBit1 | unix.PerfBitPreciseIPBit2)}, ^uint64(unix.PerfBitPreciseIPBit2)},
		{"All Bits Set", args{^uint64(0)}, ^uint64(0)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := incPreciseIPBits(tt.args.bits); got != tt.want {
				t.Errorf("incPreciseIPBits() = %v, want %v", got, tt.want)
			}
		})
	}
}
