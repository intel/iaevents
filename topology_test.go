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
	"os"
	"sort"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// Unit tests need mocks to work properly.
// Please generate mocks by `make mock` or `make test` command.

func Test_cpuAssignerImpl_assignCPUToSocket(t *testing.T) {
	ioMock := &mockIoHelper{}
	assignerMock := &cpuAssignerImpl{helper: ioMock}
	errMock := errors.New("mock error")
	t.Run("Return error if ioHelper is nil", func(t *testing.T) {
		assigner := &cpuAssignerImpl{}
		_, err := assigner.assignCPUToSocket()
		require.Error(t, err)
	})
	t.Run("Return error if glob failed", func(t *testing.T) {
		ioMock.On("glob", "/sys/devices/system/cpu/cpu*/topology/physical_package_id").Return(nil, errMock).Once()
		_, err := assignerMock.assignCPUToSocket()
		require.Error(t, err)
		ioMock.AssertExpectations(t)
	})
	t.Run("Empty map if no results from glob", func(t *testing.T) {
		expected := make(map[int]int)
		ioMock.On("glob", "/sys/devices/system/cpu/cpu*/topology/physical_package_id").Return(nil, nil).Once()
		socketCPUs, err := assignerMock.assignCPUToSocket()
		require.NoError(t, err)
		ioMock.AssertExpectations(t)
		require.Equal(t, expected, socketCPUs)
	})
	t.Run("Return error if cpu number has wrong format", func(t *testing.T) {
		globResult := []string{"/sys/devices/system/cpu/cpuTest/topology/physical_package_id"}
		ioMock.On("glob", "/sys/devices/system/cpu/cpu*/topology/physical_package_id").Return(globResult, nil).Once()
		_, err := assignerMock.assignCPUToSocket()
		require.Error(t, err)
		ioMock.AssertExpectations(t)
	})
	t.Run("Return error if failed to read physical_package_id", func(t *testing.T) {
		globResult := []string{"/sys/devices/system/cpu/cpu0/topology/physical_package_id"}
		ioMock.On("glob", "/sys/devices/system/cpu/cpu*/topology/physical_package_id").Return(globResult, nil).Once()
		ioMock.On("readAll", "/sys/devices/system/cpu/cpu0/topology/physical_package_id").Return(nil, errMock).Once()
		_, err := assignerMock.assignCPUToSocket()
		require.Error(t, err)
		ioMock.AssertExpectations(t)
	})
	t.Run("Return error if physical_package_id has wrong format", func(t *testing.T) {
		globResult := []string{"/sys/devices/system/cpu/cpu0/topology/physical_package_id"}
		ioMock.On("glob", "/sys/devices/system/cpu/cpu*/topology/physical_package_id").Return(globResult, nil).Once()
		ioMock.On("readAll", "/sys/devices/system/cpu/cpu0/topology/physical_package_id").Return([]byte("Test"), nil).Once()
		_, err := assignerMock.assignCPUToSocket()
		require.Error(t, err)
		ioMock.AssertExpectations(t)
	})
	t.Run("Correct result with map of socket CPUs", func(t *testing.T) {
		expected := map[int]int{0: 0, 1: 2}
		globResult := []string{"/sys/devices/system/cpu/cpu0/topology/physical_package_id",
			"/sys/devices/system/cpu/cpu1/topology/physical_package_id",
			"/sys/devices/system/cpu/cpu3/topology/physical_package_id",
			"/sys/devices/system/cpu/cpu2/topology/physical_package_id"}
		ioMock.On("glob", "/sys/devices/system/cpu/cpu*/topology/physical_package_id").Return(globResult, nil).Once()
		ioMock.On("readAll", "/sys/devices/system/cpu/cpu0/topology/physical_package_id").Return([]byte("0"), nil).Once().
			On("readAll", "/sys/devices/system/cpu/cpu1/topology/physical_package_id").Return([]byte("0"), nil).Once().
			On("readAll", "/sys/devices/system/cpu/cpu3/topology/physical_package_id").Return([]byte("1"), nil).Once().
			On("readAll", "/sys/devices/system/cpu/cpu2/topology/physical_package_id").Return([]byte("1"), nil).Once()
		socketCPUs, err := assignerMock.assignCPUToSocket()
		require.NoError(t, err)
		ioMock.AssertExpectations(t)
		require.Equal(t, expected, socketCPUs)
	})
}

func Test_cpuIDProviderImpl_idFromCPUInfo(t *testing.T) {
	ioMock := &mockIoHelper{}
	providerMock := &cpuIDProviderImpl{helper: ioMock}
	errMock := errors.New("mock error")
	testPath := "/testPath"
	t.Run("Error if io helper is nil", func(t *testing.T) {
		provider := &cpuIDProviderImpl{}
		_, err := provider.idFromCPUInfo(testPath)
		require.Error(t, err)
	})
	t.Run("Error if failed to open file", func(t *testing.T) {
		ioMock.On("open", testPath).Return(nil, errMock).Once()
		_, err := providerMock.idFromCPUInfo(testPath)
		require.Error(t, err)
		ioMock.AssertExpectations(t)
	})
	t.Run("Error if failed to read from file", func(t *testing.T) {
		// No positive case scenario to avoid real io operations in test.
		// The reading logic for positive cases is tested in `Test_scanCPUid`.
		// Note: the nil for *os.File still provides correct interface that will
		// return error on Read() or Close() without panic.
		fileMock := (*os.File)(nil)
		ioMock.On("open", testPath).Return(fileMock, nil).Once()
		_, err := providerMock.idFromCPUInfo(testPath)
		require.Error(t, err)
		ioMock.AssertExpectations(t)
	})
}

func Test_scanCPUid(t *testing.T) {
	t.Run("Return error if io reader is nil", func(t *testing.T) {
		_, err := scanCPUid(nil)
		require.Error(t, err)
	})
	t.Run("Return error if read returned error", func(t *testing.T) {
		errMock := errors.New("mock error")
		_, err := scanCPUid(iotest.ErrReader(errMock))
		require.Error(t, err)
	})
	t.Run("Empty struct if no data has been read", func(t *testing.T) {
		readerMock := strings.NewReader("")
		cpuid, err := scanCPUid(readerMock)
		require.NoError(t, err)
		require.Equal(t, &cpuID{}, cpuid)
	})
	t.Run("Read partial data", func(t *testing.T) {
		readerMock := strings.NewReader("vendor_id       : Test\n")
		cpuid, err := scanCPUid(readerMock)
		require.NoError(t, err)
		require.Equal(t, &cpuID{vendor: "Test"}, cpuid)
	})
	t.Run("Return error if number format is wrong", func(t *testing.T) {
		readerMock := strings.NewReader("model : NotANumber\n")
		_, err := scanCPUid(readerMock)
		require.Error(t, err)
	})
	t.Run("Fill cpuID with correct data", func(t *testing.T) {
		vendor := "vendor_id  : Test\n"
		family := "cpu family  : 6\n"
		model := "model        : 85\n"
		modelName := "model name : Test Model\n"
		stepping := "stepping : 4\n"
		readerMock := strings.NewReader(vendor + family + model + modelName + stepping)
		expected := &cpuID{
			vendor:        "Test",
			model:         85,
			family:        6,
			stepping:      4,
			steppingIsSet: true,
		}
		cpuid, err := scanCPUid(readerMock)
		require.NoError(t, err)
		require.Equal(t, expected, cpuid)
	})
}

func Test_cpuID_Full(t *testing.T) {
	type fields struct {
		vendor        string
		model         uint64
		family        uint64
		stepping      uint64
		steppingIsSet bool
	}
	tests := []struct {
		name   string
		fields fields
		want   string
	}{
		{"With stepping", fields{"Test", 5, 3, 2, true}, "Test-3-5-2"},
		{"Without stepping", fields{"Test", 15, 3, 0, false}, "Test-3-F"},
		{"Stepping is zero", fields{"Test", 15, 3, 0, true}, "Test-3-F-0"},
		{"Default data without stepping", fields{"", 0, 0, 0, false}, "-0-0"},
		{"Default data with stepping", fields{"", 0, 0, 0, true}, "-0-0-0"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cid := &cpuID{
				vendor:        tt.fields.vendor,
				model:         tt.fields.model,
				family:        tt.fields.family,
				stepping:      tt.fields.stepping,
				steppingIsSet: tt.fields.steppingIsSet,
			}
			if got := cid.Full(); got != tt.want {
				t.Errorf("cpuID.Full() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_cpuID_Short(t *testing.T) {
	type fields struct {
		vendor        string
		model         uint64
		family        uint64
		stepping      uint64
		steppingIsSet bool
	}
	tests := []struct {
		name   string
		fields fields
		want   string
	}{
		{"Do not print stepping", fields{"Test", 5, 3, 2, true}, "Test-3-5"},
		{"Print as hex number", fields{"Test", 15, 3, 0, false}, "Test-3-F"},
		{"Default data", fields{"", 0, 0, 0, false}, "-0-0"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cid := &cpuID{
				vendor:        tt.fields.vendor,
				model:         tt.fields.model,
				family:        tt.fields.family,
				stepping:      tt.fields.stepping,
				steppingIsSet: tt.fields.steppingIsSet,
			}
			if got := cid.Short(); got != tt.want {
				t.Errorf("cpuID.Short() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_readCPUid(t *testing.T) {
	providerMock := &mockCpuIDProvider{}
	errMock := errors.New("mock error")
	t.Run("Return error if ioHelper is nil", func(t *testing.T) {
		_, err := readCPUid(nil)
		require.Error(t, err)
	})
	t.Run("Return error if failed to get id from CPU info", func(t *testing.T) {
		providerMock.On("idFromCPUInfo", mock.Anything).Return(nil, errMock).Once()
		_, err := readCPUid(providerMock)
		require.Error(t, err)
		providerMock.AssertExpectations(t)
	})
	t.Run("Return correct cpuID", func(t *testing.T) {
		testID := &cpuID{
			vendor:        "test",
			model:         0x4f,
			family:        2,
			stepping:      1,
			steppingIsSet: true,
		}
		providerMock.On("idFromCPUInfo", mock.Anything).Return(testID, nil).Once()
		cpuid, err := readCPUid(providerMock)
		require.NoError(t, err)
		providerMock.AssertExpectations(t)
		require.Equal(t, testID, cpuid)
	})
}

func Test_newCPUAssigner(t *testing.T) {
	cpuAssigner := newCPUAssigner()
	require.NotNil(t, cpuAssigner)
	require.NotNil(t, cpuAssigner.helper)
}

func Test_newCPUidProvider(t *testing.T) {
	cpuProvider := newCPUidProvider()
	require.NotNil(t, cpuProvider)
	require.NotNil(t, cpuProvider.helper)
}

func Test_newCPUMonitor(t *testing.T) {
	cpuMonitor := newCPUMonitor()
	require.NotNil(t, cpuMonitor)
	require.NotNil(t, cpuMonitor.helper)
}

func Test_cpuMonitorImpl_cpuOnline(t *testing.T) {
	ioMock := &mockIoHelper{}
	errMock := errors.New("mock error")
	monitorMock := &cpuMonitorImpl{helper: ioMock}
	t.Run("Return error if ioHelper is nil", func(t *testing.T) {
		cpuMonitor := &cpuMonitorImpl{}
		res, err := cpuMonitor.cpuOnline(0)
		require.Error(t, err)
		require.Equal(t, false, res)
	})
	t.Run("Return true if file does not exists", func(t *testing.T) {
		ioMock.On("validateFile", "/sys/devices/system/cpu/cpu0/online").Return(os.ErrNotExist).Once()
		res, err := monitorMock.cpuOnline(0)
		require.NoError(t, err)
		ioMock.AssertExpectations(t)
		require.Equal(t, true, res)
	})
	t.Run("Return error if failed to validate file", func(t *testing.T) {
		ioMock.On("validateFile", "/sys/devices/system/cpu/cpu0/online").Return(errMock).Once()
		res, err := monitorMock.cpuOnline(0)
		require.Error(t, err)
		ioMock.AssertExpectations(t)
		require.Equal(t, false, res)
	})
	t.Run("Return false if failed to read file", func(t *testing.T) {
		ioMock.On("validateFile", "/sys/devices/system/cpu/cpu0/online").Return(nil).Once()
		ioMock.On("readAll", "/sys/devices/system/cpu/cpu0/online").Return(nil, errMock).Once()
		res, err := monitorMock.cpuOnline(0)
		require.Error(t, err)
		ioMock.AssertExpectations(t)
		require.Equal(t, false, res)
	})
	t.Run("Return false if failed to scan value from file content", func(t *testing.T) {
		ioMock.On("validateFile", "/sys/devices/system/cpu/cpu0/online").Return(nil).Once()
		ioMock.On("readAll", "/sys/devices/system/cpu/cpu0/online").Return([]byte(""), nil).Once()
		res, err := monitorMock.cpuOnline(0)
		require.Error(t, err)
		ioMock.AssertExpectations(t)
		require.Equal(t, false, res)
	})
	t.Run("CPU is online", func(t *testing.T) {
		ioMock.On("validateFile", "/sys/devices/system/cpu/cpu0/online").Return(nil).Once()
		ioMock.On("readAll", "/sys/devices/system/cpu/cpu0/online").Return([]byte("1"), nil).Once()
		res, err := monitorMock.cpuOnline(0)
		require.NoError(t, err)
		ioMock.AssertExpectations(t)
		require.Equal(t, true, res)
	})
	t.Run("CPU is not online", func(t *testing.T) {
		ioMock.On("validateFile", "/sys/devices/system/cpu/cpu0/online").Return(nil).Once()
		ioMock.On("readAll", "/sys/devices/system/cpu/cpu0/online").Return([]byte("0"), nil).Once()
		res, err := monitorMock.cpuOnline(0)
		require.NoError(t, err)
		ioMock.AssertExpectations(t)
		require.Equal(t, false, res)
	})
}

func Test_cpuMonitorImpl_cpusNum(t *testing.T) {
	ioMock := &mockIoHelper{}
	errMock := errors.New("mock error")
	t.Run("cpuMonitorImpl not initialized properly", func(t *testing.T) {
		cpuMonitor := &cpuMonitorImpl{}
		_, err := cpuMonitor.cpusNum()
		require.Error(t, err)
	})
	t.Run("Return error if failed to read file", func(t *testing.T) {
		cpuMonitor := &cpuMonitorImpl{helper: ioMock}
		ioMock.On("readAll", "/sys/devices/system/cpu/present").Return(nil, errMock).Once()
		_, err := cpuMonitor.cpusNum()
		require.Error(t, err)
		ioMock.AssertExpectations(t)
	})
	t.Run("Trim file content and return correct number of CPUs", func(t *testing.T) {
		cpuMonitor := &cpuMonitorImpl{helper: ioMock}
		ioMock.On("readAll", "/sys/devices/system/cpu/present").Return([]byte("  2-4\n"), nil).Once()
		num, err := cpuMonitor.cpusNum()
		require.NoError(t, err)
		ioMock.AssertExpectations(t)
		require.EqualValues(t, 3, num)
	})
}

func Test_parseCPURange(t *testing.T) {
	type args struct {
		cpus string
	}
	tests := []struct {
		name    string
		args    args
		want    int
		wantErr bool
	}{
		{"Empty string", args{""}, 0, false},
		{"Single number", args{"21"}, 1, false},
		{"Two numbers", args{"1,3"}, 2, false},
		{"Single range", args{"2-5"}, 4, false},
		{"Combined list of ranges", args{"0-2,3,5,17-19"}, 8, false},
		{"Wrong first number", args{"test"}, 0, true},
		{"Wrong second number", args{"1-test"}, 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseCPURange(tt.args.cpus)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseCPURange() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("parseCPURange() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_monitoredCPUs(t *testing.T) {
	monitorMock := &mockCpuMonitor{}
	errMock := errors.New("mock error")
	t.Run("Return error if cpu monitor is nil", func(t *testing.T) {
		_, err := monitoredCPUs(nil)
		require.Error(t, err)
	})
	t.Run("Return error if failed to get number of CPUs", func(t *testing.T) {
		monitorMock.On("cpusNum").Return(0, errMock).Once()
		_, err := monitoredCPUs(monitorMock)
		require.Error(t, err)
		monitorMock.AssertExpectations(t)
	})
	t.Run("No error on zero CPUs", func(t *testing.T) {
		monitorMock.On("cpusNum").Return(0, nil).Once()
		cpus, err := monitoredCPUs(monitorMock)
		require.NoError(t, err)
		monitorMock.AssertExpectations(t)
		require.EqualValues(t, 0, len(cpus))
	})
	t.Run("No error on negative number of CPUs", func(t *testing.T) {
		monitorMock.On("cpusNum").Return(-2, nil).Once()
		cpus, err := monitoredCPUs(monitorMock)
		require.NoError(t, err)
		monitorMock.AssertExpectations(t)
		require.EqualValues(t, 0, len(cpus))
	})
	t.Run("Return slice with one CPU", func(t *testing.T) {
		oneCPU := []int{0}
		monitorMock.On("cpusNum").Return(1, nil).Once()
		cpus, err := monitoredCPUs(monitorMock)
		require.NoError(t, err)
		monitorMock.AssertExpectations(t)
		require.Equal(t, oneCPU, cpus)
	})
	t.Run("Return slice with 10 CPUs", func(t *testing.T) {
		tenCPUs := []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}
		monitorMock.On("cpusNum").Return(10, nil).Once()
		cpus, err := monitoredCPUs(monitorMock)
		require.NoError(t, err)
		monitorMock.AssertExpectations(t)
		require.Equal(t, tenCPUs, cpus)
	})
}

func Test_assignedSockets(t *testing.T) {
	assignerMock := &mockCpuAssigner{}
	errMock := errors.New("mock error")
	t.Run("Return error if cpu assigner is nil", func(t *testing.T) {
		_, err := assignedSockets(nil)
		require.Error(t, err)
	})
	t.Run("Return error if failed to assign CPUs to sockets", func(t *testing.T) {
		assignerMock.On("assignCPUToSocket").Return(nil, errMock).Once()
		_, err := assignedSockets(assignerMock)
		require.Error(t, err)
		assignerMock.AssertExpectations(t)
	})
	t.Run("Return no sockets if assigner returned empty map", func(t *testing.T) {
		assignerMock.On("assignCPUToSocket").Return(make(map[int]int), nil).Once()
		sockets, err := assignedSockets(assignerMock)
		require.NoError(t, err)
		assignerMock.AssertExpectations(t)
		require.EqualValues(t, 0, len(sockets))
	})
	t.Run("One socket", func(t *testing.T) {
		mockMap := map[int]int{0: 1}
		assignerMock.On("assignCPUToSocket").Return(mockMap, nil).Once()
		sockets, err := assignedSockets(assignerMock)
		require.NoError(t, err)
		assignerMock.AssertExpectations(t)
		require.Equal(t, []int{0}, sockets)
	})
	t.Run("Multiple sockets", func(t *testing.T) {
		mockMap := map[int]int{0: 0, 1: 10, 3: 20}
		assignerMock.On("assignCPUToSocket").Return(mockMap, nil).Once()
		sockets, err := assignedSockets(assignerMock)
		require.NoError(t, err)
		assignerMock.AssertExpectations(t)
		sort.Ints(sockets)
		require.Equal(t, []int{0, 1, 3}, sockets)
	})
}
