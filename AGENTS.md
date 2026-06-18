# AGENTS.md - cspawn

## Project Overview

Lightweight container runtime in Go. Starts containers from local rootfs or containerd images with minimal isolation (filesystem + PID, no network).

## Key Commands

```bash
make build          # Build to _out/linux/amd64/cspawn
make test           # Unit tests with coverage (coverage.out)
make lint           # golangci-lint (errcheck, govet, ineffassign, staticcheck, unused)
make ci             # build + test + lint (what CI runs)
make e2e            # Full E2E tests (requires root/sudo)
make e2e-local      # Local runtime E2E only
make cross          # Cross compile linux/amd64 + linux/arm64
```

## Architecture

```
cmd/cspawn/main.go          → Entry point
internal/config/config.go    → CLI parsing, runtime://addr format
internal/runtime/            → Runtime interface (local, containerd)
internal/container/container.go → Container setup (pivot_root/chroot, mounts, exec)
pkg/log/                     → Logging
pkg/utils/                   → Utilities (ID generation, image normalization)
e2e/                         → E2E test scripts (prepare-rootfs.sh, run-tests.sh)
```

## Runtime Format

Flags use `runtime://address` format:
- `-r local:///var/lib/cspawn` (default)
- `-r containerd://unix:///run/containerd/containerd.sock`

## Important Constraints

- **Root required**: E2E tests and actual container execution need root
- **Mutually exclusive**: `-d` (rootfs dir) and `-i` (image) cannot be used together
- **containerd requires `-i`**: When using containerd runtime, image flag is mandatory
- **Debug mode**: `CSPAWN_DEBUG=1` environment variable
- **Build output**: `_out/` directory (gitignored)

## Code Style

- Go 1.26.3
- Bilingual comments (English/Chinese) in user-facing messages
- golangci-lint v2 config in `.golangci.yml`
