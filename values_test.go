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
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEventScaledValue(t *testing.T) {
	// MaxUint64 * 312
	veryBig, _ := new(big.Int).SetString("5755384150997380103880", 10)
	// MaxUint64 * MaxUint64
	veryVeryBig, _ := new(big.Int).SetString("340282366920938463426481119284349108225", 10)

	tests := []struct {
		name    string
		raw     uint64
		enabled uint64
		running uint64
		result  *big.Int
	}{
		{"only zeroes", 0, 0, 0, big.NewInt(0)},
		{"running value is zero", 123, 312, 0, big.NewInt(123)},
		{"enabled equal running", 333333, 11111, 11111, big.NewInt(333333)},
		{"time multiplexing happened", 69078934790, 180157854328, 144133151122, big.NewInt(86344554144)},
		{"max uint64 value multiplied", math.MaxUint64, 312, 1, veryBig},
		{"max uint64 value multiplied by max uint64", math.MaxUint64, math.MaxUint64, 1, veryVeryBig},
		{"only max uint64", math.MaxUint64, math.MaxUint64, math.MaxUint64, new(big.Int).SetUint64(math.MaxUint64)},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			scaled := EventScaledValue(CounterValue{test.raw, test.enabled, test.running})
			require.Equal(t, test.result, scaled)
		})
	}
}

func TestCounterValue_String(t *testing.T) {
	tests := []struct {
		name    string
		raw     uint64
		enabled uint64
		running uint64
		result  string
	}{
		{"all zeroes", 0, 0, 0, "Raw:0, Enabled:0, Running:0"},
		{"average values", 69078934790, 180157854328, 144133151122, "Raw:69078934790, Enabled:180157854328, Running:144133151122"},
		{"all max uint64", math.MaxUint64, math.MaxUint64, math.MaxUint64, "Raw:18446744073709551615, Enabled:18446744073709551615, Running:18446744073709551615"}, //nolint:lll // Keep format of the test cases
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cv := CounterValue{test.raw, test.enabled, test.running}
			result := cv.String()
			require.Equal(t, test.result, result)
		})
	}
}
