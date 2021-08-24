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
	"math/big"
	"strings"
)

// EventGroupMonitor is an interface for group monitoring functionalities.
//
// Events return all active events within event group that the EventGroupMonitor is monitoring.
// Deactivate closes, deactivates all active events within event group.
type EventGroupMonitor interface {
	Events() []*ActiveEvent
	Deactivate()
}

// ActiveMultiEvent contains set of active multi PMU uncore events.
type ActiveMultiEvent struct {
	events    []*ActiveEvent
	perfEvent *PerfEvent
}

func (am *ActiveMultiEvent) Events() []*ActiveEvent {
	return am.events
}

func (am *ActiveMultiEvent) Deactivate() {
	for i, e := range am.events {
		_ = e.Deactivate()
		am.events[i] = nil
	}
	am.events = nil
}

// PerfEvent returns original multi PMU perf event.
func (am *ActiveMultiEvent) PerfEvent() *PerfEvent {
	return am.perfEvent
}

// ReadValues returns all active multi PMU events counter value.
func (am *ActiveMultiEvent) ReadValues() ([]CounterValue, error) {
	var valuesGroup []CounterValue
	for _, b := range am.events {
		value, err := b.ReadValue()
		if err != nil {
			return nil, err
		}
		valuesGroup = append(valuesGroup, value)
	}
	return valuesGroup, nil
}

func (am *ActiveMultiEvent) String() string {
	var all []string
	for _, event := range am.Events() {
		all = append(all, event.String())
	}
	return strings.Join(all, ",")
}

// ActiveEventGroup contains set of active events belonging to the same perf event group.
type ActiveEventGroup struct {
	events []*ActiveEvent
}

func (ae *ActiveEventGroup) Events() []*ActiveEvent {
	return ae.events
}

func (ae *ActiveEventGroup) Deactivate() {
	for i, e := range ae.events {
		_ = e.Deactivate()
		ae.events[i] = nil
	}
	ae.events = nil
}

func (ae *ActiveEventGroup) addEvent(event *ActiveEvent) {
	ae.events = append(ae.events, event)
}

func (ae *ActiveEventGroup) String() string {
	var all []string
	for _, event := range ae.Events() {
		all = append(all, event.String())
	}
	return strings.Join(all, ",")
}

// AggregateValues lets aggregate multiple counter values.
func AggregateValues(values []CounterValue) (raw *big.Int, enabled *big.Int, running *big.Int) {
	var rawSum, enabledSum, runningSum = new(big.Int), new(big.Int), new(big.Int)
	for _, value := range values {
		enabledBig := new(big.Int).SetUint64(value.Enabled)
		runningBig := new(big.Int).SetUint64(value.Running)
		rawBig := new(big.Int).SetUint64(value.Raw)

		rawSum.Add(rawSum, rawBig)
		enabledSum.Add(enabledSum, enabledBig)
		runningSum.Add(runningSum, runningBig)
	}
	return rawSum, enabledSum, runningSum
}
