package runtime

import "github.com/yylt/cspawn/internal/config"

type Runtime interface {
	Prepare() (string, error)
	Cleanup() error
}

func New(cfg *config.Config) (Runtime, error) {
	switch cfg.Runtime {
	case "local":
		return NewLocalRuntime(cfg.DataDir, cfg.RootfsDir, cfg.Image, cfg.WorkDir), nil
	case "containerd":
		return NewContainerdRuntime(cfg.Socket, cfg.Image, cfg.DataDir), nil
	default:
		return nil, nil
	}
}
