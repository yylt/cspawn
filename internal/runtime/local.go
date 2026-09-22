package runtime

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"time"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/yylt/cspawn/pkg/log"
	"github.com/yylt/cspawn/pkg/utils"
)

type LocalRuntime struct {
	DataDir      string
	RootfsDir    string
	Image        string
	WorkDir      string
	PullTimeout  time.Duration
	LayerTimeout time.Duration
}

func NewLocalRuntime(dataDir, rootfsDir, image, workDir string, pullTimeout, layerTimeout time.Duration) *LocalRuntime {
	return &LocalRuntime{
		DataDir:      dataDir,
		RootfsDir:    rootfsDir,
		Image:        image,
		WorkDir:      workDir,
		PullTimeout:  pullTimeout,
		LayerTimeout: layerTimeout,
	}
}

func (r *LocalRuntime) Prepare() (string, error) {
	if r.RootfsDir != "" {
		log.RootfsPreparing(r.RootfsDir)
		if err := os.MkdirAll(r.RootfsDir, 0755); err != nil {
			return "", fmt.Errorf("failed to create rootfs directory: %w", err)
		}
		log.RootfsReady(r.RootfsDir)
		return r.RootfsDir, nil
	}

	if r.Image == "" {
		return "", fmt.Errorf("either rootfs-dir or image required")
	}

	rootfsName, err := utils.ImageToRootfsName(r.Image)
	if err != nil {
		return "", err
	}
	rootfsDir := filepath.Join(r.DataDir, "rootfs", rootfsName)
	configFile := filepath.Join(r.DataDir, "rootfs", rootfsName+".config.json")

	log.Debug("Rootfs name / rootfs 名称: %s", rootfsName)
	log.Debug("Rootfs path / rootfs 路径: %s", rootfsDir)
	log.Debug("Config file / 配置文件: %s", configFile)

	if _, err := os.Stat(filepath.Join(rootfsDir, "bin")); err == nil {
		log.Info("Rootfs already exists, using cache / rootfs 已存在，使用缓存: %s", rootfsDir)
		return rootfsDir, nil
	}

	log.RootfsPreparing(rootfsDir)

	if err := os.MkdirAll(rootfsDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create rootfs directory: %w", err)
	}

	if err := r.pullAndExtract(r.Image, rootfsDir, configFile); err != nil {
		_ = os.RemoveAll(rootfsDir)
		return "", fmt.Errorf("failed to pull image: %w", err)
	}

	log.RootfsReady(rootfsDir)
	return rootfsDir, nil
}

func (r *LocalRuntime) PrepareOverlay() (string, error) {
	if r.WorkDir != "" {
		workDir := r.WorkDir
		log.Info("Using custom work directory / 使用自定义工作目录: %s", workDir)
		if err := os.MkdirAll(workDir, 0755); err != nil {
			return "", fmt.Errorf("failed to create work directory: %w", err)
		}
		return workDir, nil
	}

	var overlayName string
	if r.Image != "" {
		rootfsName, err := utils.ImageToRootfsName(r.Image)
		if err != nil {
			return "", err
		}
		overlayName = rootfsName
	} else if r.RootfsDir != "" {
		overlayName = filepath.Base(r.RootfsDir)
	} else {
		overlayName = "default"
	}

	workDirsBase := filepath.Join(r.DataDir, "workdirs")
	workDir := filepath.Join(workDirsBase, overlayName)
	log.Info("Creating overlay work directory / 创建 overlay 工作目录: %s", workDir)

	if err := os.MkdirAll(workDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create work directory: %w", err)
	}

	return workDir, nil
}

func (r *LocalRuntime) Cleanup() error {
	return nil
}

func (r *LocalRuntime) pullAndExtract(imageRef, rootfsDir, configFile string) error {
	log.ImagePulling(imageRef)

	// A bounded context propagates into every registry request made through this
	// descriptor, including the per-layer blob reads. Without it a stalled or
	// very slow registry connection blocks the pull forever.
	// 超时 context 会传递到该描述符发起的所有仓库请求（包括每层 blob 的读取）。
	// 若缺少它，卡住或极慢的仓库连接会让拉取永久阻塞。
	ctx := context.Background()
	if r.PullTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, r.PullTimeout)
		defer cancel()
		log.Debug("Pull timeout / 拉取超时: %s", r.PullTimeout)
	} else {
		log.Debug("Pull timeout disabled / 拉取超时已禁用")
	}

	ref, err := name.ParseReference(imageRef)
	if err != nil {
		return fmt.Errorf("failed to parse image reference: %w", err)
	}

	// 尝试获取 manifest list（多架构镜像）
	desc, err := remote.Get(ref, remote.WithContext(ctx))
	if err != nil {
		return fmt.Errorf("failed to fetch image descriptor: %w", err)
	}

	var img v1.Image
	localArch := runtime.GOARCH

	// 检查是否为 manifest list（多架构索引）
	if desc.MediaType.IsIndex() {
		idx, err := desc.ImageIndex()
		if err != nil {
			return fmt.Errorf("failed to fetch image index: %w", err)
		}

		// 从 manifest list 中选择匹配本地架构的镜像
		manifest, err := idx.IndexManifest()
		if err != nil {
			return fmt.Errorf("failed to fetch index manifest: %w", err)
		}

		// 查找匹配的架构
		var matchedDigest *v1.Hash
		for _, m := range manifest.Manifests {
			if m.Platform != nil && m.Platform.Architecture == localArch {
				matchedDigest = &m.Digest
				log.Debug("Found matching architecture / 找到匹配架构: %s", localArch)
				break
			}
		}

		if matchedDigest == nil {
			return fmt.Errorf("no matching architecture found for %s / 未找到匹配 %s 架构的镜像", localArch, localArch)
		}

		// 获取特定架构的镜像
		img, err = idx.Image(*matchedDigest)
		if err != nil {
			return fmt.Errorf("failed to fetch image for arch %s: %w", localArch, err)
		}
	} else {
		// 单架构镜像，直接使用
		img, err = desc.Image()
		if err != nil {
			return fmt.Errorf("failed to fetch image: %w", err)
		}
	}

	config, err := img.ConfigFile()
	if err != nil {
		return fmt.Errorf("failed to get config: %w", err)
	}

	log.Debug("Image config / 镜像配置: OS=%s Arch=%s", config.OS, config.Architecture)

	configData, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}
	if err := os.WriteFile(configFile, configData, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}
	log.Debug("Config saved / 配置已保存: %s", configFile)

	layers, err := img.Layers()
	if err != nil {
		return fmt.Errorf("failed to get layers: %w", err)
	}

	var total int64
	for _, l := range layers {
		if sz, err := l.Size(); err == nil {
			total += sz
		}
	}

	log.Info("Extracting %d layers / 解压 %d 层", len(layers), len(layers))
	if total > 0 {
		log.Info("Total download size / 下载总大小: %s", utils.HumanSize(total))
	}

	for i, layer := range layers {
		size, _ := layer.Size()
		start := time.Now()
		log.Debug("Extracting layer %d/%d (%s) / 解压第 %d/%d 层 (%s)",
			i+1, len(layers), utils.HumanSize(size), i+1, len(layers), utils.HumanSize(size))
		if err := extractLayer(layer, rootfsDir, r.LayerTimeout); err != nil {
			return fmt.Errorf("failed to extract layer %d/%d: %w", i+1, len(layers), err)
		}
		log.Debug("Layer %d/%d done in %s / 第 %d/%d 层完成，耗时 %s",
			i+1, len(layers), time.Since(start).Round(time.Second), i+1, len(layers), time.Since(start).Round(time.Second))
	}

	if err := ctx.Err(); err != nil {
		return fmt.Errorf("image pull timed out after %s: %w", r.PullTimeout, err)
	}

	log.ImagePulled(imageRef)
	return nil
}

func extractLayer(layer interface{ Compressed() (io.ReadCloser, error) }, dest string, stallTimeout time.Duration) error {
	rc, err := layer.Compressed()
	if err != nil {
		return err
	}

	// When a stall timeout is configured, wrap the blob reader with a watchdog:
	// extraction also writes to disk, so we cannot simply cancel on idle, but a
	// layer that delivers no bytes for stallTimeout is treated as failed.
	// 配置了停滞超时时，用看门狗包装 blob reader：解压同时也在写磁盘，不能简单地
	// 按空闲取消，但超过 stallTimeout 没有任何字节到达即视为失败。
	var reader = io.ReadCloser(rc)
	if stallTimeout > 0 {
		reader = newStallWatchdog(rc, stallTimeout)
	}
	defer func() { _ = reader.Close() }()

	gz, err := gzip.NewReader(reader)
	if err != nil {
		return err
	}
	defer func() { _ = gz.Close() }()

	tr := tar.NewReader(gz)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		target := filepath.Join(dest, header.Name)

		if !strings.HasPrefix(filepath.Clean(target), filepath.Clean(dest)) {
			return fmt.Errorf("invalid path: %s", header.Name)
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, os.FileMode(header.Mode)); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return err
			}
			f, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY, os.FileMode(header.Mode))
			if err != nil {
				return err
			}
			if _, err := io.Copy(f, tr); err != nil {
				_ = f.Close()
				return err
			}
			_ = f.Close()
		case tar.TypeSymlink:
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return err
			}
			if err := os.Symlink(header.Linkname, target); err != nil {
				if !os.IsExist(err) {
					return err
				}
			}
		case tar.TypeLink:
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return err
			}
			linkTarget := filepath.Join(dest, header.Linkname)
			if err := os.Link(linkTarget, target); err != nil {
				if !os.IsExist(err) {
					return err
				}
			}
		}
	}

	return nil
}

// stallWatchdog turns a silent network stall into an error. A background
// goroutine watches a last-read timestamp; if the underlying (blocking) blob
// reader delivers no bytes for timeout, it closes the reader to unblock the
// pending Read and reports the stall to the caller.
// stallWatchdog 把静默的网络停滞转为错误。后台 goroutine 监控最近一次读取时间；
// 若底层（阻塞的）blob reader 在 timeout 内没有任何字节到达，就关闭 reader 以解除
// 阻塞，并把停滞作为错误返回给调用方。
type stallWatchdog struct {
	rc       io.ReadCloser
	timeout  time.Duration
	lastRead atomic.Int64 // unix nanos of last successful read / 最近一次成功读取时间
	read     atomic.Int64 // bytes read (for diagnostics) / 已读取字节数（用于诊断）
	stop     chan struct{}
	fired    atomic.Bool
}

func newStallWatchdog(rc io.ReadCloser, timeout time.Duration) *stallWatchdog {
	w := &stallWatchdog{rc: rc, timeout: timeout, stop: make(chan struct{})}
	w.lastRead.Store(time.Now().UnixNano())
	go w.watch()
	return w
}

func (w *stallWatchdog) watch() {
	interval := w.timeout / 4
	if interval < 100*time.Millisecond {
		interval = 100 * time.Millisecond
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-w.stop:
			return
		case now := <-ticker.C:
			last := time.Unix(0, w.lastRead.Load())
			if now.Sub(last) >= w.timeout {
				w.fired.Store(true)
				// Closing the blob reader unblocks the in-flight Read.
				// 关闭 blob reader 以解除正在进行的 Read 阻塞。
				_ = w.rc.Close()
				return
			}
		}
	}
}

func (w *stallWatchdog) Read(p []byte) (int, error) {
	n, err := w.rc.Read(p)
	if n > 0 {
		w.read.Add(int64(n))
		w.lastRead.Store(time.Now().UnixNano())
	}
	if w.fired.Load() {
		return n, fmt.Errorf("network stalled: no data for %s (received %s) / 网络停滞：%s 无数据（已接收 %s）",
			w.timeout, utils.HumanSize(w.read.Load()), w.timeout, utils.HumanSize(w.read.Load()))
	}
	return n, err
}

func (w *stallWatchdog) Close() error {
	close(w.stop)
	return w.rc.Close()
}
