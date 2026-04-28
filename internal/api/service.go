package api

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/pdasilem/openclaw-multi/internal/config"
	"github.com/pdasilem/openclaw-multi/internal/state"
)

var safePart = regexp.MustCompile(`[^a-z0-9-]+`)

type Publisher interface {
	Publish(context.Context, []state.Route) error
}

type RouteService struct {
	Store     *state.Store
	Config    *config.OverlayConfig
	Publisher Publisher
}

func (s RouteService) Publish(ctx context.Context) error {
	if s.Publisher == nil {
		return nil
	}
	routes, err := s.Store.ListRoutes(ctx)
	if err != nil {
		return err
	}
	return s.Publisher.Publish(ctx, routes)
}

func (s RouteService) UpsertGateway(ctx context.Context, username string, port int) (state.Route, error) {
	user, err := s.Store.GetUser(ctx, username)
	if err != nil {
		return state.Route{}, err
	}
	if port <= 0 {
		port = user.Port
	}
	hostname, err := gatewayHostname(s.Config, username)
	if err != nil {
		return state.Route{}, err
	}
	route := state.Route{ID: "gateway:" + username, Username: username, Kind: state.RouteKindGateway, LocalPort: port, Hostname: hostname, Enabled: true}
	if err := s.Store.UpsertRoute(ctx, route); err != nil {
		return state.Route{}, err
	}
	if err := s.Publish(ctx); err != nil {
		return route, err
	}
	return route, nil
}

func (s RouteService) UpsertPlugin(ctx context.Context, username, pluginID, hint string, port int) (state.Route, error) {
	if _, err := s.Store.GetUser(ctx, username); err != nil {
		return state.Route{}, err
	}
	pluginID = slug(pluginID)
	hint = slug(hint)
	if pluginID == "" || hint == "" || port <= 0 {
		return state.Route{}, errBadRequest
	}
	host, err := routeHostname(s.Config, hint, username)
	if err != nil {
		return state.Route{}, err
	}
	route := state.Route{ID: "plugin:" + username + ":" + pluginID + ":" + hint, Username: username, Kind: state.RouteKindPlugin, PluginID: pluginID, LocalPort: port, Hostname: host, Enabled: true}
	if err := s.Store.UpsertRoute(ctx, route); err != nil {
		return state.Route{}, err
	}
	if err := s.Publish(ctx); err != nil {
		return route, err
	}
	return route, nil
}

func (s RouteService) DeleteGateway(ctx context.Context, username string) error {
	if err := s.Store.DeleteRoute(ctx, "gateway:"+username); err != nil && !errors.Is(err, state.ErrNoRoute) {
		return err
	}
	return s.Publish(ctx)
}

func (s RouteService) DeletePlugin(ctx context.Context, username, id string) error {
	if !strings.HasPrefix(id, "plugin:"+username+":") {
		return errForbidden
	}
	if err := s.Store.DeleteRoute(ctx, id); err != nil {
		return err
	}
	return s.Publish(ctx)
}

func (s RouteService) SetEnabled(ctx context.Context, username string, enabled bool) error {
	if _, err := s.Store.GetUser(ctx, username); err != nil {
		return err
	}
	if err := s.Store.SetRoutesEnabled(ctx, username, enabled); err != nil {
		return err
	}
	return s.Publish(ctx)
}

func (s RouteService) Routes(ctx context.Context, username string) ([]state.Route, error) {
	routes, err := s.Store.ListRoutesByUser(ctx, username)
	if err != nil {
		return nil, err
	}
	sort.Slice(routes, func(i, j int) bool {
		if routes[i].Kind != routes[j].Kind {
			return routes[i].Kind < routes[j].Kind
		}
		if routes[i].PluginID != routes[j].PluginID {
			return routes[i].PluginID < routes[j].PluginID
		}
		return routes[i].Hostname < routes[j].Hostname
	})
	return routes, nil
}

func gatewayHostname(cfg *config.OverlayConfig, username string) (string, error) {
	return routeHostname(cfg, "gateway-"+username, "")
}

func routeHostname(cfg *config.OverlayConfig, prefix, username string) (string, error) {
	if cfg == nil || strings.TrimSpace(cfg.Domain) == "" || strings.TrimSpace(cfg.Subdomain) == "" {
		return "", errors.New("missing domain or subdomain")
	}
	left := prefix
	if username != "" {
		left += "." + slug(username)
	}
	return fmt.Sprintf("%s.%s.%s", left, strings.TrimSpace(cfg.Subdomain), strings.TrimSpace(cfg.Domain)), nil
}

func slug(v string) string {
	v = strings.ToLower(strings.TrimSpace(v))
	v = safePart.ReplaceAllString(v, "-")
	v = strings.Trim(v, "-")
	return v
}
