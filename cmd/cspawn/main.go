package main

import (
	"fmt"
	"os"

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
		fmt.Fprintf(os.Stderr, "Config: runtime=%s socket=%s datadir=%s rootfs=%s image=%s workdir=%s command=%v\n",
			cfg.Runtime, cfg.Socket, cfg.DataDir, cfg.RootfsDir, cfg.Image, cfg.WorkDir, cfg.Command)
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
