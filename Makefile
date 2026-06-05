APP_NAME := tensors-router
CMD := ./cmd/tensors-router
WEBUI_NAME := tensor-reuter-webui
WEBUI_CMD := ./cmd/tensor-reuter-webui
DIST_DIR := dist
GO ?= go
GOOS ?= linux
GOARCH ?= amd64
CGO_ENABLED ?= 0

.PHONY: test build build-router build-webui build-linux build-linux-router build-linux-webui clean install-user-service uninstall-user-service

test:
	$(GO) test ./...

build: build-router build-webui

build-router:
	$(GO) build -o $(APP_NAME) $(CMD)

build-webui:
	$(GO) build -o $(WEBUI_NAME) $(WEBUI_CMD)

build-linux: build-linux-router build-linux-webui

build-linux-router:
	mkdir -p $(DIST_DIR)
	GOOS=$(GOOS) GOARCH=$(GOARCH) CGO_ENABLED=$(CGO_ENABLED) $(GO) build -buildvcs=false -trimpath -ldflags "-s -w" -o $(DIST_DIR)/$(APP_NAME)-$(GOOS)-$(GOARCH) $(CMD)

build-linux-webui:
	mkdir -p $(DIST_DIR)
	GOOS=$(GOOS) GOARCH=$(GOARCH) CGO_ENABLED=$(CGO_ENABLED) $(GO) build -buildvcs=false -trimpath -ldflags "-s -w" -o $(DIST_DIR)/$(WEBUI_NAME)-$(GOOS)-$(GOARCH) $(WEBUI_CMD)

clean:
	rm -rf $(DIST_DIR) $(APP_NAME) $(WEBUI_NAME)

install-user-service:
	bash scripts/install-systemd-user.sh "$$PWD" "$$PWD/$(APP_NAME)" "$$PWD/config.yaml"

uninstall-user-service:
	bash scripts/uninstall-systemd-user.sh
