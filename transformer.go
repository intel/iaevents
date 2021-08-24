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
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"
)

type TransformationError struct {
	errors []error
}

func (t *TransformationError) Error() string {
	strErrors := make([]string, len(t.errors))
	for i, err := range t.errors {
		strErrors[i] = err.Error()
	}
	return strings.Join(strErrors, ", ")
}

func (t *TransformationError) Errors() []error {
	return t.errors
}

func (t *TransformationError) add(err error) {
	t.errors = append(t.errors, err)
}

// Transformer interface to provide perf events
type Transformer interface {
	Transform(reader Reader, matcher Matcher) ([]*PerfEvent, error)
}

// PerfTransformer uses reader and matcher to provide perf events
type PerfTransformer struct {
	io             ioHelper
	utils          resolverUtils
	parseJSONEvent parseJSONEventFunc
}

type resolverUtils interface {
	parseTerms(event *PerfEvent, config string) error
	getPMUType(pmu string) (uint32, error)
	isPMUUncore(pmu string) (bool, error)
}

type parseJSONEventFunc func(*JSONEvent) (*eventDefinition, error)

type eventDefinition struct {
	Name       string
	Desc       string
	Event      string
	PMU        string
	Deprecated bool
}

type resolverUtilsImpl struct {
	io ioHelper
}

type eventTerm struct {
	name  string
	value uint64
}

const (
	sysDevicesPath = "/sys/devices/"
)

// NewPerfTransformer returns new PerfTransformer with default members
func NewPerfTransformer() *PerfTransformer {
	io := &ioHelperImpl{}
	return &PerfTransformer{
		io:             io,
		utils:          &resolverUtilsImpl{io: io},
		parseJSONEvent: parseJSONEventImpl,
	}
}

// Transform transforms data from reader to perf events
func (t *PerfTransformer) Transform(reader Reader, matcher Matcher) ([]*PerfEvent, error) {
	if reader == nil || matcher == nil {
		return nil, errors.New("failed to transform, Reader and/or Matcher are nil")
	}
	transformationErr := &TransformationError{}

	jsonEvents, err := reader.Read()
	if err != nil {
		return nil, fmt.Errorf("failed to read JSON Events: %v", err)
	}
	if len(jsonEvents) == 0 {
		return nil, errors.New("reader returned no events to transform")
	}

	var perfEvents []*PerfEvent
	for _, jsonEvent := range jsonEvents {
		if jsonEvent == nil {
			continue
		}
		match, err := matcher.Match(jsonEvent)
		if err != nil {
			return nil, fmt.Errorf("matcher error for event `%s`: %v", jsonEvent.EventName, err)
		}
		if !match {
			continue
		}
		jsonEventParsed, err := t.parseJSONEvent(jsonEvent)
		if err != nil {
			transformationErr.add(fmt.Errorf("failed to parse json event `%s`: %v", jsonEvent.EventName, err))
			continue
		}
		resolvedEvent, err := t.resolve(jsonEventParsed)
		if err != nil {
			transformationErr.add(fmt.Errorf("failed to transform event `%s`: %v", jsonEventParsed.Name, err))
			continue
		}
		perfEvents = append(perfEvents, resolvedEvent)
	}
	if transformationErr.Errors() == nil {
		return perfEvents, nil
	}
	return perfEvents, transformationErr
}

// resolve - resolve PMU Event from eventData to perf attributes
func (t *PerfTransformer) resolve(eventData *eventDefinition) (*PerfEvent, error) {
	if eventData == nil {
		return nil, errors.New("eventData is nil, nothing to resolve")
	}

	pmuTypes, err := t.findPMUs(eventData.PMU)
	if err != nil {
		return nil, fmt.Errorf("failed to find PMU type %s: %v", eventData.PMU, err)
	}
	pmuRealName := pmuTypes[0].Name
	pmuType := pmuTypes[0].PMUType

	event := NewPerfEvent()
	event.Name = eventData.Name
	event.PMUName = eventData.PMU
	event.PMUTypes = pmuTypes
	event.Attr.Type = pmuType
	event.Uncore, err = t.utils.isPMUUncore(pmuRealName)
	if err != nil {
		return nil, err
	}
	event.eventFormat = eventData.Event

	err = t.utils.parseTerms(event, eventData.Event)
	if err != nil {
		return nil, err
	}
	return event, nil
}

func (t *PerfTransformer) findPMUs(pmuName string) ([]NamedPMUType, error) {
	pmus := []NamedPMUType{}
	paths := []string{}
	realPMU := pmuName
	pmuType, err := t.utils.getPMUType(realPMU)
	if err != nil {
		realPMU = "uncore_" + pmuName
		pmuType, err = t.utils.getPMUType(realPMU)
		if err != nil {
			paths, err = t.findMultiPMUPaths(pmuName)
			if err != nil {
				return nil, err
			}
			if len(paths) == 0 {
				return nil, errors.New("cannot find PMU for: " + pmuName)
			}
			realPMU = filepath.Base(paths[0])
			pmuType, err = t.utils.getPMUType(realPMU)
			if err != nil {
				return nil, fmt.Errorf("cannot find PMU type for %s: %v", realPMU, err)
			}
		}
	}
	pmus = append(pmus, NamedPMUType{Name: realPMU, PMUType: pmuType})

	for i := 1; i < len(paths); i++ {
		realPMU = filepath.Base(paths[i])
		pmuType, err = t.utils.getPMUType(realPMU)
		if err != nil {
			return nil, fmt.Errorf("cannot find PMU type for %s: %v", realPMU, err)
		}
		pmus = append(pmus, NamedPMUType{Name: realPMU, PMUType: pmuType})
	}

	return pmus, nil
}

func (t *PerfTransformer) findMultiPMUPaths(pmuName string) ([]string, error) {
	// Common part of PMU path, the remaining part should be a decimal number
	commonPart := fmt.Sprintf("%suncore_%s_", sysDevicesPath, pmuName)
	// Glob guarantees there is one digit after common part, the remainder requires additional verification
	pattern := fmt.Sprintf("%s[0-9]*", commonPart)
	globAssuredLen := len(commonPart) + 1
	candidatePaths, err := t.io.glob(pattern)
	if err != nil {
		return nil, fmt.Errorf("glob failed for pattern %s: %v", pattern, err)
	}
	verifiedPaths := []string{}
	for _, path := range candidatePaths {
		// Verify that there are only digits after part assured by glob
		if func() bool {
			for i := globAssuredLen; i < len(path); i++ {
				if path[i] < '0' || path[i] > '9' {
					return false
				}
			}
			return true
		}() {
			verifiedPaths = append(verifiedPaths, path)
		}
	}

	return verifiedPaths, nil
}

// parseTerms - parse event config using PMU format
func (u *resolverUtilsImpl) parseTerms(event *PerfEvent, terms string) error {
	if event == nil || event.Attr == nil {
		return errors.New("perf event is nil or not initialized correctly")
	}
	termsSet, err := u.splitEventTerms(terms)
	if err != nil {
		return fmt.Errorf("failed to parse event terms %s: %v", terms, err)
	}
	for _, term := range termsSet {
		if u.specialAttributes(event.Attr, term.name, term.value) {
			continue
		}
		err = u.parseTerm(event, term)
		if err != nil {
			return fmt.Errorf("failed to parse term %s: %v", term.name, err)
		}
	}

	return nil
}

// Split terms from single string into name/value pairs
func (resolverUtilsImpl) splitEventTerms(terms string) ([]*eventTerm, error) {
	splittedTerms := strings.Split(terms, ",")
	eventTerms := []*eventTerm{}
	for _, term := range splittedTerms {
		splittedTerm := strings.Split(term, "=")
		if len(splittedTerm) < 2 {
			return nil, fmt.Errorf("cannot parse term: %s", term)
		}
		value, err := strconv.ParseUint(splittedTerm[1], 0, 64)
		if err != nil {
			return nil, fmt.Errorf("cannot parse term: %s: %v", term, err)
		}
		eventTerms = append(eventTerms, &eventTerm{name: splittedTerm[0], value: value})
	}
	return eventTerms, nil
}

func (u *resolverUtilsImpl) parseTerm(event *PerfEvent, term *eventTerm) error {
	// Read kernel format from /sys/devices/{pmu}/format/{name}
	formatPath := filepath.Join(sysDevicesPath, event.PMUTypes[0].Name, "format", term.name)
	content, err := u.io.readAll(formatPath)
	if err != nil {
		return fmt.Errorf("cannot open kernel format %s: %v", formatPath, err)
	}
	configFormat := strings.TrimSpace(string(content))

	// Example of config format to parse: "config:0-7,21"
	configName, bitsFormat, err := splitConfigFormat(configFormat)
	if err != nil {
		return fmt.Errorf("cannot parse kernel format %s for %s: %v", configFormat, term.name, err)
	}
	configValue, err := u.formatValue(bitsFormat, term.value)
	if err != nil {
		return fmt.Errorf("cannot parse term %s=%d using format %s: %v", term.name, term.value, bitsFormat, err)
	}

	switch configName {
	case "config":
		event.Attr.Config |= configValue
	case "config1":
		event.Attr.Ext1 |= configValue
	case "config2":
		event.Attr.Ext2 |= configValue
		event.Attr.Size = unix.PERF_ATTR_SIZE_VER1
	default:
		return fmt.Errorf("unsupported config name %s for %s", configName, term.name)
	}

	return nil
}

func splitConfigFormat(configFormat string) (configName string, format string, err error) {
	splittedFormat := strings.Split(configFormat, ":")
	if len(splittedFormat) < 2 {
		return "", "", fmt.Errorf("invalid kernel format %s", configFormat)
	}
	configName = splittedFormat[0]
	format = splittedFormat[1]
	return configName, format, nil
}

// Place the value into the bits specified by format
func (resolverUtilsImpl) formatValue(format string, value uint64) (uint64, error) {
	remainingValue := value
	formattedValue := uint64(0)
	// Example of format to parse: "0-7,21"
	bitsFormats := strings.Split(format, ",")
	for _, bitsFormat := range bitsFormats {
		bits, start, bitLen, err := parseBitFormat(bitsFormat)
		if err != nil {
			return 0, fmt.Errorf("cannot parse kernel format %s: %v", format, err)
		}
		formattedValue = formattedValue | ((remainingValue & bits) << start)
		remainingValue = remainingValue >> bitLen
	}
	return formattedValue, nil
}

// Calculate bit mask with specified number of bits set to 1. (without shift)
// Allowed input formats: "2-8", "4" etc.
func parseBitFormat(bits string) (mask uint64, start uint64, bitsNum uint64, err error) {
	splittedBits := strings.Split(bits, "-")
	start, err = strconv.ParseUint(splittedBits[0], 0, 64)
	if err != nil {
		return 0, 0, 0, err
	}
	end := start
	if len(splittedBits) > 1 {
		end, err = strconv.ParseUint(splittedBits[1], 0, 64)
		if err != nil {
			return 0, 0, 0, err
		}
	}
	if start > end {
		return 0, 0, 0, fmt.Errorf("start is bigger than end in bit format `%s`", bits)
	}
	bitsNum = end - start + 1
	if bitsNum > 64 {
		return 0, 0, 0, fmt.Errorf("number of bits is more than expected (max 64) for config `%s`", bits)
	}
	if bitsNum == 64 {
		mask = ^uint64(0)
	} else {
		mask = (uint64(1) << bitsNum) - 1
	}
	return mask, start, bitsNum, nil
}

func (resolverUtilsImpl) specialAttributes(attr *unix.PerfEventAttr, name string, val uint64) bool {
	if name == "period" {
		attr.Sample = val
	} else if name == "freq" {
		attr.Sample = val
		attr.Bits |= unix.PerfBitFreq
	} else {
		return false
	}
	return true
}

func (u *resolverUtilsImpl) getPMUType(pmu string) (uint32, error) {
	filePath := filepath.Join(sysDevicesPath, pmu, "type")
	var val uint32
	_, n, err := u.io.scanOneValue(filePath, "%d", &val)
	if err != nil {
		return 0, err
	}
	if n != 1 {
		return 0, errors.New("could not read type for PMU: " + pmu)
	}
	return val, nil
}

func (u *resolverUtilsImpl) isPMUUncore(pmu string) (bool, error) {
	cpumaskPath := filepath.Join(sysDevicesPath, pmu, "cpumask")
	if err := u.io.validateFile(cpumaskPath); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	var cpus int
	opened, n, err := u.io.scanOneValue(cpumaskPath, "%d", &cpus)
	if !opened && err != nil {
		return false, fmt.Errorf("failed to open for uncore check, file %s: %v", cpumaskPath, err)
	}
	if err == nil && n == 1 {
		return cpus == 0, nil
	}
	// If file exists and was opened then any other problem with format
	// means that it is not an uncore event
	return false, nil
}

func parseJSONEventImpl(jsonEvent *JSONEvent) (*eventDefinition, error) {
	var unitToPMU = map[string]string{
		"cbo":    "cbox",
		"qpi ll": "qpi",
		"sbo":    "sbox",
		"imph-u": "cbox",
		"ncu":    "cbox",
		"upi ll": "upi",
	}
	var msrMap = map[string]string{
		"0x3f6": "ldlat=",
		"0x1a6": "offcore_rsp=",
		"0x1a7": "offcore_rsp=",
		"0x3f7": "frontend=",
	}
	// Handle different fixed counter encodings between JSON and perf
	var fixedEvents = map[string]string{
		"inst_retired.any":            "event=0xc0",
		"cpu_clk_unhalted.thread":     "event=0x3c",
		"cpu_clk_unhalted.thread_any": "event=0x3c,any=1",
	}

	eventData := &eventDefinition{}
	eventData.Name = strings.ToUpper(jsonEvent.EventName)
	eventData.Desc = jsonEvent.BriefDescription

	if jsonEvent.EventCode == "" {
		return eventData, errors.New("missing EventCode for event: " + eventData.Name)
	}

	var eventCode uint64
	var event []string
	fixedEventCode := strings.Split(jsonEvent.EventCode, ",")[0]
	code, err := strconv.ParseUint(fixedEventCode, 0, 64)
	if err != nil {
		return eventData, fmt.Errorf("failed to parse EventCode `%s` into UInt: %v", fixedEventCode, err)
	}
	eventCode = code

	if jsonEvent.ExtSel != "" {
		code, err := strconv.ParseUint(jsonEvent.ExtSel, 0, 64)
		if err != nil {
			return eventData, fmt.Errorf("failed to parse ExtSel `%s` into UInt: %v", jsonEvent.ExtSel, err)
		}
		eventCode = eventCode | (code << 21)
	}
	if err := validateEntry(jsonEvent.Unit, "Unit"); err != nil {
		return eventData, err
	}
	if jsonEvent.Unit == "" {
		eventData.PMU = "cpu"
	} else {
		unit := strings.ToLower(jsonEvent.Unit)
		val, exist := unitToPMU[unit]
		if exist {
			eventData.PMU = val
		} else {
			eventData.PMU = unit
		}
	}
	if jsonEvent.Deprecated == "1" {
		eventData.Deprecated = true
	}

	// Check if there is fixed encoding for the event
	fixedEvent, exist := fixedEvents[strings.ToLower(eventData.Name)]
	if exist {
		eventData.Event = fixedEvent
		return eventData, nil
	}

	if entryNotZero(jsonEvent.UMask) {
		value := strings.Split(jsonEvent.UMask, ",")[0]
		event = append(event, "umask="+value)
	}
	if entryNotZero(jsonEvent.CounterMask) {
		value := strings.Split(jsonEvent.CounterMask, ",")[0]
		event = append(event, "cmask="+value)
	}
	if entryNotZero(jsonEvent.Invert) {
		value := strings.Split(jsonEvent.Invert, ",")[0]
		event = append(event, "inv="+value)
	}
	if entryNotZero(jsonEvent.AnyThread) {
		value := strings.Split(jsonEvent.AnyThread, ",")[0]
		event = append(event, "any="+value)
	}
	if entryNotZero(jsonEvent.EdgeDetect) {
		value := strings.Split(jsonEvent.EdgeDetect, ",")[0]
		event = append(event, "edge="+value)
	}
	if entryNotZero(jsonEvent.SampleAfterValue) {
		value := strings.Split(jsonEvent.SampleAfterValue, ",")[0]
		event = append(event, "period="+value)
	}

	event = append(event, fmt.Sprintf("event=%#x", eventCode))

	if entryNotZero(jsonEvent.MSRIndex) {
		msrIndex := strings.ToLower(strings.Split(jsonEvent.MSRIndex, ",")[0])
		msr, exist := msrMap[msrIndex]
		if exist {
			event = append(event, msr+jsonEvent.MSRValue)
		}
	}

	eventData.Event = strings.Join(event, ",")
	return eventData, nil
}

func entryNotZero(entry string) bool {
	return entry != "" && entry != "0" && entry != "0x0" && entry != "0x00"
}

func validateEntry(entry string, name string) error {
	const maxEntryLen = 128
	if len(entry) > maxEntryLen {
		return fmt.Errorf("JSON entry %s=`%s` is too long, max length = %d", name, entry, maxEntryLen)
	}
	if strings.ContainsAny(entry, ".*/") {
		return fmt.Errorf("JSON entry %s=`%s` contains illegal character(s)", name, entry)
	}
	return nil
}

func appendQualifiers(event string, qualifiers []string) string {
	if len(qualifiers) == 0 {
		return event
	}
	if len(event) >= 1 && event[len(event)-1] != '/' {
		return fmt.Sprintf("%s/%s", event, strings.Join(qualifiers, ","))
	}
	return fmt.Sprintf("%s%s", event, strings.Join(qualifiers, ","))
}
