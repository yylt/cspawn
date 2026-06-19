package config

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/yylt/cspawn/pkg/utils"
	"gopkg.in/yaml.v3"
)

const defaultDataDir = "/var/lib/cspawn"

const defaultConfigFile = "/etc/cspawn/config.yaml"

type Config struct {
	Runtime   string   `yaml:"runtime,omitempty"`
	Socket    string   `yaml:"socket,omitempty"`
	DataDir   string   `yaml:"data_dir,omitempty"`
	RootfsDir string   `yaml:"rootfs_dir,omitempty"`
	Image     string   `yaml:"image,omitempty"`
	EnvFile   string   `yaml:"env_file,omitempty"`
	Env       []string `yaml:"env,omitempty"`
	User      string   `yaml:"user,omitempty"`
	Chdir     string   `yaml:"chdir,omitempty"`
	Binds     []string `yaml:"binds,omitempty"`
	Command   []string `yaml:"command,omitempty"`
	Debug     bool     `yaml:"debug,omitempty"`
	WorkDir   string   `yaml:"work_dir,omitempty"`
	NoOverlay bool     `yaml:"no_overlay,omitempty"`
	Version   bool     `yaml:"-"`
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
	var configFile string
	flag.StringVar(&configFile, "f", "", "")

	cfg := &Config{}

	flag.StringVar(&cfg.Runtime, "r", "", "")
	flag.StringVar(&cfg.RootfsDir, "d", "", "")
	flag.StringVar(&cfg.Image, "i", "", "")
	flag.StringVar(&cfg.EnvFile, "E", "", "")
	flag.StringVar(&cfg.User, "u", "", "")
	flag.StringVar(&cfg.Chdir, "c", "", "")
	flag.StringVar(&cfg.WorkDir, "w", "", "")
	flag.BoolVar(&cfg.NoOverlay, "no-overlay", false, "")
	flag.BoolVar(&cfg.Version, "v", false, "")

	flag.Var(&stringSliceFlag{&cfg.Env}, "e", "")
	flag.Var(&stringSliceFlag{&cfg.Binds}, "b", "")

	flag.Usage = printUsage
	flag.Parse()

	if configFile == "" {
		if _, err := os.Stat(defaultConfigFile); err == nil {
			configFile = defaultConfigFile
		}
	}

	if configFile != "" {
		fileCfg, err := loadConfigFile(configFile)
		if err != nil {
			return nil, fmt.Errorf("failed to load config file: %w", err)
		}
		cfg = mergeConfig(fileCfg, cfg)
	}

	if cfg.Command == nil {
		cfg.Command = flag.Args()
	} else if len(flag.Args()) > 0 {
		cfg.Command = flag.Args()
	}

	cfg.Debug = cfg.Debug || os.Getenv("CSPAWN_DEBUG") == "1"

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

func loadConfigFile(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	cfg := &Config{}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	return cfg, nil
}

func mergeConfig(fileCfg, cliCfg *Config) *Config {
	result := &Config{}

	if cliCfg.Runtime != "" {
		result.Runtime = cliCfg.Runtime
	} else {
		result.Runtime = fileCfg.Runtime
	}

	if cliCfg.RootfsDir != "" {
		result.RootfsDir = cliCfg.RootfsDir
	} else {
		result.RootfsDir = fileCfg.RootfsDir
	}

	if cliCfg.Image != "" {
		result.Image = cliCfg.Image
	} else {
		result.Image = fileCfg.Image
	}

	if cliCfg.EnvFile != "" {
		result.EnvFile = cliCfg.EnvFile
	} else {
		result.EnvFile = fileCfg.EnvFile
	}

	if cliCfg.User != "" {
		result.User = cliCfg.User
	} else {
		result.User = fileCfg.User
	}

	if cliCfg.Chdir != "" {
		result.Chdir = cliCfg.Chdir
	} else {
		result.Chdir = fileCfg.Chdir
	}

	if cliCfg.WorkDir != "" {
		result.WorkDir = cliCfg.WorkDir
	} else {
		result.WorkDir = fileCfg.WorkDir
	}

	result.Env = append(fileCfg.Env, cliCfg.Env...)
	result.Binds = append(fileCfg.Binds, cliCfg.Binds...)
	result.Debug = cliCfg.Debug || fileCfg.Debug
	result.NoOverlay = cliCfg.NoOverlay || fileCfg.NoOverlay

	if len(cliCfg.Command) > 0 {
		result.Command = cliCfg.Command
	} else {
		result.Command = fileCfg.Command
	}

	return result
}

func (c *Config) Validate() error {
	if c.Version {
		return nil
	}

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
  -f, --config    Config file path (default: /etc/cspawn/config.yaml) / 配置文件路径 (默认: /etc/cspawn/config.yaml)
  -r, --runtime   Runtime type / 运行时类型 (default: local:///var/lib/cspawn)
                  local:///path        - local directory / 本地目录
                  containerd://unix://  - containerd socket
  -d, --dir       Container rootfs directory / 容器 rootfs 目录
  -i, --image     Container image (name:tag or name@sha256:digest) / 容器镜像 (名称:标签 或 名称@sha256:摘要)
  -w, --workdir   Overlay work directory (default: workdirs/<name>) / overlay 工作目录 (默认: workdirs/<名称>)
  --no-overlay    Disable overlay filesystem / 禁用 overlay 文件系统
  -e, --env       Container env (KEY=VALUE) / 容器内环境变量 (可多次指定)
  -E, --envfile   Container env file path / 容器内环境变量文件路径
  -u, --user      Container run user (uid:gid) / 容器内运行用户
  -c, --chdir     Container working directory / 容器内工作目录
  -b, --bind      Bind mount (host:container[:ro|rw]) / 绑定挂载到容器内 (可多次指定)
  -v, --version   Show version / 显示版本信息
  -h, --help      Show help / 显示帮助

Config file format (YAML):
  runtime: local:///var/lib/cspawn
  image: golang:1.25
  work_dir: /var/lib/cspawn/workdirs/myapp
  env:
    - GOPATH=/go
    - GOCACHE=/root/.cache/go-build
  binds:
    - /host/path:/container/path:rw
  user: "1000:1000"
  chdir: /workspace
  debug: false

Examples:
  cspawn -d /path/to/rootfs /bin/bash
  cspawn -i golang:1.25 -e GOPATH=/go /bin/bash
  cspawn -f /path/to/config.yaml /bin/bash
  cspawn -r containerd://unix:///run/containerd.sock -i ubuntu:22.04 /bin/bash
`)
}
