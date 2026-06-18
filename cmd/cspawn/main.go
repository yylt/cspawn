package main

import (
	"fmt"
	"os"

	"github.com/yylt/cspawn/internal/config"
	"github.com/yylt/cspawn/internal/container"
	"github.com/yylt/cspawn/internal/runtime"
	"github.com/yylt/cspawn/pkg/utils"
)

func main() {
	cfg, err := config.Parse()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if cfg.Debug {
		fmt.Fprintf(os.Stderr, "Config: runtime=%s socket=%s datadir=%s rootfs=%s image=%s command=%v\n",
			cfg.Runtime, cfg.Socket, cfg.DataDir, cfg.RootfsDir, cfg.Image, cfg.Command)
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

	c := container.New(
		id,
		rootfs,
		cfg.Command,
		cfg.Env,
		cfg.User,
		cfg.Chdir,
		cfg.Binds,
	)

	if err := c.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
