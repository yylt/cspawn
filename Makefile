BINARY_NAME=cspawn
OUT_DIR=_out
GO=go
LDFLAGS=-s -w
E2E_IMAGE=ghcr.io/containerd/busybox:1.36
E2E_ROOTFS=/tmp/cspawn-e2e-rootfs

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
	@echo "Preparing E2E rootfs from $(E2E_IMAGE)..."
	@mkdir -p $(E2E_ROOTFS)
	docker pull $(E2E_IMAGE)
	docker create --name cspawn-e2e-export $(E2E_IMAGE) 2>/dev/null || true
	docker export cspawn-e2e-export | tar -x -C $(E2E_ROOTFS)
	docker rm cspawn-e2e-export 2>/dev/null || true
	@echo "E2E rootfs prepared at $(E2E_ROOTFS)"

e2e-local: build e2e-prepare-rootfs
	@echo "=== E2E: Local Runtime ==="
	sudo $(OUT_DIR)/$(shell go env GOOS)/$(shell go env GOARCH)/$(BINARY_NAME) -r local -d $(E2E_ROOTFS) /bin/echo "Hello from cspawn"
	sudo $(OUT_DIR)/$(shell go env GOOS)/$(shell go env GOARCH)/$(BINARY_NAME) -r local -d $(E2E_ROOTFS) -e TEST_VAR=hello123 /bin/sh -c 'echo $$TEST_VAR' | grep -q "hello123"
	sudo $(OUT_DIR)/$(shell go env GOOS)/$(shell go env GOARCH)/$(BINARY_NAME) -r local -d $(E2E_ROOTFS) -e VAR1=value1 -e VAR2=value2 /bin/sh -c 'echo $$VAR1-$$VAR2' | grep -q "value1-value2"
	@echo "ENV_FILE_VAR=test_value" > /tmp/cspawn-e2e-env.txt
	@echo "ENV_FILE_VAR2=test_value2" >> /tmp/cspawn-e2e-env.txt
	sudo $(OUT_DIR)/$(shell go env GOOS)/$(shell go env GOARCH)/$(BINARY_NAME) -r local -d $(E2E_ROOTFS) -E /tmp/cspawn-e2e-env.txt /bin/sh -c 'echo $$ENV_FILE_VAR-$$ENV_FILE_VAR2' | grep -q "test_value-test_value2"
	sudo $(OUT_DIR)/$(shell go env GOOS)/$(shell go env GOARCH)/$(BINARY_NAME) -r local -d $(E2E_ROOTFS) -c /tmp /bin/sh -c 'pwd' | grep -q "/tmp"
	@mkdir -p /tmp/cspawn-e2e-host-data
	@echo "bound data" > /tmp/cspawn-e2e-host-data/test.txt
	sudo $(OUT_DIR)/$(shell go env GOOS)/$(shell go env GOARCH)/$(BINARY_NAME) -r local -d $(E2E_ROOTFS) -b /tmp/cspawn-e2e-host-data:/container/data /bin/cat /container/data/test.txt | grep -q "bound data"
	sudo $(OUT_DIR)/$(shell go env GOOS)/$(shell go env GOARCH)/$(BINARY_NAME) -r local -d $(E2E_ROOTFS) -b /tmp/cspawn-e2e-host-data:/container/ro-data:ro /bin/cat /container/ro-data/test.txt | grep -q "bound data"
	@echo "=== E2E Local: All tests passed ==="

e2e-containerd: build
	@echo "=== E2E: Containerd Runtime ==="
	sudo ctr images pull $(E2E_IMAGE)
	sudo $(OUT_DIR)/$(shell go env GOOS)/$(shell go env GOARCH)/$(BINARY_NAME) -r containerd -i $(E2E_IMAGE) /bin/echo "Hello from containerd"
	sudo $(OUT_DIR)/$(shell go env GOOS)/$(shell go env GOARCH)/$(BINARY_NAME) -r containerd -i $(E2E_IMAGE) -e CONTAINERD_TEST=works /bin/sh -c 'echo $$CONTAINERD_TEST' | grep -q "works"
	sudo $(OUT_DIR)/$(shell go env GOOS)/$(shell go env GOARCH)/$(BINARY_NAME) -r containerd -i $(E2E_IMAGE) -e TERM=xterm -e LANG=C -c /tmp /bin/sh -c 'echo $$TERM $$LANG $$(pwd)' | grep -q "xterm C /tmp"
	@echo "=== E2E Containerd: All tests passed ==="

e2e-user: build e2e-prepare-rootfs
	@echo "=== E2E: User Switching ==="
	sudo $(OUT_DIR)/$(shell go env GOOS)/$(shell go env GOARCH)/$(BINARY_NAME) -r local -d $(E2E_ROOTFS) -u root:root /bin/id | grep -q "uid=0"
	sudo $(OUT_DIR)/$(shell go env GOOS)/$(shell go env GOARCH)/$(BINARY_NAME) -r local -d $(E2E_ROOTFS) -u nobody:nogroup /bin/id | grep -q "uid=65534"
	@echo "=== E2E User: All tests passed ==="

e2e-combined: build e2e-prepare-rootfs
	@echo "=== E2E: Combined Options ==="
	@mkdir -p /tmp/cspawn-e2e-test-bind
	@echo "test content" > /tmp/cspawn-e2e-test-bind/file.txt
	@printf "COMBINED_A=alpha\nCOMBINED_B=beta\n" > /tmp/cspawn-e2e-test-env.txt
	sudo $(OUT_DIR)/$(shell go env GOOS)/$(shell go env GOARCH)/$(BINARY_NAME) -r local \
		-d $(E2E_ROOTFS) \
		-e EXTRA_VAR=extra \
		-E /tmp/cspawn-e2e-test-env.txt \
		-u root:root \
		-c /tmp \
		-b /tmp/cspawn-e2e-test-bind:/mnt/data \
		/bin/sh -c 'echo "$$COMBINED_A $$COMBINED_B $$EXTRA_VAR $$(pwd) $$(cat /mnt/data/file.txt)"' | grep -q "alpha beta extra /tmp test content"
	sudo $(OUT_DIR)/$(shell go env GOOS)/$(shell go env GOARCH)/$(BINARY_NAME) -r local -d $(E2E_ROOTFS) /bin/sh -c 'echo "arg1 arg2"' | grep -q "arg1 arg2"
	@echo "=== E2E Combined: All tests passed ==="

e2e: e2e-local e2e-user e2e-combined
	@echo "=== All E2E tests passed ==="

e2e-full: e2e e2e-containerd
	@echo "=== Full E2E tests (including containerd) passed ==="

clean:
	rm -rf $(OUT_DIR)
	rm -rf $(E2E_ROOTFS)
	docker rm -f cspawn-e2e-export 2>/dev/null || true
