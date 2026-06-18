package config

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/yylt/cspawn/pkg/utils"
)

const defaultDataDir = "/var/lib/cspawn"

type Config struct {
	Runtime   string
	Socket    string
	DataDir   string
	RootfsDir string
	Image     string
	EnvFile   string
	Env       []string
	User      string
	Chdir     string
	Binds     []string
	Command   []string
	Debug     bool
}

type stringSliceFlag struct {
	slice *[]string
}

func (f *stringSliceFlag) String() string {
	if f == nil || f.slice == nil {
		return ""
	}
	return strings.Join(*f.slice, ", ")
}

func (f *stringSliceFlag) Set(value string) error {
	*f.slice = append(*f.slice, value)
	return nil
}

func Parse() (*Config, error) {
	cfg := &Config{}

	flag.StringVar(&cfg.Runtime, "r", "", "")
	flag.StringVar(&cfg.RootfsDir, "d", "", "")
	flag.StringVar(&cfg.Image, "i", "", "")
	flag.StringVar(&cfg.EnvFile, "E", "", "")
	flag.StringVar(&cfg.User, "u", "", "")
	flag.StringVar(&cfg.Chdir, "c", "", "")

	flag.Var(&stringSliceFlag{&cfg.Env}, "e", "")
	flag.Var(&stringSliceFlag{&cfg.Binds}, "b", "")

	flag.Usage = printUsage
	flag.Parse()

	cfg.Command = flag.Args()
	cfg.Debug = os.Getenv("CSPAWN_DEBUG") == "1"

	if cfg.Runtime == "" {
		cfg.Runtime = "local://" + defaultDataDir
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	if cfg.EnvFile != "" {
		envFromFile, err := loadEnvFile(cfg.EnvFile)
		if err != nil {
			return nil, fmt.Errorf("failed to load env file: %w", err)
		}
		cfg.Env = append(envFromFile, cfg.Env...)
	}

	return cfg, nil
}

func (c *Config) Validate() error {
	if len(c.Command) == 0 {
		return fmt.Errorf("command required / 需要指定命令")
	}

	runtimeType, addr, err := ParseRuntime(c.Runtime)
	if err != nil {
		return err
	}
	c.Runtime = runtimeType

	switch c.Runtime {
	case "local":
		c.DataDir = addr
	case "containerd":
		c.Socket = addr
	default:
		return fmt.Errorf("unsupported runtime: %s / 不支持的运行时", c.Runtime)
	}

	if c.RootfsDir != "" && c.Image != "" {
		return fmt.Errorf("-d and -i are mutually exclusive / -d 和 -i 不能同时使用")
	}

	if c.RootfsDir == "" && c.Image == "" {
		return fmt.Errorf("either -d or -i required / 需要指定 -d 或 -i")
	}

	if c.Runtime == "containerd" && c.Image == "" {
		return fmt.Errorf("-i required when using containerd / 使用 containerd 时需要指定 -i")
	}

	if c.Image != "" {
		normalized, err := utils.NormalizeImage(c.Image)
		if err != nil {
			return err
		}
		c.Image = normalized
	}

	return nil
}

func ParseRuntime(runtime string) (string, string, error) {
	switch {
	case strings.HasPrefix(runtime, "local://"):
		addr := strings.TrimPrefix(runtime, "local://")
		if addr == "" {
			addr = defaultDataDir
		}
		return "local", addr, nil
	case strings.HasPrefix(runtime, "containerd://"):
		addr := strings.TrimPrefix(runtime, "containerd://")
		if addr == "" {
			return "", "", fmt.Errorf("containerd socket address required / 需要 containerd socket 地址")
		}
		return "containerd", addr, nil
	default:
		return "", "", fmt.Errorf("invalid runtime format: %s / 无效的运行时格式", runtime)
	}
}

func loadEnvFile(path string) ([]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()

	var envs []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		envs = append(envs, line)
	}

	return envs, scanner.Err()
}

func printUsage() {
	fmt.Fprintf(os.Stderr, `Usage: cspawn [options] <command> [args...]

Options:
  -r, --runtime   Runtime type / 运行时类型 (default: local:///var/lib/cspawn)
                  local:///path        - local directory / 本地目录
                  containerd://unix://  - containerd socket
  -d, --dir       Container rootfs directory / 容器 rootfs 目录
  -i, --image     Container image (name:tag) / 容器镜像 (名称:标签)
  -e, --env       Container env (KEY=VALUE) / 容器内环境变量 (可多次指定)
  -E, --envfile   Container env file path / 容器内环境变量文件路径
  -u, --user      Container run user (uid:gid) / 容器内运行用户
  -c, --chdir     Container working directory / 容器内工作目录
  -b, --bind      Bind mount (host:container[:ro|rw]) / 绑定挂载到容器内 (可多次指定)
  -h, --help      Show help / 显示帮助

Examples:
  cspawn -d /path/to/rootfs /bin/bash
  cspawn -i golang:1.25 -e GOPATH=/go /bin/bash
  cspawn -r containerd://unix:///run/containerd.sock -i ubuntu:22.04 /bin/bash
`)
}
