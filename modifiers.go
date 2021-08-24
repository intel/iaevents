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
	"strings"

	"golang.org/x/sys/unix"
)

type modifiersHelper interface {
	updateQualifiers(attr *unix.PerfEventAttr, qualifiers []string) error
	updateModifiers(attr *unix.PerfEventAttr, modifiers string) error
}

type perfModifiersHelper struct{}

// updateQualifiers - parse custom qualifiers
func (h *perfModifiersHelper) updateQualifiers(attr *unix.PerfEventAttr, qualifiers []string) error {
	if attr == nil {
		return errors.New("perf attributes are nil")
	}
	for _, qualifier := range qualifiers {
		if !strings.HasPrefix(qualifier, "config") {
			err := h.updateModifiers(attr, qualifier)
			if err != nil {
				return fmt.Errorf("failed to update modifiers: %v", err)
			}
			continue
		}

		splittedQualifier := strings.Split(qualifier, "=")
		if len(splittedQualifier) != 2 {
			return fmt.Errorf("failed to split qualifier `%s`", qualifier)
		}
		var value uint64
		var modifiers string
		n, err := fmt.Sscanf(splittedQualifier[1], "%v%s", &value, &modifiers)
		if n < 1 {
			return fmt.Errorf("failed to parse qualifier `%s`: %v", qualifier, err)
		}
		switch splittedQualifier[0] {
		case "config":
			attr.Config |= value
		case "config1":
			attr.Ext1 |= value
		case "config2":
			attr.Ext2 |= value
		default:
			return fmt.Errorf("unsupported qualifier `%s`", qualifier)
		}

		if n == 1 {
			// No modifiers, we can continue.
			continue
		}

		err = h.updateModifiers(attr, modifiers)
		if err != nil {
			return err
		}
	}
	return nil
}

// updateModifiers - update perf attributes according to provided modifiers
func (perfModifiersHelper) updateModifiers(attr *unix.PerfEventAttr, modifiers string) error {
	for _, chr := range modifiers {
		switch chr {
		case 'p':
			attr.Bits = incPreciseIPBits(attr.Bits)
		case 'k':
			attr.Bits |= unix.PerfBitExcludeUser
		case 'u':
			attr.Bits |= unix.PerfBitExcludeKernel
		case 'h', 'H':
			attr.Bits |= unix.PerfBitExcludeGuest
		case 'I':
			attr.Bits |= unix.PerfBitExcludeIdle
		case 'G':
			attr.Bits |= unix.PerfBitExcludeHv
		case 'D':
			attr.Bits |= unix.PerfBitPinned
		default:
			return fmt.Errorf("unknown modifier %c in %s", chr, modifiers)
		}
	}
	return nil
}

// incPreciseIPBits - increment IPBit1 and IPBit2 in sequence: 00->01->10->11
func incPreciseIPBits(bits uint64) uint64 {
	if bits&unix.PerfBitPreciseIPBit2 != 0 {
		return bits | unix.PerfBitPreciseIPBit1
	} else if bits&unix.PerfBitPreciseIPBit1 != 0 {
		bits &= ^uint64(unix.PerfBitPreciseIPBit1)
		return bits | unix.PerfBitPreciseIPBit2
	}
	return bits | unix.PerfBitPreciseIPBit1
}
