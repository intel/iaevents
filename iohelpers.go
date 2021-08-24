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
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
)

type ioHelper interface {
	open(name string) (*os.File, error)
	getEnv(name string) string
	glob(pattern string) ([]string, error)
	validateFile(path string) error
	readAll(path string) ([]byte, error)
	scanOneValue(filename string, format string, val interface{}) (bool, int, error)
}

type ioHelperImpl struct {
}

func (ioHelperImpl) open(name string) (*os.File, error) {
	linkInfo, err := os.Lstat(name)
	if err != nil {
		return nil, err
	}
	if linkInfo.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("file %s is a symlink", name)
	}
	fd, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	info, err := fd.Stat()
	if err != nil {
		_ = fd.Close()
		return nil, err
	}
	if !os.SameFile(info, linkInfo) {
		_ = fd.Close()
		return nil, fmt.Errorf("file %s was replaced", name)
	}
	return fd, nil
}

func (ioHelperImpl) getEnv(name string) string {
	return os.Getenv(name)
}

func (ioHelperImpl) glob(pattern string) ([]string, error) {
	return filepath.Glob(pattern)
}

func (ioHelperImpl) validateFile(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	} else if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("path %s is a symlink", path)
	} else if info.IsDir() {
		return fmt.Errorf("path %s is a dir", path)
	}
	return nil
}

func (h *ioHelperImpl) readAll(path string) ([]byte, error) {
	fd, err := h.open(path)
	if err != nil {
		return nil, err
	}
	defer fd.Close()
	byteValue, err := ioutil.ReadAll(fd)
	if err != nil {
		return nil, err
	}
	return byteValue, nil
}

func (h *ioHelperImpl) scanOneValue(filename string, format string, val interface{}) (bool, int, error) {
	fd, err := h.open(filename)
	if err != nil {
		return false, 0, err
	}
	defer fd.Close()
	n, err := fmt.Fscanf(fd, format, val)
	return true, n, err
}
