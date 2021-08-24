# iaevents

`iaevents` is a golang library that makes accessing the Linux kernel's perf interface easier. It provides tools to
load, parse and read Intel CPU events specified in JSON format available from https://download.01.org/perfmon. It is
based on Andi Kleen's [pmu-tools/jevents](https://github.com/andikleen/pmu-tools/tree/master/jevents) library that
provides such functionality in C language .

Library exports main features as implementation of interfaces:
* *Reader*             - provide events definition from JSON file(s)
* *Matcher*            - match selected events for *Transformer*
* *Transformer*        - transform event from event definition to proper perf event attributes
* *Activator*          - set up event and start measuring
* *MultiActivator*     - set up multi PMU event (for uncore events) and start measuring on all units
* *ActiveEventMonitor* - read current value of perf event
* *EventGroupMonitor*  - read current values for perf events aggregated in one group
* *PlacementFactory*   - provide valid placement provider for selected unit and CPUs
* *PlacementProvider*  - provide placement of particular event

Library contains default implementation for all the listed interfaces.
It handles both core and uncore type events. To transform the event from JSON definition to corresponding perf
attributes the library read the perf specific data from `sysfs` files. The measurements are obtained with the
use of `perf_event_open` syscall.

### System requirements
The `iaevents` library is intended to be used only on **linux 64-bit** systems.
To make system wide measurements for all processes/threads with *"PID = -1"* the process using the library requires
**CAP_SYS_ADMIN** capability or `/proc/sys/kernel/perf_event_paranoid` has to be set to value less than 1.
> Note: Library access all the required files in read mode only. According to principle of least privilege the user
should restrict the files to read-only mode.

### Note on number of open file descriptors
The library opens file descriptors whose quantity depends on number of monitored CPUs and number of monitored
counters. It can easily exceed the default per process limit of allowed file descriptors. Depending on
configuration, it might be required to increase the limit on the number of open file descriptors allowed.
This can be done for example by using `ulimit -n` command.

## Gather JSON files
User has to acquire json events file for specific system architecture. It can be done automatically 
by using the [pmu-tools](https://github.com/andikleen/pmu-tools) event_download.py script, 
or files can be obtained directly from this link: https://download.01.org/perfmon/.

## Create events reader
The library requires to provide event definitions for *Transfomer*. It is fulfilled by *Reader* interface.
To instantiate the default *Reader* implementation create new *JSONFilesReader* with *NewFilesReader* method.
```go
reader := iaevents.NewFilesReader()
```
Next run *AddFiles* method with a path(s) to desired JSON file(s).
```go
err := reader.AddFiles(pathToFile1, pathToFile2)
```
It is possible to look for default JSON file names for local CPU in provided path.
```go
cpuid, err := iaevents.CheckCPUid()
err = iaevents.AddDefaultFiles(reader, pathToDir, cpuid)
```

## Transform selected event(s) into perf attributes
To select the events to transform, user must use *Matcher* interface. The `iaevents` provide default matcher
that matches the events by names.
```go
matcher := iaevents.NewNameMatcher(EventName1, EventName2, MoreEventNames...)
```
To match all events provided by *Reader*, create new matcher without arguments.
```go
matcher := iaevents.NewNameMatcher()
```
To resolve perf attributes of selected events call *Transform* method from *PerfTransfomer*. It returns
slice of events with resolved perf attributes.
```go
transformer := iaevents.NewPerfTransformer()
perfEvents, err := transformer.Transform(reader, matcher)
```

## Activate events
To start measuring on PMU counters perf events need to be activated. During activation library makes call to
`perf_event_open` syscall and obtain specific file descriptors for given event and cpu(s).
The required function to activate the event is different for core and uncore events. To check if event belongs
to uncore category user can check *Uncore* flag for perf event.
```go
if perfEvent.Uncore {
    // Handle uncore event
}
```

### Target process and extra perf options
*TargetProcess* can be used to monitor event for selected process or cgroup. To enable system wide
monitoring of event use `-1` as PID.
```go
process := iaevents.NewEventTargetProcess(-1, 0)
```
*Options* gives user the possibility to modify the event with custom modifiers and perf attributes.
The `iaevents` library provides builder to simplify creation of *Options* object.
```go
builder := iaevents.NewOptions()
```
Use *SetEnableOnExec* to set perf event attribute bits with `PerfBitDisabled` and `PerfBitEnableOnExec`.
```go
builder.SetEnableOnExec(true)
```
Use *SetAttrModifiers* to set list of custom perf event attribute modifiers.
More info can be found on [Perf Wiki](https://perf.wiki.kernel.org/index.php/Tutorial#Modifiers).
```go
builder.SetAttrModifiers(modifiers)
```
*Build* finds the correct setting for perf attributes and returns options to use with `Activate` methods.
```go
options, err := builder.Build()
```
More about perf event attributes on [linux MAN page](https://man7.org/linux/man-pages/man2/perf_event_open.2.html).

### Activate core event
To activate core event user need to provide on which CPU cores the event should be measured.
For example to create placement providers for cores 0, 2 and 4 use *NewCorePlacements* function.
```go
placements, err := iaevents.NewCorePlacements(perfEvent, 0, 2, 4)
```
The *Activate* method starts the underlying PMU counter and returns *ActiveEvent* for single placement.
```go
activeEvent, err := perfEvent.Activate(placement, process, options)
```

### Activate uncore event
Single uncore event can use several counters for every socket. The example are *SBO* events.
```
/sys/devices/uncore_sbox_0
/sys/devices/uncore_sbox_1
/sys/devices/uncore_sbox_2
/sys/devices/uncore_sbox_3
```
To activate uncore event a special *ActivateMulti* method should be used. To get all placements for event on given
socket use *NewUncoreAllPlacements* function. The below example shows how to start uncore event on socket 1.
```go
placements, err := iaevents.NewUncoreAllPlacements(perfEvent, 1)
activeMultiEvent, err := perfEvent.ActivateMulti(placements, process, options)
```

### Events grouping
An event group has one event which is the group leader. The leader is created first, and the rest of the group members
are created with subsequent `perf_event_open()` calls with *group_fd* being set to the file descriptor of the group
leader. An event group is scheduled onto the CPU as a unit: it will be put onto the CPU only if all of the events in
the group can be put onto the CPU. This means that the values of the member events can be meaningfully compared
(added, divided (to get ratios), and so on) with each other, since they have counted events for the same set of
executed instructions.
To activate set of events in single group user needs to use *ActivateGroup* function on selected perf events.
```go
activeEventGroup, err := iaevents.ActivateGroup(placement, process, perfEvents)
```

## Monitor events values
While configured events stay active, user can read measurements from them.
Depending on type of event, different monitors are returned by activators. It can be *ActiveEvent* for core event,
*ActiveMultiEvent* for uncore event or *ActiveEventGroup* for group of events.

### Read core event value
```go
value, err := activeEvent.ReadValue()
```

### Read uncore event value
For uncore event the *ReadValues* method returns slice of values for all placements on given multi PMU.
```go
values, err := activeMultiEvent.ReadValues()
```

### Read events group
For group of events user has to iterate through every event in the group and read the value separately.
```go
for _, activeEvent := range activeEventGroup.Events() {
    value, err := activeEvent.ReadValue()
}
```

### Counter value
Obtained measurements are stored in *CounterValue* structure as three values:
* Raw
* Enabled
* Running

*Raw* is a total count of event. *Enabled* and *running* are total time the event was enabled and running.
Normally these are the same. If more events are started than available counter slots on the PMU, then multiplexing
occurs and events only run part of the time. In that case the *enabled* and *running* values can be used to scale
an estimated value for the counter.

For that case there is also an *EventScaledValue* method which calculates scaled value as: 
`scaled = raw * enabled / running` and returns the result as `big.Int`.

To aggregate slice of values the library provides function *AggregateValues*, which helps to aggregate the results across
PMU units or multiple cores.

## Deactivate events
To finish work with library all active events should be deactivated to close corresponding file descriptors.
All types of active events have method *Deactivate*. After deactivation PMU counters are stopped and user can not
read new values.
```go
err := activeEvent.Deactivate()
activeMultiEvent.Deactivate()
activeEventGroup.Deactivate()
```

## Example application
Along with the library there is provided example application `iastat` to show usage of `iaevents` package.
It has features to list the events read from file and gather measurements with configured options like interval
or aggregation.

Build application:
```
make example
```
More info:
```
./iastat -help
```
Usage:
```
./iastat [-file <file>] -listevents
./iastat [-file <file>] -events <events> -cpu <CPUs> [-interval <interval>] [-loop <N>] [-no-aggr] [-raw] [-merge]
```

## Licensing
iaevents is distributed under the Apache License, Version 2.0.
