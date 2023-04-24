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
	"testing"

	"github.com/stretchr/testify/require"
)

// Unit tests need mocks to work properly.
// Please generate mocks by `make mock` or `make test` command.

func TestNewNameMatcher(t *testing.T) {
	t.Run("Create matcher without arguments", func(t *testing.T) {
		matcher := NewNameMatcher()
		require.NotNil(t, matcher)
		require.Equal(t, 0, len(matcher.names))
	})
	t.Run("Create matcher for one event", func(t *testing.T) {
		matcher := NewNameMatcher("event1")
		require.NotNil(t, matcher)
		require.Equal(t, 1, len(matcher.names))
		require.EqualValues(t, []string{"EVENT1"}, matcher.names)
	})
	t.Run("Create matcher for two events", func(t *testing.T) {
		matcher := NewNameMatcher("Event1", "Event2")
		require.NotNil(t, matcher)
		require.Equal(t, 2, len(matcher.names))
		require.EqualValues(t, []string{"EVENT1", "EVENT2"}, matcher.names)
	})
}

func TestNameMatcher_String(t *testing.T) {
	tests := []struct {
		name  string
		names []string
		want  string
	}{
		{"No names set in matcher", []string{}, ""},
		{"One event set in matcher", []string{"EVENT1"}, "EVENT1"},
		{"Two events set in matcher", []string{"EVENT1", "EVENT2"}, "EVENT1,EVENT2"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &NameMatcher{
				names: tt.names,
			}
			if got := m.String(); got != tt.want {
				t.Errorf("NameMatcher.String() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNameMatcher_Match(t *testing.T) {
	t.Run("Return error if event is nil", func(t *testing.T) {
		testMatcher := &NameMatcher{}
		_, err := testMatcher.Match(nil)
		require.Error(t, err)
	})
	t.Run("Return false if event is deprecated", func(t *testing.T) {
		testMatcher := &NameMatcher{names: []string{"EVENT1"}}
		testEvent := &JSONEvent{Deprecated: "1", EventName: "Event1"}
		match, err := testMatcher.Match(testEvent)
		require.NoError(t, err)
		require.Equal(t, false, match)
	})
	t.Run("Match event if names list is empty", func(t *testing.T) {
		testMatcher := &NameMatcher{}
		testEvent := &JSONEvent{EventName: "Event1"}
		match, err := testMatcher.Match(testEvent)
		require.NoError(t, err)
		require.Equal(t, true, match)
	})
	t.Run("Return false if event not on names list", func(t *testing.T) {
		testMatcher := &NameMatcher{names: []string{"EVENT1", "EVENT2"}}
		testEvent := &JSONEvent{EventName: "Event3"}
		match, err := testMatcher.Match(testEvent)
		require.NoError(t, err)
		require.Equal(t, false, match)
	})
	t.Run("Match event that is on the names list", func(t *testing.T) {
		testMatcher := &NameMatcher{names: []string{"EVENT1", "EVENT2"}}
		testEvent := &JSONEvent{EventName: "Event2"}
		match, err := testMatcher.Match(testEvent)
		require.NoError(t, err)
		require.Equal(t, true, match)
	})
}
