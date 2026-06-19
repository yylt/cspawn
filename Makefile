BINARY_NAME=cspawn
OUT_DIR=_out
GO=go
LDFLAGS=-s -w
E2E_IMAGE=ghcr.io/containerd/busybox:1.36
E2E_ROOTFS=/tmp/cspawn-e2e-rootfs

VERSION=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT=$(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE=$(shell date -u '+%Y-%m-%dT%H:%M:%SZ')
LDFLAGS+= -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.buildDate=$(BUILD_DATE)

PLATFORMS=linux/amd64 linux/arm64

.PHONY: all build clean lint vuln test ci e2e e2e-local e2e-containerd e2e-user e2e-combined e2e-prepare-rootfs mod-download $(PLATFORMS)

all: build

mod-download:
	$(GO) mod download

build: mod-download
	@mkdir -p $(OUT_DIR)/$(shell go env GOOS)/$(shell go env GOARCH)
	$(GO) build -ldflags "$(LDFLAGS)" -o $(OUT_DIR)/$(shell go env GOOS)/$(shell go env GOARCH)/$(BINARY_NAME) ./cmd/cspawn

$(PLATFORMS):
	@mkdir -p $(OUT_DIR)/$(word 1,$(subst /, ,$@))/$(word 2,$(subst /, ,$@))
	GOOS=$(word 1,$(subst /, ,$@)) GOARCH=$(word 2,$(subst /, ,$@)) $(GO) build -ldflags "$(LDFLAGS)" -o $(OUT_DIR)/$(word 1,$(subst /, ,$@))/$(word 2,$(subst /, ,$@))/$(BINARY_NAME) ./cmd/cspawn

cross: mod-download $(PLATFORMS)

test: mod-download
	$(GO) test -v -coverprofile=coverage.out ./...

lint: mod-download
	$(GO) run github.com/golangci/golangci-lint/v2/cmd/golangci-lint run ./...

vuln: mod-download
	$(GO) run golang.org/x/vuln/cmd/govulncheck ./...

ci: build test lint

e2e-prepare-rootfs: 
	@echo "Preparing E2E rootfs using cspawn from $(E2E_IMAGE)..."
	./e2e/prepare-rootfs.sh $(E2E_IMAGE) $(OUT_DIR)/$(shell go env GOOS)/$(shell go env GOARCH)/$(BINARY_NAME)
	@echo "E2E rootfs prepared"

e2e-local: build e2e-prepare-rootfs
	@echo "=== E2E: Local Runtime ==="
	./e2e/run-tests.sh $(E2E_IMAGE) $(OUT_DIR)/$(shell go env GOOS)/$(shell go env GOARCH)/$(BINARY_NAME) /var/lib/cspawn test_local
	@echo "=== E2E Local: All tests passed ==="

e2e-containerd: build
	@echo "=== E2E: Containerd Runtime ==="
	sudo ctr images pull $(E2E_IMAGE)
	sudo $(OUT_DIR)/$(shell go env GOOS)/$(shell go env GOARCH)/$(BINARY_NAME) -r containerd://unix:///run/containerd/containerd.sock -i $(E2E_IMAGE) /bin/echo "Hello from containerd"
	sudo $(OUT_DIR)/$(shell go env GOOS)/$(shell go env GOARCH)/$(BINARY_NAME) -r containerd://unix:///run/containerd/containerd.sock -i $(E2E_IMAGE) -e CONTAINERD_TEST=works /bin/sh -c 'echo $$CONTAINERD_TEST' | grep -q "works"
	sudo $(OUT_DIR)/$(shell go env GOOS)/$(shell go env GOARCH)/$(BINARY_NAME) -r containerd://unix:///run/containerd/containerd.sock -i $(E2E_IMAGE) -e TERM=xterm -e LANG=C -c /tmp /bin/sh -c 'echo $$TERM $$LANG $$(pwd)' | grep -q "xterm C /tmp"
	@echo "=== E2E Containerd: All tests passed ==="

e2e-user: build e2e-prepare-rootfs
	@echo "=== E2E: User Switching ==="
	./e2e/run-tests.sh $(E2E_IMAGE) $(OUT_DIR)/$(shell go env GOOS)/$(shell go env GOARCH)/$(BINARY_NAME) /var/lib/cspawn test_user
	@echo "=== E2E User: All tests passed ==="

e2e-combined: build e2e-prepare-rootfs
	@echo "=== E2E: Combined Options ==="
	./e2e/run-tests.sh $(E2E_IMAGE) $(OUT_DIR)/$(shell go env GOOS)/$(shell go env GOARCH)/$(BINARY_NAME) /var/lib/cspawn test_combined
	@echo "=== E2E Combined: All tests passed ==="

e2e: e2e-local e2e-user e2e-combined
	@echo "=== All E2E tests passed ==="

e2e-full: e2e e2e-containerd
	@echo "=== Full E2E tests (including containerd) passed ==="

clean:
	rm -rf $(OUT_DIR)
	# Clean up cspawn test data (optional)
	# sudo rm -rf /var/lib/cspawn/rootfs/busybox_* 2>/dev/null || true
