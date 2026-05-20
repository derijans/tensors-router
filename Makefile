APP_NAME := tensors-router
CMD := ./cmd/tensors-router
DIST_DIR := dist
GO ?= go
GOOS ?= linux
GOARCH ?= amd64
CGO_ENABLED ?= 0

.PHONY: test build build-linux clean install-user-service uninstall-user-service

test:
	$(GO) test ./...

build:
	$(GO) build -o $(APP_NAME) $(CMD)

build-linux:
	mkdir -p $(DIST_DIR)
	GOOS=$(GOOS) GOARCH=$(GOARCH) CGO_ENABLED=$(CGO_ENABLED) $(GO) build -buildvcs=false -trimpath -ldflags "-s -w" -o $(DIST_DIR)/$(APP_NAME)-$(GOOS)-$(GOARCH) $(CMD)

clean:
	rm -rf $(DIST_DIR) $(APP_NAME)

install-user-service:
	bash scripts/install-systemd-user.sh "$$PWD" "$$PWD/$(APP_NAME)" "$$PWD/config.yaml"

uninstall-user-service:
	bash scripts/uninstall-systemd-user.sh
