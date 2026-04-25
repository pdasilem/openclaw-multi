package users

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/pdasilem/openclaw-multi/internal/config"
	"github.com/pdasilem/openclaw-multi/internal/state"
)

func TestGatewayHostnameRequiresDomainAndSubdomain(t *testing.T) {
	cfg := config.Defaults()
	cfg.Domain = ""
	_, err := GatewayHostname(cfg, "alice")
	if !errors.Is(err, ErrMissingDomain) {
		t.Fatalf("expected ErrMissingDomain, got %v", err)
	}
}

func TestStateRoutePublisherRecordsGatewayRoute(t *testing.T) {
	ctx := context.Background()
	store := openUserTestStore(t)
	cfg := config.Defaults()
	cfg.Domain = "example.com"
	cfg.Subdomain = "ui"

	user := state.User{Username: "alice", Port: 18789, Status: state.UserStatusActive}
	if err := store.UpsertUser(ctx, user); err != nil {
		t.Fatalf("UpsertUser: %v", err)
	}

	route, err := (StateRoutePublisher{Store: store, Config: cfg}).EnableUserGateway(ctx, user)
	if err != nil {
		t.Fatalf("EnableUserGateway: %v", err)
	}
	if route.Hostname != "gateway-alice.ui.example.com" {
		t.Fatalf("hostname: got %q", route.Hostname)
	}
	routes, err := store.ListRoutesByUser(ctx, "alice")
	if err != nil {
		t.Fatalf("ListRoutesByUser: %v", err)
	}
	if len(routes) != 1 || !routes[0].Enabled {
		t.Fatalf("unexpected routes: %+v", routes)
	}
}

func TestStateRoutePublisherDisableAndDelete(t *testing.T) {
	ctx := context.Background()
	store := openUserTestStore(t)
	cfg := config.Defaults()
	cfg.Domain = "example.com"
	cfg.Subdomain = "ui"
	user := state.User{Username: "alice", Port: 18789, Status: state.UserStatusActive}
	if err := store.UpsertUser(ctx, user); err != nil {
		t.Fatalf("UpsertUser: %v", err)
	}
	pub := StateRoutePublisher{Store: store, Config: cfg}
	if _, err := pub.EnableUserGateway(ctx, user); err != nil {
		t.Fatalf("EnableUserGateway: %v", err)
	}
	if err := pub.DisableUserRoutes(ctx, "alice"); err != nil {
		t.Fatalf("DisableUserRoutes: %v", err)
	}
	routes, err := store.ListRoutesByUser(ctx, "alice")
	if err != nil {
		t.Fatalf("ListRoutesByUser: %v", err)
	}
	if routes[0].Enabled {
		t.Fatal("expected disabled route")
	}
	if err := pub.DeleteUserRoutes(ctx, "alice"); err != nil {
		t.Fatalf("DeleteUserRoutes: %v", err)
	}
	routes, err = store.ListRoutesByUser(ctx, "alice")
	if err != nil {
		t.Fatalf("ListRoutesByUser after delete: %v", err)
	}
	if len(routes) != 0 {
		t.Fatalf("expected no routes, got %+v", routes)
	}
}

func TestStateRoutePublisherRequiresStore(t *testing.T) {
	pub := StateRoutePublisher{Config: config.Defaults()}
	if _, err := pub.EnableUserGateway(context.Background(), state.User{Username: "alice"}); err == nil {
		t.Fatal("expected EnableUserGateway store error")
	}
	if err := pub.DisableUserRoutes(context.Background(), "alice"); err == nil {
		t.Fatal("expected DisableUserRoutes store error")
	}
	if err := pub.DeleteUserRoutes(context.Background(), "alice"); err == nil {
		t.Fatal("expected DeleteUserRoutes store error")
	}
}

func openUserTestStore(t *testing.T) *state.Store {
	t.Helper()
	store, err := state.Open(context.Background(), filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatalf("state.Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}
