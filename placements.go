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
)

// PlacementFactory interface to get placement providers for specified unit and CPUs
type PlacementFactory interface {
	NewPlacements(unit string, cpu int, cpus ...int) ([]PlacementProvider, error)
}

// PlacementProvider interface to provide single placement by means of CPU and PMU TypeID
type PlacementProvider interface {
	PMUPlacement() (int, uint32) // cpu_id, pmu_type
}

const (
	corePMUUnit = "cpu"
)

// Placement holds info about single placement for event
type Placement struct {
	CPU     int
	PMUType uint32
}

// PMUPlacement returns single placement - CPU and PMU TypeID on which the event should be placed
func (p *Placement) PMUPlacement() (int, uint32) {
	return p.CPU, p.PMUType
}

func (p *Placement) String() string {
	return fmt.Sprintf("cpu:%d, PMU type:%d", p.CPU, p.PMUType)
}

// NewCorePlacement creates new core placement for given CPU
func NewCorePlacement(factory PlacementFactory, cpu int) (PlacementProvider, error) {
	if factory == nil {
		return nil, errors.New("placement factory is nil")
	}
	providers, err := factory.NewPlacements(corePMUUnit, cpu)
	if err != nil {
		return nil, err
	}
	if len(providers) == 0 {
		return nil, errors.New("failed to get any placements")
	}
	return providers[0], nil
}

// NewCorePlacements creates new core placements for given CPUs
func NewCorePlacements(factory PlacementFactory, cpu int, cpus ...int) ([]PlacementProvider, error) {
	if factory == nil {
		return nil, errors.New("placement factory is nil")
	}
	providers, err := factory.NewPlacements(corePMUUnit, cpu, cpus...)
	if err != nil {
		return nil, err
	}
	if len(providers) == 0 {
		return nil, errors.New("failed to get any placements")
	}
	return providers, nil
}

// NewUncorePlacement creates new uncore placement for given unit and socket
func NewUncorePlacement(factory PlacementFactory, unit string, socket int) (PlacementProvider, error) {
	socketToCPU, err := newCPUAssigner().assignCPUToSocket()
	if err != nil {
		return nil, fmt.Errorf("failed to translate socket `%d` to corresponding CPU: %v", socket, err)
	}
	return uncorePlacementForUnit(factory, socketToCPU, unit, socket)
}

func uncorePlacementForUnit(factory PlacementFactory, socketToCPU map[int]int, unit string, socket int) (PlacementProvider, error) {
	if factory == nil {
		return nil, errors.New("placement factory is nil")
	}
	cpu, exist := socketToCPU[socket]
	if !exist {
		return nil, fmt.Errorf("socket `%d` is invalid", socket)
	}
	providers, err := factory.NewPlacements(unit, cpu)
	if err != nil {
		return nil, err
	}
	if len(providers) == 0 {
		return nil, errors.New("failed to get any placements")
	}
	return providers[0], nil
}

// NewUncoreAllPlacements creates new uncore placements for given socket and all units available from given factory
func NewUncoreAllPlacements(factory PlacementFactory, socket int) ([]PlacementProvider, error) {
	socketToCPU, err := newCPUAssigner().assignCPUToSocket()
	if err != nil {
		return nil, fmt.Errorf("failed to translate socket `%d` to corresponding CPU: %v", socket, err)
	}
	return uncorePlacementsForAllUnits(factory, socketToCPU, socket)
}

func uncorePlacementsForAllUnits(factory PlacementFactory, socketToCPU map[int]int, socket int) ([]PlacementProvider, error) {
	if factory == nil {
		return nil, errors.New("placement factory is nil")
	}
	cpu, exist := socketToCPU[socket]
	if !exist {
		return nil, fmt.Errorf("socket `%d` is invalid", socket)
	}
	providers, err := factory.NewPlacements("", cpu)
	if err != nil {
		return nil, err
	}
	if len(providers) == 0 {
		return nil, errors.New("failed to get any placements")
	}
	return providers, nil
}
