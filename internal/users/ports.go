package users

import (
	"errors"
	"fmt"

	"github.com/pdasilem/openclaw-multi/internal/config"
	"github.com/pdasilem/openclaw-multi/internal/state"
)

var ErrInvalidPortConfig = errors.New("invalid port allocation config")

// AllocateGatewayPort returns the lowest free gateway port for cfg.
func AllocateGatewayPort(cfg *config.OverlayConfig, existing []state.User) (int, error) {
	if cfg == nil {
		cfg = config.Defaults()
	}
	start := cfg.PortRangeStart
	step := cfg.PortRangeStep
	if start <= 0 {
		return 0, fmt.Errorf("%w: port_range_start must be positive", ErrInvalidPortConfig)
	}
	if step < 20 {
		return 0, fmt.Errorf("%w: port_range_step must be at least 20", ErrInvalidPortConfig)
	}

	used := make(map[int]struct{}, len(existing))
	for _, u := range existing {
		if u.Port > 0 {
			used[u.Port] = struct{}{}
		}
	}
	for port := start; ; port += step {
		if _, ok := used[port]; !ok {
			return port, nil
		}
	}
}
