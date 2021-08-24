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

build:
	go build

test: mock
	go test -count=1

coverage:
	go test -coverprofile=coverage.out

example:
	go build ./examples/iastat

lint: lint-deps
	golangci-lint run
	golangci-lint run ./examples/iastat

.PHONY: lint-deps
lint-deps:
	go get github.com/golangci/golangci-lint/cmd/golangci-lint@v1.38.0
	go mod tidy

.PHONY: vet
vet:
	go vet ./...

.PHONY: deps
deps:
	go mod download

.PHONY: fmt
fmt:
	@gofmt -s -w $(GOFILES)

.PHONY: mock
mock:
	go get github.com/vektra/mockery/v2/.../@v2.7.4
	mockery --inpackage --all --testonly
	go mod tidy

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
