GOBUILD := go build
GOTEST := go test
GOCLEAN := go clean
GOFMT := go fmt -w

BUILD_DIR := build
PKG_NAME := isg_exporter
OS := linux darwin

.DEFAULT_GOAL: all
all: build

.PHONY: build
build: $(patsubst %,build.%,$(OS))

build.%: test
	@echo "Building $(PKG_NAME) for $* ..."
	GOOS=$* $(GOBUILD) -o $(BUILD_DIR)/$(PKG_NAME).$*

test:
	$(GOTEST)

clean:
	$(GOCLEAN)
	rm -fr $(BUILD_DIR)