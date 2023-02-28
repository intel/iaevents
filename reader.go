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
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"

	"golang.org/x/exp/maps"
)

// DeprecatedFormatError - error to indicate if a file has the deprecated JSON format
type DeprecatedFormatError struct {
	paths []string
}

func (e *DeprecatedFormatError) Error() string {
	return fmt.Sprintf("Deprecated JSON format was used while reading following files: %s", strings.Join(e.paths, ", "))
}

func (e *DeprecatedFormatError) addPath(path string) {
	e.paths = append(e.paths, path)
}

// Is - the implementation to support errors.Is
func (e *DeprecatedFormatError) Is(target error) bool {
	var err *DeprecatedFormatError
	if !errors.As(target, &err) {
		return false
	}
	return reflect.DeepEqual(e.paths, err.paths)
}

// Reader interface to read JSON events
type Reader interface {
	Read() ([]*JSONEvent, error)
}

// FilesAdder interface to add JSON files with events
type FilesAdder interface {
	AddFiles(firstPath string, optionalPaths ...string) error
}

// JSONData - new data format of perfmon
type JSONData struct {
	Header *JSONHeader  `json:"Header"`
	Events []*JSONEvent `json:"Events"`
}

// JSONHeader - struct contains the Header info of events
type JSONHeader struct {
	Copyright     string `json:"Copyright"`
	Info          string `json:"Info"`
	DatePublished string `json:"DatePublished"`
	Version       string `json:"Version"`
	Legend        string `json:"Legend"`
}

// JSONEvent - fields in JSON definition of PMU events
type JSONEvent struct {
	AnyThread        string `json:"AnyThread"`
	BriefDescription string `json:"BriefDescription"`
	CounterMask      string `json:"CounterMask"`
	Deprecated       string `json:"Deprecated"`
	EdgeDetect       string `json:"EdgeDetect"`
	EventCode        string `json:"EventCode"`
	EventName        string `json:"EventName"`
	ExtSel           string `json:"ExtSel"`
	Invert           string `json:"Invert"`
	MSRIndex         string `json:"MSRIndex"`
	MSRValue         string `json:"MSRValue"`
	SampleAfterValue string `json:"SampleAfterValue"`
	UMask            string `json:"UMask"`
	Unit             string `json:"Unit"`
}

// JSONFilesReader load events definition from specified JSON files
type JSONFilesReader struct {
	io    ioHelper
	read  readJSONFileFunc
	paths []string

	cachedData map[string]*JSONData
}

type readJSONFileFunc func(io ioHelper, path string) (*JSONData, error)

// NewFilesReader returns new JSONFilesReader with default members
func NewFilesReader() *JSONFilesReader {
	return &JSONFilesReader{
		io:         &ioHelperImpl{},
		read:       readJSONFile,
		cachedData: make(map[string]*JSONData),
	}
}

// AddDefaultFiles adds core and uncore events from specified dir for provided cpuid
func AddDefaultFiles(adder FilesAdder, dir string, cpuid CPUidStringer) error {
	if adder == nil {
		return errors.New("files adder interface is nil")
	}
	if cpuid == nil {
		return errors.New("cpu id stringer interface is nil")
	}
	coreEvents := fmt.Sprintf("%s/%s-core.json", dir, cpuid.Full())
	if adder.AddFiles(coreEvents) == nil {
		uncoreEvents := fmt.Sprintf("%s/%s-uncore.json", dir, cpuid.Full())
		err := adder.AddFiles(uncoreEvents)
		if err != nil {
			return fmt.Errorf("failed to add default uncore events file: %w", err)
		}
		return nil
	}

	coreEvents = fmt.Sprintf("%s/%s-core.json", dir, cpuid.Short())
	uncoreEvents := fmt.Sprintf("%s/%s-uncore.json", dir, cpuid.Short())
	err := adder.AddFiles(coreEvents, uncoreEvents)
	if err != nil {
		return fmt.Errorf("failed to add default events files: %w", err)
	}
	return nil
}

func (r *JSONFilesReader) String() string {
	return strings.Join(r.paths, ",")
}

// Convert cache from map[string]*JSONData to []*JSONEvent
func (r *JSONFilesReader) returnCache() []*JSONEvent {
	if len(r.cachedData) == 0 {
		return nil
	}

	cacheSlice := maps.Values(r.cachedData)
	result := make([]*JSONEvent, 0, len(cacheSlice))
	for _, v := range cacheSlice {
		result = append(result, v.Events...)
	}
	return result
}

// AddFiles add files for Reader to read
func (r *JSONFilesReader) AddFiles(firstPath string, optionalPaths ...string) error {
	if r.io == nil {
		return errors.New("reader is not initialized correctly: io is nil")
	}

	if r.read == nil {
		return errors.New("reader is not initialized correctly: missing read implementation")
	}

	if r.cachedData == nil {
		r.cachedData = make(map[string]*JSONData)
	}

	deprecatedFormatError := &DeprecatedFormatError{}
	paths := []string{firstPath}
	paths = append(paths, optionalPaths...)

	cachedData := make(map[string]*JSONData)

	for _, path := range paths {
		if err := r.io.validateFile(path); err != nil {
			return fmt.Errorf("not a valid file: `%s`: %w", path, err)
		}
		data, err := r.read(r.io, path)
		if err != nil {
			if !r.checkDeprecatedJSONFormat(err) {
				return err
			}
			deprecatedFormatError.addPath(path)
		}
		cachedData[path] = data
	}

	maps.Copy(r.cachedData, cachedData)
	r.paths = append(r.paths, paths...)

	if len(deprecatedFormatError.paths) != 0 {
		return deprecatedFormatError
	}
	return nil
}

// Read and return JSON events from path(s)
func (r *JSONFilesReader) Read() ([]*JSONEvent, error) {
	if len(r.paths) == 0 || len(r.cachedData) == 0 {
		return nil, fmt.Errorf("failed to read any JSON events")
	}

	return r.returnCache(), nil
}

// GetHeaders - return the headers from event files
// Key of the map is the path of the file
func (r *JSONFilesReader) GetHeaders() map[string]JSONHeader {
	if len(r.cachedData) == 0 {
		return nil
	}

	eventsHeaders := make(map[string]JSONHeader)
	for k, v := range r.cachedData {
		if v.Header == nil {
			continue
		}
		eventsHeaders[k] = *v.Header
	}
	return eventsHeaders
}

// Func will check if the err is DeprecatedFormatError.
func (r *JSONFilesReader) checkDeprecatedJSONFormat(err error) bool {
	var deprecatedFormatError *DeprecatedFormatError
	return errors.As(err, &deprecatedFormatError)
}

// Try to parse JSON format
// the function supports the deprecated and the new JSON format
// https://github.com/intel/perfmon/issues/22
func readJSONFile(io ioHelper, path string) (*JSONData, error) {
	if io == nil {
		return nil, errors.New("ioHelper is nil")
	}
	byteValue, err := io.readAll(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read file `%s`: %w", path, err)
	}

	// try to parse with the new JSON format
	var jsonFormat JSONData
	err = json.Unmarshal(byteValue, &jsonFormat)
	if err == nil {
		return &jsonFormat, nil
	}

	// try to parse with the deprecated JSON format
	var jsonEvents []*JSONEvent
	err = json.Unmarshal(byteValue, &jsonEvents)
	if err == nil {
		return &JSONData{Events: jsonEvents}, &DeprecatedFormatError{paths: []string{path}}
	}

	return nil, fmt.Errorf("failed to unmarshal JSON file `%s`: %w", path, err)
}
