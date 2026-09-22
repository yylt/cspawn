package main

import (
	"fmt"
	"os"
	goruntime "runtime"

	"github.com/yylt/cspawn/internal/config"
	"github.com/yylt/cspawn/internal/container"
	"github.com/yylt/cspawn/internal/runtime"
	"github.com/yylt/cspawn/pkg/utils"
)

var (
	version   = "dev"
	commit    = "unknown"
	buildDate = "unknown"
)

func main() {
	// cspawn is a bootstrap process: it sets up the container and hands off to
	// the command, doing essentially no CPU-bound work. Two Ps are enough to run
	// the GOMAXPROCS-1 runtime (sysmon, GC assist) alongside the main goroutine,
	// and it keeps a burst of parallel cspawn launches from flooding the host.
	//
	// cspawn 是引导进程：只做容器搭建并交接给目标命令，几乎没有 CPU 密集工作。
	// 2 个 P 足以让运行时(GOMAXPROCS-1 的 sysmon/GC)与主协程并行，
	// 同时避免大量 cspawn 并发启动时打满宿主机。
	goruntime.GOMAXPROCS(2)

	cfg, err := config.Parse()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if cfg.Version {
		fmt.Printf("cspawn %s (commit: %s, built: %s)\n", version, commit, buildDate)
		os.Exit(0)
	}

	if cfg.Debug {
		fmt.Fprintf(os.Stderr, "cspawn %s (commit: %s, built: %s)\n", version, commit, buildDate)
		fmt.Fprintf(os.Stderr, "Config: runtime=%s socket=%s datadir=%s rootfs=%s image=%s workdir=%s pullTimeout=%s layerTimeout=%s command=%v\n",
			cfg.Runtime, cfg.Socket, cfg.DataDir, cfg.RootfsDir, cfg.Image, cfg.WorkDir,
			cfg.PullTimeout, cfg.LayerTimeout, cfg.Command)
	}

	rt, err := runtime.New(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	rootfs, err := rt.Prepare()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error preparing rootfs: %v\n", err)
		os.Exit(1)
	}

	id := utils.GenerateID()

	var overlayDir string
	if cfg.Overlay {
		if localRt, ok := rt.(*runtime.LocalRuntime); ok {
			overlayDir, err = localRt.PrepareOverlay()
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error preparing overlay: %v\n", err)
				os.Exit(1)
			}
		}
	}

	c := container.New(
		id,
		rootfs,
		cfg.Command,
		cfg.Env,
		cfg.User,
		cfg.Chdir,
		cfg.Binds,
		overlayDir,
	)

	if err := c.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
