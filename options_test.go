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
	"fmt"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

// Unit tests need mocks to work properly.
// Please generate mocks by `make mock` or `make test` command.

func TestOptionsBuilder_Build(t *testing.T) {
	mHelper := &mockModifiersHelper{}
	mAttr := &unix.PerfEventAttr{}
	typeOfAttr := fmt.Sprintf("%T", mAttr)
	mModifiers := []string{"mock attr modifiers"}

	mBuilder := optionsBuilder{
		attrModifiers:   mModifiers,
		modifiersHelper: mHelper,
	}

	t.Run("error during qualifiers updating", func(t *testing.T) {
		mHelper.On("updateQualifiers", mock.AnythingOfType(typeOfAttr), mModifiers).Return(fmt.Errorf("mock error")).Once()
		result, err := mBuilder.Build()
		require.Error(t, err)
		require.Nil(t, result)
		mHelper.AssertExpectations(t)
	})

	t.Run("Enable on exec disabled", func(t *testing.T) {
		mHelper.On("updateQualifiers", mock.AnythingOfType(typeOfAttr), mModifiers).Return(nil).Once()
		result, err := mBuilder.Build()
		require.NoError(t, err)
		require.Equal(t, mAttr.Bits, result.Attr().Bits)
		require.Equal(t, mModifiers, result.PerfStatString())
		mHelper.AssertExpectations(t)
	})

	mBuilder.enableOnExec = true

	t.Run("Enable on exec enabled", func(t *testing.T) {
		bits := mAttr.Bits
		bits |= unix.PerfBitDisabled
		bits |= unix.PerfBitEnableOnExec

		mHelper.On("updateQualifiers", mock.AnythingOfType(typeOfAttr), mModifiers).Return(nil).Once()
		result, err := mBuilder.Build()
		require.NoError(t, err)
		require.Equal(t, bits, result.Attr().Bits)
		require.Equal(t, mModifiers, result.PerfStatString())
		mHelper.AssertExpectations(t)
	})
}

func TestPerfEventOptions_String(t *testing.T) {
	mStatString := []string{"option1=123", "option2=44d", "option3=2222"}
	expected := "option1=123,option2=44d,option3=2222"
	mOptions := PerfEventOptions{perfStatString: mStatString}

	result := mOptions.String()
	require.Equal(t, expected, result)
}
