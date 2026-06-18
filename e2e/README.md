# E2E Testing

This directory contains end-to-end tests for cspawn.

## Overview

The E2E tests verify that cspawn works correctly by:
1. Pulling container images using cspawn itself (no Docker dependency)
2. Running containers with various options
3. Verifying expected behavior

## Prerequisites

- Linux system with root access (tests use `sudo`)
- Go 1.21+ installed
- Network access to pull container images

## Running Tests

### Quick Start

```bash
# Run all E2E tests
make e2e

# Run specific test suites
make e2e-local      # Local runtime tests
make e2e-user       # User switching tests
make e2e-combined   # Combined options tests
```

### Manual Execution

```bash
# 1. Build cspawn
make build

# 2. Prepare rootfs using cspawn
./e2e/prepare-rootfs.sh

# 3. Run tests
./e2e/run-tests.sh
```

## Test Structure

- `prepare-rootfs.sh` - Prepares rootfs by pulling images with cspawn
- `run-tests.sh` - Runs the actual test cases

## Test Image

By default, tests use `ghcr.io/containerd/busybox:1.36`. This can be changed by setting the `E2E_IMAGE` environment variable.

## Cleanup

```bash
# Clean up test artifacts
make clean
```

## Adding New Tests

To add new test cases:
1. Add test commands to `run-tests.sh`
2. Follow the existing pattern for test organization
3. Ensure tests are idempotent and can run multiple times

## Troubleshooting

### Permission Denied
Tests require root access. Run with `sudo` or as root.

### Image Pull Failures
Check network connectivity and image availability.

### Rootfs Not Found
Ensure `prepare-rootfs.sh` completed successfully.