package runtime

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	containerd "github.com/containerd/containerd/v2/client"
	"github.com/containerd/containerd/v2/pkg/namespaces"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/opencontainers/image-spec/identity"
	"github.com/yylt/cspawn/pkg/log"
	"github.com/yylt/cspawn/pkg/utils"
)

type ContainerdRuntime struct {
	Socket    string
	Image     string
	DataDir   string
	Namespace string
}

func NewContainerdRuntime(socket, image, dataDir string) *ContainerdRuntime {
	r := &ContainerdRuntime{
		Socket:    socket,
		Image:     image,
		DataDir:   dataDir,
		Namespace: "default",
	}
	return r
}

func (r *ContainerdRuntime) Prepare() (string, error) {
	rootfsName, err := utils.ImageToRootfsName(r.Image)
	if err != nil {
		return "", err
	}
	rootfsDir := filepath.Join(r.DataDir, "rootfs", rootfsName)

	log.Debug("Rootfs name / rootfs 名称: %s", rootfsName)
	log.Debug("Rootfs path / rootfs 路径: %s", rootfsDir)

	if _, err := os.Stat(filepath.Join(rootfsDir, "bin")); err == nil {
		log.Info("Rootfs already exists, using cache / rootfs 已存在，使用缓存: %s", rootfsDir)
		return rootfsDir, nil
	}

	log.RootfsPreparing(rootfsDir)

	log.Debug("Connecting to containerd / 连接 containerd: %s", r.Socket)
	ctx := namespaces.WithNamespace(context.Background(), r.Namespace)

	client, err := containerd.New(r.Socket)
	if err != nil {
		return "", fmt.Errorf("failed to connect to containerd at %s: %w", r.Socket, err)
	}
	defer func() { _ = client.Close() }()
	log.Debug("Connected to containerd / 已连接 containerd")

	ref, err := name.ParseReference(r.Image)
	if err != nil {
		return "", fmt.Errorf("failed to parse image reference: %w", err)
	}

	log.ImagePulling(r.Image)

	// 构建本地平台字符串
	localPlatform := fmt.Sprintf("linux/%s", runtime.GOARCH)
	image, err := client.Pull(ctx, ref.String(), containerd.WithPullUnpack, containerd.WithPlatform(localPlatform))
	if err != nil {
		return "", fmt.Errorf("failed to pull image %s: %w", r.Image, err)
	}
	log.ImagePulled(r.Image)

	diffIDs, err := image.RootFS(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to get rootfs: %w", err)
	}

	chainID := identity.ChainID(diffIDs).String()
	log.Debug("Chain ID / 链 ID: %s", chainID)

	log.Debug("Preparing snapshot / 准备快照")
	snapshotter := client.SnapshotService("")
	mounts, err := snapshotter.Prepare(ctx, rootfsName+"-view", chainID)
	if err != nil {
		return "", fmt.Errorf("failed to prepare snapshot: %w", err)
	}

	if err := os.MkdirAll(rootfsDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create rootfs directory: %w", err)
	}

	if len(mounts) > 0 {
		log.Debug("Mounting rootfs / 挂载 rootfs")
		m := mounts[0]
		if err := m.Mount(rootfsDir); err != nil {
			return "", fmt.Errorf("failed to mount rootfs: %w", err)
		}
	}

	log.RootfsReady(rootfsDir)
	return rootfsDir, nil
}

func (r *ContainerdRuntime) Cleanup() error {
	return nil
}
