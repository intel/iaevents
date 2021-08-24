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
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// Unit tests need mocks to work properly.
// Please generate mocks by `make mock` or `make test` command.

func TestNewFilesReader(t *testing.T) {
	reader := NewFilesReader()
	require.NotNil(t, reader)
	require.NotNil(t, reader.io)
	require.NotNil(t, reader.read)
}

func TestAddDefaultFiles(t *testing.T) {
	adderMock := &MockFilesAdder{}
	cpuIDMock := &MockCPUidStringer{}
	errMock := errors.New("mock error")
	testDir := "/test"
	t.Run("Return error if interfaces are nil", func(t *testing.T) {
		err := AddDefaultFiles(nil, testDir, cpuIDMock)
		require.Error(t, err)
		err = AddDefaultFiles(adderMock, testDir, nil)
		require.Error(t, err)
	})
	t.Run("Add files named after full CPU id", func(t *testing.T) {
		cpuIDMock.On("Full").Return("FullID").Twice()
		adderMock.On("AddFiles", "/test/FullID-core.json").Return(nil).Once().
			On("AddFiles", "/test/FullID-uncore.json").Return(nil).Once()
		err := AddDefaultFiles(adderMock, testDir, cpuIDMock)
		require.NoError(t, err)
		cpuIDMock.AssertExpectations(t)
		adderMock.AssertExpectations(t)
	})
	t.Run("Return error if failed to add uncore file", func(t *testing.T) {
		cpuIDMock.On("Full").Return("FullID").Twice()
		adderMock.On("AddFiles", "/test/FullID-core.json").Return(nil).Once().
			On("AddFiles", "/test/FullID-uncore.json").Return(errMock).Once()
		err := AddDefaultFiles(adderMock, testDir, cpuIDMock)
		require.Error(t, err)
		cpuIDMock.AssertExpectations(t)
		adderMock.AssertExpectations(t)
	})
	t.Run("Return error if failed to add any files", func(t *testing.T) {
		cpuIDMock.On("Full").Return("FullID").Once()
		cpuIDMock.On("Short").Return("ShortID").Twice()
		adderMock.On("AddFiles", "/test/FullID-core.json").Return(errMock).Once().
			On("AddFiles", "/test/ShortID-core.json", "/test/ShortID-uncore.json").Return(errMock).Once()
		err := AddDefaultFiles(adderMock, testDir, cpuIDMock)
		require.Error(t, err)
		cpuIDMock.AssertExpectations(t)
		adderMock.AssertExpectations(t)
	})
	t.Run("Add files named after short CPU id without stepping", func(t *testing.T) {
		cpuIDMock.On("Full").Return("FullID").Once()
		cpuIDMock.On("Short").Return("ShortID").Twice()
		adderMock.On("AddFiles", "/test/FullID-core.json").Return(errMock).Once().
			On("AddFiles", "/test/ShortID-core.json", "/test/ShortID-uncore.json").Return(nil).Once()
		err := AddDefaultFiles(adderMock, testDir, cpuIDMock)
		require.NoError(t, err)
		cpuIDMock.AssertExpectations(t)
		adderMock.AssertExpectations(t)
	})
}

func Test_readJSONFile(t *testing.T) {
	ioMock := &mockIoHelper{}
	errMock := errors.New("mock error")
	t.Run("Return error if io helper is nil", func(t *testing.T) {
		_, err := readJSONFile(nil, "/testPath")
		require.Error(t, err)
	})
	t.Run("Return error if failed to read the file", func(t *testing.T) {
		ioMock.On("readAll", "/wrongPath").Return(nil, errMock).Once()
		_, err := readJSONFile(ioMock, "/wrongPath")
		require.Error(t, err)
		ioMock.AssertExpectations(t)
	})
	t.Run("Return error on invalid JSON format", func(t *testing.T) {
		ioMock.On("readAll", "/goodPath").Return([]byte("[invalid json]"), nil).Once()
		_, err := readJSONFile(ioMock, "/goodPath")
		require.Error(t, err)
		ioMock.AssertExpectations(t)
	})
	t.Run("Return json events", func(t *testing.T) {
		testJSON := []byte("[{\"EventCode\": \"0x00\", \"EventName\": \"test1\", \"Invert\": \"0\"}," +
			"{\"EventCode\": \"0x01\", \"EventName\": \"test2\", \"Invert\": \"1\"}]")
		ioMock.On("readAll", "/goodPath").Return(testJSON, nil).Once()
		jsonEvents, err := readJSONFile(ioMock, "/goodPath")
		require.NoError(t, err)
		ioMock.AssertExpectations(t)
		require.NotNil(t, jsonEvents)
		require.Equal(t, 2, len(jsonEvents))
		require.Equal(t, "test1", jsonEvents[0].EventName)
		require.Equal(t, "test2", jsonEvents[1].EventName)
	})
}

func TestJSONFilesReader_AddFiles(t *testing.T) {
	reader, ioMock, _ := newJSONReaderForTest()
	errMock := errors.New("mock error")

	t.Run("Return error if JSONFilesReader is not initialized properly", func(t *testing.T) {
		emptyReader := &JSONFilesReader{}
		require.Error(t, emptyReader.AddFiles("/path1"))
	})
	t.Run("Return error if not a valid file", func(t *testing.T) {
		ioMock.On("validateFile", "/path1").Return(errMock).Once()
		require.Error(t, reader.AddFiles("/path1"))
		ioMock.AssertExpectations(t)
	})
	t.Run("Return error if one file is not valid", func(t *testing.T) {
		ioMock.On("validateFile", "/path1").Return(nil).Once().
			On("validateFile", "/path2").Return(errMock).Once()
		require.Error(t, reader.AddFiles("/path1", "/path2"))
		ioMock.AssertExpectations(t)
	})
	t.Run("Add a single valid file", func(t *testing.T) {
		reader.paths = []string{}
		ioMock.On("validateFile", "/path1").Return(nil).Once()
		require.NoError(t, reader.AddFiles("/path1"))
		ioMock.AssertExpectations(t)
		require.Equal(t, 1, len(reader.paths))
		require.Equal(t, "/path1", reader.paths[0])
	})
	t.Run("Add several valid files", func(t *testing.T) {
		reader.paths = []string{}
		ioMock.On("validateFile", mock.Anything).Return(nil).Times(3)
		require.NoError(t, reader.AddFiles("/path1", "/path2", "/path3"))
		ioMock.AssertExpectations(t)
		require.Equal(t, 3, len(reader.paths))
		require.Equal(t, "/path1", reader.paths[0])
		require.Equal(t, "/path2", reader.paths[1])
		require.Equal(t, "/path3", reader.paths[2])
	})
	t.Run("Add files several times", func(t *testing.T) {
		reader.paths = []string{}
		ioMock.On("validateFile", "/path1").Return(nil).Once()
		require.NoError(t, reader.AddFiles("/path1"))
		ioMock.AssertExpectations(t)
		require.Equal(t, 1, len(reader.paths))
		require.Equal(t, "/path1", reader.paths[0])
		// Check that cached is changed to false after new files are added
		reader.cached = true
		ioMock.On("validateFile", mock.Anything).Return(nil).Twice()
		require.NoError(t, reader.AddFiles("/path2", "/path3"))
		ioMock.AssertExpectations(t)
		require.Equal(t, false, reader.cached)
		require.Equal(t, 3, len(reader.paths))
		require.Equal(t, "/path1", reader.paths[0])
		require.Equal(t, "/path2", reader.paths[1])
		require.Equal(t, "/path3", reader.paths[2])
	})
}

func TestJSONFilesReader_Read(t *testing.T) {
	jsonTestEvents := []*JSONEvent{{
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
	}, {
		AnyThread:        "0",
		BriefDescription: "Short description.",
		CounterMask:      "0",
		Deprecated:       "0",
		EdgeDetect:       "0",
		EventCode:        "0x03",
		EventName:        "LD_BLOCKS.STORE_FORWARD",
		ExtSel:           "0x12",
		Invert:           "0",
		MSRIndex:         "0",
		MSRValue:         "0",
		SampleAfterValue: "100003",
		UMask:            "0x02",
		Unit:             "",
	}}
	reader, _, readFuncMock := newJSONReaderForTest()
	errMock := errors.New("mock error")

	t.Run("Return error if read is not initialized properly", func(t *testing.T) {
		emptyReader := &JSONFilesReader{}
		_, err := emptyReader.Read()
		require.Error(t, err)
	})
	t.Run("Return error if no events", func(t *testing.T) {
		_, err := reader.Read()
		require.Error(t, err)
		require.Equal(t, false, reader.cached)
	})
	t.Run("Return events correctly from files and cached", func(t *testing.T) {
		reader.paths = []string{"/path1", "/path2"}
		readFuncMock.On("Execute", mock.Anything, "/path1").Return(jsonTestEvents[:1], nil).Once().
			On("Execute", mock.Anything, "/path2").Return(jsonTestEvents[1:], nil).Once()
		events, err := reader.Read()
		require.NoError(t, err)
		readFuncMock.AssertExpectations(t)
		require.Equal(t, true, reader.cached)
		require.EqualValues(t, jsonTestEvents, events)

		events, err = reader.Read()
		require.NoError(t, err)
		require.Equal(t, true, reader.cached)
		require.EqualValues(t, jsonTestEvents, events)
	})
	t.Run("Return error if failed to read", func(t *testing.T) {
		reader.paths = []string{"/path1"}
		reader.cached = false
		readFuncMock.On("Execute", mock.Anything, "/path1").Return(nil, errMock).Once()
		_, err := reader.Read()
		require.Error(t, err)
		readFuncMock.AssertExpectations(t)
		require.Equal(t, false, reader.cached)
	})
}

func TestJSONFilesReader_String(t *testing.T) {
	testPathSingle := []string{"/test1"}
	testPathsMany := []string{"/test1", "/test2", "/test3"}
	reader := NewFilesReader()
	require.NotNil(t, reader)
	require.Equal(t, "", reader.String())
	reader.paths = testPathSingle
	require.Equal(t, "/test1", reader.String())
	reader.paths = testPathsMany
	require.Equal(t, "/test1,/test2,/test3", reader.String())
}

func newJSONReaderForTest() (*JSONFilesReader, *mockIoHelper, *mockReadJSONFileFunc) {
	io := &mockIoHelper{}
	readFunc := &mockReadJSONFileFunc{}
	reader := &JSONFilesReader{
		io:   io,
		read: readFunc.Execute,
	}
	return reader, io, readFunc
}
