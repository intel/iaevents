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
)

type unixHelper interface {
	open(attr *unix.PerfEventAttr, pid int, cpu int, groupFd int, flags int) (fd int, err error)
	read(fileDescriptor int, buffer *[]byte) (int, error)
	close(fileDescriptor int) error
	setInt(fd int, req uint, value int) error
}

type unixHelperImpl struct {
}

// perfEventOpen - syscall https://man7.org/linux/man-pages/man2/perf_event_open.2.html
func (unixHelperImpl) open(attr *unix.PerfEventAttr, pid int, cpu int, groupFd int, flags int) (fd int, err error) {
	return unix.PerfEventOpen(attr, pid, cpu, groupFd, flags)
}

func (unixHelperImpl) read(fileDescriptor int, buffer *[]byte) (int, error) {
	return unix.Read(fileDescriptor, *buffer)
}

func (unixHelperImpl) close(fileDescriptor int) error {
	return unix.Close(fileDescriptor)
}

func (unixHelperImpl) setInt(fd int, req uint, value int) error {
	return unix.IoctlSetInt(fd, req, value)
}
