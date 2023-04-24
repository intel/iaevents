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

//go:build linux && amd64

package iaevents

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewEventTargetProcess(t *testing.T) {
	tests := []struct {
		name     string
		pid      int
		procType int
	}{
		{"process type", 123, 0},
		{"cgroup type", 123123, 1},
		{"pid max int64", math.MaxInt64, 0},
		{"unknown process type", 82937492, 32},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := NewEventTargetProcess(test.pid, test.procType)
			require.Equal(t, test.procType, result.ProcessType())
			require.Equal(t, test.pid, result.ProcessID())
		})
	}
}

func TestEventTargetProcess_String(t *testing.T) {
	tests := []struct {
		name     string
		pid      int
		procType int
		result   string
	}{
		{"process type", 123, 0, "pid:123, process type:process"},
		{"cgroup type", 123123, 1, "pid:123123, process type:cgroup"},
		{"pid max int64", math.MaxInt64, 0, "pid:9223372036854775807, process type:process"},
		{"unknown process type", 82937492, 32, "pid:82937492, process type:unknown"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := (&EventTargetProcess{test.pid, test.procType}).String()
			require.Equal(t, test.result, result)
		})
	}
}
