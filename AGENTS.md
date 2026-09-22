## AI Coding Rules

### 1. Think Before Coding

**Don't assume. Don't hide confusion. Surface tradeoffs.**

Before implementing:
- State your assumptions explicitly. If uncertain, ask.
- If multiple interpretations exist, present them - don't pick silently.
- If a simpler approach exists, say so. Push back when warranted.
- If something is unclear, stop. Name what's confusing. Ask.

### 2. Simplicity First

**Minimum code that solves the problem. Nothing speculative.**

- No features beyond what was asked.
- No abstractions for single-use code.
- No "flexibility" or "configurability" that wasn't requested.
- No error handling for impossible scenarios.
- If you write 200 lines and it could be 50, rewrite it.

Ask yourself: "Would a senior engineer say this is overcomplicated?" If yes, simplify.

### 3. Surgical Changes

**Touch only what you must. Clean up only your own mess.**

When editing existing code:
- Don't "improve" adjacent code, comments, or formatting.
- Don't refactor things that aren't broken.
- Match existing style, even if you'd do it differently.
- If you notice unrelated dead code, mention it - don't delete it.

When your changes create orphans:
- Remove imports/variables/functions that YOUR changes made unused.
- Don't remove pre-existing dead code unless asked.

The test: Every changed line should trace directly to the user's request.

### 4. Goal-Driven Execution

**Define success criteria. Loop until verified.**

Transform tasks into verifiable goals:
- "Add validation" → "Write tests for invalid inputs, then make them pass"
- "Fix the bug" → "Write a test that reproduces it, then make it pass"
- "Refactor X" → "Ensure tests pass before and after"

For multi-step tasks, state a brief plan:
```
1. [Step] → verify: [check]
2. [Step] → verify: [check]
3. [Step] → verify: [check]
```

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
