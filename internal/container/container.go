package container

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/yylt/cspawn/pkg/log"
)

type Container struct {
	ID       string
	RootFS   string
	Command  []string
	Env      []string
	User     string
	WorkDir  string
	Binds    []string
}

func New(id, rootfs string, command, env []string, user, workdir string, binds []string) *Container {
	return &Container{
		ID:      id,
		RootFS:  rootfs,
		Command: command,
		Env:     env,
		User:    user,
		WorkDir: workdir,
		Binds:   binds,
	}
}

func (c *Container) Run() error {
	log.Info("Container ID / 容器 ID: %s", c.ID)
	log.Info("RootFS / rootfs 路径: %s", c.RootFS)
	log.ContainerStarting(c.Command)

	if c.User != "" {
		log.Debug("User / 用户: %s", c.User)
	}
	if c.WorkDir != "" {
		log.Debug("WorkDir / 工作目录: %s", c.WorkDir)
	}
	if len(c.Binds) > 0 {
		log.Debug("Binds / 绑定挂载: %v", c.Binds)
	}
	if len(c.Env) > 0 {
		log.Debug("Env / 环境变量: %v", c.Env)
	}

	log.Debug("Unsharing mount namespace / 分离挂载命名空间")
	if err := syscall.Unshare(syscall.CLONE_NEWNS); err != nil {
		return fmt.Errorf("failed to unshare mount namespace: %w", err)
	}

	if err := syscall.Mount("", "/", "", syscall.MS_REC|syscall.MS_PRIVATE, ""); err != nil {
		log.Debug("Failed to make / private, trying bind mount / 使 / 私有失败，尝试绑定挂载")
	}

	if err := SetupRootfs(c.RootFS); err != nil {
		return fmt.Errorf("failed to setup rootfs: %w", err)
	}
	defer func() { _ = CleanupMounts(c.RootFS) }()

	if len(c.Binds) > 0 {
		log.Info("Applying bind mounts / 应用绑定挂载")
		if err := ApplyBindMounts(c.RootFS, c.Binds); err != nil {
			return fmt.Errorf("failed to apply bind mounts: %w", err)
		}
	}

	log.Debug("Performing pivot_root / 执行 pivot_root")
	if err := c.pivotRoot(); err != nil {
		log.Debug("pivot_root failed, falling back to chroot / pivot_root 失败，回退到 chroot: %v", err)
		if err := c.chroot(); err != nil {
			return fmt.Errorf("failed to chroot: %w", err)
		}
	}

	if err := c.setHostname(); err != nil {
		return fmt.Errorf("failed to set hostname: %w", err)
	}

	if c.WorkDir != "" {
		log.Debug("Changing to workdir / 切换到工作目录: %s", c.WorkDir)
		if err := os.Chdir(c.WorkDir); err != nil {
			return fmt.Errorf("failed to change directory to %s: %w", c.WorkDir, err)
		}
	}

	if err := c.setUser(); err != nil {
		return fmt.Errorf("failed to set user: %w", err)
	}

	binary, err := exec.LookPath(c.Command[0])
	if err != nil {
		return fmt.Errorf("command not found: %s", c.Command[0])
	}
	log.Debug("Binary / 可执行文件: %s", binary)

	env := c.getEnv()

	log.ContainerExec(c.Command)
	if err := syscall.Exec(binary, c.Command, env); err != nil {
		return fmt.Errorf("failed to exec: %w", err)
	}

	return nil
}

func (c *Container) pivotRoot() error {
	putOld := filepath.Join(c.RootFS, "/.pivot_root")
	if err := os.MkdirAll(putOld, 0700); err != nil {
		return err
	}

	log.Debug("Bind mounting rootfs / 绑定挂载 rootfs")
	if err := syscall.Mount(c.RootFS, c.RootFS, "", syscall.MS_BIND|syscall.MS_REC, ""); err != nil {
		return fmt.Errorf("bind mount rootfs failed: %w", err)
	}

	log.Debug("Calling pivot_root / 调用 pivot_root")
	if err := syscall.PivotRoot(c.RootFS, putOld); err != nil {
		return fmt.Errorf("pivot_root failed: %w", err)
	}

	if err := os.Chdir("/"); err != nil {
		return err
	}

	putOld = "/.pivot_root"
	log.Debug("Unmounting old root / 卸载旧根目录")
	if err := syscall.Unmount(putOld, syscall.MNT_DETACH); err != nil {
		return fmt.Errorf("unmount pivot_root old failed: %w", err)
	}

	return os.RemoveAll(putOld)
}

func (c *Container) chroot() error {
	log.Debug("Using chroot / 使用 chroot")
	if err := syscall.Chroot(c.RootFS); err != nil {
		return fmt.Errorf("chroot failed: %w", err)
	}
	return os.Chdir("/")
}

func (c *Container) setHostname() error {
	hostname := "cspawn"
	log.Debug("Setting hostname / 设置主机名: %s", hostname)
	if err := syscall.Sethostname([]byte(hostname)); err != nil {
		return fmt.Errorf("sethostname failed: %w", err)
	}
	return nil
}

func (c *Container) setUser() error {
	if c.User == "" {
		return nil
	}

	parts := strings.SplitN(c.User, ":", 2)
	userName := parts[0]

	uid, err := strconv.ParseUint(userName, 10, 32)
	if err != nil {
		return fmt.Errorf("invalid user: %s (only numeric UID supported)", c.User)
	}

	gid := uint64(uid)
	if len(parts) == 2 {
		gid, err = strconv.ParseUint(parts[1], 10, 32)
		if err != nil {
			return fmt.Errorf("invalid group: %s (only numeric GID supported)", parts[1])
		}
	}

	log.Debug("Setting UID/GID / 设置 UID/GID: %d/%d", uid, gid)

	if err := syscall.Setgid(int(gid)); err != nil {
		return fmt.Errorf("setgid failed: %w", err)
	}

	if err := syscall.Setuid(int(uid)); err != nil {
		return fmt.Errorf("setuid failed: %w", err)
	}

	return nil
}

func (c *Container) getEnv() []string {
	if len(c.Env) > 0 {
		return c.Env
	}

	defaultEnv := []string{
		"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
		"HOME=/root",
		"TERM=xterm",
		"HOSTNAME=cspawn",
	}

	return defaultEnv
}
