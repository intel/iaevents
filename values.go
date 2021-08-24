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
	"math/big"
)

// CounterValue holds specific perf event Raw, Enabled and Running counter values.
type CounterValue struct {
	Raw     uint64
	Enabled uint64
	Running uint64
}

// EventScaledValue calculates scaled value from CounterValue. Scaled value is equal to raw * enabled / running.
// If more events are activated than available counter slots on the PMU, then multiplexing happens and events only run part of the time.
// In that case the enabled and running values can be used to scale an estimated value for the count.
func EventScaledValue(values CounterValue) *big.Int {
	enabledBig := new(big.Int).SetUint64(values.Enabled)
	runningBig := new(big.Int).SetUint64(values.Running)
	rawBig := new(big.Int).SetUint64(values.Raw)
	if values.Enabled != values.Running && values.Running != uint64(0) {
		product := new(big.Int).Mul(rawBig, enabledBig)
		scaled := new(big.Int).Div(product, runningBig)
		return scaled
	}
	return rawBig
}

func (cv *CounterValue) String() string {
	return fmt.Sprintf("Raw:%d, Enabled:%d, Running:%d", cv.Raw, cv.Enabled, cv.Running)
}
