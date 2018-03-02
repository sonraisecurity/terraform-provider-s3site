TEST?=$$(go list ./... | grep -v 'vendor')
GOFMT_FILES?=$$(find . -name '*.go' | grep -v vendor)

PKG_OS ?= darwin linux
PKG_ARCH ?= amd64
BASE_PATH ?= $(shell pwd)
BUILD_PATH := $(BASE_PATH)/build
PROVIDER := $(shell basename $(BASE_PATH))
BRANCH := $(shell git rev-parse --abbrev-ref HEAD)
ifneq ($(origin TRAVIS_TAG), undefined)
	BRANCH := $(TRAVIS_TAG)
endif

default: build

build:
	go build -o $(BUILD_PATH)/terraform-provider-s3site

packages:
	@for os in $(PKG_OS); do \
		for arch in $(PKG_ARCH); do \
			mkdir -p $(BUILD_PATH)/$(PROVIDER)_$${os}_$${arch} && \
				cd $(BASE_PATH) && \
				GOOS=$${os} GOARCH=$${arch} go build -o $(BUILD_PATH)/$(PROVIDER)_$${os}_$${arch}/$(PROVIDER) . && \
				cd $(BUILD_PATH) && \
				tar -cvzf $(BUILD_PATH)/$(PROVIDER)_$(BRANCH)_$${os}_$${arch}.tar.gz $(PROVIDER)_$${os}_$${arch}/; \
		done; \
	done;

clean:
	@rm -rf $(BUILD_PATH)

.PHONY: build