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
	"strings"
	"syscall"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

// Unit tests need mocks to work properly.
// Please generate mocks by `make mock` or `make test` command.

func TestPerfEvent_NewPlacements(t *testing.T) {
	errMock := fmt.Errorf("mock error")
	maxCPU := 12
	cpuPMU := uint32(4)
	cpuName := "cpu"
	cboxName := "cbox"

	mCorePMUTypes := []NamedPMUType{{Name: "cpu", PMUType: cpuPMU}}
	mUncorePMUTypes := []NamedPMUType{
		{Name: "mcbox_1", PMUType: 8},
		{Name: "mcbox_2", PMUType: 9},
		{Name: "mcbox_3", PMUType: 10},
	}

	mPerfEvent, _, mMonitor := newPerfEventForTest()
	cpus := []int{0, 2, 3, 4, 5}

	t.Run("missing cpu monitor", func(t *testing.T) {
		emptyPerfEvent := &PerfEvent{}
		pp, err := emptyPerfEvent.NewPlacements("", cpus[0])
		require.Error(t, err)
		require.Nil(t, pp)
	})

	t.Run("cannot get max cpu number", func(t *testing.T) {
		mPerfEvent.PMUTypes = mCorePMUTypes
		mPerfEvent.PMUName = cpuName

		mMonitor.On("cpusNum").Return(-1, errMock).Once()

		pp, err := mPerfEvent.NewPlacements(cpuName, cpus[0])
		require.Error(t, err)
		require.Nil(t, pp)
		mMonitor.AssertExpectations(t)
	})

	t.Run("cpu exceeded", func(t *testing.T) {
		mPerfEvent.PMUTypes = mCorePMUTypes
		mPerfEvent.PMUName = cpuName

		mMonitor.On("cpusNum").Return(maxCPU, nil).Once()

		pp, err := mPerfEvent.NewPlacements(cpuName, maxCPU+1)
		require.Error(t, err)
		require.Nil(t, pp)
		mMonitor.AssertExpectations(t)
	})

	t.Run("pmu not available", func(t *testing.T) {
		mPerfEvent.PMUTypes = mCorePMUTypes
		mPerfEvent.PMUName = cpuName

		pp, err := mPerfEvent.NewPlacements(cboxName, cpus[0])
		require.Error(t, err)
		require.Nil(t, pp)
	})

	testCases := []struct {
		name          string
		eventPMUTypes []NamedPMUType
		eventPMUName  string
		argPMUName    string
		argCPUs       []int
		plcNumber     int
	}{
		{"specific pmu", mUncorePMUTypes, cboxName, mUncorePMUTypes[0].Name, cpus, len(cpus)},
		{"unit equal event pmu name", mUncorePMUTypes, cboxName, cboxName, cpus, len(mUncorePMUTypes) * len(cpus)},
		{"core cpu placement", mCorePMUTypes, cpuName, cpuName, cpus, len(cpus)},
		{"all pmus", mUncorePMUTypes, cboxName, "", cpus, len(cpus) * len(mUncorePMUTypes)},
	}
	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			var requiredPMUs []uint32
			for _, p := range tt.eventPMUTypes {
				requiredPMUs = append(requiredPMUs, p.PMUType)
			}
			mPerfEvent.PMUTypes = tt.eventPMUTypes
			mPerfEvent.PMUName = tt.eventPMUName

			mMonitor.On("cpusNum").Return(maxCPU, nil).Once()

			pp, err := mPerfEvent.NewPlacements(tt.argPMUName, tt.argCPUs[0], tt.argCPUs[1:]...)

			require.NoError(t, err)
			require.NotNil(t, pp)
			require.Len(t, pp, tt.plcNumber)
			mMonitor.AssertExpectations(t)

			for _, p := range pp {
				cpu, pmu := p.PMUPlacement()
				require.Contains(t, cpus, cpu)
				require.Contains(t, requiredPMUs, pmu)
			}
		})
	}
}

func TestPerfEvent_Activate(t *testing.T) {
	errMock := fmt.Errorf("mock error")
	mockTypeName := "mock"
	mockPMUType := uint32(123)
	mockNamedPMUType := NamedPMUType{mockTypeName, mockPMUType}
	mockPerfEventName := "mock_perf_event"
	mockPMUName := "mock_pmu_name"
	mockEventFormat := "mock_event_format"
	mockQualifiers := []string{"config=321", "config1=543"}

	mockPerfEvent := PerfEvent{Name: mockPerfEventName, PMUName: mockPMUName, eventFormat: mockEventFormat}

	emptyPlacement := &Placement{}
	emptyProcess := &EventTargetProcess{}
	emptyOptions := &PerfEventOptions{}

	t.Run("TestMissingPerfOpener", func(t *testing.T) {
		activeEvent, err := mockPerfEvent.Activate(emptyPlacement, emptyProcess, emptyOptions)
		require.Error(t, err)
		require.Nil(t, activeEvent)
	})

	mockUnix := &mockUnixHelper{}
	mockPerfEvent.unix = mockUnix

	t.Run("missing attribute", func(t *testing.T) {
		activeEvent, err := mockPerfEvent.Activate(emptyPlacement, emptyProcess, emptyOptions)
		require.Error(t, err)
		require.Nil(t, activeEvent)
	})

	attr := &unix.PerfEventAttr{}
	mockPerfEvent.Attr = attr

	t.Run("missing cpu monitor", func(t *testing.T) {
		activeEvent, err := mockPerfEvent.Activate(emptyPlacement, emptyProcess, emptyOptions)
		require.Error(t, err)
		require.Nil(t, activeEvent)
	})

	mockCPUMonitor := &mockCpuMonitor{}
	mockPerfEvent.cpuMonitor = mockCPUMonitor

	t.Run("missing placement provider", func(t *testing.T) {
		activeEvent, err := mockPerfEvent.Activate(nil, emptyProcess, emptyOptions)
		require.Error(t, err)
		require.Nil(t, activeEvent)
	})

	t.Run("failed to check if cpu is online", func(t *testing.T) {
		mockCPUMonitor.On("cpuOnline", emptyPlacement.CPU).Once().Return(false, errMock)
		activeEvent, err := mockPerfEvent.Activate(emptyPlacement, emptyProcess, emptyOptions)
		require.Error(t, err)
		require.Nil(t, activeEvent)
		mockCPUMonitor.AssertExpectations(t)
	})

	t.Run("cpu is offline", func(t *testing.T) {
		mockCPUMonitor.On("cpuOnline", emptyPlacement.CPU).Once().Return(false, nil)
		activeEvent, err := mockPerfEvent.Activate(emptyPlacement, emptyProcess, emptyOptions)
		require.Error(t, err)
		require.Nil(t, activeEvent)
		mockCPUMonitor.AssertExpectations(t)
	})

	mockCPUMonitor.On("cpuOnline", emptyPlacement.CPU).Return(true, nil)

	t.Run("PMU type is nil", func(t *testing.T) {
		activeEvent, err := mockPerfEvent.Activate(emptyPlacement, emptyProcess, emptyOptions)
		require.Error(t, err)
		require.Nil(t, activeEvent)
	})

	mockPerfEvent.PMUTypes = []NamedPMUType{mockNamedPMUType}

	t.Run("pmuType from placement is nil", func(t *testing.T) {
		activeEvent, err := mockPerfEvent.Activate(emptyPlacement, emptyProcess, emptyOptions)
		require.Error(t, err)
		require.Nil(t, activeEvent)
	})

	mockPlacement := &Placement{PMUType: 9}

	t.Run("no match", func(t *testing.T) {
		activeEvent, err := mockPerfEvent.Activate(mockPlacement, emptyProcess, emptyOptions)
		require.Error(t, err)
		require.Nil(t, activeEvent)
	})

	mockPlacement.PMUType = mockPMUType

	t.Run("missing options attribute", func(t *testing.T) {
		options := &PerfEventOptions{
			attr: nil,
		}
		activeEvent, err := mockPerfEvent.Activate(mockPlacement, emptyProcess, options)
		require.Error(t, err)
		require.Nil(t, activeEvent)
	})

	t.Run("enable on exec while reading all processes", func(t *testing.T) {
		mockOptions, err := NewOptions().SetEnableOnExec(true).Build()
		require.NoError(t, err)
		emptyProcess.pid = -1
		activeEvent, err := mockPerfEvent.Activate(mockPlacement, emptyProcess, mockOptions)
		require.Error(t, err)
		require.Nil(t, activeEvent)
	})

	mockOptions, err := NewOptions().SetAttrModifiers(mockQualifiers).Build()
	require.NoError(t, err)

	t.Run("missing target process", func(t *testing.T) {
		activeEvent, err := mockPerfEvent.Activate(mockPlacement, nil, mockOptions)
		require.Error(t, err)
		require.Nil(t, activeEvent)
	})

	t.Run("perf event open failed", func(t *testing.T) {
		expectedAttribute := prepareAttribute(*mockPerfEvent.Attr, mockOptions, mockPlacement)

		mockUnix.On("open", expectedAttribute, emptyProcess.pid, mockPlacement.CPU, -1, 0).Once().Return(-1, errMock)

		activeEvent, err := mockPerfEvent.Activate(mockPlacement, emptyProcess, mockOptions)
		require.Error(t, err)
		require.Nil(t, activeEvent)
		mockUnix.AssertExpectations(t)
	})

	t.Run("perf event open failed with einval", func(t *testing.T) {
		expectedAttribute := prepareAttribute(*mockPerfEvent.Attr, mockOptions, mockPlacement)

		mockUnix.On("open", expectedAttribute, emptyProcess.pid, mockPlacement.CPU, -1, 0).Once().Return(-1, syscall.EINVAL)

		activeEvent, err := mockPerfEvent.Activate(mockPlacement, emptyProcess, mockOptions)
		require.Error(t, err)
		require.Nil(t, activeEvent)
		mockUnix.AssertExpectations(t)
	})

	t.Run("perf event open succeeded", func(t *testing.T) {
		fileDescriptor := 2134
		expectedAttribute := prepareAttribute(*mockPerfEvent.Attr, mockOptions, mockPlacement)
		qualifiers := strings.Join(mockQualifiers, ",")
		decoded := fmt.Sprintf("%s/%s/%s", mockPMUName, mockEventFormat, qualifiers)

		t.Run("no flags", func(t *testing.T) {
			mockUnix.On("open", expectedAttribute, emptyProcess.pid, mockPlacement.CPU, -1, 0).Once().Return(fileDescriptor, nil)

			activeEvent, err := mockPerfEvent.Activate(mockPlacement, emptyProcess, mockOptions)
			require.NoError(t, err)
			require.NotNil(t, activeEvent)

			require.Equal(t, &mockPerfEvent, activeEvent.PerfEvent)
			require.Equal(t, fileDescriptor, activeEvent.FileDescriptor)
			require.Equal(t, mockPlacement.CPU, activeEvent.cpu)
			require.Equal(t, mockNamedPMUType, activeEvent.namedPMUType)
			require.Equal(t, decoded, activeEvent.Decoded)
			mockUnix.AssertExpectations(t)
		})

		t.Run("cgroup flag set", func(t *testing.T) {
			cGroupProcess := EventTargetProcess{pid: 111, processType: 1}
			require.NoError(t, err)
			mockUnix.On("open", expectedAttribute, cGroupProcess.pid, mockPlacement.CPU, -1, unix.PERF_FLAG_PID_CGROUP).
				Once().Return(fileDescriptor, nil)

			activeEvent, err := mockPerfEvent.Activate(mockPlacement, &cGroupProcess, mockOptions)
			require.NoError(t, err)
			require.NotNil(t, activeEvent)

			require.Equal(t, &mockPerfEvent, activeEvent.PerfEvent)
			require.Equal(t, fileDescriptor, activeEvent.FileDescriptor)
			require.Equal(t, mockPlacement.CPU, activeEvent.cpu)
			require.Equal(t, mockNamedPMUType, activeEvent.namedPMUType)
			require.Equal(t, decoded, activeEvent.Decoded)
			mockUnix.AssertExpectations(t)
		})

		t.Run("enable on exec set", func(t *testing.T) {
			emptyProcess.pid = 11111
			options, _ := NewOptions().SetEnableOnExec(true).Build()
			expectedAttribute := prepareAttribute(*mockPerfEvent.Attr, options, mockPlacement)
			decoded := fmt.Sprintf("%s/%s/", mockPMUName, mockEventFormat)

			require.NoError(t, err)
			mockUnix.On("open", expectedAttribute, emptyProcess.pid, mockPlacement.CPU, -1, 0).Once().Return(fileDescriptor, nil)

			activeEvent, err := mockPerfEvent.Activate(mockPlacement, emptyProcess, options)
			require.NoError(t, err)
			require.NotNil(t, activeEvent)

			require.Equal(t, &mockPerfEvent, activeEvent.PerfEvent)
			require.Equal(t, fileDescriptor, activeEvent.FileDescriptor)
			require.Equal(t, mockPlacement.CPU, activeEvent.cpu)
			require.Equal(t, mockNamedPMUType, activeEvent.namedPMUType)
			require.Equal(t, decoded, activeEvent.Decoded)
			mockUnix.AssertExpectations(t)
		})
	})
}

func TestActiveEvent_ReadValue(t *testing.T) {
	errMock := fmt.Errorf("mock error")
	mockActiveEvent, mUnix := newActiveEventForTest(nil, 0, 0)
	mockFileDescriptor := 123
	buffer := make([]byte, 3*8)
	typeOfBuffer := fmt.Sprintf("%T", &buffer)

	tests := []struct {
		testID      int
		rawByte     []byte
		enabledByte []byte
		runningByte []byte
		rawInt      uint64
		enabledInt  uint64
		runningInt  uint64
		shouldFail  bool
	}{
		{1, []byte{1, 1, 0, 0, 0, 0, 0, 0}, []byte{1, 0, 0, 0, 0, 0, 0, 0}, []byte{0, 0, 0, 0, 0, 0, 0, 0}, uint64(257), uint64(1), uint64(0), false},
		{2, []byte{125, 1, 0, 0, 0, 0, 0, 0}, []byte{1, 0, 0, 0, 0, 0, 0, 0}, []byte{255, 255, 255, 255, 255, 255, 255, 255}, uint64(381), uint64(1), uint64(18446744073709551615), false},
		{3, []byte{1, 1, 0, 0, 0, 0, 0}, []byte{1, 0, 0, 0, 0, 0, 0, 0}, []byte{0, 0, 0, 0, 0, 0, 0, 0}, 0, 0, 0, true},
		{4, []byte{1, 1, 0, 0, 0, 0, 0, 0}, []byte{1, 0, 0, 0, 0, 0}, []byte{0, 0, 0, 0, 0, 0, 0, 0}, 0, 0, 0, true},
		{5, []byte{1, 1, 0, 0, 0, 0, 0, 0, 1, 1, 1, 12, 13, 14, 15, 16, 15, 14}, []byte{1, 0, 0, 0, 0, 0, 0, 0}, []byte{0}, 0, 0, 0, true},
	}
	t.Run("reader is nil", func(t *testing.T) {
		emptyActiveEvent := &ActiveEvent{}
		values, err := emptyActiveEvent.ReadValue()
		require.Error(t, err)
		require.Equal(t, CounterValue{}, values)
	})

	mockActiveEvent.FileDescriptor = mockFileDescriptor

	t.Run("file descriptor reading error", func(t *testing.T) {
		mUnix.On("read", mockFileDescriptor, mock.AnythingOfType(typeOfBuffer)).Return(-1, errMock).Once()

		values, err := mockActiveEvent.ReadValue()
		require.Error(t, err)
		require.Equal(t, CounterValue{}, values)
		mUnix.AssertExpectations(t)
	})

	for _, tt := range tests {
		t.Run(fmt.Sprintf("read values test %d", tt.testID), func(t *testing.T) {
			buffer = append(tt.rawByte, append(tt.enabledByte, tt.runningByte...)...)

			mUnix.On("read", mockFileDescriptor, mock.AnythingOfType(typeOfBuffer)).Return(0, nil).
				Run(func(args mock.Arguments) {
					arg := args.Get(1).(*[]byte)
					*arg = buffer
				}).Once()

			values, err := mockActiveEvent.ReadValue()
			if tt.shouldFail {
				require.Error(t, err)
				require.Equal(t, CounterValue{}, values)
			} else {
				require.NoError(t, err)
				require.NotNil(t, values)
				require.Equal(t, tt.rawInt, values.Raw)
				require.Equal(t, tt.enabledInt, values.Enabled)
				require.Equal(t, tt.runningInt, values.Running)
			}
			mUnix.AssertExpectations(t)
		})
	}
}

func TestReadActiveEventValues(t *testing.T) {
	// most cases covered in TestActiveEvent_ReadValue
	t.Run("unix helper is nil", func(t *testing.T) {
		result, err := readActiveEventValues(nil, 0)
		require.Error(t, err)
		require.Empty(t, result)
	})
}

func TestByteArrayToUint64(t *testing.T) {
	// most cases covered in TestActiveEvent_ReadValue
	t.Run("byte array does not have length equal 8", func(t *testing.T) {
		result, err := byteArrayToUint64([]byte{1, 0, 1, 0})
		require.Error(t, err)
		require.Empty(t, result)
	})
}

func TestActiveEvent_Deactivate(t *testing.T) {
	errMock := fmt.Errorf("mock error")
	mockFd := 111
	mockActiveEvent, mUnix := newActiveEventForTest(nil, mockFd, 0)

	t.Run("missing closer", func(t *testing.T) {
		emptyActiveEvent := &ActiveEvent{}
		err := emptyActiveEvent.Deactivate()
		require.Error(t, err)
	})

	t.Run("closer return error", func(t *testing.T) {
		mUnix.On("close", mockFd).Return(errMock).Once()
		err := mockActiveEvent.Deactivate()
		require.Error(t, err)
		mUnix.AssertExpectations(t)
	})

	t.Run("deactivate succeeded", func(t *testing.T) {
		mUnix.On("close", mockFd).Return(nil).Once()
		err := mockActiveEvent.Deactivate()
		require.NoError(t, err)
		require.Equal(t, -1, mockActiveEvent.FileDescriptor)
		mUnix.AssertExpectations(t)
	})
}

func TestDeactivateFile(t *testing.T) {
	// most cases covered in TestActiveEvent_Deactivate
	t.Run("unix helper is nil", func(t *testing.T) {
		err := deactivateFile(nil, 0)
		require.Error(t, err)
	})
}

func TestActivateGroup(t *testing.T) {
	errMock := fmt.Errorf("mock error")
	groupLength := 5
	mCPU := 12
	mPID := -1

	type eventWithMocks struct {
		event   CustomizableEvent
		unix    *mockUnixHelper
		monitor *mockCpuMonitor
		fd      int
	}

	mockEvents := []eventWithMocks{}
	for i := 0; i < groupLength; i++ {
		newEvent, newOpener, newMonitor := newPerfEventForTest()
		newTest := eventWithMocks{CustomizableEvent{newEvent, nil}, newOpener, newMonitor, i + 1}
		mockEvents = append(mockEvents, newTest)
	}
	mockPMUType := uint32(123)
	mockPlacement := &Placement{CPU: mCPU, PMUType: mockPMUType}
	mockProcess := &EventTargetProcess{pid: mPID, processType: 0}
	var events []CustomizableEvent

	mLeader := mockEvents[0]
	events = append(events, mLeader.event)
	for _, mEvent := range mockEvents[1:] {
		events = append(events, mEvent.event)
	}

	t.Run("no events", func(t *testing.T) {
		group, err := ActivateGroup(mockPlacement, mockProcess, nil)
		require.Error(t, err)
		require.Nil(t, group)
	})

	t.Run("leader no attrs", func(t *testing.T) {
		emptyLeader := CustomizableEvent{&PerfEvent{}, nil}
		group, err := ActivateGroup(mockPlacement, mockProcess, []CustomizableEvent{emptyLeader})
		require.Error(t, err)
		require.Nil(t, group)
	})

	t.Run("leader activation failed", func(t *testing.T) {
		mLeader.unix.On("open", mock.Anything, mPID, mCPU, -1, 0).Return(-1, errMock).Once()
		mLeader.monitor.On("cpuOnline", mCPU).Return(true, nil).Once()

		group, err := ActivateGroup(mockPlacement, mockProcess, events)
		require.Error(t, err)
		require.Nil(t, group)
		mLeader.unix.AssertExpectations(t)
		mLeader.monitor.AssertExpectations(t)
	})

	t.Run("leader reset failed", func(t *testing.T) {
		mLeader.unix.On("open", mock.Anything, mPID, mCPU, -1, 0).Return(mLeader.fd, nil).Once()
		mLeader.monitor.On("cpuOnline", mCPU).Return(true, nil).Once()
		mLeader.unix.On("setInt", mLeader.fd, uint(unix.PERF_EVENT_IOC_RESET), 0).Return(errMock).Once()

		events := []CustomizableEvent{mLeader.event}
		group, err := ActivateGroup(mockPlacement, mockProcess, events)

		require.Error(t, err)
		require.Nil(t, group)
		mLeader.unix.AssertExpectations(t)
		mLeader.monitor.AssertExpectations(t)
	})

	t.Run("leader enable failed", func(t *testing.T) {
		mLeader.unix.On("open", mock.Anything, mPID, mCPU, -1, 0).Return(mLeader.fd, nil).Once()
		mLeader.monitor.On("cpuOnline", mCPU).Return(true, nil).Once()

		mLeader.unix.On("setInt", mLeader.fd, uint(unix.PERF_EVENT_IOC_RESET), 0).Return(nil).Once()
		mLeader.unix.On("setInt", mLeader.fd, uint(unix.PERF_EVENT_IOC_ENABLE), 0).Return(errMock).Once()

		events := []CustomizableEvent{mLeader.event}
		group, err := ActivateGroup(mockPlacement, mockProcess, events)

		require.Error(t, err)
		require.Nil(t, group)
		mLeader.unix.AssertExpectations(t)
		mLeader.monitor.AssertExpectations(t)
	})

	t.Run("activation succeeded", func(t *testing.T) {
		mLeader.unix.On("open", mock.Anything, mPID, mCPU, -1, 0).Return(mLeader.fd, nil).Once()
		mLeader.monitor.On("cpuOnline", mCPU).Return(true, nil).Once()

		for _, mEvent := range mockEvents[1:] {
			mEvent.unix.On("open", mock.Anything, -1, mCPU, mLeader.fd, 0).Return(mEvent.fd, nil).Once()
			mEvent.monitor.On("cpuOnline", mCPU).Return(true, nil).Once()
		}

		mLeader.unix.On("setInt", mLeader.fd, uint(unix.PERF_EVENT_IOC_RESET), 0).Return(nil).Once()
		mLeader.unix.On("setInt", mLeader.fd, uint(unix.PERF_EVENT_IOC_ENABLE), 0).Return(nil).Once()

		group, err := ActivateGroup(mockPlacement, mockProcess, events)
		require.NoError(t, err)
		require.NotNil(t, group)
		resultEvents := group.Events()
		require.Len(t, resultEvents, groupLength)

		mLeader.unix.AssertExpectations(t)
		mLeader.monitor.AssertExpectations(t)
		for _, mEvent := range mockEvents {
			mEvent.unix.AssertExpectations(t)
			mEvent.monitor.AssertExpectations(t)
		}
	})

	t.Run("activation failed", func(t *testing.T) {
		mLeader.unix.On("open", mock.Anything, mPID, mCPU, -1, 0).Return(mLeader.fd, nil).Once()
		mLeader.monitor.On("cpuOnline", mCPU).Return(true, nil).Once()
		mLeader.unix.On("close", mLeader.fd).Return(nil).Once()

		for _, mEvent := range mockEvents[1 : len(mockEvents)-1] {
			mEvent.unix.On("open", mock.Anything, -1, mCPU, mLeader.fd, 0).Return(mEvent.fd, nil).Once()
			mEvent.monitor.On("cpuOnline", mCPU).Return(true, nil).Once()
			mLeader.unix.On("close", mEvent.fd).Return(nil).Once()
		}

		mockEvents[len(mockEvents)-1].monitor.On("cpuOnline", mCPU).Return(true, nil).Once()
		mockEvents[len(mockEvents)-1].unix.On("open", mock.Anything, mPID, mCPU, mLeader.fd, 0).Return(-1, errMock).Once()

		group, err := ActivateGroup(mockPlacement, mockProcess, events)
		require.Error(t, err)
		require.Nil(t, group)

		mLeader.unix.AssertExpectations(t)
		mLeader.monitor.AssertExpectations(t)
		for _, mEvent := range mockEvents {
			mEvent.unix.AssertExpectations(t)
			mEvent.monitor.AssertExpectations(t)
		}
	})
}

func TestPerfEvent_ActivateMulti(t *testing.T) {
	errMock := fmt.Errorf("mock error")
	mockPerfEvent, mUnix, mMonitor := newPerfEventForTest()
	mockPlacements := []PlacementProvider{
		&Placement{CPU: 0, PMUType: uint32(0)},
		&Placement{CPU: 0, PMUType: uint32(1)},
		&Placement{CPU: 0, PMUType: uint32(2)},
		&Placement{CPU: 0, PMUType: uint32(3)},
		&Placement{CPU: 0, PMUType: uint32(4)},
	}
	mockPerfEvent.PMUTypes = []NamedPMUType{
		{Name: "mock_box_0", PMUType: uint32(0)},
		{Name: "mock_box_0", PMUType: uint32(1)},
		{Name: "mock_box_0", PMUType: uint32(2)},
		{Name: "mock_box_0", PMUType: uint32(3)},
		{Name: "mock_box_0", PMUType: uint32(4)},
		{Name: "mock_box_1", PMUType: uint32(0)},
		{Name: "mock_box_1", PMUType: uint32(1)},
	}

	mockProcess := &EventTargetProcess{pid: -1, processType: 0}
	mockOptions := &PerfEventOptions{perfStatString: []string{""}, attr: &unix.PerfEventAttr{}}

	t.Run("multi event activation succeeded", func(t *testing.T) {
		mUnix.On("open", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(123, nil).Times(len(mockPlacements))
		mMonitor.On("cpuOnline", mock.Anything).Return(true, nil)

		result, err := mockPerfEvent.ActivateMulti(mockPlacements, mockProcess, mockOptions)
		require.NoError(t, err)
		require.NotNil(t, result)
		require.Len(t, result.events, len(mockPlacements))
		mUnix.AssertExpectations(t)
		mMonitor.AssertExpectations(t)
	})

	t.Run("activation failed", func(t *testing.T) {
		mUnix.On("open", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(-1, errMock).Once()

		mMonitor.On("cpuOnline", mock.Anything).Return(true, nil)

		result, err := mockPerfEvent.ActivateMulti(mockPlacements, mockProcess, mockOptions)
		require.Error(t, err)
		require.Nil(t, result)
		mUnix.AssertExpectations(t)
		mMonitor.AssertExpectations(t)
	})

	t.Run("activation failed for not all placements", func(t *testing.T) {
		mUnix.On("open", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(123, nil).Times(len(mockPlacements) - 1)
		mUnix.On("open", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(-1, errMock).Once()
		mMonitor.On("cpuOnline", mock.Anything).Return(true, nil)

		result, err := mockPerfEvent.ActivateMulti(mockPlacements, mockProcess, mockOptions)
		require.Error(t, err)
		require.Nil(t, result)
		mUnix.AssertExpectations(t)
		mMonitor.AssertExpectations(t)
	})
}

func TestActiveEvent_PMUPlacement(t *testing.T) {
	mActiveEvent := &ActiveEvent{}
	mCPU := 10
	mNamedPMU := NamedPMUType{Name: "mock", PMUType: 32}

	mActiveEvent.cpu = mCPU
	mActiveEvent.namedPMUType = mNamedPMU

	cpu, pmu := mActiveEvent.PMUPlacement()
	require.Equal(t, mCPU, cpu)
	require.Equal(t, mNamedPMU.PMUType, pmu)
}

func TestActiveEvent_PMUName(t *testing.T) {
	mActiveEvent := &ActiveEvent{}
	mNamedPMU := NamedPMUType{Name: "mock", PMUType: 32}

	mActiveEvent.namedPMUType = mNamedPMU

	pmu := mActiveEvent.PMUName()
	require.Equal(t, mNamedPMU.Name, pmu)
}

func TestResetEventCounter(t *testing.T) {
	mUnix := &mockUnixHelper{}

	t.Run("unix is nil", func(t *testing.T) {
		err := resetEventCounter(0, nil)
		require.Error(t, err)
	})

	t.Run("setting succeeded", func(t *testing.T) {
		fd := 111
		mUnix.On("setInt", fd, uint(unix.PERF_EVENT_IOC_RESET), 0).Return(fmt.Errorf("mock error")).Once()
		err := resetEventCounter(fd, mUnix)
		require.Error(t, err)
		mUnix.AssertExpectations(t)
	})

	t.Run("setting failed", func(t *testing.T) {
		fd := 111
		mUnix.On("setInt", fd, uint(unix.PERF_EVENT_IOC_RESET), 0).Return(nil).Once()
		err := resetEventCounter(fd, mUnix)
		require.NoError(t, err)
		mUnix.AssertExpectations(t)
	})
}

func TestEnableEventCounter(t *testing.T) {
	mUnix := &mockUnixHelper{}

	t.Run("unix is nil", func(t *testing.T) {
		err := enableEventCounter(0, nil)
		require.Error(t, err)
	})

	t.Run("setting succeeded", func(t *testing.T) {
		fd := 111
		mUnix.On("setInt", fd, uint(unix.PERF_EVENT_IOC_ENABLE), 0).Return(fmt.Errorf("mock error")).Once()
		err := enableEventCounter(fd, mUnix)
		require.Error(t, err)
		mUnix.AssertExpectations(t)
	})

	t.Run("setting failed", func(t *testing.T) {
		fd := 111
		mUnix.On("setInt", fd, uint(unix.PERF_EVENT_IOC_ENABLE), 0).Return(nil).Once()
		err := enableEventCounter(fd, mUnix)
		require.NoError(t, err)
		mUnix.AssertExpectations(t)
	})
}

func newPerfEventForTest() (*PerfEvent, *mockUnixHelper, *mockCpuMonitor) {
	mockPMUType := uint32(123)
	mockNamedPMUType := NamedPMUType{"mock pmu name", mockPMUType}
	mOpener := &mockUnixHelper{}
	mMonitor := &mockCpuMonitor{}

	e := &PerfEvent{
		Name:        "mock perf event",
		Uncore:      false,
		PMUName:     "mock pmu name",
		PMUTypes:    []NamedPMUType{mockNamedPMUType},
		Attr:        &unix.PerfEventAttr{},
		eventFormat: "mock format",
		unix:        mOpener,
		cpuMonitor:  mMonitor,
	}
	return e, mOpener, mMonitor
}

func newActiveEventForTest(perf *PerfEvent, fd int, cpu int) (*ActiveEvent, *mockUnixHelper) {
	mUnix := &mockUnixHelper{}

	ae := &ActiveEvent{
		PerfEvent:      perf,
		FileDescriptor: fd,
		Decoded:        "",
		cpu:            cpu,
		namedPMUType:   NamedPMUType{},
		unix:           mUnix,
		read:           readActiveEventValues,
	}
	return ae, mUnix
}

func prepareAttribute(attr unix.PerfEventAttr, options Options, provider PlacementProvider) *unix.PerfEventAttr {
	_, attr.Type = provider.PMUPlacement()

	optionsAttr := options.Attr()

	attr.Config |= optionsAttr.Config
	attr.Ext1 |= optionsAttr.Ext1
	attr.Ext2 |= optionsAttr.Ext2
	attr.Bits |= optionsAttr.Bits

	attr.Bits |= unix.PerfBitInherit
	attr.Read_format |= unix.PERF_FORMAT_TOTAL_TIME_ENABLED | unix.PERF_FORMAT_TOTAL_TIME_RUNNING

	return &attr
}
