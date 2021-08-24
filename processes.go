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

import "fmt"

// TargetProcess is an interface with methods specialized in gathering information about desired system process.
//
// ProcessID returns specific process identifier.
// ProcessType returns type of process identifier. Value 0 is for process id (PID) and 1 is for cgroup identifier.
type TargetProcess interface {
	ProcessID() int
	ProcessType() int
}

// EventTargetProcess holds target process data.
type EventTargetProcess struct {
	pid         int
	processType int
}

// NewEventTargetProcess returns new EventTargetProcess.
func NewEventTargetProcess(pid int, processType int) *EventTargetProcess {
	return &EventTargetProcess{pid: pid, processType: processType}
}

func (etp *EventTargetProcess) ProcessID() int {
	return etp.pid
}

func (etp *EventTargetProcess) ProcessType() int {
	return etp.processType
}

func (etp *EventTargetProcess) String() string {
	var procType string
	switch etp.processType {
	case 0:
		procType = "process"
	case 1:
		procType = "cgroup"
	default:
		procType = "unknown"
	}
	return fmt.Sprintf("pid:%d, process type:%s", etp.pid, procType)
}
