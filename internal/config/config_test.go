package config

import (
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

func TestDurationUnmarshalYAML(t *testing.T) {
	var cfg struct {
		PullTimeout  Duration `yaml:"pull_timeout"`
		LayerTimeout Duration `yaml:"layer_timeout"`
	}
	if err := yaml.Unmarshal([]byte("pull_timeout: 45m\nlayer_timeout: 90s\n"), &cfg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if cfg.PullTimeout.Value() != 45*time.Minute {
		t.Fatalf("pull_timeout = %s, want 45m", cfg.PullTimeout)
	}
	if cfg.LayerTimeout.Value() != 90*time.Second {
		t.Fatalf("layer_timeout = %s, want 90s", cfg.LayerTimeout)
	}
}

func TestDurationUnmarshalInvalid(t *testing.T) {
	var cfg struct {
		PullTimeout Duration `yaml:"pull_timeout"`
	}
	if err := yaml.Unmarshal([]byte("pull_timeout: nope\n"), &cfg); err == nil {
		t.Fatal("expected error for invalid duration")
	}
}

func TestDurationSet(t *testing.T) {
	var d Duration
	if err := d.Set("2h"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if d.Value() != 2*time.Hour {
		t.Fatalf("duration = %s, want 2h", d)
	}
	if err := d.Set("bogus"); err == nil {
		t.Fatal("expected error for invalid duration")
	}
}

// TestMergeConfigTimeoutPrecedence verifies that command-line values override
// the config file while zero (unset) values fall back to the file value.
// TestMergeConfigTimeoutPrecedence 验证命令行值覆盖配置文件，而零值（未设置）回退到文件值。
func TestMergeConfigTimeoutPrecedence(t *testing.T) {
	fileCfg := &Config{
		PullTimeout:  Duration(10 * time.Minute),
		LayerTimeout: Duration(time.Minute),
	}
	cliCfg := &Config{PullTimeout: Duration(5 * time.Minute)}

	got := mergeConfig(fileCfg, cliCfg)
	if got.PullTimeout.Value() != 5*time.Minute {
		t.Fatalf("PullTimeout = %s, want CLI value 5m", got.PullTimeout)
	}
	if got.LayerTimeout.Value() != time.Minute {
		t.Fatalf("LayerTimeout = %s, want file value 1m", got.LayerTimeout)
	}
}

// TestMergeConfigNegativeDisables verifies a negative CLI value survives the
// merge (an explicit "disable the timeout" request).
// TestMergeConfigNegativeDisables 验证负的命令行值能在合并后保留（显式禁用超时）。
func TestMergeConfigNegativeDisables(t *testing.T) {
	fileCfg := &Config{PullTimeout: Duration(10 * time.Minute)}
	cliCfg := &Config{PullTimeout: Duration(-1)}

	got := mergeConfig(fileCfg, cliCfg)
	if got.PullTimeout != Duration(-1) {
		t.Fatalf("PullTimeout = %s, want -1s (disabled)", got.PullTimeout)
	}
}
