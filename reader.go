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
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// Reader interface to read JSON events
type Reader interface {
	Read() ([]*JSONEvent, error)
}

// FilesAdder interface to add JSON files with events
type FilesAdder interface {
	AddFiles(firstPath string, optionalPaths ...string) error
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
	io         ioHelper
	read       readJSONFileFunc
	jsonEvents []*JSONEvent
	paths      []string
	cached     bool
}

type readJSONFileFunc func(io ioHelper, path string) ([]*JSONEvent, error)

// NewFilesReader returns new JSONFilesReader with default members
func NewFilesReader() *JSONFilesReader {
	return &JSONFilesReader{
		io:   &ioHelperImpl{},
		read: readJSONFile,
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
			return fmt.Errorf("failed to add default uncore events file: %v", err)
		}
		return nil
	}

	coreEvents = fmt.Sprintf("%s/%s-core.json", dir, cpuid.Short())
	uncoreEvents := fmt.Sprintf("%s/%s-uncore.json", dir, cpuid.Short())
	err := adder.AddFiles(coreEvents, uncoreEvents)
	if err != nil {
		return fmt.Errorf("failed to add default events files: %v", err)
	}
	return nil
}

func (r *JSONFilesReader) String() string {
	return strings.Join(r.paths, ",")
}

// AddFiles add files for Reader to read
func (r *JSONFilesReader) AddFiles(firstPath string, optionalPaths ...string) error {
	if r.io == nil {
		return errors.New("reader is not initialized correctly: io is nil")
	}
	if err := r.io.validateFile(firstPath); err != nil {
		return fmt.Errorf("not a valid file: `%s`: %v", firstPath, err)
	}
	r.paths = append(r.paths, firstPath)
	r.cached = false
	for _, path := range optionalPaths {
		if err := r.io.validateFile(path); err != nil {
			return fmt.Errorf("not a valid file: `%s`: %v", path, err)
		}
		r.paths = append(r.paths, path)
	}
	return nil
}

// Read reads and return JSON events from path(s)
func (r *JSONFilesReader) Read() ([]*JSONEvent, error) {
	if r.cached {
		return r.jsonEvents, nil
	}
	if r.read == nil {
		return nil, errors.New("reader is not initialized correctly: missing read implementation")
	}
	for _, path := range r.paths {
		events, err := r.read(r.io, path)
		if err != nil {
			return nil, err
		}
		r.jsonEvents = append(r.jsonEvents, events...)
	}
	if len(r.jsonEvents) == 0 {
		return nil, errors.New("failed to read any JSON events")
	}
	r.cached = true
	return r.jsonEvents, nil
}

func readJSONFile(io ioHelper, path string) ([]*JSONEvent, error) {
	if io == nil {
		return nil, errors.New("ioHelper is nil")
	}
	byteValue, err := io.readAll(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read file `%s`: %v", path, err)
	}

	var jsonEvents []*JSONEvent
	err = json.Unmarshal(byteValue, &jsonEvents)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal JSON file `%s`: %v", path, err)
	}
	return jsonEvents, nil
}
