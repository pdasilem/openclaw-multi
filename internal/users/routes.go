package users

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/pdasilem/openclaw-multi/internal/config"
	"github.com/pdasilem/openclaw-multi/internal/state"
)

// ErrMissingDomain is returned when a gateway hostname cannot be built.
var ErrMissingDomain = errors.New("missing domain or subdomain")

// RoutePublisher records or publishes user gateway routes.
type RoutePublisher interface {
	EnableUserGateway(ctx context.Context, user state.User) (state.Route, error)
	DisableUserRoutes(ctx context.Context, username string) error
	DeleteUserRoutes(ctx context.Context, username string) error
}

// StateRoutePublisher records intended routes in state without mutating cloudflared.
type StateRoutePublisher struct {
	Store  *state.Store
	Config *config.OverlayConfig
}

// EnableUserGateway creates or enables the user's gateway route in state.
func (p StateRoutePublisher) EnableUserGateway(ctx context.Context, user state.User) (state.Route, error) {
	if p.Store == nil {
		return state.Route{}, errors.New("state route publisher requires store")
	}
	hostname, err := GatewayHostname(p.Config, user.Username)
	if err != nil {
		return state.Route{}, err
	}
	route := state.Route{
		ID:        "gateway:" + user.Username,
		Username:  user.Username,
		Kind:      state.RouteKindGateway,
		LocalPort: user.Port,
		Hostname:  hostname,
		Enabled:   true,
	}
	if err := p.Store.UpsertRoute(ctx, route); err != nil {
		return state.Route{}, err
	}
	return route, nil
}

// DisableUserRoutes marks all user routes disabled in state.
func (p StateRoutePublisher) DisableUserRoutes(ctx context.Context, username string) error {
	if p.Store == nil {
		return errors.New("state route publisher requires store")
	}
	return p.Store.SetRoutesEnabled(ctx, username, false)
}

// DeleteUserRoutes removes all user routes from state.
func (p StateRoutePublisher) DeleteUserRoutes(ctx context.Context, username string) error {
	if p.Store == nil {
		return errors.New("state route publisher requires store")
	}
	return p.Store.DeleteRoutesByUser(ctx, username)
}

// GatewayHostname builds the deterministic public hostname for a user gateway.
func GatewayHostname(cfg *config.OverlayConfig, username string) (string, error) {
	if cfg == nil || strings.TrimSpace(cfg.Domain) == "" || strings.TrimSpace(cfg.Subdomain) == "" {
		return "", ErrMissingDomain
	}
	return fmt.Sprintf("gateway-%s.%s.%s", username, strings.TrimSpace(cfg.Subdomain), strings.TrimSpace(cfg.Domain)), nil
}

// GatewayURL returns the HTTPS URL for the user gateway.
func GatewayURL(cfg *config.OverlayConfig, username string) (string, error) {
	hostname, err := GatewayHostname(cfg, username)
	if err != nil {
		return "", err
	}
	return "https://" + hostname, nil
}
