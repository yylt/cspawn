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

echo "Preparing rootfs using cspawn..."
echo "Image: $IMAGE"
echo "cspawn binary: $CSPAWN_BIN"
echo "Data directory: $DATA_DIR"

# Check if cspawn binary exists
if [ ! -f "$CSPAWN_BIN" ]; then
    echo "Error: cspawn binary not found at $CSPAWN_BIN"
    echo "Please build cspawn first: make build"
    exit 1
fi

# Make binary executable
chmod +x "$CSPAWN_BIN"

# Use cspawn to pull the image
# We run /bin/true to trigger image pull without actually starting a container
echo "Pulling image using cspawn..."

# Check if we need sudo
if [ "$(id -u)" -eq 0 ]; then
    # Already root
    "$CSPAWN_BIN" -r local -i "$IMAGE" /bin/true || true
else
    # Try sudo without password first
    if sudo -n true 2>/dev/null; then
        sudo "$CSPAWN_BIN" -r local -i "$IMAGE" /bin/true || true
    else
        echo "Warning: sudo requires password. Please run as root or configure passwordless sudo."
        echo "Attempting to run without sudo (may fail)..."
        "$CSPAWN_BIN" -r local -i "$IMAGE" /bin/true || true
    fi
fi

# Calculate rootfs path
# Extract image name and tag, normalize to match cspawn's ImageToRootfsName function
IMAGE_NAME=$(echo "$IMAGE" | sed 's|.*/||' | sed 's|:|_|')
ROOTFS_PATH="$DATA_DIR/rootfs/$IMAGE_NAME"

echo "Rootfs path: $ROOTFS_PATH"

# Verify rootfs exists
if [ -d "$ROOTFS_PATH" ]; then
    echo "Rootfs prepared successfully at: $ROOTFS_PATH"
    # List some contents to verify
    echo "Rootfs contents (first 10 items):"
    ls -la "$ROOTFS_PATH" | head -10
else
    echo "Error: Rootfs directory not found at $ROOTFS_PATH"
    echo "Available directories in $DATA_DIR/rootfs/:"
    ls -la "$DATA_DIR/rootfs/" 2>/dev/null || echo "Directory does not exist"
    exit 1
fi

echo "Rootfs preparation completed."