# Copyright 2024 Christoph Raitzig
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

.PHONY: local container push modules update-modules verify-modules build-dirs clean

PKG := github.com/vmware-tanzu/velero-plugin-for-webdav
BIN := velero-plugin-for-webdav

REGISTRY ?= talinx
IMAGE    ?= $(REGISTRY)/velero-plugin-for-webdav
VERSION  ?= 1.0.0

GOOS   ?= $(shell go env GOOS)
GOARCH ?= $(shell go env GOARCH)

# local builds the binary using 'go build' in the local environment.
local: build-dirs
	CGO_ENABLED=0 go build -v -o _output/bin/$(GOOS)/$(GOARCH) .

# container builds a Docker image containing the binary.
container:
	docker build -t $(IMAGE):$(VERSION) .

# push pushes the Docker image to its registry as both AMD64 and ARM64 images
push:
ifeq ($(TAG_LATEST), true)
	docker buildx build --pull --push --platform "linux/amd64,linux/arm64" -t "$(IMAGE):$(VERSION)" -t "$(IMAGE):latest" .
else
	docker buildx build --pull --push --platform "linux/amd64,linux/arm64" -t "$(IMAGE):$(VERSION)" .
endif

# modules tidies up go module files
modules:
	go mod tidy

# update-modules updates go modules to their latest version
update-modules:
	go get -u
	go mod tidy

# verify-modules ensures Go module files are up to date
verify-modules: modules
	@if !(git diff --quiet HEAD -- go.sum go.mod); then \
		echo "go module files are out of date, please commit the changes to go.mod and go.sum"; exit 1; \
	fi

# build-dirs creates the necessary directories for a build in the local environment.
build-dirs:
	@mkdir -p _output/bin/$(GOOS)/$(GOARCH)

# clean removes build artifacts from the local environment.
clean:
	@echo "cleaning"
	rm -rf _output
