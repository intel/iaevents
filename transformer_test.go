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
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

// Unit tests need mocks to work properly.
// Please generate mocks by `make mock` or `make test` command.

func TestPerfTransformer_Transform(t *testing.T) {
	testJSONEvents := []*JSONEvent{{EventName: "EVENT1"}, {EventName: "EVENT2"}}
	readerMock := &MockReader{}
	matcherMock := &MockMatcher{}
	transformer, _, utilsMock, parserMock := newTransformerForTest()
	errMock := errors.New("mock error")

	t.Run("Return error if reader or matcher is nil", func(t *testing.T) {
		_, err := transformer.Transform(nil, matcherMock)
		require.Error(t, err)

		_, err = transformer.Transform(readerMock, nil)
		require.Error(t, err)

		_, err = transformer.Transform(nil, nil)
		require.Error(t, err)
	})
	t.Run("Return error if reader failed to read the events", func(t *testing.T) {
		readerMock.On("Read").Return(nil, errMock).Once()
		_, err := transformer.Transform(readerMock, matcherMock)
		require.Error(t, err)
		readerMock.AssertExpectations(t)
	})
	t.Run("Return error if reader returned no events", func(t *testing.T) {
		readerMock.On("Read").Return(nil, nil).Once()
		_, err := transformer.Transform(readerMock, matcherMock)
		require.Error(t, err)
		readerMock.AssertExpectations(t)
	})
	t.Run("Ignore nil event from reader", func(t *testing.T) {
		jsonNilEvent := []*JSONEvent{nil}
		readerMock.On("Read").Return(jsonNilEvent, nil).Once()
		events, err := transformer.Transform(readerMock, matcherMock)
		require.NoError(t, err)
		readerMock.AssertExpectations(t)
		require.Equal(t, 0, len(events))
	})
	t.Run("Return error if matcher failed", func(t *testing.T) {
		readerMock.On("Read").Return(testJSONEvents, nil).Once()
		matcherMock.On("Match", testJSONEvents[0]).Return(false, errMock).Once()
		_, err := transformer.Transform(readerMock, matcherMock)
		require.Error(t, err)
		readerMock.AssertExpectations(t)
		matcherMock.AssertExpectations(t)
	})
	t.Run("Return error if parse JSON function failed", func(t *testing.T) {
		errMock1 := errors.New("mock error 1")
		errMock2 := errors.New("mock error 2")
		expectedErrMsg := fmt.Sprintf("failed to parse json event `%s`: %s, failed to parse json event `%s`: %s",
			testJSONEvents[0].EventName, errMock1.Error(), testJSONEvents[1].EventName, errMock2.Error())

		readerMock.On("Read").Return(testJSONEvents, nil).Once()
		matcherMock.On("Match", testJSONEvents[0]).Return(true, nil).Once()
		parserMock.On("Execute", testJSONEvents[0]).Return(nil, errMock1).Once()
		matcherMock.On("Match", testJSONEvents[1]).Return(true, nil).Once()
		parserMock.On("Execute", testJSONEvents[1]).Return(nil, errMock2).Once()

		perfEvents, err := transformer.Transform(readerMock, matcherMock)
		require.Error(t, err)
		require.Nil(t, perfEvents)
		require.IsType(t, &TransformationError{}, err)
		require.Equal(t, expectedErrMsg, err.Error())
		readerMock.AssertExpectations(t)
		matcherMock.AssertExpectations(t)
		parserMock.AssertExpectations(t)
	})
	t.Run("Return error if failed to resolve event", func(t *testing.T) {
		testDefinition1 := &eventDefinition{Name: "EVENT1"}
		testDefinition2 := &eventDefinition{Name: "EVENT2"}

		expectedErrMsg := fmt.Sprintf("failed to transform event `%s`: %s, failed to transform event `%s`: %s",
			testDefinition1.Name, errMock.Error(), testDefinition2.Name, errMock.Error())

		readerMock.On("Read").Return(testJSONEvents, nil).Once()
		matcherMock.On("Match", testJSONEvents[0]).Return(true, nil).Once().
			On("Match", testJSONEvents[1]).Return(true, nil).Once()

		parserMock.On("Execute", testJSONEvents[0]).Return(testDefinition1, nil).Once().
			On("Execute", testJSONEvents[1]).Return(testDefinition2, nil).Once()

		utilsMock.On("getPMUType", mock.Anything).Return(uint32(2), nil).Twice().
			On("isPMUUncore", mock.Anything).Return(false, errMock).Twice()

		perfEvents, err := transformer.Transform(readerMock, matcherMock)
		require.Error(t, err)
		require.Nil(t, perfEvents)
		require.IsType(t, &TransformationError{}, err)
		require.Equal(t, expectedErrMsg, err.Error())

		readerMock.AssertExpectations(t)
		matcherMock.AssertExpectations(t)
		parserMock.AssertExpectations(t)
		utilsMock.AssertExpectations(t)
	})
	t.Run("Return resolved perf events and proper error message with those unresolved", func(t *testing.T) {
		errMock1, errMock2 := errors.New("mock error 1"), errors.New("mock error 2")
		testJSONEvents := []*JSONEvent{{EventName: "EVENT1"}, {EventName: "EVENT2"}, {EventName: "EVENT3"}}
		testDefinition1, testDefinition2 := &eventDefinition{Name: "EVENT2"}, &eventDefinition{Name: "EVENT3"}

		expectedErrMsg := fmt.Sprintf("failed to parse json event `%s`: %s, failed to transform event `%s`: %s",
			testJSONEvents[0].EventName, errMock1.Error(), testJSONEvents[1].EventName, errMock2.Error())

		readerMock.On("Read").Return(testJSONEvents, nil).Once()

		// mock event 1 - parse JSON function failed
		matcherMock.On("Match", testJSONEvents[0]).Return(true, nil).Once()
		parserMock.On("Execute", testJSONEvents[0]).Return(nil, errMock1).Once()

		// mock event 2 - failed to resolve event
		matcherMock.On("Match", testJSONEvents[1]).Return(true, nil).Once()
		parserMock.On("Execute", testJSONEvents[1]).Return(testDefinition1, nil).Once()
		utilsMock.On("getPMUType", mock.Anything).Return(uint32(2), nil).Once().
			On("isPMUUncore", mock.Anything).Return(false, errMock2).Once()

		// mock event 3 - successful transformation
		matcherMock.On("Match", testJSONEvents[2]).Return(true, nil).Once()
		parserMock.On("Execute", testJSONEvents[2]).Return(testDefinition2, nil).Once()
		utilsMock.On("getPMUType", mock.Anything).Return(uint32(2), nil).Once().
			On("isPMUUncore", mock.Anything).Return(false, nil).Once().
			On("parseTerms", mock.Anything, mock.Anything).Return(nil).Once()

		perfEvents, err := transformer.Transform(readerMock, matcherMock)
		require.Error(t, err)
		require.NotNil(t, perfEvents)
		require.Equal(t, testJSONEvents[2].EventName, perfEvents[0].Name)
		require.IsType(t, &TransformationError{}, err)
		require.Equal(t, expectedErrMsg, err.Error())

		readerMock.AssertExpectations(t)
		matcherMock.AssertExpectations(t)
		parserMock.AssertExpectations(t)
		utilsMock.AssertExpectations(t)
	})
	t.Run("Return resolved perf events for events matched by matcher", func(t *testing.T) {
		testDefinition := &eventDefinition{Name: "EVENT2"}
		readerMock.On("Read").Return(testJSONEvents, nil).Once()
		matcherMock.On("Match", testJSONEvents[0]).Return(false, nil).Once().
			On("Match", testJSONEvents[1]).Return(true, nil).Once()
		parserMock.On("Execute", testJSONEvents[1]).Return(testDefinition, nil).Once()
		utilsMock.On("getPMUType", mock.Anything).Return(uint32(2), nil).Once().
			On("isPMUUncore", mock.Anything).Return(false, nil).Once().
			On("parseTerms", mock.Anything, mock.Anything).Return(nil).Once()
		events, err := transformer.Transform(readerMock, matcherMock)
		require.NoError(t, err)
		readerMock.AssertExpectations(t)
		matcherMock.AssertExpectations(t)
		parserMock.AssertExpectations(t)
		utilsMock.AssertExpectations(t)
		require.Equal(t, 1, len(events))
		require.Equal(t, testDefinition.Name, events[0].Name)
	})
}

func TestPerfTransformer_resolve(t *testing.T) {
	transformer, ioMock, utilsMock, _ := newTransformerForTest()
	errMock := errors.New("mock error")

	t.Run("Return error if event data is nil", func(t *testing.T) {
		_, err := transformer.resolve(nil)
		require.Error(t, err)
	})
	t.Run("Return error if failed to find PMU", func(t *testing.T) {
		testDefinition := &eventDefinition{PMU: "test"}
		utilsMock.On("getPMUType", mock.Anything).Return(uint32(0), errMock).Twice()
		ioMock.On("glob", mock.Anything).Return(nil, errMock).Once()
		_, err := transformer.resolve(testDefinition)
		require.Error(t, err)
		utilsMock.AssertExpectations(t)
		ioMock.AssertExpectations(t)
	})
	t.Run("Return error if uncore check failed", func(t *testing.T) {
		testDefinition := &eventDefinition{PMU: "test"}
		utilsMock.On("getPMUType", mock.Anything).Return(uint32(2), nil).Once().
			On("isPMUUncore", "test").Return(false, errMock).Once()
		_, err := transformer.resolve(testDefinition)
		require.Error(t, err)
		utilsMock.AssertExpectations(t)
	})
	t.Run("Return error if failed to parse event kernel terms", func(t *testing.T) {
		testDefinition := &eventDefinition{Event: "term=0xc", PMU: "test"}
		utilsMock.On("getPMUType", mock.Anything).Return(uint32(2), nil).Once().
			On("isPMUUncore", "test").Return(false, nil).Once().
			On("parseTerms", mock.Anything, "term=0xc").Return(errMock).Once()
		_, err := transformer.resolve(testDefinition)
		require.Error(t, err)
		utilsMock.AssertExpectations(t)
	})
	t.Run("Create new perf event based on provided event definition", func(t *testing.T) {
		testDefinition := &eventDefinition{Name: "TEST_EVENT", Event: "term=0xc", PMU: "test"}
		utilsMock.On("getPMUType", mock.Anything).Return(uint32(2), nil).Once().
			On("isPMUUncore", "test").Return(true, nil).Once().
			On("parseTerms", mock.Anything, "term=0xc").Return(nil).Once()
		event, err := transformer.resolve(testDefinition)
		require.NoError(t, err)
		utilsMock.AssertExpectations(t)
		require.Equal(t, testDefinition.Name, event.Name)
		require.Equal(t, testDefinition.PMU, event.PMUName)
		require.Equal(t, testDefinition.Event, event.eventFormat)
		require.Equal(t, true, event.Uncore)
		require.EqualValues(t, 2, event.Attr.Type)
		require.Equal(t, 1, len(event.PMUTypes))
		require.Equal(t, testDefinition.PMU, event.PMUTypes[0].Name)
		require.EqualValues(t, 2, event.PMUTypes[0].PMUType)
	})
}

func TestPerfTransformer_findPMUs(t *testing.T) {
	transformer, ioMock, utilsMock, _ := newTransformerForTest()
	errMock := errors.New("mock error")

	t.Run("Find Type for single event", func(t *testing.T) {
		utilsMock.On("getPMUType", "test").Return(uint32(2), nil).Once()
		types, err := transformer.findPMUs("test")
		require.NoError(t, err)
		utilsMock.AssertExpectations(t)
		require.Equal(t, 1, len(types))
		require.Equal(t, "test", types[0].Name)
		require.EqualValues(t, 2, types[0].PMUType)
	})
	t.Run("Find Type for single uncore event", func(t *testing.T) {
		utilsMock.On("getPMUType", "test").Return(uint32(0), errMock).Once().
			On("getPMUType", "uncore_test").Return(uint32(7), nil).Once()
		types, err := transformer.findPMUs("test")
		require.NoError(t, err)
		utilsMock.AssertExpectations(t)
		require.Equal(t, 1, len(types))
		require.Equal(t, "uncore_test", types[0].Name)
		require.EqualValues(t, 7, types[0].PMUType)
	})
	t.Run("Return error if no single events and glob failed", func(t *testing.T) {
		utilsMock.On("getPMUType", "test").Return(uint32(0), errMock).Once().
			On("getPMUType", "uncore_test").Return(uint32(0), errMock).Once()
		ioMock.On("glob", mock.Anything).Return(nil, errMock).Once()
		_, err := transformer.findPMUs("test")
		require.Error(t, err)
		utilsMock.AssertExpectations(t)
		ioMock.AssertExpectations(t)
	})
	t.Run("Return error if glob return no data", func(t *testing.T) {
		utilsMock.On("getPMUType", "test").Return(uint32(0), errMock).Once().
			On("getPMUType", "uncore_test").Return(uint32(0), errMock).Once()
		ioMock.On("glob", mock.Anything).Return(nil, nil).Once()
		_, err := transformer.findPMUs("test")
		require.Error(t, err)
		utilsMock.AssertExpectations(t)
		ioMock.AssertExpectations(t)
	})
	t.Run("Find Types for multi event", func(t *testing.T) {
		utilsMock.On("getPMUType", "test").Return(uint32(0), errMock).Once().
			On("getPMUType", "uncore_test").Return(uint32(0), errMock).Once().
			On("getPMUType", "test_1").Return(uint32(5), nil).Once().
			On("getPMUType", "test_2").Return(uint32(6), nil).Once()
		ioMock.On("glob", mock.Anything).Return([]string{"test_1", "test_2"}, nil).Once()
		types, err := transformer.findPMUs("test")
		require.NoError(t, err)
		utilsMock.AssertExpectations(t)
		ioMock.AssertExpectations(t)
		require.Equal(t, 2, len(types))
		require.Equal(t, "test_1", types[0].Name)
		require.EqualValues(t, 5, types[0].PMUType)
		require.Equal(t, "test_2", types[1].Name)
		require.EqualValues(t, 6, types[1].PMUType)
	})
	t.Run("Return error if failed to find PMU type for event", func(t *testing.T) {
		utilsMock.On("getPMUType", "test").Return(uint32(0), errMock).Once().
			On("getPMUType", "uncore_test").Return(uint32(0), errMock).Once().
			On("getPMUType", "test_1").Return(uint32(0), errMock).Once()
		ioMock.On("glob", mock.Anything).Return([]string{"test_1", "test_2"}, nil).Once()
		_, err := transformer.findPMUs("test")
		require.Error(t, err)
		utilsMock.AssertExpectations(t)
		ioMock.AssertExpectations(t)
	})
	t.Run("Return error if failed to find PMU type for one of multi event", func(t *testing.T) {
		utilsMock.On("getPMUType", "test").Return(uint32(0), errMock).Once().
			On("getPMUType", "uncore_test").Return(uint32(0), errMock).Once().
			On("getPMUType", "test_1").Return(uint32(5), nil).Once().
			On("getPMUType", "test_2").Return(uint32(0), errMock).Once()
		ioMock.On("glob", mock.Anything).Return([]string{"test_1", "test_2"}, nil).Once()
		_, err := transformer.findPMUs("test")
		require.Error(t, err)
		utilsMock.AssertExpectations(t)
		ioMock.AssertExpectations(t)
	})
}

func TestPerfTransformer_findMultiPMUPaths(t *testing.T) {
	transformer, ioMock, _, _ := newTransformerForTest()
	errMock := errors.New("mock error")
	testPMU := "test"
	pattern := fmt.Sprintf("%suncore_%s_[0-9]*", sysDevicesPath, testPMU)
	t.Run("Return error if glob failed", func(t *testing.T) {
		ioMock.On("glob", pattern).Return(nil, errMock).Once()
		_, err := transformer.findMultiPMUPaths(testPMU)
		require.Error(t, err)
		ioMock.AssertExpectations(t)
	})
	t.Run("Return empty slice if glob did not return any paths", func(t *testing.T) {
		ioMock.On("glob", pattern).Return([]string{}, nil).Once()
		paths, err := transformer.findMultiPMUPaths(testPMU)
		require.NoError(t, err)
		ioMock.AssertExpectations(t)
		require.Equal(t, 0, len(paths))
	})
	t.Run("Ignore paths that extend the PMU name", func(t *testing.T) {
		globPaths := []string{
			fmt.Sprintf("%suncore_%s_0_alias_0", sysDevicesPath, testPMU),
			fmt.Sprintf("%suncore_%s_0_alias_1", sysDevicesPath, testPMU),
			fmt.Sprintf("%suncore_%s_1E2", sysDevicesPath, testPMU),
			fmt.Sprintf("%suncore_%s_4free", sysDevicesPath, testPMU),
		}
		ioMock.On("glob", pattern).Return(globPaths, nil).Once()
		paths, err := transformer.findMultiPMUPaths(testPMU)
		require.NoError(t, err)
		ioMock.AssertExpectations(t)
		require.Equal(t, 0, len(paths))
	})
	t.Run("Filter out paths with more than a decimal number in suffix", func(t *testing.T) {
		globPaths := []string{
			fmt.Sprintf("%suncore_%s_0", sysDevicesPath, testPMU),
			fmt.Sprintf("%suncore_%s_10", sysDevicesPath, testPMU),
			fmt.Sprintf("%suncore_%s_1E", sysDevicesPath, testPMU),
			fmt.Sprintf("%suncore_%s_4_free", sysDevicesPath, testPMU),
			fmt.Sprintf("%suncore_%s_26786784321321232121424078965474467253", sysDevicesPath, testPMU),
		}
		filteredPaths := []string{
			fmt.Sprintf("%suncore_%s_0", sysDevicesPath, testPMU),
			fmt.Sprintf("%suncore_%s_10", sysDevicesPath, testPMU),
			fmt.Sprintf("%suncore_%s_26786784321321232121424078965474467253", sysDevicesPath, testPMU),
		}
		ioMock.On("glob", pattern).Return(globPaths, nil).Once()
		paths, err := transformer.findMultiPMUPaths(testPMU)
		require.NoError(t, err)
		ioMock.AssertExpectations(t)
		require.EqualValues(t, paths, filteredPaths)
	})
}

func TestEntryNotZero(t *testing.T) {
	tests := []struct {
		name   string
		arg    string
		result bool
	}{
		{"Entry is empty", "", false},
		{"Entry is zero", "0", false},
		{"Entry is zero hex 1", "0x0", false},
		{"Entry is zero hex 2", "0x00", false},
		{"Entry is letter", "A", true},
		{"Entry is number", "1", true},
		{"Entry is alphanumeric", "a1", true},
		{"Entry is hex", "0x1", true},
		{"Entry has two parts", "0x1,0x0", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.result, entryNotZero(tt.arg))
		})
	}
}

func TestValidateEntry(t *testing.T) {
	type args struct {
		entry string
		name  string
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{"Valid entry 1", args{"test", "Test"}, false},
		{"Empty entry", args{"", "Unit"}, false},
		{"Empty entry and name", args{"", ""}, false},
		{"Valid entry 2", args{"iMC", ".*/_"}, false},
		{"Valid entry 3", args{"imc", "Unit"}, false},
		{"Valid entry 4", args{"IMC", "UNIT"}, false},
		{"Slash in entry", args{"imc/", "test"}, true},
		{"Path in entry", args{"/folder/test", "test"}, true},
		{"Dots and slash in entry 1", args{"../test", "test"}, true},
		{"Dots and slash in entry 2", args{"../../secret/test", "test"}, true},
		{"Dots and slash in entry 3", args{"..../test", "test"}, true},
		{"Dots and slash in entry 4", args{"../..../test", "test"}, true},
		{"Encoded path in entry", args{"\x2e\x2e/test", "test"}, true},
		{"Encoded path in entry 2", args{"\x2e\x2e\x2ftest", "test"}, true},
		{"Entry is asterix", args{"*", "test"}, true},
		{"Asterix in entry", args{"IMC*", "test"}, true},
		{"Entry is dot", args{".", "test"}, true},
		{"Entry is too long", args{"veryveryveryveryveryveryveryveryveryveryveryveryveryveryveryveryveryveryveryveryveryveryveryveryveryveryveryveryveryveryverylongentry", "test"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.wantErr, validateEntry(tt.args.entry, tt.args.name) != nil)
		})
	}
}

func Test_appendQualifiers(t *testing.T) {
	type args struct {
		event      string
		qualifiers []string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{"Event with slash", args{"test/", []string{"QAL1"}}, "test/QAL1"},
		{"Event without slash", args{"test", []string{"QAL1"}}, "test/QAL1"},
		{"Complex event with single qalifier", args{"first=1,second=2", []string{"QAL1"}}, "first=1,second=2/QAL1"},
		{"Event with slash and two qualifiers", args{"test/", []string{"QAL1", "QAL2"}}, "test/QAL1,QAL2"},
		{"Event without slash with four qualifiers", args{"test", []string{"QAL1", "QAL2", "qal3", "Qal4"}}, "test/QAL1,QAL2,qal3,Qal4"},
		{"Event and zero qualifiers", args{"test", []string{}}, "test"},
		{"Event and nil qualifiers", args{"test", nil}, "test"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := appendQualifiers(tt.args.event, tt.args.qualifiers); got != tt.want {
				t.Errorf("appendQualifiers() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_parseJSONEventImpl(t *testing.T) {
	t.Run("Return error on empty JSONEvent", func(t *testing.T) {
		_, err := parseJSONEventImpl(&JSONEvent{})
		require.Error(t, err)
	})
	t.Run("Parse jsonEvent, return filled event definition", func(t *testing.T) {
		jsonEvent := &JSONEvent{
			AnyThread:        "1",
			BriefDescription: "Short description.",
			CounterMask:      "0x2",
			Deprecated:       "0",
			EdgeDetect:       "1",
			EventCode:        "0xC6,1F",
			EventName:        "FRONTEND_RETIRED.L2_MISS",
			ExtSel:           "0x12",
			Invert:           "1",
			MSRIndex:         "0x3F7",
			MSRValue:         "0x13",
			SampleAfterValue: "100007",
			UMask:            "0x01",
			Unit:             "",
		}
		event, err := parseJSONEventImpl(jsonEvent)
		require.NoError(t, err)
		require.Equal(t, "FRONTEND_RETIRED.L2_MISS", event.Name)
		require.Equal(t, "Short description.", event.Desc)
		require.Equal(t, "umask=0x01,cmask=0x2,inv=1,any=1,edge=1,period=100007,event=0x24000c6,frontend=0x13", event.Event)
		require.Equal(t, "cpu", event.PMU)
		require.Equal(t, false, event.Deprecated)
	})
	t.Run("Parse deprecated field", func(t *testing.T) {
		jsonEvent := &JSONEvent{
			Deprecated: "1",
			EventCode:  "0xC6",
		}
		event, err := parseJSONEventImpl(jsonEvent)
		require.NoError(t, err)
		require.Equal(t, true, event.Deprecated)
		jsonEvent.Deprecated = ""
		event, err = parseJSONEventImpl(jsonEvent)
		require.NoError(t, err)
		require.Equal(t, false, event.Deprecated)
	})
	t.Run("Parse single EventCode field", func(t *testing.T) {
		jsonEvent := &JSONEvent{
			EventCode: "0xC6",
		}
		event, err := parseJSONEventImpl(jsonEvent)
		require.NoError(t, err)
		require.Equal(t, "event=0xc6", event.Event)
	})
	t.Run("Return error if EventCode is invalid", func(t *testing.T) {
		jsonEvent := &JSONEvent{
			EventCode: "0xG",
		}
		_, err := parseJSONEventImpl(jsonEvent)
		require.Error(t, err)
	})
	t.Run("Return error if ExtSel is invalid", func(t *testing.T) {
		jsonEvent := &JSONEvent{
			EventCode: "0xC6",
			ExtSel:    "G",
		}
		_, err := parseJSONEventImpl(jsonEvent)
		require.Error(t, err)
	})
	t.Run("Return error on not allowed Unit value", func(t *testing.T) {
		jsonEvent := &JSONEvent{
			EventCode: "0xC6",
			Unit:      "../cpu",
		}
		_, err := parseJSONEventImpl(jsonEvent)
		require.Error(t, err)
	})
	t.Run("Parse Unit field", func(t *testing.T) {
		jsonEvent := &JSONEvent{
			EventCode: "0xC6",
			Unit:      "CBO",
		}
		event, err := parseJSONEventImpl(jsonEvent)
		require.NoError(t, err)
		require.Equal(t, "cbox", event.PMU)
		jsonEvent.Unit = "TEST"
		event, err = parseJSONEventImpl(jsonEvent)
		require.NoError(t, err)
		require.Equal(t, "test", event.PMU)
	})
	t.Run("Handle fixed counter encodings", func(t *testing.T) {
		jsonEvent := &JSONEvent{
			EventCode: "0xC6",
			EventName: "INST_RETIRED.ANY",
		}
		event, err := parseJSONEventImpl(jsonEvent)
		require.NoError(t, err)
		require.Equal(t, "event=0xc0", event.Event)
	})
	t.Run("Ignore unknown MSR", func(t *testing.T) {
		jsonEvent := &JSONEvent{
			EventCode: "0xC6",
			EventName: "FRONTEND_RETIRED.L2_MISS",
			MSRIndex:  "0xFFF",
			MSRValue:  "0x13",
		}
		event, err := parseJSONEventImpl(jsonEvent)
		require.NoError(t, err)
		require.Equal(t, "event=0xc6", event.Event)
	})
}

func Test_parseBitFormat(t *testing.T) {
	type args struct {
		bits string
	}
	tests := []struct {
		name        string
		args        args
		wantMask    uint64
		wantStart   uint64
		wantBitsNum uint64
		wantErr     bool
	}{
		{"Single bit 0", args{"0"}, 1, 0, 1, false},
		{"Single bit 2", args{"2"}, 1, 2, 1, false},
		{"Single bit 63", args{"63"}, 1, 63, 1, false},
		{"Range 3", args{"0-2"}, 0b111, 0, 3, false},
		{"Range 4", args{"60-63"}, 0b1111, 60, 4, false},
		{"Range 16", args{"11-26"}, 0xffff, 11, 16, false},
		{"Max range", args{"0-63"}, 0xffffffffffffffff, 0, 64, false},
		{"Wrong bit", args{"h"}, 0, 0, 0, true},
		{"No start bit", args{"-2"}, 0, 0, 0, true},
		{"Wrong start bit", args{"bad-10"}, 0, 0, 0, true},
		{"Wrong end bit", args{"0-g"}, 0, 0, 0, true},
		{"Range too big", args{"0-64"}, 0, 0, 0, true},
		{"Start bigger than end", args{"3-2"}, 0, 0, 0, true},
		{"Wrong format", args{"2-4,5"}, 0, 0, 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotMask, gotStart, gotBitsNum, err := parseBitFormat(tt.args.bits)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseBitFormat() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if gotMask != tt.wantMask {
				t.Errorf("parseBitFormat() gotMask = %v, want %v", gotMask, tt.wantMask)
			}
			if gotStart != tt.wantStart {
				t.Errorf("parseBitFormat() gotStart = %v, want %v", gotStart, tt.wantStart)
			}
			if gotBitsNum != tt.wantBitsNum {
				t.Errorf("parseBitFormat() gotBitsNum = %v, want %v", gotBitsNum, tt.wantBitsNum)
			}
		})
	}
}

func Test_resolverUtilsImpl_formatValue(t *testing.T) {
	type args struct {
		format string
		value  uint64
	}
	tests := []struct {
		name    string
		args    args
		want    uint64
		wantErr bool
	}{
		{"Format 1 bit", args{"8-15", 0x1}, 1 << 8, false},
		{"Format 8 bits", args{"4-12", 0xff}, 0xFF << 4, false},
		{"Format on combined bits", args{"0-2,5", 0b1110}, 0b100110, false},
		{"Invalid bits format", args{"t-e-s-t", 0xff}, 0, true},
	}
	utils, _ := newResolverUtilsForTest()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := utils.formatValue(tt.args.format, tt.args.value)
			if (err != nil) != tt.wantErr {
				t.Errorf("resolverUtilsImpl.formatValue() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("resolverUtilsImpl.formatValue() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_splitConfigFormat(t *testing.T) {
	type args struct {
		configFormat string
	}
	tests := []struct {
		name           string
		args           args
		wantConfigName string
		wantFormat     string
		wantErr        bool
	}{
		{"Correct format range", args{"config:2-4"}, "config", "2-4", false},
		{"Correct format list", args{"test:0,2-3,21"}, "test", "0,2-3,21", false},
		{"Wrong separator", args{"config,2-4"}, "", "", true},
		{"No config name", args{"2-4"}, "", "", true},
		{"No config bits", args{"test"}, "", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotConfigName, gotFormat, err := splitConfigFormat(tt.args.configFormat)
			if (err != nil) != tt.wantErr {
				t.Errorf("splitConfigFormat() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if gotConfigName != tt.wantConfigName {
				t.Errorf("splitConfigFormat() gotConfigName = %v, want %v", gotConfigName, tt.wantConfigName)
			}
			if gotFormat != tt.wantFormat {
				t.Errorf("splitConfigFormat() gotFormat = %v, want %v", gotFormat, tt.wantFormat)
			}
		})
	}
}

func Test_resolverUtilsImpl_getPMUType(t *testing.T) {
	const testPMU = "test_pmu"
	const testPMUType uint32 = 13
	testPath := filepath.Join(sysDevicesPath, testPMU, "type")
	utils, ioMock := newResolverUtilsForTest()
	errMock := errors.New("mock error")

	t.Run("Return error on scanOneValue failure", func(t *testing.T) {
		ioMock.On("scanOneValue", testPath, mock.Anything, mock.Anything).Return(false, 1, errMock).Once()
		_, err := utils.getPMUType(testPMU)
		require.Error(t, err)
		ioMock.AssertExpectations(t)
	})
	t.Run("Return error if number of scanned values is 0", func(t *testing.T) {
		ioMock.On("scanOneValue", testPath, mock.Anything, mock.Anything).Return(true, 0, nil).Once()
		_, err := utils.getPMUType(testPMU)
		require.Error(t, err)
		ioMock.AssertExpectations(t)
	})
	t.Run("Return PMU type obtained from file", func(t *testing.T) {
		ioMock.On("scanOneValue", testPath, mock.Anything, mock.Anything).Return(true, 1, nil).
			Run(func(args mock.Arguments) {
				arg := args.Get(2).(*uint32)
				*arg = testPMUType
			}).Once()
		num, err := utils.getPMUType(testPMU)
		require.NoError(t, err)
		ioMock.AssertExpectations(t)
		require.Equal(t, testPMUType, num)
	})
}

func Test_resolverUtilsImpl_isPMUUncore(t *testing.T) {
	const testPMU = "test_pmu"
	testPath := filepath.Join(sysDevicesPath, testPMU, "cpumask")
	utils, ioMock := newResolverUtilsForTest()
	errMock := errors.New("mock error")

	t.Run("Return false if no file with cpumask", func(t *testing.T) {
		ioMock.On("validateFile", testPath).Return(os.ErrNotExist).Once()
		uncore, err := utils.isPMUUncore(testPMU)
		require.NoError(t, err)
		ioMock.AssertExpectations(t)
		require.Equal(t, false, uncore)
	})
	t.Run("Return error if file with cpumask is invalid", func(t *testing.T) {
		ioMock.On("validateFile", testPath).Return(errMock).Once()
		_, err := utils.isPMUUncore(testPMU)
		require.Error(t, err)
		ioMock.AssertExpectations(t)
	})
	t.Run("Return error on scanOneValue failure while opening the file", func(t *testing.T) {
		ioMock.On("validateFile", testPath).Return(nil).Once().
			On("scanOneValue", testPath, mock.Anything, mock.Anything).Return(false, 0, errMock).Once()
		_, err := utils.isPMUUncore(testPMU)
		require.Error(t, err)
		ioMock.AssertExpectations(t)
	})
	t.Run("Return false without error on scanOneValue failure while scanning", func(t *testing.T) {
		ioMock.On("validateFile", testPath).Return(nil).Once().
			On("scanOneValue", testPath, mock.Anything, mock.Anything).Return(true, 0, errMock).Once()
		uncore, err := utils.isPMUUncore(testPMU)
		require.NoError(t, err)
		ioMock.AssertExpectations(t)
		require.Equal(t, false, uncore)
	})
	t.Run("Return false if cpumask is not zero", func(t *testing.T) {
		ioMock.On("validateFile", testPath).Return(nil).Once().
			On("scanOneValue", testPath, mock.Anything, mock.Anything).Return(true, 1, nil).
			Run(func(args mock.Arguments) {
				arg := args.Get(2).(*int)
				*arg = 1
			}).Once()
		uncore, err := utils.isPMUUncore(testPMU)
		require.NoError(t, err)
		ioMock.AssertExpectations(t)
		require.Equal(t, false, uncore)
	})
	t.Run("Return true if cpumask is zero", func(t *testing.T) {
		ioMock.On("validateFile", testPath).Return(nil).Once().
			On("scanOneValue", testPath, mock.Anything, mock.Anything).Return(true, 1, nil).
			Run(func(args mock.Arguments) {
				arg := args.Get(2).(*int)
				*arg = 0
			}).Once()
		uncore, err := utils.isPMUUncore(testPMU)
		require.NoError(t, err)
		ioMock.AssertExpectations(t)
		require.Equal(t, true, uncore)
	})
}

func Test_resolverUtilsImpl_parseTerms(t *testing.T) {
	testType := NamedPMUType{Name: "type", PMUType: 2}
	utils, ioMock := newResolverUtilsForTest()
	errMock := errors.New("mock error")

	t.Run("Return error if perf event is invalid", func(t *testing.T) {
		testEvent := &PerfEvent{}
		err := utils.parseTerms(testEvent, "wrongTerm")
		require.Error(t, err)
		err = utils.parseTerms(nil, "testTerm=1")
		require.Error(t, err)
	})
	t.Run("Return error if format is incorrect", func(t *testing.T) {
		testEvent := &PerfEvent{Attr: &unix.PerfEventAttr{}}
		err := utils.parseTerms(testEvent, "wrongTerm")
		require.Error(t, err)
	})
	t.Run("Return error if failed to read kernel config format for term", func(t *testing.T) {
		testEvent := &PerfEvent{Attr: &unix.PerfEventAttr{}}
		testEvent.PMUTypes = append(testEvent.PMUTypes, testType)
		ioMock.On("readAll", mock.Anything).Return([]byte(""), errMock).Once()
		err := utils.parseTerms(testEvent, "term=1")
		require.Error(t, err)
		ioMock.AssertExpectations(t)
	})
	t.Run("Parse special attribute", func(t *testing.T) {
		testEvent := &PerfEvent{Attr: &unix.PerfEventAttr{}}
		err := utils.parseTerms(testEvent, "period=0x3f")
		require.NoError(t, err)
		require.EqualValues(t, 0x3f, testEvent.Attr.Sample)
		require.EqualValues(t, 0, testEvent.Attr.Bits)
	})
	t.Run("Parse single term", func(t *testing.T) {
		testEvent := &PerfEvent{Attr: &unix.PerfEventAttr{}}
		testEvent.PMUTypes = append(testEvent.PMUTypes, testType)
		ioMock.On("readAll", mock.Anything).Return([]byte("config:0-3"), nil).Once()
		err := utils.parseTerms(testEvent, "test=0xf")
		require.NoError(t, err)
		ioMock.AssertExpectations(t)
		require.EqualValues(t, 0xf, testEvent.Attr.Config)
	})
	t.Run("Parse multiple terms", func(t *testing.T) {
		testEvent := &PerfEvent{Attr: &unix.PerfEventAttr{}}
		testEvent.PMUTypes = append(testEvent.PMUTypes, testType)
		ioMock.On("readAll", mock.Anything).Return([]byte("config:0-3"), nil).Once()
		err := utils.parseTerms(testEvent, "test=0xf,period=0x3f")
		require.NoError(t, err)
		ioMock.AssertExpectations(t)
		require.EqualValues(t, 0xf, testEvent.Attr.Config)
		require.EqualValues(t, 0x3f, testEvent.Attr.Sample)
		require.EqualValues(t, 0, testEvent.Attr.Bits)
	})
}

func Test_resolverUtilsImpl_specialAttributes(t *testing.T) {
	utils, _ := newResolverUtilsForTest()

	t.Run("Ignore unknown attribute and return false", func(t *testing.T) {
		attr := &unix.PerfEventAttr{}
		require.Equal(t, false, utils.specialAttributes(attr, "test", 0))
		require.Equal(t, uint64(0), attr.Sample)
		require.Equal(t, uint64(0), attr.Bits)
	})
	t.Run("Parse special attribute for period", func(t *testing.T) {
		attr := &unix.PerfEventAttr{}
		require.Equal(t, true, utils.specialAttributes(attr, "period", 0x1234))
		require.Equal(t, uint64(0x1234), attr.Sample)
		require.Equal(t, uint64(0), attr.Bits)
	})
	t.Run("Parse special attribute for frequency", func(t *testing.T) {
		attr := &unix.PerfEventAttr{Bits: 0x8}
		require.Equal(t, true, utils.specialAttributes(attr, "freq", 0x1234))
		require.Equal(t, uint64(0x1234), attr.Sample)
		require.Equal(t, uint64(unix.PerfBitFreq|0x8), attr.Bits)
	})
}

func Test_resolverUtilsImpl_parseTerm(t *testing.T) {
	testEvent := &PerfEvent{Attr: &unix.PerfEventAttr{}}
	testTerm := &eventTerm{name: "term", value: 0xf}
	testType := NamedPMUType{Name: "type", PMUType: 2}
	testEvent.PMUTypes = append(testEvent.PMUTypes, testType)
	testPath := filepath.Join(sysDevicesPath, testType.Name, "format", testTerm.name)
	utils, ioMock := newResolverUtilsForTest()
	errMock := errors.New("mock error")

	t.Run("Return error if failed to read the format from file", func(t *testing.T) {
		testPath2 := "/sys/devices/type/format/term"
		ioMock.On("readAll", testPath2).Return([]byte(""), errMock).Once()
		err := utils.parseTerm(testEvent, testTerm)
		require.Error(t, err)
		ioMock.AssertExpectations(t)
	})
	t.Run("Return error if format is not valid", func(t *testing.T) {
		ioMock.On("readAll", testPath).Return([]byte("1"), nil).Once()
		err := utils.parseTerm(testEvent, testTerm)
		require.Error(t, err)
		ioMock.AssertExpectations(t)
	})
	t.Run("Return error if bits in format are not valid", func(t *testing.T) {
		ioMock.On("readAll", testPath).Return([]byte("test:abc"), nil).Once()
		err := utils.parseTerm(testEvent, testTerm)
		require.Error(t, err)
		ioMock.AssertExpectations(t)
	})
	t.Run("Return error if config name is not supported", func(t *testing.T) {
		ioMock.On("readAll", testPath).Return([]byte("test:0-3"), nil).Once()
		err := utils.parseTerm(testEvent, testTerm)
		require.Error(t, err)
		ioMock.AssertExpectations(t)
	})
	t.Run("Set value in attributes according to format", func(t *testing.T) {
		ioMock.On("readAll", testPath).Return([]byte("config:0-3"), nil).Once()
		err := utils.parseTerm(testEvent, testTerm)
		require.NoError(t, err)
		ioMock.AssertExpectations(t)
		require.EqualValues(t, testTerm.value, testEvent.Attr.Config)
		require.EqualValues(t, 0, testEvent.Attr.Ext1)
		require.EqualValues(t, 0, testEvent.Attr.Ext2)

		ioMock.On("readAll", testPath).Return([]byte("config1:0-3"), nil).Once()
		err = utils.parseTerm(testEvent, testTerm)
		require.NoError(t, err)
		ioMock.AssertExpectations(t)
		require.EqualValues(t, testTerm.value, testEvent.Attr.Ext1)
		require.EqualValues(t, 0, testEvent.Attr.Ext2)

		ioMock.On("readAll", testPath).Return([]byte("config2:0-3"), nil).Once()
		err = utils.parseTerm(testEvent, testTerm)
		require.NoError(t, err)
		ioMock.AssertExpectations(t)
		require.EqualValues(t, testTerm.value, testEvent.Attr.Ext2)
		require.EqualValues(t, unix.PERF_ATTR_SIZE_VER1, testEvent.Attr.Size)
	})
}

func Test_resolverUtilsImpl_splitEventTerms(t *testing.T) {
	type args struct {
		terms string
	}
	tests := []struct {
		name    string
		args    args
		want    []*eventTerm
		wantErr bool
	}{
		{"Empty string", args{""}, nil, true},
		{"No value for term", args{"testTerm"}, nil, true},
		{"Wrong value", args{"testTerm=bad"}, nil, true},
		{"Single term", args{"testTerm=1"}, []*eventTerm{{name: "testTerm", value: 1}}, false},
		{"Multiple terms", args{"testTerm=1,other=2"}, []*eventTerm{{name: "testTerm", value: 1}, {name: "other", value: 2}}, false},
	}
	utils, _ := newResolverUtilsForTest()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := utils.splitEventTerms(tt.args.terms)
			if (err != nil) != tt.wantErr {
				t.Errorf("resolverUtilsImpl.splitEventTerms() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("resolverUtilsImpl.splitEventTerms() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNewPerfTransformer(t *testing.T) {
	transformer := NewPerfTransformer()
	require.NotNil(t, transformer)
	require.NotNil(t, transformer.io)
	require.NotNil(t, transformer.utils)
	require.NotNil(t, transformer.parseJSONEvent)
}

func newResolverUtilsForTest() (*resolverUtilsImpl, *mockIoHelper) {
	io := &mockIoHelper{}
	return &resolverUtilsImpl{io: io}, io
}

func newTransformerForTest() (*PerfTransformer, *mockIoHelper, *mockResolverUtils, *mockParseJSONEventFunc) {
	io := &mockIoHelper{}
	utils := &mockResolverUtils{}
	jsonParser := &mockParseJSONEventFunc{}
	transformer := &PerfTransformer{
		io:             io,
		utils:          utils,
		parseJSONEvent: jsonParser.Execute,
	}
	return transformer, io, utils, jsonParser
}
