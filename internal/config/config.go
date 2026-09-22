package config

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/yylt/cspawn/pkg/utils"
	"gopkg.in/yaml.v3"
)

const defaultDataDir = "/var/lib/cspawn"

const defaultConfigFile = "/etc/cspawn/config.yaml"

// defaultPullTimeout bounds the whole image pull (all layers). The default is
// deliberately very large: registries may serve multi-GiB layers over slow
// links, and cspawn must not abort a pull that is still making progress. True
// hangs are caught much sooner by defaultLayerTimeout.
// defaultPullTimeout 限制整个镜像拉取（所有层）的耗时。默认值特意设置得很大：
// 镜像仓库可能通过慢速链路传输数 GB 的层，只要有进展就不应中断。真正的卡死会由
// defaultLayerTimeout 更快地发现。
const defaultPullTimeout = 2 * time.Hour

// defaultLayerTimeout aborts a pull when a single layer stops delivering data
// for this long, turning a silent hang into an actionable error.
// defaultLayerTimeout 在单个层超过该时长没有任何数据时中断拉取，将静默卡死转为明确报错。
const defaultLayerTimeout = 5 * time.Minute

// Duration is a time.Duration that can be parsed from YAML strings ("30m")
// and from command-line flags ("--pull-timeout 30m").
// Duration 是可从 YAML 字符串（"30m"）与命令行参数（"--pull-timeout 30m"）解析的时长。
type Duration time.Duration

func (d *Duration) UnmarshalYAML(value *yaml.Node) error {
	var s string
	if err := value.Decode(&s); err != nil {
		return err
	}
	v, err := time.ParseDuration(s)
	if err != nil {
		return fmt.Errorf("invalid duration %q: %w", s, err)
	}
	*d = Duration(v)
	return nil
}

func (d *Duration) Set(s string) error {
	v, err := time.ParseDuration(s)
	if err != nil {
		return err
	}
	*d = Duration(v)
	return nil
}

func (d Duration) String() string { return time.Duration(d).String() }

// Value returns the underlying time.Duration.
func (d Duration) Value() time.Duration { return time.Duration(d) }

type Config struct {
	Runtime      string   `yaml:"runtime,omitempty"`
	Socket       string   `yaml:"socket,omitempty"`
	DataDir      string   `yaml:"data_dir,omitempty"`
	RootfsDir    string   `yaml:"rootfs_dir,omitempty"`
	Image        string   `yaml:"image,omitempty"`
	EnvFile      string   `yaml:"env_file,omitempty"`
	Env          []string `yaml:"env,omitempty"`
	User         string   `yaml:"user,omitempty"`
	Chdir        string   `yaml:"chdir,omitempty"`
	Binds        []string `yaml:"binds,omitempty"`
	Command      []string `yaml:"command,omitempty"`
	Debug        bool     `yaml:"debug,omitempty"`
	WorkDir      string   `yaml:"work_dir,omitempty"`
	Overlay      bool     `yaml:"overlay,omitempty"`
	PullTimeout  Duration `yaml:"pull_timeout,omitempty"`
	LayerTimeout Duration `yaml:"layer_timeout,omitempty"`
	Version      bool     `yaml:"-"`
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
	flag.BoolVar(&cfg.Overlay, "overlay", false, "")
	flag.BoolVar(&cfg.Version, "v", false, "")

	flag.Var(&cfg.PullTimeout, "pull-timeout", "")
	flag.Var(&cfg.LayerTimeout, "layer-timeout", "")

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

	// A negative value explicitly disables the timeout; only the zero value
	// (unset) falls back to the default. / 负值表示显式禁用超时；仅零值（未设置）回退到默认值。
	if cfg.PullTimeout == 0 {
		cfg.PullTimeout = Duration(defaultPullTimeout)
	}
	if cfg.LayerTimeout == 0 {
		cfg.LayerTimeout = Duration(defaultLayerTimeout)
	}

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
	result.Overlay = cliCfg.Overlay || fileCfg.Overlay

	if cliCfg.PullTimeout != 0 {
		result.PullTimeout = cliCfg.PullTimeout
	} else {
		result.PullTimeout = fileCfg.PullTimeout
	}
	if cliCfg.LayerTimeout != 0 {
		result.LayerTimeout = cliCfg.LayerTimeout
	} else {
		result.LayerTimeout = fileCfg.LayerTimeout
	}

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
  --overlay       Enable overlay filesystem / 启用 overlay 文件系统
  --pull-timeout  Overall image pull timeout, e.g. 2h, 30m; negative disables (default: 2h) / 镜像拉取总超时；负数表示禁用 (默认: 2h)
  --layer-timeout Stall timeout per layer, e.g. 5m; negative disables (default: 5m) / 单层无数据超时；负数表示禁用 (默认: 5m)
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
  pull_timeout: 2h
  layer_timeout: 5m
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
