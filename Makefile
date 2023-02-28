# Copyright (c) 2021, Intel Corporation.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

GOFILES ?= $(shell git ls-files '*.go')
GOFMT ?= $(shell gofmt -l -s $(GOFILES))

.PHONY: build
build:
	go build

.PHONY: test
test: mock
	go test -race -cover 

coverage:
	go test -coverprofile=coverage.out

.PHONY: example
example:
	go build ./examples/iastat

.PHONY: tidy
tidy:
	go mod tidy

.PHONY: lint
lint: lint-deps
	golangci-lint run
	golangci-lint run ./examples/iastat

.PHONY: lint-deps
lint-deps:
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@v1.51.2

.PHONY: vet
vet:
	go vet ./...

.PHONY: deps
deps:
	go mod download

.PHONY: fmt
fmt:
	@gofmt -s -w $(GOFILES)
	go fix ./...

.PHONY: mock
mock:
	mockery --inpackage --all --testonly
	go mod tidy

.PHONY: mock_install
mock_install:
	go install github.com/vektra/mockery/v2@v2.20.0

.PHONY: clean
clean:
	rm ./mock*

.PHONY: fmtcheck
fmtcheck:
	@if [ ! -z "$(GOFMT)" ]; then \
		echo "[ERROR] gofmt has found errors in the following files:"  ; \
		echo "$(GOFMT)" ; \
		echo "" ;\
		echo "Run make fmt to fix them." ; \
		exit 1 ;\
	fi
