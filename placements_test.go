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
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

// Unit tests need mocks to work properly.
// Please generate mocks by `make mock` or `make test` command.

func TestNewCorePlacement(t *testing.T) {
	mFactory := &MockPlacementFactory{}
	mCPU := 0

	t.Run("placement factory is nil", func(t *testing.T) {
		provider, err := NewCorePlacement(nil, mCPU)
		require.Error(t, err)
		require.Nil(t, provider)
	})

	t.Run("factory failed", func(t *testing.T) {
		mFactory.On("NewPlacements", corePMUUnit, mCPU).Return(nil, errors.New("mock error")).Once()
		provider, err := NewCorePlacement(mFactory, mCPU)
		require.Error(t, err)
		require.Nil(t, provider)
		mFactory.AssertExpectations(t)
	})

	t.Run("no providers", func(t *testing.T) {
		pp := make([]PlacementProvider, 0)
		mFactory.On("NewPlacements", corePMUUnit, mCPU).Return(pp, nil).Once()
		provider, err := NewCorePlacement(mFactory, mCPU)
		require.Error(t, err)
		require.Nil(t, provider)
		mFactory.AssertExpectations(t)
	})

	t.Run("succeeded", func(t *testing.T) {
		pp := []PlacementProvider{&Placement{}}
		mFactory.On("NewPlacements", corePMUUnit, mCPU).Return(pp, nil).Once()
		provider, err := NewCorePlacement(mFactory, mCPU)
		require.NoError(t, err)
		require.Equal(t, pp[0], provider)
		mFactory.AssertExpectations(t)
	})
}

func TestNewCorePlacements(t *testing.T) {
	mFactory := &MockPlacementFactory{}
	cpu1, cpu2, cpu3, cpu4 := 0, 1, 2, 3

	cpus := []int{cpu1, cpu2, cpu3, cpu4}

	t.Run("placement factory is nil", func(t *testing.T) {
		providers, err := NewCorePlacements(nil, cpus[0], cpus[1:]...)
		require.Error(t, err)
		require.Nil(t, providers)
	})

	t.Run("factory failed", func(t *testing.T) {
		mFactory.On("NewPlacements", corePMUUnit, cpu1, cpu2, cpu3, cpu4).Return(nil, errors.New("mock error")).Once()
		providers, err := NewCorePlacements(mFactory, cpus[0], cpus[1:]...)
		require.Error(t, err)
		require.Nil(t, providers)
		mFactory.AssertExpectations(t)
	})

	t.Run("no providers", func(t *testing.T) {
		pp := make([]PlacementProvider, 0)
		mFactory.On("NewPlacements", corePMUUnit, cpu1, cpu2, cpu3, cpu4).Return(pp, nil).Once()
		providers, err := NewCorePlacements(mFactory, cpus[0], cpus[1:]...)
		require.Error(t, err)
		require.Nil(t, providers)
		mFactory.AssertExpectations(t)
	})

	t.Run("succeeded", func(t *testing.T) {
		pp := []PlacementProvider{&Placement{}, &Placement{}, &Placement{}, &Placement{}}
		mFactory.On("NewPlacements", corePMUUnit, cpu1, cpu2, cpu3, cpu4).Return(pp, nil).Once()
		providers, err := NewCorePlacements(mFactory, cpus[0], cpus[1:]...)
		require.NoError(t, err)
		require.Equal(t, pp, providers)
		mFactory.AssertExpectations(t)
	})
}

func TestNewUncorePlacement(t *testing.T) {
	mFactory := &MockPlacementFactory{}
	mUnit := "mock_unit"
	mSocketToCPU := map[int]int{
		0: 0,
		1: 8,
		2: 16,
	}

	t.Run("placement factory is nil", func(t *testing.T) {
		provider, err := uncorePlacementForUnit(nil, mSocketToCPU, mUnit, 43)
		require.Error(t, err)
		require.Nil(t, provider)
	})

	t.Run("wrong socket", func(t *testing.T) {
		provider, err := uncorePlacementForUnit(mFactory, mSocketToCPU, mUnit, 43)
		require.Error(t, err)
		require.Nil(t, provider)
	})

	t.Run("factory error", func(t *testing.T) {
		mSocket := 0
		mFactory.On("NewPlacements", mUnit, mSocketToCPU[mSocket]).Return(nil, errors.New("mock error")).Once()
		provider, err := uncorePlacementForUnit(mFactory, mSocketToCPU, mUnit, mSocket)
		require.Error(t, err)
		require.Nil(t, provider)
		mFactory.AssertExpectations(t)
	})

	t.Run("no providers", func(t *testing.T) {
		pp := make([]PlacementProvider, 0)

		mSocket := 0
		mFactory.On("NewPlacements", mUnit, mSocketToCPU[mSocket]).Return(pp, nil).Once()
		provider, err := uncorePlacementForUnit(mFactory, mSocketToCPU, mUnit, mSocket)
		require.Error(t, err)
		require.Nil(t, provider)
		mFactory.AssertExpectations(t)
	})

	t.Run("succeeded", func(t *testing.T) {
		pp := []PlacementProvider{&Placement{}}

		mSocket := 0
		mFactory.On("NewPlacements", mUnit, mSocketToCPU[mSocket]).Return(pp, nil).Once()
		provider, err := uncorePlacementForUnit(mFactory, mSocketToCPU, mUnit, mSocket)
		require.NoError(t, err)
		require.NotNil(t, provider)
		require.Equal(t, pp[0], provider)
		mFactory.AssertExpectations(t)
	})
}

func TestNewUncoreAllPlacements(t *testing.T) {
	mFactory := &MockPlacementFactory{}
	mSocketToCPU := map[int]int{
		0: 0,
		1: 8,
		2: 16,
	}

	t.Run("placement factory is nil", func(t *testing.T) {
		providers, err := uncorePlacementsForAllUnits(nil, mSocketToCPU, 0)
		require.Error(t, err)
		require.Nil(t, providers)
	})

	t.Run("wrong socket", func(t *testing.T) {
		providers, err := uncorePlacementsForAllUnits(mFactory, mSocketToCPU, 43)
		require.Error(t, err)
		require.Nil(t, providers)
	})

	t.Run("factory error", func(t *testing.T) {
		mSocket := 0
		mFactory.On("NewPlacements", "", mSocketToCPU[mSocket]).Return(nil, errors.New("mock error")).Once()
		providers, err := uncorePlacementsForAllUnits(mFactory, mSocketToCPU, mSocket)
		require.Error(t, err)
		require.Nil(t, providers)
		mFactory.AssertExpectations(t)
	})

	t.Run("no providers", func(t *testing.T) {
		pp := make([]PlacementProvider, 0)

		mSocket := 0
		mFactory.On("NewPlacements", "", mSocketToCPU[mSocket]).Return(pp, nil).Once()
		providers, err := uncorePlacementsForAllUnits(mFactory, mSocketToCPU, mSocket)
		require.Error(t, err)
		require.Nil(t, providers)
		mFactory.AssertExpectations(t)
	})

	t.Run("succeeded", func(t *testing.T) {
		pp := []PlacementProvider{&Placement{}, &Placement{}, &Placement{}}

		mSocket := 0
		mFactory.On("NewPlacements", "", mSocketToCPU[mSocket]).Return(pp, nil).Once()
		providers, err := uncorePlacementsForAllUnits(mFactory, mSocketToCPU, mSocket)
		require.NoError(t, err)
		require.NotNil(t, providers)
		require.Equal(t, pp, providers)
		mFactory.AssertExpectations(t)
	})
}

func TestPlacement_String(t *testing.T) {
	tests := []struct {
		name   string
		cpu    int
		pmu    uint32
		result string
	}{
		{"small", 0, 8, "cpu:0, PMU type:8"},
		{"max", math.MaxInt32, math.MaxUint32, "cpu:2147483647, PMU type:4294967295"},
		{"min", math.MinInt32, 0, "cpu:-2147483648, PMU type:0"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			newPlacements := Placement{CPU: test.cpu, PMUType: test.pmu}
			require.Equal(t, test.result, newPlacements.String())
		})
	}
}
