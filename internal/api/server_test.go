package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/pdasilem/openclaw-multi/internal/config"
	"github.com/pdasilem/openclaw-multi/internal/state"
)

type fakePublisher struct{ calls int }

func (f *fakePublisher) Publish(context.Context, []state.Route) error {
	f.calls++
	return nil
}

func testStore(t *testing.T) *state.Store {
	t.Helper()
	store, err := state.Open(context.Background(), filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func TestServerGatewayRoute(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	if err := store.UpsertUser(ctx, state.User{Username: "alice", UID: 1001, Port: 18001}); err != nil {
		t.Fatal(err)
	}
	pub := &fakePublisher{}
	srv := Server{
		Store:  store,
		Routes: RouteService{Store: store, Config: &config.OverlayConfig{Domain: "example.com", Subdomain: "ui"}, Publisher: pub},
		Peers:  StaticPeerProvider{Cred: PeerCred{UID: 1001}},
	}
	req := httptest.NewRequest(http.MethodPost, "/users/alice/gateway-route", bytes.NewBufferString(`{}`))
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	var out RouteResponse
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if out.RouteID != "gateway:alice" || out.Hostname != "gateway-alice.ui.example.com" {
		t.Fatalf("unexpected response: %+v", out)
	}
	if pub.calls != 1 {
		t.Fatalf("publisher calls=%d", pub.calls)
	}
}

func TestServerRejectsWrongUID(t *testing.T) {
	store := testStore(t)
	if err := store.UpsertUser(context.Background(), state.User{Username: "alice", UID: 1001, Port: 18001}); err != nil {
		t.Fatal(err)
	}
	srv := Server{Store: store, Routes: RouteService{Store: store, Config: &config.OverlayConfig{Domain: "example.com", Subdomain: "ui"}}, Peers: StaticPeerProvider{Cred: PeerCred{UID: 1002}}}
	req := httptest.NewRequest(http.MethodPost, "/users/alice/gateway-route", bytes.NewBufferString(`{}`))
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
}

func TestServerPluginRouteIsDaemonDerived(t *testing.T) {
	store := testStore(t)
	if err := store.UpsertUser(context.Background(), state.User{Username: "alice", UID: 1001, Port: 18001}); err != nil {
		t.Fatal(err)
	}
	srv := Server{Store: store, Routes: RouteService{Store: store, Config: &config.OverlayConfig{Domain: "example.com", Subdomain: "ui"}}, Peers: StaticPeerProvider{Cred: PeerCred{UID: 0}}}
	body := `{"plugin_id":"Calendar","hostname_hint":"OAuth Callback","local_port":19000}`
	for range 2 {
		req := httptest.NewRequest(http.MethodPost, "/users/alice/routes", bytes.NewBufferString(body))
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
		}
	}
	routes, err := store.ListRoutesByUser(context.Background(), "alice")
	if err != nil {
		t.Fatal(err)
	}
	if len(routes) != 1 || routes[0].ID != "plugin:alice:calendar:oauth-callback" {
		t.Fatalf("unexpected routes: %+v", routes)
	}
}
