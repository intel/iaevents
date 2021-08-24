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
	"encoding/binary"
	"fmt"

	"golang.org/x/sys/unix"
)

// Activator is the interface that wraps the activate method.
//
// Activate is able to return activated (being counted) perf event as ActiveEvent.
// Returned active event is configured to fulfill provided configuration:
// 	placement provides a specific hardware placement for event,
//  targetProcess provides a specific process (or cgroup) for event to monitor,
//	options specify any additional perf configuration options.
// Activate must return a non-nil error if event cannot be activated, or provided parameters are invalid.
type Activator interface {
	Activate(placement PlacementProvider, targetProcess TargetProcess, options Options) (*ActiveEvent, error)
}

// MultiActivator is an interface that wraps ActivateMulti method.
//
// ActivateMulti activates multi PMU perf event on specific placements. Returns this activated events as ActiveMultiEvent.
type MultiActivator interface {
	ActivateMulti(placements []PlacementProvider, process TargetProcess, options Options) (*ActiveMultiEvent, error)
}

// NamedPMUType wraps PMUType with its name
type NamedPMUType struct {
	Name    string
	PMUType uint32
}

// PerfEvent contains event general data.
type PerfEvent struct {
	Name     string
	Uncore   bool
	PMUName  string
	PMUTypes []NamedPMUType

	Attr *unix.PerfEventAttr

	eventFormat string
	unix        unixHelper
	cpuMonitor  cpuMonitor
}

// CustomizableEvent unites PerfEvent with specific Options.
type CustomizableEvent struct {
	Event   *PerfEvent
	Options Options
}

// NewPerfEvent creates new PerfEvent.
func NewPerfEvent() *PerfEvent {
	return &PerfEvent{
		Attr:       &unix.PerfEventAttr{Size: unix.PERF_ATTR_SIZE_VER0},
		unix:       &unixHelperImpl{},
		cpuMonitor: newCPUMonitor(),
	}
}

func (pe *PerfEvent) String() string {
	return fmt.Sprintf("%s/%s", pe.Name, pe.PMUName)
}

// Decoded returns a perf representation of the event
func (pe *PerfEvent) Decoded() string {
	return fmt.Sprintf("%s/%s/", pe.PMUName, pe.eventFormat)
}

// NewPlacements returns slice of newly created placements for provided cpus and unit. It also checks if provided
// unit and cpus are valid for the PerfEvent. In case of uncore events, cpus are interpreted as socket CPUs.
// If unit == "" it returns all units for distributed uncore PMU.
func (pe *PerfEvent) NewPlacements(unit string, cpu int, cpus ...int) ([]PlacementProvider, error) {
	if pe.cpuMonitor == nil {
		return nil, fmt.Errorf("the perf event is not initialized correctly: missing cpu monitor")
	}
	var pmuTypes []uint32
	for _, namedType := range pe.PMUTypes {
		if "" == unit || namedType.Name == unit || pe.PMUName == unit {
			pmuTypes = append(pmuTypes, namedType.PMUType)
		}
	}
	if len(pmuTypes) == 0 {
		return nil, fmt.Errorf("provided PMU unit `%s` is not available for event `%s`", unit, pe.Name)
	}

	cpusNumber, err := pe.cpuMonitor.cpusNum()
	if err != nil {
		return nil, err
	}

	cores := []int{cpu}
	cores = append(cores, cpus...)

	var placements []PlacementProvider
	for _, pmuType := range pmuTypes {
		for _, core := range cores {
			if core >= cpusNumber || core < 0 {
				return nil, fmt.Errorf("provided cpu: %d is not valid, max cpu index is: %d", core, cpusNumber-1)
			}
			placements = append(placements, &Placement{CPU: core, PMUType: pmuType})
		}
	}
	return placements, nil
}

func (pe *PerfEvent) Activate(placement PlacementProvider, targetProcess TargetProcess, options Options) (*ActiveEvent, error) {
	return pe.activateWithLeader(placement, targetProcess, options, -1)
}

func (pe *PerfEvent) activateWithLeader(placement PlacementProvider, targetProcess TargetProcess, options Options, groupFd int) (*ActiveEvent, error) {
	if pe.unix == nil {
		return nil, fmt.Errorf("the perf event is not initialized correctly: missing unix helper")
	}
	if pe.Attr == nil {
		return nil, fmt.Errorf("the perf event is not initialized correctly: missing attributes")
	}
	attr := *pe.Attr

	cpu, pmuType, pmuName, err := pe.configurePlacement(placement, &attr)
	if err != nil {
		return nil, fmt.Errorf("error during placement configuration: %v", err)
	}

	err = pe.configurePerfEvent(options, &attr)
	if err != nil {
		return nil, fmt.Errorf("error during event options configuration: %v", err)
	}

	pid, procFlags, err := pe.configureProcess(targetProcess, &attr)
	if err != nil {
		return nil, fmt.Errorf("error during process configuration: %v", err)
	}

	attr.Bits |= unix.PerfBitInherit
	attr.Read_format |= unix.PERF_FORMAT_TOTAL_TIME_ENABLED | unix.PERF_FORMAT_TOTAL_TIME_RUNNING

	fd, err := pe.unix.open(&attr, pid, cpu, groupFd, procFlags)
	if err != nil {
		return nil, fmt.Errorf("cannot open event: %s on pmu: %s due to perf error: %v", pe.Name, pmuName, err)
	}

	decoded := pe.Decoded()
	if options != nil {
		decoded = appendQualifiers(decoded, options.PerfStatString())
	}
	pmu := NamedPMUType{PMUType: pmuType, Name: pmuName}
	newEvent := newActiveEvent(pe, decoded, fd, cpu, pmu)

	return newEvent, nil
}

func (PerfEvent) configureProcess(process TargetProcess, attr *unix.PerfEventAttr) (pid, flags int, err error) {
	if process == nil {
		return 0, 0, fmt.Errorf("missing target process")
	}
	pid = process.ProcessID()
	if (attr.Bits&unix.PerfBitEnableOnExec != 0) && pid == -1 {
		return -1, 0, fmt.Errorf("cannot set enable on exec while reading all processes")
	}
	if process.ProcessType() == 1 {
		flags = unix.PERF_FLAG_PID_CGROUP
	}
	return pid, flags, nil
}

func (PerfEvent) configurePerfEvent(options Options, attr *unix.PerfEventAttr) error {
	if options == nil {
		return nil
	}
	optionsAttr := options.Attr()
	if optionsAttr == nil {
		return fmt.Errorf("missing options attributes")
	}

	attr.Config |= optionsAttr.Config
	attr.Ext1 |= optionsAttr.Ext1
	attr.Ext2 |= optionsAttr.Ext2
	attr.Bits |= optionsAttr.Bits

	return nil
}

func (pe *PerfEvent) configurePlacement(placement PlacementProvider, attr *unix.PerfEventAttr) (cpu int, pmuType uint32, pmuName string, err error) {
	if placement == nil {
		return 0, 0, "", fmt.Errorf("missing placement provider")
	}
	cpu, pmuType = placement.PMUPlacement()
	if pe.cpuMonitor == nil {
		return 0, 0, "", fmt.Errorf("the perf event is not initialized correctly: missing cpu monitor")
	}
	online, err := pe.cpuMonitor.cpuOnline(cpu)
	if err != nil {
		return 0, 0, "", fmt.Errorf("failed to check online status for cpu %d: %v", cpu, err)
	}
	if !online {
		return 0, 0, "", fmt.Errorf("cpu %d is offline", cpu)
	}
	for _, eventPMU := range pe.PMUTypes {
		if eventPMU.PMUType == pmuType {
			attr.Type = pmuType
			pmuName = eventPMU.Name
			return cpu, pmuType, pmuName, nil
		}
	}
	return 0, 0, "", fmt.Errorf("invalid placement PMU type: %v for event: %v ", pmuType, pe.Name)
}

func (pe *PerfEvent) ActivateMulti(placements []PlacementProvider, process TargetProcess, options Options) (*ActiveMultiEvent, error) {
	var activeEvents []*ActiveEvent
	for _, pl := range placements {
		activeEvent, err := pe.Activate(pl, process, options)
		if err != nil {
			for _, ae := range activeEvents {
				_ = ae.Deactivate()
			}
			return nil, err
		}
		activeEvents = append(activeEvents, activeEvent)
	}

	return &ActiveMultiEvent{activeEvents, pe}, nil
}

func (pe *PerfEvent) setDisable() {
	pe.Attr.Bits |= unix.PerfBitDisabled
}

// ActivateGroup is able to create ActiveEventGroup from provided events, placement, process and options. If events size
// is 0 no group will be created, and error will be returned. In case of single event contained in events slice, only one event
// perf group will be activated. In other cases, first event in slice will be treated as a leader and rest of them
// will be activated with its file descriptor as group file descriptor (group_fd argument) in perf_event_open syscall
// (https://man7.org/linux/man-pages/man2/perf_event_open.2.html).
func ActivateGroup(placement PlacementProvider, targetProcess TargetProcess, events []CustomizableEvent) (*ActiveEventGroup, error) {
	var activeEventsGroup = &ActiveEventGroup{}
	if len(events) == 0 {
		return nil, fmt.Errorf("no events in group")
	}
	leader := events[0].Event
	leaderOptions := events[0].Options
	// copy original attribute values
	if leader.Attr == nil {
		return nil, fmt.Errorf("the perf events are not initialized correctly: leader attributes are missing")
	}
	leaderOrigAttr := *leader.Attr
	// leader should wait for rest events activation
	leader.setDisable()

	// activate leader
	activeLeader, err := leader.Activate(placement, targetProcess, leaderOptions)
	if err != nil {
		return nil, err
	}
	activeEventsGroup.addEvent(activeLeader)

	groupFd := activeLeader.FileDescriptor

	if len(events) > 1 {
		// activate rest of group with leader's fd
		for _, e := range events[1:] {
			activeEvent, err := e.Event.activateWithLeader(placement, targetProcess, e.Options, groupFd)
			if err != nil {
				for _, ae := range activeEventsGroup.Events() {
					_ = deactivateFile(leader.unix, ae.FileDescriptor)
				}
				return nil, err
			}
			activeEventsGroup.addEvent(activeEvent)
		}
	}

	// reset and enable counting
	err = resetEventCounter(activeLeader.FileDescriptor, leader.unix)
	if err != nil {
		return nil, fmt.Errorf("cannot reset leader's event counting: %v", err)
	}
	err = enableEventCounter(activeLeader.FileDescriptor, leader.unix)
	if err != nil {
		return nil, fmt.Errorf("cannot enable leader's event counting: %v", err)
	}
	// restore original attributes values
	*leader.Attr = leaderOrigAttr

	return activeEventsGroup, nil
}

// ActiveEventMonitor is and interface that wraps methods around monitoring active perf event.
//
// ReadValue reads actual active perf event counter values and returns CounterValue. If values cannot be obtained,
// error is propagated.
// Close deactivates and closes active perf event.
type ActiveEventMonitor interface {
	ReadValue() (CounterValue, error)
	Deactivate() error
}

// ActiveEvent holds active perf event. Active - means that perf event has been opened by perf_event_open syscall
// (https://man7.org/linux/man-pages/man2/perf_event_open.2.html) and is able to provide counter values for specific,
// previously settled configuration and placement.
type ActiveEvent struct {
	PerfEvent      *PerfEvent
	FileDescriptor int
	Decoded        string

	cpu          int
	namedPMUType NamedPMUType

	read readValues
	unix unixHelper
}

type readValues func(helper unixHelper, fd int) (CounterValue, error)

func newActiveEvent(origin *PerfEvent, decoded string, fd int, cpu int, pmu NamedPMUType) *ActiveEvent {
	return &ActiveEvent{
		PerfEvent:      origin,
		FileDescriptor: fd,
		Decoded:        decoded,
		cpu:            cpu,
		namedPMUType:   pmu,
		unix:           &unixHelperImpl{},
		read:           readActiveEventValues,
	}
}

func (ae *ActiveEvent) String() string {
	return fmt.Sprintf("%v/%d/%s", ae.PerfEvent.Name, ae.cpu, ae.namedPMUType.Name)
}

func (ae *ActiveEvent) PMUPlacement() (int, uint32) {
	return ae.cpu, ae.namedPMUType.PMUType
}

// PMUName returns name of the ActiveEvent's PMU.
func (ae *ActiveEvent) PMUName() string {
	return ae.namedPMUType.Name
}

func (ae *ActiveEvent) ReadValue() (CounterValue, error) {
	if ae.read == nil {
		return CounterValue{}, fmt.Errorf("missing read method")
	}
	if ae.unix == nil {
		return CounterValue{}, fmt.Errorf("the active event is not initialized correctly: missing unix helper")
	}
	values, err := ae.read(ae.unix, ae.FileDescriptor)
	if err != nil {
		return values, fmt.Errorf("cannot read values for event `%s`: %v", ae, err)
	}
	return values, err
}

// Deactivate stops counting active event by closing its file descriptor.
func (ae *ActiveEvent) Deactivate() error {
	if ae.unix == nil {
		return fmt.Errorf("the active event is not initialized correctly: missing unix helper")
	}
	if err := deactivateFile(ae.unix, ae.FileDescriptor); err != nil {
		return err
	}
	ae.FileDescriptor = -1
	return nil
}

func deactivateFile(helper unixHelper, fd int) error {
	if helper == nil {
		return fmt.Errorf("missing unix helper")
	}
	if err := helper.close(fd); err != nil {
		return fmt.Errorf("cannot deactivate file: %v", err)
	}
	return nil
}

func readActiveEventValues(helper unixHelper, fd int) (CounterValue, error) {
	newValues := CounterValue{}
	if helper == nil {
		return newValues, fmt.Errorf("missing unix helper")
	}
	var raw = make([]byte, 8)
	var enabled = make([]byte, 8)
	var running = make([]byte, 8)
	var buffer = make([]byte, 24)

	// read event perf file content to byte buffer
	_, err := helper.read(fd, &buffer)
	if err != nil {
		return newValues, fmt.Errorf("cannot read from file descriptor: %v", err)
	}
	if len(buffer) != 24 {
		return newValues, fmt.Errorf("cannot read values from file descriptor: bytes size not equal 24")
	}
	// read raw, enabled and running values from buffer
	for i := 0; i < 8; i++ {
		// 0-7 bytes define raw value
		raw[i] = buffer[7-i]
		// 8-15 bytes define enabled value
		enabled[i] = buffer[15-i]
		// 16-23 bytes define running value
		running[i] = buffer[23-i]
	}
	// convert each byte array to corresponding int value
	// errors never happen here because of previously assurance that all byte arrays have length 8
	newValues.Raw, _ = byteArrayToUint64(raw)
	newValues.Enabled, _ = byteArrayToUint64(enabled)
	newValues.Running, _ = byteArrayToUint64(running)

	return newValues, nil
}

func resetEventCounter(fd int, helper unixHelper) error {
	if helper == nil {
		return fmt.Errorf("missing unix helper")
	}
	err := helper.setInt(fd, unix.PERF_EVENT_IOC_RESET, 0)
	if err != nil {
		return err
	}
	return nil
}

func enableEventCounter(fd int, helper unixHelper) error {
	if helper == nil {
		return fmt.Errorf("missing unix helper")
	}
	err := helper.setInt(fd, unix.PERF_EVENT_IOC_ENABLE, 0)
	if err != nil {
		return err
	}
	return nil
}

func byteArrayToUint64(arr []byte) (uint64, error) {
	if len(arr) != 8 {
		return 0, fmt.Errorf("byte array needs to have length equal 8 but has %d", len(arr))
	}
	return binary.BigEndian.Uint64(arr), nil
}
