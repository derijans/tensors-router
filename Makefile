APP_NAME := tensors-router
CMD := ./cmd/tensors-router
WEBUI_NAME := tensor-router-webui
WEBUI_CMD := ./cmd/tensor-router-webui
WEBUI_DIR := webui
DIST_DIR := dist
GO ?= go
NPM ?= npm
GOOS ?= linux
GOARCH ?= amd64
CGO_ENABLED ?= 0
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null)
COMMIT ?= $(shell git rev-parse HEAD 2>/dev/null)
BUILD_DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
BUILDINFO_LDFLAGS := -X tensors-router/internal/buildinfo.Version=$(VERSION) -X tensors-router/internal/buildinfo.Commit=$(COMMIT) -X tensors-router/internal/buildinfo.Date=$(BUILD_DATE)

.PHONY: test webui-build webui-check build build-router build-webui build-linux build-linux-router build-linux-webui clean install-user-service uninstall-user-service

test: webui-build
	$(GO) test ./...

webui-build:
	cd $(WEBUI_DIR) && $(NPM) run build

webui-check:
	cd $(WEBUI_DIR) && $(NPM) run check

build: build-router build-webui

build-router:
	$(GO) build -ldflags "$(BUILDINFO_LDFLAGS)" -o $(APP_NAME) $(CMD)

build-webui: webui-build
	$(GO) build -ldflags "$(BUILDINFO_LDFLAGS)" -o $(WEBUI_NAME) $(WEBUI_CMD)

build-linux: build-linux-router build-linux-webui

build-linux-router:
	mkdir -p $(DIST_DIR)
	GOOS=$(GOOS) GOARCH=$(GOARCH) CGO_ENABLED=$(CGO_ENABLED) $(GO) build -buildvcs=false -trimpath -ldflags "-s -w $(BUILDINFO_LDFLAGS)" -o $(DIST_DIR)/$(APP_NAME)-$(GOOS)-$(GOARCH) $(CMD)

build-linux-webui: webui-build
	mkdir -p $(DIST_DIR)
	GOOS=$(GOOS) GOARCH=$(GOARCH) CGO_ENABLED=$(CGO_ENABLED) $(GO) build -buildvcs=false -trimpath -ldflags "-s -w $(BUILDINFO_LDFLAGS)" -o $(DIST_DIR)/$(WEBUI_NAME)-$(GOOS)-$(GOARCH) $(WEBUI_CMD)

clean:
	rm -rf $(DIST_DIR) $(APP_NAME) $(WEBUI_NAME)

install-user-service:
	bash scripts/install-systemd-user.sh "$$PWD" "$$PWD/$(APP_NAME)" "$$PWD/config.yaml"

uninstall-user-service:
	bash scripts/uninstall-systemd-user.sh
