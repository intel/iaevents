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
	"golang.org/x/sys/unix"
	"strings"
)

// Options is an interface for extra perf options.
//
// PerfStatString() returns user specified extra modifiers as string.
// Attr() returns modified unix.PerfEventAttr struct properly set.
type Options interface {
	PerfStatString() []string
	Attr() *unix.PerfEventAttr
}

// OptionsBuilder is an builder for Options.
//
// SetEnableOnExec decides if set perf event attribute bits with PerfBitDisabled and PerfBitEnableOnExec.
// SetAttrModifiers updates perf event attribute qualifiers.
// Build builds new Options.
type OptionsBuilder interface {
	SetEnableOnExec(bool) OptionsBuilder
	SetAttrModifiers([]string) OptionsBuilder
	Build() (Options, error)
}

type optionsBuilder struct {
	attrModifiers   []string
	enableOnExec    bool
	modifiersHelper modifiersHelper
}

func (ob *optionsBuilder) SetEnableOnExec(enable bool) OptionsBuilder {
	ob.enableOnExec = enable
	return ob
}

func (ob *optionsBuilder) SetAttrModifiers(modifiers []string) OptionsBuilder {
	ob.attrModifiers = modifiers
	return ob
}

func (ob *optionsBuilder) Build() (Options, error) {
	attr := &unix.PerfEventAttr{}
	err := ob.modifiersHelper.updateQualifiers(attr, ob.attrModifiers)
	if err != nil {
		return nil, err
	}
	if ob.enableOnExec {
		attr.Bits |= unix.PerfBitDisabled
		attr.Bits |= unix.PerfBitEnableOnExec
	}
	return &PerfEventOptions{attr: attr, perfStatString: ob.attrModifiers}, nil
}

// PerfEventOptions contains some of the perf event configuration options and modifiers.
type PerfEventOptions struct {
	perfStatString []string
	attr           *unix.PerfEventAttr
}

func (pfo *PerfEventOptions) PerfStatString() []string {
	return pfo.perfStatString
}

func (pfo *PerfEventOptions) Attr() *unix.PerfEventAttr {
	return pfo.attr
}

func (pfo *PerfEventOptions) String() string {
	return strings.Join(pfo.perfStatString, ",")
}

// NewOptions returns new options builder.
func NewOptions() OptionsBuilder {
	return &optionsBuilder{modifiersHelper: &perfModifiersHelper{}}
}
