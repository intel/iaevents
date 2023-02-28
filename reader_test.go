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
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"golang.org/x/exp/maps"
)

// Unit tests need mocks to work properly.
// Please generate mocks by `make mock` or `make test` command.

var (
	jsonTestEvents = []*JSONEvent{{
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
	jsonHeadersData = []JSONHeader{{
		Info:    "info1",
		Version: "1.14",
	}, {
		Info:    "info2",
		Version: "1.15",
	}}
	jsonTestData = []*JSONData{{
		Header: &jsonHeadersData[0],
		Events: []*JSONEvent{jsonTestEvents[0]},
	}, {
		Header: &jsonHeadersData[1],
		Events: []*JSONEvent{jsonTestEvents[1]},
	}}
)

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
	t.Run("Return json events with the deprecated format and DeprecatedFormatError", func(t *testing.T) {
		testJSON := []byte("[{\"EventCode\": \"0x00\", \"EventName\": \"test1\", \"Invert\": \"0\"}," +
			"{\"EventCode\": \"0x01\", \"EventName\": \"test2\", \"Invert\": \"1\"}]")
		ioMock.On("readAll", "/goodPath").Return(testJSON, nil).Once()
		jsonData, err := readJSONFile(ioMock, "/goodPath")
		require.ErrorIs(t, err, &DeprecatedFormatError{paths: []string{"/goodPath"}})
		require.NotNil(t, jsonData)
		require.Nil(t, jsonData.Header)
		jsonEvents := jsonData.Events
		require.NotNil(t, jsonEvents)
		ioMock.AssertExpectations(t)
		require.NotNil(t, jsonEvents)
		require.Equal(t, 2, len(jsonEvents))
		require.Equal(t, "0x00", jsonEvents[0].EventCode)
		require.Equal(t, "0x01", jsonEvents[1].EventCode)
		require.Equal(t, "test1", jsonEvents[0].EventName)
		require.Equal(t, "test2", jsonEvents[1].EventName)
	})
	t.Run("Return json events with the new format", func(t *testing.T) {
		testJSON := []byte("{\"Header\": {\"Version\": \"1.15\"}, \"Events\": [{\"EventCode\": \"0x00\", \"EventName\": \"test1\", \"Invert\": \"0\"}," +
			"{\"EventCode\": \"0x01\", \"EventName\": \"test2\", \"Invert\": \"1\"}]}")
		ioMock.On("readAll", "/goodPath").Return(testJSON, nil).Once()
		jsonData, err := readJSONFile(ioMock, "/goodPath")
		require.NoError(t, err)
		require.NotNil(t, jsonData)
		require.NotNil(t, jsonData.Header)
		require.Equal(t, "1.15", jsonData.Header.Version)
		jsonEvents := jsonData.Events
		require.NotNil(t, jsonEvents)
		ioMock.AssertExpectations(t)
		require.NotNil(t, jsonEvents)
		require.Equal(t, 2, len(jsonEvents))
		require.Equal(t, "0x00", jsonEvents[0].EventCode)
		require.Equal(t, "0x01", jsonEvents[1].EventCode)
		require.Equal(t, "test1", jsonEvents[0].EventName)
		require.Equal(t, "test2", jsonEvents[1].EventName)
	})
}

func TestJSONFilesReader_AddFiles(t *testing.T) {
	reader, ioMock, readFuncMock := newJSONReaderForTest()
	errMock := errors.New("mock error")

	t.Run("Return error if JSONFilesReader is not initialized properly", func(t *testing.T) {
		emptyReader := &JSONFilesReader{}
		require.Error(t, emptyReader.AddFiles("/path1"))
	})
	t.Run("Return error if JSONFilesReader is not initialized properly", func(t *testing.T) {
		emptyReader := NewFilesReader()
		emptyReader.read = nil
		require.Error(t, emptyReader.AddFiles("/path1"))
	})
	t.Run("Return error if not a valid file", func(t *testing.T) {
		ioMock.On("validateFile", "/path1").Return(errMock).Once()
		require.Error(t, reader.AddFiles("/path1"))
		ioMock.AssertExpectations(t)
		readFuncMock.AssertExpectations(t)
	})
	t.Run("Return error if one file is not valid", func(t *testing.T) {
		require.Empty(t, reader.cachedData)
		ioMock.On("validateFile", "/path1").Return(nil).Once().
			On("validateFile", "/path2").Return(errMock).Once()
		readFuncMock.On("Execute", mock.Anything, "/path1").Return(jsonTestData[0], nil).Once()

		require.Error(t, reader.AddFiles("/path1", "/path2"))
		require.Empty(t, reader.cachedData)
		ioMock.AssertExpectations(t)
		readFuncMock.AssertExpectations(t)
	})
	t.Run("Return error if failed to read from one of the files", func(t *testing.T) {
		require.Empty(t, reader.cachedData)
		ioMock.On("validateFile", "/path1").Return(nil).Once().
			On("validateFile", "/path2").Return(nil).Once()
		readFuncMock.On("Execute", mock.Anything, "/path1").Return(jsonTestData[0], nil).Once().
			On("Execute", mock.Anything, "/path2").Return(nil, errMock).Once()
		err := reader.AddFiles("/path1", "/path2")
		require.Error(t, err)
		require.NotErrorIs(t, err, &DeprecatedFormatError{paths: []string{"/path2"}})
		require.Empty(t, reader.paths)
		require.Empty(t, reader.cachedData)
		ioMock.AssertExpectations(t)
		readFuncMock.AssertExpectations(t)
	})
	t.Run("Return DeprecatedFormatError if one of the file has the deprecated format", func(t *testing.T) {
		jsonData := &JSONData{Header: nil, Events: jsonTestEvents}
		readErr := &DeprecatedFormatError{paths: []string{"/path2"}}
		reader.paths = []string{}
		ioMock.On("validateFile", "/path1").Return(nil).Once().
			On("validateFile", "/path2").Return(nil).Once()
		readFuncMock.On("Execute", mock.Anything, "/path1").Return(jsonTestData[0], nil).Once().
			On("Execute", mock.Anything, "/path2").Return(jsonData, readErr).Once()
		err := reader.AddFiles("/path1", "/path2")
		require.ErrorIs(t, &DeprecatedFormatError{paths: []string{"/path2"}}, err)
		require.ElementsMatch(t, reader.paths, []string{"/path1", "/path2"})
		require.NotEmpty(t, reader.cachedData)
		require.Len(t, reader.cachedData, 2)
		ioMock.AssertExpectations(t)
		readFuncMock.AssertExpectations(t)
	})
	t.Run("Add a single valid file", func(t *testing.T) {
		reader.paths = []string{}
		ioMock.On("validateFile", "/path1").Return(nil).Once()
		readFuncMock.On("Execute", mock.Anything, "/path1").Return(nil, nil).Once()
		require.NoError(t, reader.AddFiles("/path1"))
		ioMock.AssertExpectations(t)
		readFuncMock.AssertExpectations(t)
		require.Equal(t, 1, len(reader.paths))
		require.Equal(t, "/path1", reader.paths[0])
	})
	t.Run("Add several valid files", func(t *testing.T) {
		reader.paths = []string{}
		ioMock.On("validateFile", mock.Anything).Return(nil).Times(3)
		readFuncMock.On("Execute", mock.Anything, mock.Anything).Return(nil, nil).Times(3)
		require.NoError(t, reader.AddFiles("/path1", "/path2", "/path3"))
		ioMock.AssertExpectations(t)
		readFuncMock.AssertExpectations(t)
		require.Equal(t, 3, len(reader.paths))
		require.Equal(t, "/path1", reader.paths[0])
		require.Equal(t, "/path2", reader.paths[1])
		require.Equal(t, "/path3", reader.paths[2])
	})
	t.Run("Add files several times", func(t *testing.T) {
		reader.paths = []string{}
		reader.cachedData = map[string]*JSONData{}
		ioMock.On("validateFile", "/path1").Return(nil).Once()
		readFuncMock.On("Execute", mock.Anything, "/path1").Return(nil, nil).Once()
		require.NoError(t, reader.AddFiles("/path1"))
		ioMock.AssertExpectations(t)
		readFuncMock.AssertExpectations(t)
		require.Equal(t, 1, len(reader.paths))
		require.Equal(t, "/path1", reader.paths[0])
		require.Len(t, reader.cachedData, 1)

		// Try to add more files
		ioMock.On("validateFile", mock.Anything).Return(nil).Twice()
		readFuncMock.On("Execute", mock.Anything, mock.Anything).Return(nil, nil).Twice()
		require.NoError(t, reader.AddFiles("/path2", "/path3"))
		ioMock.AssertExpectations(t)
		require.Len(t, reader.cachedData, 3)
		require.Len(t, reader.paths, 3)
		require.Equal(t, "/path1", reader.paths[0])
		require.Equal(t, "/path2", reader.paths[1])
		require.Equal(t, "/path3", reader.paths[2])
	})
}

func TestJSONFilesReader_Read(t *testing.T) {
	reader, ioMock, readFuncMock := newJSONReaderForTest()
	t.Run("Return error if read is not initialized properly", func(t *testing.T) {
		emptyReader := &JSONFilesReader{}
		_, err := emptyReader.Read()
		require.Error(t, err)
	})
	t.Run("Return error if no events", func(t *testing.T) {
		_, err := reader.Read()
		require.Error(t, err)
		require.Empty(t, reader.cachedData)
	})
	t.Run("Return events correctly from files and cached", func(t *testing.T) {
		reader.paths = []string{}
		ioMock.On("validateFile", mock.Anything).Return(nil).Twice()
		readFuncMock.On("Execute", mock.Anything, "/path1").Return(jsonTestData[0], nil).Once().
			On("Execute", mock.Anything, "/path2").Return(jsonTestData[1], nil).Once()
		err := reader.AddFiles("/path1", "/path2")
		require.NoError(t, err)
		require.Len(t, reader.paths, 2)
		require.Len(t, reader.cachedData, 2)
		events, err := reader.Read()
		require.NoError(t, err)
		ioMock.AssertExpectations(t)
		readFuncMock.AssertExpectations(t)
		require.Len(t, reader.paths, 2)
		require.Len(t, reader.cachedData, 2)
		require.ElementsMatch(t, jsonTestEvents, events)

		events, err = reader.Read()
		require.NoError(t, err)
		require.Len(t, reader.paths, 2)
		require.Len(t, reader.cachedData, 2)
		require.ElementsMatch(t, jsonTestEvents, events)
	})
	t.Run("Return error if no path were added before", func(t *testing.T) {
		reader.paths = []string{""}
		reader.cachedData = nil
		_, err := reader.Read()
		require.Error(t, err)
		ioMock.AssertExpectations(t)
		readFuncMock.AssertExpectations(t)
		require.Empty(t, reader.cachedData)
	})
	t.Run("Return error if there is no cache", func(t *testing.T) {
		reader.paths = []string{"/path1"}
		reader.cachedData = nil
		_, err := reader.Read()
		require.Error(t, err)
		readFuncMock.AssertExpectations(t)
		require.Empty(t, reader.cachedData)
	})
}

func TestGetHeaders(t *testing.T) {
	reader, ioMock, readFuncMock := newJSONReaderForTest()
	t.Run("Return nil Headers if didn't read before", func(t *testing.T) {
		meta := reader.GetHeaders()
		require.Nil(t, meta)
	})
	t.Run("Return headers correctly", func(t *testing.T) {
		ioMock.On("validateFile", mock.Anything).Return(nil).Twice()
		readFuncMock.On("Execute", mock.Anything, "/path1").Return(jsonTestData[0], nil).Once().
			On("Execute", mock.Anything, "/path2").Return(jsonTestData[1], nil).Once()
		err := reader.AddFiles("/path1", "/path2")
		require.NoError(t, err)
		events, err := reader.Read()
		require.NoError(t, err)
		ioMock.AssertExpectations(t)
		readFuncMock.AssertExpectations(t)
		require.Len(t, reader.cachedData, 2)
		require.ElementsMatch(t, jsonTestEvents, events)

		readerHeaders := reader.GetHeaders()
		require.NotNil(t, readerHeaders)
		require.Len(t, reader.paths, 2)
		require.Len(t, readerHeaders, len(reader.paths))
		keys := maps.Keys(readerHeaders)
		require.ElementsMatch(t, reader.paths, keys)
		val := maps.Values(readerHeaders)
		require.Len(t, val, len(reader.paths))
		require.ElementsMatch(t, val, jsonHeadersData)
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
		io:         io,
		read:       readFunc.Execute,
		cachedData: make(map[string]*JSONData),
	}
	return reader, io, readFunc
}
