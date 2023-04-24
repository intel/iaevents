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
	"errors"
	"math"
	"math/big"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/require"
)

// Unit tests need mocks to work properly.
// Please generate mocks by `make mock` or `make test` command.

func TestActiveMultiEvent_Deactivate(t *testing.T) {
	number := 10
	var events []*ActiveEvent
	var mock []*mockUnixHelper

	for i := 0; i < number; i++ {
		unixMock := &mockUnixHelper{}
		newActive := &ActiveEvent{FileDescriptor: i, unix: unixMock}
		unixMock.On("close", i).Return(nil).Once()
		events = append(events, newActive)
		mock = append(mock, unixMock)
	}

	newActiveMulti := ActiveMultiEvent{events: events}
	newActiveMulti.Deactivate()

	require.Nil(t, newActiveMulti.events)
	for _, event := range events {
		require.Nil(t, event)
	}
	for _, m := range mock {
		m.AssertExpectations(t)
	}
}

func TestActiveMultiEvent_PerfEvent(t *testing.T) {
	mPerfEvent, _, _ := newPerfEventForTest()
	newActiveMulti := ActiveMultiEvent{perfEvent: mPerfEvent}

	result := newActiveMulti.PerfEvent()
	require.Equal(t, mPerfEvent, result)
}

func TestActiveMultiEvent_ReadValues(t *testing.T) {
	mPerfEvent, _, _ := newPerfEventForTest()
	number := 10

	type eventWithMock struct {
		event *ActiveEvent
		read  *mockReadValues
	}
	var eventsWithMocks []eventWithMock
	var events []*ActiveEvent
	var counterValues []CounterValue

	t.Run("no events", func(t *testing.T) {
		newActiveMulti := ActiveMultiEvent{events: nil, perfEvent: mPerfEvent}
		res, err := newActiveMulti.ReadValues()
		require.NoError(t, err)
		require.Empty(t, res)
	})

	t.Run("error while reading", func(t *testing.T) {
		newMock := &mockReadValues{}
		newActive := &ActiveEvent{PerfEvent: mPerfEvent, FileDescriptor: 0, read: newMock.Execute}
		newMock.On("Execute", nil, 0).Return(CounterValue{}, errors.New("mock error")).Once()

		newActiveMulti := ActiveMultiEvent{events: []*ActiveEvent{newActive}, perfEvent: mPerfEvent}
		res, err := newActiveMulti.ReadValues()
		require.Error(t, err)
		require.Nil(t, res)
	})

	t.Run("different values", func(t *testing.T) {
		for i := 0; i < number; i++ {
			values := CounterValue{rand.Uint64(), rand.Uint64(), rand.Uint64()}
			newMock := &mockReadValues{}
			mUnix := &mockUnixHelper{}
			newActive := &ActiveEvent{PerfEvent: mPerfEvent, FileDescriptor: i, unix: mUnix, read: newMock.Execute}

			newMock.On("Execute", mUnix, i).Return(values, nil).Once()
			events = append(events, newActive)
			eventsWithMocks = append(eventsWithMocks, eventWithMock{newActive, newMock})
			counterValues = append(counterValues, values)
		}

		newActiveMulti := ActiveMultiEvent{events: events, perfEvent: mPerfEvent}

		values, err := newActiveMulti.ReadValues()
		require.NoError(t, err)
		require.NotNil(t, values)
		require.Equal(t, counterValues, values)
		for _, m := range eventsWithMocks {
			m.read.AssertExpectations(t)
		}
	})
}

func TestActiveEventGroup_Deactivate(t *testing.T) {
	number := 10
	var events []*ActiveEvent
	var mock []*mockUnixHelper

	for i := 0; i < number; i++ {
		unixMock := &mockUnixHelper{}
		newActive := &ActiveEvent{FileDescriptor: i, unix: unixMock}
		unixMock.On("close", i).Return(nil).Once()
		events = append(events, newActive)
		mock = append(mock, unixMock)
	}

	newEventGroup := ActiveEventGroup{events: events}
	newEventGroup.Deactivate()

	require.Nil(t, newEventGroup.events)
	for _, event := range events {
		require.Nil(t, event)
	}
	for _, m := range mock {
		m.AssertExpectations(t)
	}
}

func TestString(t *testing.T) {
	type eventStr struct {
		name string
		cpu  int
		pmu  string
	}

	tests := []struct {
		name   string
		events []eventStr
		result string
	}{
		{"test 1", []eventStr{{"test", 1, "test_pmu"}, {"test", 2, "test_cpu"}}, "test/1/test_pmu,test/2/test_cpu"},
		{"test 2", []eventStr{{"test2", 100, "test_pmu"}, {"test2", 20, "asdzxcasd"}, {"test2", 20, "test_string"}},
			"test2/100/test_pmu,test2/20/asdzxcasd,test2/20/test_string"},
		{"test 3", []eventStr{{"CPU_UNCORE_EVENT", 0, "cbox_1"}}, "CPU_UNCORE_EVENT/0/cbox_1"},
	}

	for _, test := range tests {
		t.Run(test.result, func(t *testing.T) {
			var events []*ActiveEvent
			for _, e := range test.events {
				pe := &PerfEvent{Name: e.name}
				pmu := NamedPMUType{e.pmu, 0}
				newActive := &ActiveEvent{PerfEvent: pe, cpu: e.cpu, namedPMUType: pmu}
				events = append(events, newActive)
			}

			t.Run("active event group", func(t *testing.T) {
				newEventGroup := &ActiveEventGroup{events: events}
				res := newEventGroup.String()
				require.Equal(t, test.result, res)
			})

			t.Run("active multi event", func(t *testing.T) {
				newActiveMulti := &ActiveMultiEvent{events: events}
				res := newActiveMulti.String()
				require.Equal(t, test.result, res)
			})
		})
	}
}

func TestAggregateValues(t *testing.T) {
	// MaxUint64 + 10
	veryBig, _ := new(big.Int).SetString("18446744073709551715", 10)
	// MaxUint64 * 2
	veryVeryBig, _ := new(big.Int).SetString("36893488147419103230", 10)

	tests := []struct {
		name     string
		values   []CounterValue
		rRaw     *big.Int
		rEnabled *big.Int
		rRunning *big.Int
	}{
		{"small numbers", []CounterValue{{100, 100, 100}, {100, 100, 100}}, big.NewInt(200), big.NewInt(200), big.NewInt(200)},
		{"very big integer", []CounterValue{{math.MaxInt64 - 1, 0, 2316432}, {1, 100, 3358123}}, big.NewInt(math.MaxInt64), big.NewInt(100), big.NewInt(5674555)}, //nolint:lll // Keep format of the test cases
		{"more then max uint 64", []CounterValue{{math.MaxUint64, 100, 0}, {100, 100, 0}}, veryBig, big.NewInt(200), big.NewInt(0)},
		{"maximum", []CounterValue{{math.MaxUint64, 700, 0}, {math.MaxUint64, 100, 0}}, veryVeryBig, big.NewInt(800), big.NewInt(0)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw, enabled, running := AggregateValues(tt.values)
			require.Equal(t, tt.rRaw, raw)
			require.Equal(t, tt.rEnabled, enabled)
			require.Equal(t, tt.rRunning, running)
		})
	}
}
