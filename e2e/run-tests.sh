#!/bin/bash
set -e

# Default values
DEFAULT_IMAGE="ghcr.io/containerd/busybox:1.36"
DEFAULT_CSPAWN_BIN="_out/linux/amd64/cspawn"
DEFAULT_DATA_DIR="/var/lib/cspawn"

# Parse arguments
IMAGE="${1:-$DEFAULT_IMAGE}"
CSPAWN_BIN="${2:-$DEFAULT_CSPAWN_BIN}"
DATA_DIR="${3:-$DEFAULT_DATA_DIR}"
TEST_FUNCTION="${4:-}"

# Calculate rootfs path
IMAGE_NAME=$(echo "$IMAGE" | sed 's|.*/||' | sed 's|:|_|')
ROOTFS_PATH="$DATA_DIR/rootfs/$IMAGE_NAME"

echo "Running E2E tests..."
echo "Image: $IMAGE"
echo "cspawn binary: $CSPAWN_BIN"
echo "Rootfs path: $ROOTFS_PATH"

# Check if cspawn binary exists
if [ ! -f "$CSPAWN_BIN" ]; then
    echo "Error: cspawn binary not found at $CSPAWN_BIN"
    echo "Please build cspawn first: make build"
    exit 1
fi

# Check if rootfs exists
if [ ! -d "$ROOTFS_PATH" ]; then
    echo "Error: Rootfs directory not found at $ROOTFS_PATH"
    echo "Please run prepare-rootfs.sh first"
    exit 1
fi

# Make binary executable
chmod +x "$CSPAWN_BIN"

# Function to run commands with sudo if needed
run_sudo() {
    if [ "$(id -u)" -eq 0 ]; then
        # Already root
        "$@"
    else
        # Try sudo without password first
        if sudo -n true 2>/dev/null; then
            sudo "$@"
        else
            echo "Warning: sudo requires password. Please run as root or configure passwordless sudo."
            echo "Attempting to run without sudo (may fail)..."
            "$@"
        fi
    fi
}

# Test functions
test_local() {
    echo "=== E2E: Local Runtime ==="
    
    # Test 1: Basic command execution
    echo "Test 1: Basic command execution"
    run_sudo "$CSPAWN_BIN" -r local://$DATA_DIR -d "$ROOTFS_PATH" /bin/echo "Hello from cspawn"
    
    # Test 2: Single environment variable
    echo "Test 2: Single environment variable"
    run_sudo "$CSPAWN_BIN" -r local://$DATA_DIR -d "$ROOTFS_PATH" -e TEST_VAR=hello123 /bin/sh -c 'echo $TEST_VAR' | grep -q "hello123"
    
    # Test 3: Multiple environment variables
    echo "Test 3: Multiple environment variables"
    run_sudo "$CSPAWN_BIN" -r local://$DATA_DIR -d "$ROOTFS_PATH" -e VAR1=value1 -e VAR2=value2 /bin/sh -c 'echo $VAR1-$VAR2' | grep -q "value1-value2"
    
    # Test 4: Environment file
    echo "Test 4: Environment file"
    echo "ENV_FILE_VAR=test_value" > /tmp/cspawn-e2e-env.txt
    echo "ENV_FILE_VAR2=test_value2" >> /tmp/cspawn-e2e-env.txt
    run_sudo "$CSPAWN_BIN" -r local://$DATA_DIR -d "$ROOTFS_PATH" -E /tmp/cspawn-e2e-env.txt /bin/sh -c 'echo $ENV_FILE_VAR-$ENV_FILE_VAR2' | grep -q "test_value-test_value2"
    
    # Test 5: Working directory
    echo "Test 5: Working directory"
    run_sudo "$CSPAWN_BIN" -r local://$DATA_DIR -d "$ROOTFS_PATH" -c /tmp /bin/sh -c 'pwd' | grep -q "/tmp"
    
    # Test 6: Bind mount (read-write)
    echo "Test 6: Bind mount (read-write)"
    mkdir -p /tmp/cspawn-e2e-host-data
    echo "bound data" > /tmp/cspawn-e2e-host-data/test.txt
    run_sudo "$CSPAWN_BIN" -r local://$DATA_DIR -d "$ROOTFS_PATH" -b /tmp/cspawn-e2e-host-data:/container/data /bin/cat /container/data/test.txt | grep -q "bound data"
    
    # Test 7: Bind mount (read-only)
    echo "Test 7: Bind mount (read-only)"
    run_sudo "$CSPAWN_BIN" -r local://$DATA_DIR -d "$ROOTFS_PATH" -b /tmp/cspawn-e2e-host-data:/container/ro-data:ro /bin/cat /container/ro-data/test.txt | grep -q "bound data"
    
    echo "=== E2E Local: All tests passed ==="
}

test_user() {
    echo "=== E2E: User Switching ==="
    
    # Test 1: Root user
    echo "Test 1: Root user"
    run_sudo "$CSPAWN_BIN" -r local://$DATA_DIR -d "$ROOTFS_PATH" -u 0:0 /bin/id | grep -q "uid=0"
    
    # Test 2: Nobody user
    echo "Test 2: Nobody user"
    run_sudo "$CSPAWN_BIN" -r local://$DATA_DIR -d "$ROOTFS_PATH" -u 65534:65534 /bin/id | grep -q "uid=65534"
    
    echo "=== E2E User: All tests passed ==="
}

test_combined() {
    echo "=== E2E: Combined Options ==="
    
    # Test 1: Combined options
    echo "Test 1: Combined options"
    mkdir -p /tmp/cspawn-e2e-test-bind
    echo "test content" > /tmp/cspawn-e2e-test-bind/file.txt
    printf "COMBINED_A=alpha\nCOMBINED_B=beta\n" > /tmp/cspawn-e2e-test-env.txt
    run_sudo "$CSPAWN_BIN" -r local://$DATA_DIR \
        -d "$ROOTFS_PATH" \
        -e EXTRA_VAR=extra \
        -E /tmp/cspawn-e2e-test-env.txt \
        -u 0:0 \
        -c /tmp \
        -b /tmp/cspawn-e2e-test-bind:/mnt/data \
        /bin/sh -c 'echo "$COMBINED_A $COMBINED_B $EXTRA_VAR $(pwd) $(cat /mnt/data/file.txt)"' | grep -q "alpha beta extra /tmp test content"
    
    # Test 2: Arguments
    echo "Test 2: Arguments"
    run_sudo "$CSPAWN_BIN" -r local://$DATA_DIR -d "$ROOTFS_PATH" /bin/sh -c 'echo "arg1 arg2"' | grep -q "arg1 arg2"
    
    echo "=== E2E Combined: All tests passed ==="
}

# Main execution
main() {
    echo "Starting E2E tests..."
    
    if [ -n "$TEST_FUNCTION" ]; then
        case "$TEST_FUNCTION" in
            test_local)
                test_local
                ;;
            test_user)
                test_user
                ;;
            test_combined)
                test_combined
                ;;
            *)
                echo "Error: Unknown test function: $TEST_FUNCTION"
                echo "Valid options: test_local, test_user, test_combined"
                exit 1
                ;;
        esac
    else
        test_local
        test_user
        test_combined
    fi
    
    echo "=== All E2E tests passed ==="
}

# Run main function
main
