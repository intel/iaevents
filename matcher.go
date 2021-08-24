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
	"strings"
)

// Matcher interface to match event by name or other property.
type Matcher interface {
	Match(e *JSONEvent) (bool, error)
}

// NameMatcher match events by exact name.
type NameMatcher struct {
	names []string
}

// NewNameMatcher returns new NameMatcher with default members.
// If no arguments are passed the matcher will match all events.
func NewNameMatcher(eventsNames ...string) *NameMatcher {
	matcher := &NameMatcher{}
	if len(eventsNames) == 0 {
		return matcher
	}
	for _, name := range eventsNames {
		matcher.names = append(matcher.names, strings.ToUpper(name))
	}
	return matcher
}

func (m *NameMatcher) String() string {
	return strings.Join(m.names, ",")
}

// Match checks if the provided event matches the stored list of event names.
func (m *NameMatcher) Match(event *JSONEvent) (bool, error) {
	if event == nil {
		return false, errors.New("JSONEvent is nil")
	}
	if event.Deprecated == "1" {
		// Do not match events that are deprecated
		return false, nil
	}
	if len(m.names) == 0 {
		// Match all events
		return true, nil
	}

	for _, name := range m.names {
		if name == strings.ToUpper(event.EventName) {
			return true, nil
		}
	}
	return false, nil
}
