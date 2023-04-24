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

// Example program to show usage of iaevents package

package main

import (
	"errors"
	"flag"
	"fmt"
	"math/big"
	"os"
	"os/signal"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/intel/iaevents"
)

const (
	defaultFilesDir = ".cache/pmu-events"
)

type cpusList []int

type event struct {
	name          string
	attrModifiers []string
	perfEvent     *iaevents.PerfEvent
}

type group struct {
	events []event
}

type eventValue struct {
	activeEvent *iaevents.ActiveEvent
	value       iaevents.CounterValue
}

type eventMultiValue struct {
	activeMultiEvent *iaevents.ActiveMultiEvent
	values           []iaevents.CounterValue
}

type eventPrinter struct {
	aggr  bool
	raw   bool
	merge bool
}

func printHelp() {
	printLine(os.Args[0], "[-file <file>] -listevents")
	printLine(os.Args[0], "[-file <file>] -events <events> -cpu <CPUs> [-interval <interval>] [-loop <N>] [-no-aggr] [-raw] [-merge]")
	flag.PrintDefaults()
}

func main() {
	help := flag.Bool("help", false, "Print help and exit.")
	filePath := flag.String("file", "", "JSON file with events definition (optional).")
	listOption := flag.Bool("listevents", false, "List all events from pmu-events JSON file.")
	eventNames := flag.String("events", "", "Comma separated list of events to measure. Use {} for groups.")
	interval := flag.Uint("interval", 1000, "Print events every <interval> ms.")
	count := flag.Uint("loop", 0, "Repeat the measure only <loop> times. (default 0 = infinite)")
	var cpus cpusList
	flag.Var(&cpus, "cpu", "Comma separated list of CPUs to measure or ranges a-b. (default All)")
	var sockets cpusList
	flag.Var(&sockets, "socket", "Comma separated list of sockets to measure for uncore events. (default All)")
	noAggregate := flag.Bool("no-aggr", false, "Print values for individual CPUs.")
	printRaw := flag.Bool("raw", false, "Print raw values used for scaling.")
	merge := flag.Bool("merge", false, "Sum multiple instances of uncore events.")
	flag.Parse()

	if len(os.Args) < 2 {
		printHelp()
		os.Exit(1)
	}
	if *help {
		printHelp()
		os.Exit(0)
	}
	// Equivalent of logical XOR
	// Only one of this options have to be set
	if *listOption != (*eventNames == "") {
		printHelp()
		os.Exit(1)
	}

	// Reader is an interface used by Transformer to read JSON Events, typically from file(s)
	reader := iaevents.NewFilesReader()
	// If filePath is empty try to read events from default location for current CPU
	if *filePath == "" {
		cpuid, err := iaevents.CheckCPUid()
		if err != nil {
			printLine(err)
			os.Exit(1)
		}
		homePath := os.Getenv("HOME")
		if homePath == "" {
			printLine("Failed to read {$HOME} variable, do not know where to look for JSON files.",
				"Please provide file with events definition.")
			os.Exit(1)
		}
		defaultPath := fmt.Sprintf("%s/%s", homePath, defaultFilesDir)
		err = iaevents.AddDefaultFiles(reader, defaultPath, cpuid)
		if err != nil {
			printLine(err)
			os.Exit(1)
		}
	} else { // Read events from user provided file
		err := reader.AddFiles(*filePath)
		if err != nil {
			printLine(err)
			printHelp()
			os.Exit(1)
		}
	}

	// Print perf format of all events provided by reader
	if *listOption {
		err := listEvents(reader)
		if err != nil {
			printLine(err)
			os.Exit(1)
		}
		os.Exit(0)
	}

	// Option `events` requires a parameter
	if len(*eventNames) == 0 || (*eventNames)[0] == '-' {
		printHelp()
		os.Exit(1)
	}
	// If `cpu` option was omitted set to all available CPUs
	if len(cpus) == 0 {
		var err error // Create local variable for `err` to avoid declaring `cpus` in local scope
		cpus, err = iaevents.AllCPUs()
		if err != nil {
			printf("Failed to get all available CPUs : %v.\n", err)
			os.Exit(1)
		}
	}
	// If `socket` option was omitted set to all available sockets
	if len(sockets) == 0 {
		var err error // Create local variable for `err` to avoid declaring `sockets` in local scope
		sockets, err = iaevents.AllSockets()
		if err != nil {
			printf("Failed to get all available sockets: %v.\n", err)
			os.Exit(1)
		}
	}

	// Parse the input events to find out groups and separate event names from custom modifiers
	singles, groups, err := parseEvents(*eventNames)
	if err != nil {
		printf("Failed to parse event names: %v.\n", err)
		printHelp()
		os.Exit(1)
	}

	// Transform the provided events into perf format,
	// transformer resolves the event definition provided by reader into perf event attributes.
	// Only events that meet the criteria provided by matcher are processed.
	transformer := iaevents.NewPerfTransformer()
	for i := 0; i < len(singles); i++ {
		perfEvents, err := transformer.Transform(reader, iaevents.NewNameMatcher(singles[i].name))
		if err != nil {
			printf("Failed to transform to Perf events: %v.\n", err)
			os.Exit(1)
		}
		if len(perfEvents) < 1 {
			printf("Failed to resolve unknown event `%s`.\n", singles[i].name)
			os.Exit(1)
		}
		singles[i].perfEvent = perfEvents[0]
	}
	for i := 0; i < len(groups); i++ {
		for j := 0; j < len(groups[i].events); j++ {
			perfEvents, err := transformer.Transform(reader, iaevents.NewNameMatcher(groups[i].events[j].name))
			if err != nil {
				printf("Failed to transform to Perf events: %v.\n", err)
				os.Exit(1)
			}
			if len(perfEvents) < 1 {
				printf("Failed to resolve unknown event `%s`.\n", groups[i].events[j].name)
				os.Exit(1)
			}
			groups[i].events[j].perfEvent = perfEvents[0]
		}
	}

	// Activate all events, we need lists of core events and uncore events
	activeEvents := make(map[string][]*eventValue)           // map perf event to its activated instances
	activeEventNames := []string{}                           // slice of ordered names for activeEvents map
	activeMultiEvents := make(map[string][]*eventMultiValue) // map uncore perf event to its activated instances
	activeMultiEventNames := []string{}                      // slice of ordered names for activeMultiEvents map
	process := iaevents.NewEventTargetProcess(-1, 0)         // -1 for system wide monitoring
	// Activate single events, both core and uncore
	for _, event := range singles {
		perfEvent := event.perfEvent
		name := perfEvent.Name
		options, err := iaevents.NewOptions().SetAttrModifiers(event.attrModifiers).Build()
		if err != nil {
			printf("Failed to set modifiers for event %s: %v.\n", name, err)
			os.Exit(1)
		}
		if perfEvent.Uncore {
			eventMultiValues, err := activateUncoreEvent(sockets, perfEvent, process, options)
			if err != nil {
				printf("Failed to activate uncore event %s: %v.\n", name, err)
				os.Exit(1)
			}
			activeMultiEvents[name] = append(activeMultiEvents[name], eventMultiValues...)
			activeMultiEventNames = append(activeMultiEventNames, name)
			continue
		}
		eventValues, err := activateCoreEvent(cpus, perfEvent, process, options)
		if err != nil {
			printf("Failed to activate event %s: %v.\n", name, err)
			os.Exit(1)
		}
		activeEvents[name] = append(activeEvents[name], eventValues...)
		activeEventNames = append(activeEventNames, name)
	}
	// Activate events in groups. Grouping is supported only for core events.
	for _, group := range groups {
		customEvents, err := group.customEvents()
		if err != nil {
			printf("Error: %v.\n", err)
			os.Exit(1)
		}
		if len(customEvents) < 1 {
			continue
		}
		for _, customEvent := range customEvents {
			if customEvent.Event.Uncore {
				printf("Uncore event found in a group: %s. Grouping of uncore events is not supported!\n",
					customEvent.Event.Name)
				os.Exit(1)
			}
			activeEventNames = append(activeEventNames, customEvent.Event.Name)
		}
		err = activateGroup(cpus, customEvents, process, activeEvents)
		if err != nil {
			printf("Failed to activate group %v: %v.\n", group.names(), err)
			os.Exit(1)
		}
	}

	// Now all events are active, which means that file descriptors are opened and ready to read values.
	// Prepare cleanUp function to deactivate all events.
	cleanUp := func() {
		for _, val := range activeEvents {
			for _, event := range val {
				_ = event.activeEvent.Deactivate()
			}
		}
		for _, val := range activeMultiEvents {
			for _, multiEvent := range val {
				multiEvent.activeMultiEvent.Deactivate()
			}
		}
	}
	// Close all file descriptors once main() will finish
	defer cleanUp()

	printer := newEventPrinter(!*noAggregate, *printRaw, *merge)
	// Print correct header once
	printer.printHeader()

	// Handle signals and do cleanup on interrupt
	interruptChan := make(chan os.Signal, 2)
	signal.Notify(interruptChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-interruptChan
		os.Exit(1)
	}()

	// Condition for main reading loop
	// For count > 0 do `count` loops, if count == 0 loop is infinite
	condition := func(i uint) bool {
		if *count > 0 {
			return i < *count
		}
		return true
	}
	for i := uint(0); condition(i); i++ {
		time.Sleep(time.Duration(*interval) * time.Millisecond)

		// Read values of all configured events, both core and uncore.
		for name, activeList := range activeEvents {
			err := readValues(activeList)
			if err != nil {
				printf("Error while reading values of event %s: %v.\n", name, err)
				continue
			}
		}
		for name, activeMultiList := range activeMultiEvents {
			err := readMultiValues(activeMultiList)
			if err != nil {
				printf("Error while reading values of event %s: %v.\n", name, err)
				continue
			}
		} // End of reading part.

		// Print core events
		for _, name := range activeEventNames {
			activeList, exist := activeEvents[name]
			if !exist {
				printf("No data for event %s.\n", name)
				continue
			}
			printer.printCore(name, activeList)
		}
		// Print uncore events
		for _, name := range activeMultiEventNames {
			activeMultiList, exist := activeMultiEvents[name]
			if !exist {
				printf("No data for event %s.\n", name)
				continue
			}
			printer.printUncore(name, activeMultiList)
		}
	}

	// Stop relaying incoming signals to chan
	signal.Stop(interruptChan)
}

// Just to satisfy interface required by flag for Var types
func (s *cpusList) String() string {
	return fmt.Sprintf("%v", *s)
}

// Set CPUs list from user provided config
func (s *cpusList) Set(value string) error {
	valList := strings.Split(value, ",")
	for _, val := range valList {
		var start, end uint
		// Support for ranges a-b
		n, err := fmt.Sscanf(val, "%d-%d", &start, &end)
		if err == nil && n == 2 {
			for ; start <= end; start++ {
				*s = append(*s, int(start))
			}
			continue
		}
		// Single value
		num, err := strconv.ParseUint(val, 0, 32)
		if err != nil {
			return fmt.Errorf("wrong format for cpu number `%s`: %w", val, err)
		}
		*s = append(*s, int(num))
	}
	return nil
}

func (g *group) names() []string {
	names := []string{}
	for _, event := range g.events {
		names = append(names, event.name)
	}
	return names
}

func (g *group) customEvents() ([]iaevents.CustomizableEvent, error) {
	customEvents := []iaevents.CustomizableEvent{}
	for _, event := range g.events {
		options, err := iaevents.NewOptions().SetAttrModifiers(event.attrModifiers).Build()
		if err != nil {
			return nil, fmt.Errorf("failed to set modifiers for event %s: %w", event.name, err)
		}
		customEvents = append(customEvents, iaevents.CustomizableEvent{Event: event.perfEvent, Options: options})
	}
	return customEvents, nil
}

func readValues(events []*eventValue) (err error) {
	for _, event := range events {
		event.value, err = event.activeEvent.ReadValue()
		if err != nil {
			return err
		}
	}
	return nil
}

func readMultiValues(multiEvents []*eventMultiValue) (err error) {
	for _, event := range multiEvents {
		event.values, err = event.activeMultiEvent.ReadValues()
		if err != nil {
			return err
		}
	}
	return nil
}

// List events along with their perf format, sort the output
func listEvents(reader iaevents.Reader) error {
	// Matcher that will match all events
	matcher := iaevents.NewNameMatcher()
	events, err := iaevents.NewPerfTransformer().Transform(reader, matcher)
	if err != nil {
		return err
	}
	sort.Slice(events, func(i, j int) bool {
		return events[i].Name < events[j].Name
	})
	for _, event := range events {
		printf("%-50v %v\n", event.Name, event.Decoded())
	}
	return nil
}

func newEvent(definition string) event {
	parts := strings.Split(definition, ":")
	event := event{name: parts[0]}
	if len(parts) > 1 {
		event.attrModifiers = parts[1:]
	}
	return event
}

// Parse event names from user input, uses for {} groups of events
func parseEvents(names string) (singles []event, groups []group, err error) {
	separatedNames := strings.Split(names, ",")
	currentToAdd := &singles
	inGroup := false
	for _, name := range separatedNames {
		name = strings.TrimSpace(name)
		if strings.HasPrefix(name, "{") {
			if inGroup {
				return nil, nil, errors.New("use of `{` before closing previous group")
			}
			inGroup = true
			name = strings.TrimPrefix(name, "{")
			currentToAdd = &[]event{} // New group of names
		}
		if strings.HasSuffix(name, "}") {
			if !inGroup {
				return nil, nil, errors.New("use of `}` without opening `{`")
			}
			inGroup = false
			name = strings.TrimSuffix(name, "}")
			*currentToAdd = append(*currentToAdd, newEvent(name))
			group := group{events: *currentToAdd}
			groups = append(groups, group)
			currentToAdd = &singles
			continue
		}
		*currentToAdd = append(*currentToAdd, newEvent(name))
	}
	if inGroup {
		return nil, nil, errors.New("group not correctly closed with `}`")
	}
	return singles, groups, nil
}

func newEventPrinter(aggregate bool, raw bool, merge bool) *eventPrinter {
	return &eventPrinter{
		aggr:  aggregate,
		raw:   raw,
		merge: merge,
	}
}

func (ep *eventPrinter) printHeader() {
	// Print header
	// If there are more events than counters, the kernel uses time multiplexing. With multiplexing,
	// at the end of the run, the counter is scaled basing on total time enabled vs time running.
	// scaled_value = raw_value * time_enabled/time_running
	// Print all values only if `printRaw` is set. Print CPU number only if `noAggregate` if set.
	if !ep.aggr && ep.raw {
		printf("cpu %-45v: %-15v %-15v %-15v %-15v\n", "event name", "scaled_value", "raw_value", "time_enabled", "time_running")
	} else if ep.aggr && ep.raw {
		printf("%-45v: %-15v %-15v %-15v %-15v\n", "event name", "scaled_value", "raw_value", "time_enabled", "time_running")
	} else if !ep.aggr && !ep.raw {
		printf("cpu %-45v: value\n", "event name")
	} else { // aggr && !raw
		printf("%-45v: value\n", "event name")
	}
}

// Aggregate values and calculate total scaled value by scaling every single value separately
func mergeValues(values []iaevents.CounterValue) (scaled *big.Int, raw *big.Int, enabled *big.Int, running *big.Int) {
	scaled = new(big.Int).SetUint64(0)
	for _, value := range values {
		oneScaled := iaevents.EventScaledValue(value)
		scaled = scaled.Add(scaled, oneScaled)
	}
	raw, enabled, running = iaevents.AggregateValues(values)
	return scaled, raw, enabled, running
}

// Print value aggregated per CPU (or socket)
func (ep *eventPrinter) printAggr(name string, values []iaevents.CounterValue) {
	scaled, raw, enabled, running := mergeValues(values)
	if ep.raw {
		printf("%-45v: %15d %15d %15d %15d\n", name, scaled, raw, enabled, running)
	} else {
		printf("%-45v: %d\n", name, scaled)
	}
}

// Print value per single CPU, provide CPU number in output
func (ep *eventPrinter) printValue(name string, cpu int, values ...iaevents.CounterValue) {
	scaled, raw, enabled, running := mergeValues(values)
	if ep.raw {
		printf("%3d %-45v: %15v %15d %15d %15d\n", cpu, name, scaled, raw, enabled, running)
	} else {
		printf("%3d %-45v: %d\n", cpu, name, scaled)
	}
}

// Print core event with different level of aggregation
func (ep *eventPrinter) printCore(name string, events []*eventValue) {
	if !ep.aggr {
		ep.printCoreEvent(name, events)
	} else {
		ep.printCoreEventAggr(name, events)
	}
}

// Print single core event
func (ep *eventPrinter) printCoreEvent(name string, events []*eventValue) {
	for _, event := range events {
		cpu, _ := event.activeEvent.PMUPlacement()
		ep.printValue(name, cpu, event.value)
	}
}

// Print core events aggregated (over CPUs)
func (ep *eventPrinter) printCoreEventAggr(name string, events []*eventValue) {
	var values []iaevents.CounterValue
	for _, event := range events {
		values = append(values, event.value)
	}
	ep.printAggr(name, values)
}

// Print uncore event with different level of aggregation
func (ep *eventPrinter) printUncore(name string, multiEvents []*eventMultiValue) {
	if !ep.aggr && ep.merge {
		ep.printUncoreEventMerge(name, multiEvents)
	} else if ep.aggr && ep.merge {
		ep.printUncoreEventMergeAggr(name, multiEvents)
	} else if !ep.aggr && !ep.merge {
		ep.printUncoreEvent(name, multiEvents)
	} else { // aggr && !merge
		ep.printUncoreEventAggr(name, multiEvents)
	}
}

// Print uncore event, print multi events separately
func (ep *eventPrinter) printUncoreEvent(name string, multiEvents []*eventMultiValue) {
	for _, multiEvent := range multiEvents {
		events := multiEvent.activeMultiEvent.Events()
		values := multiEvent.values
		multiEventLen := len(events)
		if multiEventLen != len(values) {
			printf("Error, different number of values for multi event: %s.\n", name)
			return
		}
		cpu, _ := events[0].PMUPlacement()
		for i := 0; i < multiEventLen; i++ {
			uncoreName := fmt.Sprintf("%s/%s", name, strings.TrimPrefix(events[i].PMUName(), "uncore_"))
			ep.printValue(uncoreName, cpu, values[i])
		}
	}
}

// Print uncore event aggregated across sockets, but not across multi PMU
func (ep *eventPrinter) printUncoreEventAggr(name string, multiEvents []*eventMultiValue) {
	multiEventLen := len(multiEvents[0].values)
	for i := 0; i < multiEventLen; i++ {
		var values []iaevents.CounterValue
		for _, event := range multiEvents {
			if len(event.values) != multiEventLen || len(event.activeMultiEvent.Events()) != multiEventLen {
				printf("Error, different sizes of multi event: %s.\n", name)
				continue
			}
			values = append(values, event.values[i])
		}
		pmuName := multiEvents[0].activeMultiEvent.Events()[i].PMUName()
		uncoreName := fmt.Sprintf("%s/%s", name, strings.TrimPrefix(pmuName, "uncore_"))
		ep.printAggr(uncoreName, values)
	}
}

// Print uncore event aggregated across multi PMU, but not across sockets
func (ep *eventPrinter) printUncoreEventMerge(name string, multiEvents []*eventMultiValue) {
	for _, event := range multiEvents {
		cpu, _ := event.activeMultiEvent.Events()[0].PMUPlacement()
		ep.printValue(name, cpu, event.values...)
	}
}

// Print uncore event aggregated across sockets and multi PMU
func (ep *eventPrinter) printUncoreEventMergeAggr(name string, multiEvents []*eventMultiValue) {
	var values []iaevents.CounterValue
	for _, event := range multiEvents {
		values = append(values, event.values...)
	}
	ep.printAggr(name, values)
}

// End of print functions

func activateCoreEvent(cpus []int, perfEvent *iaevents.PerfEvent, process iaevents.TargetProcess, options iaevents.Options) ([]*eventValue, error) {
	var eventValues []*eventValue
	placements, err := iaevents.NewCorePlacements(perfEvent, cpus[0], cpus[1:]...)
	if err != nil {
		return nil, err
	}
	for _, placement := range placements {
		activeEvent, err := perfEvent.Activate(placement, process, options)
		if err != nil {
			return nil, err
		}
		eventValues = append(eventValues, &eventValue{activeEvent: activeEvent})
	}
	return eventValues, nil
}

func activateUncoreEvent(sockets []int, perfEvent *iaevents.PerfEvent, process iaevents.TargetProcess, options iaevents.Options) ([]*eventMultiValue, error) {
	var eventMultiValues []*eventMultiValue
	for _, socket := range sockets {
		placements, err := iaevents.NewUncoreAllPlacements(perfEvent, socket)
		if err != nil {
			return nil, err
		}
		activeMultiEvent, err := perfEvent.ActivateMulti(placements, process, options)
		if err != nil {
			return nil, err
		}
		eventMultiValues = append(eventMultiValues, &eventMultiValue{activeMultiEvent: activeMultiEvent})
	}
	return eventMultiValues, nil
}

// Activate group and add activated events to namedValues map
func activateGroup(cpus []int, events []iaevents.CustomizableEvent, process iaevents.TargetProcess, namedValues map[string][]*eventValue) error {
	placements, err := iaevents.NewCorePlacements(events[0].Event, cpus[0], cpus[1:]...)
	if err != nil {
		return err
	}
	for _, placement := range placements {
		activeGroup, err := iaevents.ActivateGroup(placement, process, events)
		if err != nil {
			return err
		}
		for _, activeEvent := range activeGroup.Events() {
			name := activeEvent.PerfEvent.Name
			namedValues[name] = append(namedValues[name], &eventValue{activeEvent: activeEvent})
		}
	}
	return nil
}

func printLine(a ...interface{}) {
	_, _ = fmt.Println(a...)
}

func printf(format string, a ...interface{}) {
	_, _ = fmt.Printf(format, a...)
}
