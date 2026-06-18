package runtime

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/yylt/cspawn/pkg/log"
	"github.com/yylt/cspawn/pkg/utils"
)

type LocalRuntime struct {
	DataDir   string
	RootfsDir string
	Image     string
	WorkDir   string
}

func NewLocalRuntime(dataDir, rootfsDir, image, workDir string) *LocalRuntime {
	return &LocalRuntime{
		DataDir:   dataDir,
		RootfsDir: rootfsDir,
		Image:     image,
		WorkDir:   workDir,
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

	ref, err := name.ParseReference(imageRef)
	if err != nil {
		return fmt.Errorf("failed to parse image reference: %w", err)
	}

	img, err := remote.Image(ref)
	if err != nil {
		return fmt.Errorf("failed to fetch image: %w", err)
	}

	config, err := img.ConfigFile()
	if err != nil {
		return fmt.Errorf("failed to get config: %w", err)
	}

	log.Debug("Image config / 镜像配置: OS=%s Arch=%s", config.OS, config.Architecture)

	localArch := runtime.GOARCH
	if config.Architecture != localArch {
		return fmt.Errorf("image architecture mismatch / 镜像架构不匹配: image=%s, local=%s", config.Architecture, localArch)
	}

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

	log.Info("Extracting %d layers / 解压 %d 层", len(layers), len(layers))

	for i, layer := range layers {
		log.Debug("Extracting layer %d / 解压第 %d 层", i+1, i+1)
		if err := extractLayer(layer, rootfsDir); err != nil {
			return fmt.Errorf("failed to extract layer: %w", err)
		}
	}

	log.ImagePulled(imageRef)
	return nil
}

func extractLayer(layer interface{ Compressed() (io.ReadCloser, error) }, dest string) error {
	rc, err := layer.Compressed()
	if err != nil {
		return err
	}
	defer func() { _ = rc.Close() }()

	gz, err := gzip.NewReader(rc)
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
