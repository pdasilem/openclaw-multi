package users

import (
	"errors"
	"testing"

	"github.com/pdasilem/openclaw-multi/internal/config"
	"github.com/pdasilem/openclaw-multi/internal/state"
)

func TestAllocateGatewayPortReusesLowestFreePort(t *testing.T) {
	cfg := config.Defaults()
	port, err := AllocateGatewayPort(cfg, []state.User{
		{Username: "alice", Port: 18789},
		{Username: "carol", Port: 18829},
	})
	if err != nil {
		t.Fatalf("AllocateGatewayPort: %v", err)
	}
	if port != 18809 {
		t.Fatalf("expected lowest free port 18809, got %d", port)
	}
}

func TestAllocateGatewayPortRejectsSmallStep(t *testing.T) {
	cfg := config.Defaults()
	cfg.PortRangeStep = 10
	_, err := AllocateGatewayPort(cfg, nil)
	if !errors.Is(err, ErrInvalidPortConfig) {
		t.Fatalf("expected ErrInvalidPortConfig, got %v", err)
	}
}
