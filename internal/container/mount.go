package container

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/yylt/cspawn/pkg/log"
)

type Mount struct {
	Source      string
	Destination string
	Type        string
	Flags       uintptr
	Data        string
}

var defaultMounts = []Mount{
	{Source: "proc", Destination: "/proc", Type: "proc", Flags: syscall.MS_NOSUID | syscall.MS_NODEV | syscall.MS_NOEXEC},
	{Source: "sysfs", Destination: "/sys", Type: "sysfs", Flags: syscall.MS_NOSUID | syscall.MS_NODEV | syscall.MS_NOEXEC | syscall.MS_RDONLY},
	{Source: "tmpfs", Destination: "/dev", Type: "tmpfs", Flags: syscall.MS_NOSUID | syscall.MS_STRICTATIME, Data: "mode=755,size=65536k"},
	{Source: "tmpfs", Destination: "/tmp", Type: "tmpfs", Flags: syscall.MS_NOSUID | syscall.MS_NODEV, Data: "mode=1777,size=65536k"},
	{Source: "tmpfs", Destination: "/run", Type: "tmpfs", Flags: syscall.MS_NOSUID | syscall.MS_NODEV, Data: "mode=755,size=65536k"},
	{Source: "devpts", Destination: "/dev/pts", Type: "devpts", Flags: syscall.MS_NOSUID | syscall.MS_NOEXEC, Data: "newinstance,ptmxmode=0666,mode=0620"},
	{Source: "tmpfs", Destination: "/dev/shm", Type: "tmpfs", Flags: syscall.MS_NOSUID | syscall.MS_NODEV, Data: "mode=1777,size=65536k"},
}

type device struct {
	path  string
	mode  uint32
	major int
	minor int
}

var defaultDevices = []device{
	{path: "/dev/null", mode: 0666 | syscall.S_IFCHR, major: 1, minor: 3},
	{path: "/dev/zero", mode: 0666 | syscall.S_IFCHR, major: 1, minor: 5},
	{path: "/dev/full", mode: 0666 | syscall.S_IFCHR, major: 1, minor: 7},
	{path: "/dev/random", mode: 0666 | syscall.S_IFCHR, major: 1, minor: 8},
	{path: "/dev/urandom", mode: 0666 | syscall.S_IFCHR, major: 1, minor: 9},
	{path: "/dev/tty", mode: 0666 | syscall.S_IFCHR, major: 5, minor: 0},
}

func SetupRootfs(rootfs string) error {
	log.Info("Setting up rootfs / 设置 rootfs: %s", rootfs)

	for _, m := range defaultMounts {
		if isMounted(rootfs, m.Destination) {
			log.MountExists(m.Destination)
			continue
		}
		log.MountAdding(m.Destination)
		if err := m.Apply(rootfs); err != nil {
			return fmt.Errorf("failed to mount %s: %w", m.Destination, err)
		}
	}

	log.Debug("Creating devices / 创建设备")
	if err := createDevices(rootfs); err != nil {
		return fmt.Errorf("failed to create devices: %w", err)
	}

	log.Debug("Creating symlinks / 创建符号链接")
	if err := createSymlinks(rootfs); err != nil {
		return fmt.Errorf("failed to create symlinks: %w", err)
	}

	log.Info("Rootfs setup complete / rootfs 设置完成")
	return nil
}

func isMounted(rootfs, dest string) bool {
	mountPoint := filepath.Join(rootfs, dest)

	f, err := os.Open("/proc/mounts")
	if err != nil {
		return false
	}
	defer func() { _ = f.Close() }()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) >= 2 && fields[1] == mountPoint {
			return true
		}
	}

	return false
}

func (m *Mount) Apply(rootFS string) error {
	dest := filepath.Join(rootFS, m.Destination)

	if err := os.MkdirAll(dest, 0755); err != nil {
		return fmt.Errorf("failed to create mount point %s: %w", dest, err)
	}

	if err := syscall.Mount(m.Source, dest, m.Type, m.Flags, m.Data); err != nil {
		return fmt.Errorf("failed to mount %s to %s: %w", m.Source, dest, err)
	}

	return nil
}

func ParseBindMount(bind string) (*Mount, error) {
	parts := strings.SplitN(bind, ":", 3)
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid bind mount format: %s (expected host:container[:options])", bind)
	}

	m := &Mount{
		Source:      parts[0],
		Destination: parts[1],
		Type:        "bind",
		Flags:       syscall.MS_BIND | syscall.MS_REC,
	}

	if len(parts) == 3 {
		for _, opt := range strings.Split(parts[2], ",") {
			switch opt {
			case "ro":
				m.Flags |= syscall.MS_RDONLY
			case "rw":
			default:
				return nil, fmt.Errorf("unknown bind mount option: %s", opt)
			}
		}
	}

	return m, nil
}

func ApplyBindMounts(rootfs string, binds []string) error {
	for _, bind := range binds {
		m, err := ParseBindMount(bind)
		if err != nil {
			return err
		}
		if err := m.Apply(rootfs); err != nil {
			return fmt.Errorf("failed to apply bind mount %s: %w", bind, err)
		}
	}
	return nil
}

func createDevices(rootfs string) error {
	for _, d := range defaultDevices {
		path := filepath.Join(rootfs, d.path)

		if _, err := os.Stat(path); err == nil {
			log.DeviceExists(d.path)
			continue
		}

		log.DeviceCreating(d.path)

		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return fmt.Errorf("failed to create device directory for %s: %w", d.path, err)
		}

		dev := int(d.major<<8 | d.minor)
		if err := syscall.Mknod(path, d.mode, dev); err != nil {
			if os.IsExist(err) {
				continue
			}
			return fmt.Errorf("failed to create device %s: %w", d.path, err)
		}
	}

	ptmxPath := filepath.Join(rootfs, "/dev/ptmx")
	if _, err := os.Lstat(ptmxPath); os.IsNotExist(err) {
		log.Debug("Creating ptmx symlink / 创建 ptmx 符号链接")
		if err := os.Symlink("pts/ptmx", ptmxPath); err != nil {
			return fmt.Errorf("failed to create ptmx symlink: %w", err)
		}
	}

	return nil
}

func createSymlinks(rootfs string) error {
	if err := os.MkdirAll(filepath.Join(rootfs, "/dev/pts"), 0755); err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Join(rootfs, "/dev/shm"), 0755); err != nil {
		return err
	}

	return nil
}

func CleanupMounts(rootfs string) error {
	mountPoints := []string{
		"/dev/shm",
		"/dev/pts",
		"/run",
		"/tmp",
		"/dev",
		"/sys",
		"/proc",
	}

	for _, mp := range mountPoints {
		dest := filepath.Join(rootfs, mp)
		if err := syscall.Unmount(dest, syscall.MNT_DETACH); err != nil {
			if !os.IsNotExist(err) {
				return fmt.Errorf("failed to unmount %s: %w", mp, err)
			}
		}
	}

	return nil
}

func SetupOverlayRootfs(lower, upper, work, merged string) error {
	log.Info("Setting up overlay rootfs / 设置 overlay rootfs")
	log.Debug("Lower layer / 只读层: %s", lower)
	log.Debug("Upper layer / 可写层: %s", upper)
	log.Debug("Work directory / 工作目录: %s", work)
	log.Debug("Merged directory / 合并目录: %s", merged)

	if isOverlayMounted(merged) {
		log.Info("Overlay already mounted, skipping / overlay 已挂载，跳过")
		return nil
	}

	if err := os.MkdirAll(upper, 0755); err != nil {
		return fmt.Errorf("failed to create upper directory: %w", err)
	}

	if err := os.MkdirAll(work, 0755); err != nil {
		return fmt.Errorf("failed to create work directory: %w", err)
	}

	if err := os.MkdirAll(merged, 0755); err != nil {
		return fmt.Errorf("failed to create merged directory: %w", err)
	}

	if err := syscall.Mount("", merged, "", syscall.MS_PRIVATE, ""); err != nil {
		log.Debug("Failed to make merged private / 设置 merged 私有失败: %v", err)
	}

	data := fmt.Sprintf("lowerdir=%s,upperdir=%s,workdir=%s", lower, upper, work)
	if err := syscall.Mount("overlay", merged, "overlay", 0, data); err != nil {
		return fmt.Errorf("failed to mount overlay: %w", err)
	}

	log.Info("Overlay rootfs setup complete / overlay rootfs 设置完成")
	return nil
}

func isOverlayMounted(path string) bool {
	f, err := os.Open("/proc/mounts")
	if err != nil {
		return false
	}
	defer func() { _ = f.Close() }()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) >= 3 && fields[1] == path && fields[2] == "overlay" {
			return true
		}
	}

	return false
}

func CleanupOverlayMounts(merged string) error {
	log.Info("Cleaning up overlay mounts / 清理 overlay 挂载")
	if err := syscall.Unmount(merged, syscall.MNT_DETACH); err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("failed to unmount overlay: %w", err)
		}
	}
	return nil
}
