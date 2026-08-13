APP_NAME := tensors-router
CMD := ./cmd/tensors-router
WEBUI_NAME := tensor-router-webui
WEBUI_CMD := ./cmd/tensor-router-webui
DOWNLOADER_NAME := tensor-router-downloader
DOWNLOADER_CMD := ./cmd/tensor-router-downloader
VLLM_NAME := tensor-router-vllm
VLLM_CMD := ./cmd/tensor-router-vllm
VLLM_UV_ASSET := internal/vllm/assets/uv
UV ?= uv
UV_VERSION ?= 0.12.0
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
VLLM_SUPPORTED := $(filter linux/amd64 linux/arm64 darwin/arm64,$(GOOS)/$(GOARCH))
HOST_GOOS := $(shell $(GO) env GOOS)
HOST_GOARCH := $(shell $(GO) env GOARCH)
VLLM_HOST_SUPPORTED := $(filter linux/amd64 linux/arm64 darwin/arm64,$(HOST_GOOS)/$(HOST_GOARCH))
VLLM_NATIVE_TARGET := $(filter $(VLLM_SUPPORTED),$(HOST_GOOS)/$(HOST_GOARCH))

.PHONY: test webui-build webui-check build build-router build-webui build-downloader build-vllm stage-vllm-uv build-linux build-linux-router build-linux-webui build-linux-downloader build-linux-vllm clean install-user-service uninstall-user-service

test: webui-build
	$(GO) test ./...

webui-build:
	cd $(WEBUI_DIR) && $(NPM) run build

webui-check:
	cd $(WEBUI_DIR) && $(NPM) run check

build: build-router build-webui build-downloader $(if $(VLLM_HOST_SUPPORTED),build-vllm)

build-router:
	$(GO) build -ldflags "$(BUILDINFO_LDFLAGS)" -o $(APP_NAME) $(CMD)

build-webui: webui-build
	$(GO) build -ldflags "$(BUILDINFO_LDFLAGS)" -o $(WEBUI_NAME) $(WEBUI_CMD)

build-downloader:
	$(GO) build -ldflags "$(BUILDINFO_LDFLAGS)" -o $(DOWNLOADER_NAME) $(DOWNLOADER_CMD)

stage-vllm-uv:
	test "$$($(UV) --version | awk '{print $$2}')" = "$(UV_VERSION)"
	mkdir -p $(dir $(VLLM_UV_ASSET))
	cp -L "$$(command -v $(UV))" $(VLLM_UV_ASSET)

build-vllm: $(if $(VLLM_HOST_SUPPORTED),stage-vllm-uv)
	$(if $(VLLM_HOST_SUPPORTED),,$(error tensor-router-vllm is unsupported for $(HOST_GOOS)/$(HOST_GOARCH)))
	$(GO) build -tags vllm_embedded_uv -ldflags "$(BUILDINFO_LDFLAGS)" -o $(VLLM_NAME) $(VLLM_CMD)
	./$(VLLM_NAME) bootstrap-info

build-linux: build-linux-router build-linux-webui build-linux-downloader $(if $(VLLM_SUPPORTED),build-linux-vllm)

build-linux-router:
	mkdir -p $(DIST_DIR)
	GOOS=$(GOOS) GOARCH=$(GOARCH) CGO_ENABLED=$(CGO_ENABLED) $(GO) build -buildvcs=false -trimpath -ldflags "-s -w $(BUILDINFO_LDFLAGS)" -o $(DIST_DIR)/$(APP_NAME)-$(GOOS)-$(GOARCH) $(CMD)

build-linux-webui: webui-build
	mkdir -p $(DIST_DIR)
	GOOS=$(GOOS) GOARCH=$(GOARCH) CGO_ENABLED=$(CGO_ENABLED) $(GO) build -buildvcs=false -trimpath -ldflags "-s -w $(BUILDINFO_LDFLAGS)" -o $(DIST_DIR)/$(WEBUI_NAME)-$(GOOS)-$(GOARCH) $(WEBUI_CMD)

build-linux-downloader:
	mkdir -p $(DIST_DIR)
	GOOS=$(GOOS) GOARCH=$(GOARCH) CGO_ENABLED=$(CGO_ENABLED) $(GO) build -buildvcs=false -trimpath -ldflags "-s -w $(BUILDINFO_LDFLAGS)" -o $(DIST_DIR)/$(DOWNLOADER_NAME)-$(GOOS)-$(GOARCH) $(DOWNLOADER_CMD)

build-linux-vllm: $(if $(VLLM_NATIVE_TARGET),stage-vllm-uv)
	$(if $(VLLM_SUPPORTED),,$(error tensor-router-vllm is unsupported for $(GOOS)/$(GOARCH)))
	$(if $(filter $(GOOS)/$(GOARCH),$(HOST_GOOS)/$(HOST_GOARCH)),,$(error tensor-router-vllm requires a matching native $(GOOS)/$(GOARCH) build host))
	mkdir -p $(DIST_DIR)
	GOOS=$(GOOS) GOARCH=$(GOARCH) CGO_ENABLED=$(CGO_ENABLED) $(GO) build -tags vllm_embedded_uv -buildvcs=false -trimpath -ldflags "-s -w $(BUILDINFO_LDFLAGS)" -o $(DIST_DIR)/$(VLLM_NAME)-$(GOOS)-$(GOARCH) $(VLLM_CMD)
	$(DIST_DIR)/$(VLLM_NAME)-$(GOOS)-$(GOARCH) bootstrap-info

clean:
	rm -rf $(DIST_DIR) $(APP_NAME) $(WEBUI_NAME) $(DOWNLOADER_NAME) $(VLLM_NAME) $(VLLM_UV_ASSET)

install-user-service:
	bash scripts/install-systemd-user.sh "$$PWD" "$$PWD/$(APP_NAME)" "$$PWD/config.yaml"

uninstall-user-service:
	bash scripts/uninstall-systemd-user.sh
