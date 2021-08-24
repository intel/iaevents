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
	"bufio"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

type cpuAssigner interface {
	assignCPUToSocket() (map[int]int, error)
}

type cpuAssignerImpl struct {
	helper ioHelper
}

func newCPUAssigner() *cpuAssignerImpl {
	return &cpuAssignerImpl{helper: &ioHelperImpl{}}
}

// assignCPUToSocket - Returns map of single CPU for each socket
func (ca *cpuAssignerImpl) assignCPUToSocket() (map[int]int, error) {
	if ca.helper == nil {
		return nil, fmt.Errorf("io helper is nil")
	}
	socketCpus := make(map[int]int)
	matches, err := ca.helper.glob("/sys/devices/system/cpu/cpu*/topology/physical_package_id")
	if err != nil {
		return nil, err
	}
	for i := 0; i < len(matches); i++ {
		var cpu int
		var socketID int
		_, err := fmt.Sscanf(matches[i], "/sys/devices/system/cpu/cpu%d", &cpu)
		if err != nil {
			return nil, err
		}
		content, err := ca.helper.readAll(matches[i])
		if err != nil {
			return nil, err
		}
		_, err = fmt.Sscanf(string(content), "%d", &socketID)
		if err != nil {
			return nil, err
		}
		if _, exists := socketCpus[socketID]; !exists {
			socketCpus[socketID] = cpu
		} else if cpu < socketCpus[socketID] {
			socketCpus[socketID] = cpu
		}
	}
	return socketCpus, nil
}

type cpuID struct {
	vendor        string
	model         uint64
	family        uint64
	stepping      uint64
	steppingIsSet bool
}

// CPUidProvider interface for getting cpuID from provided path to cpuinfo
type cpuIDProvider interface {
	idFromCPUInfo(path string) (*cpuID, error)
}

type cpuIDProviderImpl struct {
	helper ioHelper
}

func newCPUidProvider() *cpuIDProviderImpl {
	return &cpuIDProviderImpl{helper: &ioHelperImpl{}}
}

// cpuIDfromCPUinfo - read CPU ID information from file formatted like /proc/cpuinfo e.g.:
// processor       : 0
// vendor_id       : GenuineIntel
// cpu family      : 6
// model           : 85
// model name      : Intel(R) Xeon(R) Gold 6140 CPU @ 2.30GHz
// stepping        : 4
// (...)
func (cip *cpuIDProviderImpl) idFromCPUInfo(path string) (*cpuID, error) {
	if cip.helper == nil {
		return nil, fmt.Errorf("io helper is nil")
	}
	cpuInfo, err := cip.helper.open(path)
	if err != nil {
		return nil, err
	}
	defer cpuInfo.Close()

	return scanCPUid(cpuInfo)
}

func scanCPUid(reader io.Reader) (*cpuID, error) {
	if reader == nil {
		return nil, fmt.Errorf("io reader is nil")
	}
	cpu := &cpuID{}
	count := 0
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		var err error
		line := strings.Fields(scanner.Text())
		if len(line) >= 3 && line[0] == "vendor_id" && line[1] == ":" {
			cpu.vendor = line[2]
		} else if len(line) >= 3 && line[0] == "model" && line[1] == ":" {
			cpu.model, err = strconv.ParseUint(line[2], 0, 64)
		} else if len(line) >= 4 && line[0] == "cpu" && line[1] == "family" {
			cpu.family, err = strconv.ParseUint(line[3], 0, 64)
		} else if len(line) >= 3 && line[0] == "stepping" && line[1] == ":" {
			cpu.stepping, err = strconv.ParseUint(line[2], 0, 64)
			cpu.steppingIsSet = true
		} else {
			continue
		}
		count++

		if err != nil {
			return nil, err
		}
		// There are maximum 4 fields to read, so we can break here.
		if count == 4 {
			break
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return cpu, nil
}

const (
	cpuInfoPath = "/proc/cpuinfo"
)

// CPUidStringer interface to get the CPU ID in full form 'vendor-family-model-stepping'
// or short without stepping 'vendor-family-model'
type CPUidStringer interface {
	Full() string
	Short() string
}

// Full returns the full string for CPU ID with stepping if available: 'vendor-family-model-stepping'
// (for example 'GenuineIntel-6-55-4') or without stepping if this information is not available:
// 'vendor-family-model' (for example 'GenuineIntel-6-4F')
func (cid *cpuID) Full() string {
	cpuIDstr := ""
	if !cid.steppingIsSet {
		cpuIDstr = fmt.Sprintf("%s-%d-%X", cid.vendor, cid.family, cid.model)
	} else {
		cpuIDstr = fmt.Sprintf("%s-%d-%X-%X", cid.vendor, cid.family, cid.model, cid.stepping)
	}
	return cpuIDstr
}

// Short returns the string for CPU ID without stepping: 'vendor-family-model',
// for example 'GenuineIntel-6-4F'
func (cid *cpuID) Short() string {
	return fmt.Sprintf("%s-%d-%X", cid.vendor, cid.family, cid.model)
}

// CheckCPUid gets the CPU ID Stringer which provides the ID for current cpu in forms:
// 'vendor-family-model-stepping' (for example 'GenuineIntel-6-55-4') or shorter
// 'vendor-family-model' (for example 'GenuineIntel-6-55')
func CheckCPUid() (CPUidStringer, error) {
	provider := newCPUidProvider()
	return readCPUid(provider)
}

func readCPUid(provider cpuIDProvider) (CPUidStringer, error) {
	if provider == nil {
		return nil, fmt.Errorf("cpu id provider is nil")
	}
	cpu, err := provider.idFromCPUInfo(cpuInfoPath)
	if err != nil {
		return nil, fmt.Errorf("failed to get cpu ID from %s: %v", cpuInfoPath, err)
	}
	return cpu, nil
}

type cpuMonitor interface {
	cpuOnline(int) (bool, error)
	cpusNum() (int, error)
}

type cpuMonitorImpl struct {
	helper ioHelper
}

func newCPUMonitor() *cpuMonitorImpl {
	return &cpuMonitorImpl{helper: &ioHelperImpl{}}
}

// cpuOnline - check if cpu is online
func (cm *cpuMonitorImpl) cpuOnline(cpu int) (bool, error) {
	if cm.helper == nil {
		return false, fmt.Errorf("io helper is nil")
	}
	var online string
	cpuOnlinePath := fmt.Sprintf("/sys/devices/system/cpu/cpu%d/online", cpu)
	if err := cm.helper.validateFile(cpuOnlinePath); err != nil {
		// online by default, if file doesn't exist
		if os.IsNotExist(err) {
			return true, nil
		}
		return false, err
	}
	content, err := cm.helper.readAll(cpuOnlinePath)
	if err != nil {
		return false, err
	}
	_, err = fmt.Sscanf(string(content), "%s", &online)
	if err != nil {
		return false, err
	}
	if online == "1" {
		return true, nil
	}
	return false, nil
}

func (cm *cpuMonitorImpl) cpusNum() (int, error) {
	if cm.helper == nil {
		return 0, fmt.Errorf("io helper is nil")
	}
	cpuPresentPath := "/sys/devices/system/cpu/present"
	content, err := cm.helper.readAll(cpuPresentPath)
	if err != nil {
		return 0, err
	}
	return parseCPURange(strings.Trim(string(content), "\n "))
}

func parseCPURange(cpus string) (int, error) {
	n := 0
	for _, cpuRange := range strings.Split(cpus, ",") {
		if len(cpuRange) == 0 {
			continue
		}
		rangeOp := strings.SplitN(cpuRange, "-", 2)
		first, err := strconv.ParseUint(rangeOp[0], 10, 32)
		if err != nil {
			return 0, err
		}
		if len(rangeOp) == 1 {
			n++
			continue
		}
		last, err := strconv.ParseUint(rangeOp[1], 10, 32)
		if err != nil {
			return 0, err
		}
		n += int(last - first + 1)
	}
	return n, nil
}

// AllCPUs returns slice of all available cpus.
func AllCPUs() ([]int, error) {
	return monitoredCPUs(newCPUMonitor())
}

func monitoredCPUs(monitor cpuMonitor) (cpus []int, err error) {
	if monitor == nil {
		return nil, fmt.Errorf("cpu monitor is nil")
	}
	maxCPU, err := monitor.cpusNum()
	if err != nil {
		return nil, err
	}
	for i := 0; i < maxCPU; i++ {
		cpus = append(cpus, i)
	}
	return cpus, nil
}

// AllSockets returns slice of all available sockets.
func AllSockets() ([]int, error) {
	return assignedSockets(newCPUAssigner())
}

func assignedSockets(assigner cpuAssigner) (sockets []int, err error) {
	if assigner == nil {
		return nil, fmt.Errorf("cpu assigner is nil")
	}
	socketCPUs, err := assigner.assignCPUToSocket()
	if err != nil {
		return nil, err
	}
	for socket := range socketCPUs {
		sockets = append(sockets, socket)
	}
	return sockets, nil
}
