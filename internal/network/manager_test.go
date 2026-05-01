package network

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/pdasilem/openclaw-multi/internal/config"
	"github.com/pdasilem/openclaw-multi/internal/shell"
	"github.com/pdasilem/openclaw-multi/internal/state"
)

type fakeDNS struct {
	records []DNSRecord
	created []DNSRecord
	updated []DNSRecord
	err     error
}

func (f *fakeDNS) ListRecords(context.Context, string, string, string, string) ([]DNSRecord, error) {
	return f.records, f.err
}

func (f *fakeDNS) CreateRecord(_ context.Context, _ string, _ string, record DNSRecord) (DNSRecord, error) {
	f.created = append(f.created, record)
	return record, f.err
}

func (f *fakeDNS) UpdateRecord(_ context.Context, _ string, _ string, _ string, record DNSRecord) (DNSRecord, error) {
	f.updated = append(f.updated, record)
	return record, f.err
}

func TestSnapshotCollectsNetworkState(t *testing.T) {
	ctx := context.Background()
	store := testStore(t)
	upsertRoute(t, store, state.Route{
		ID:        "gateway-alice",
		Username:  "alice",
		Kind:      state.RouteKindGateway,
		LocalPort: 18789,
		Hostname:  "alice.openclaw.example.com",
		Enabled:   true,
	})
	exec := &shell.MockExecutor{Responses: []shell.ExecResult{
		shell.OKResponse(`{"BackendState":"Running","TailscaleIPs":["100.64.0.1"],"Self":{"HostName":"vps"}}`),
		shell.OKResponse("cloudflared version 2026.4.0"),
		shell.OKResponse("active\n"),
		shell.OKResponse("Status: active\nDefault: deny (incoming), allow (outgoing), disabled (routed)\n18789/tcp ALLOW Anywhere\n"),
		shell.OKResponse("State Recv-Q Send-Q Local Address:Port Peer Address:Port Process\nLISTEN 0 4096 127.0.0.1:18789 0.0.0.0:* users:((\"node\",pid=1,fd=2))\n"),
	}}
	manager := NewManager(store, exec, &config.OverlayConfig{
		Domain: "example.com", Subdomain: "openclaw", TunnelID: "abc", TunnelMode: "account",
	}, &fakeDNS{}, nil)

	report, err := manager.Snapshot(ctx)
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	if report.Tailscale.Status != StatusOK || report.Cloudflare.Status != StatusOK || report.UFW.Status != StatusOK {
		t.Fatalf("unexpected report: %+v", report)
	}
	if len(report.Cloudflare.Routes) != 1 || len(report.Ports) != 1 {
		t.Fatalf("expected route and port data: %+v", report)
	}
}

func TestProbeDoesNotFailBatchOnSingleGatewayFailure(t *testing.T) {
	ctx := context.Background()
	store := testStore(t)
	upsertRoute(t, store, state.Route{ID: "r1", Username: "alice", Kind: state.RouteKindGateway, LocalPort: 18789, Hostname: "alice.example.com", Enabled: true})
	res, err := shell.ErrResponse("connection refused", 7)
	exec := &shell.MockExecutor{Responses: []shell.ExecResult{res}, Errors: []error{err}}
	manager := NewManager(store, exec, config.Defaults(), &fakeDNS{}, nil)

	probes, err := manager.Probe(ctx)
	if err != nil {
		t.Fatalf("Probe: %v", err)
	}
	if len(probes) != 1 || probes[0].Status != StatusFail {
		t.Fatalf("unexpected probes: %+v", probes)
	}
	if probes[0].Message != "connection refused" {
		t.Fatalf("expected curl stderr in probe message, got %q", probes[0].Message)
	}
}

func TestPlanAndApplyWildcardDNSCreate(t *testing.T) {
	dns := &fakeDNS{}
	manager := NewManager(nil, &shell.MockExecutor{}, &config.OverlayConfig{
		Domain: "example.com", Subdomain: "openclaw", TunnelID: "abc", TunnelMode: "account",
		CloudflareZoneID: "zone", CloudflareAPIToken: "token",
	}, dns, nil)

	plan, err := manager.PlanWildcardDNS(context.Background())
	if err != nil {
		t.Fatalf("PlanWildcardDNS: %v", err)
	}
	if plan.Action != DNSActionCreate || plan.Record.Content != "abc.cfargotunnel.com" {
		t.Fatalf("unexpected plan: %+v", plan)
	}
	if err := manager.ApplyWildcardDNS(context.Background(), plan); err != nil {
		t.Fatalf("ApplyWildcardDNS: %v", err)
	}
	if len(dns.created) != 1 {
		t.Fatalf("expected one created record, got %+v", dns.created)
	}
}

func TestPlanWildcardDNSNoopAndUpdate(t *testing.T) {
	cfg := &config.OverlayConfig{
		Domain: "example.com", Subdomain: "openclaw", TunnelID: "abc", TunnelMode: "account",
		CloudflareZoneID: "zone", CloudflareAPIToken: "token",
	}
	manager := NewManager(nil, &shell.MockExecutor{}, cfg, &fakeDNS{records: []DNSRecord{{
		ID: "id1", Type: "CNAME", Name: "*.openclaw.example.com", Content: "abc.cfargotunnel.com", Proxied: true,
	}}}, nil)
	plan, err := manager.PlanWildcardDNS(context.Background())
	if err != nil {
		t.Fatalf("PlanWildcardDNS noop: %v", err)
	}
	if plan.Action != DNSActionNoop {
		t.Fatalf("expected noop, got %+v", plan)
	}

	dns := &fakeDNS{records: []DNSRecord{{ID: "id1", Content: "old.cfargotunnel.com"}}}
	manager.DNS = dns
	plan, err = manager.PlanWildcardDNS(context.Background())
	if err != nil {
		t.Fatalf("PlanWildcardDNS update: %v", err)
	}
	if plan.Action != DNSActionUpdate || plan.Record.ID != "id1" {
		t.Fatalf("expected update, got %+v", plan)
	}
	if err := manager.ApplyWildcardDNS(context.Background(), plan); err != nil {
		t.Fatalf("ApplyWildcardDNS update: %v", err)
	}
	if len(dns.updated) != 1 {
		t.Fatalf("expected update call, got %+v", dns.updated)
	}
}

func TestPlanAndApplyUFW(t *testing.T) {
	ctx := context.Background()
	store := testStore(t)
	upsertRoute(t, store, state.Route{ID: "r1", Username: "alice", Kind: state.RouteKindGateway, LocalPort: 18789, Hostname: "alice.example.com", Enabled: true})
	exec := &shell.MockExecutor{Responses: []shell.ExecResult{
		shell.OKResponse("Status: active\nDefault: deny (incoming), allow (outgoing), disabled (routed)\n"),
		shell.OKResponse("Status: active\nDefault: deny (incoming), allow (outgoing), disabled (routed)\n"),
		shell.OKResponse(""),
	}}
	manager := NewManager(store, exec, config.Defaults(), &fakeDNS{}, nil)
	plan, err := manager.PlanUFW(ctx)
	if err != nil {
		t.Fatalf("PlanUFW: %v", err)
	}
	if len(plan.MissingPorts) != 1 || plan.MissingPorts[0] != 18789 {
		t.Fatalf("unexpected UFW plan: %+v", plan)
	}
	if err := manager.ApplyUFW(ctx, plan); err != nil {
		t.Fatalf("ApplyUFW: %v", err)
	}
}

func TestPlanWildcardDNSRejectsQuickTunnelAndMissingCredentials(t *testing.T) {
	manager := NewManager(nil, &shell.MockExecutor{}, &config.OverlayConfig{TunnelMode: "quick"}, &fakeDNS{}, nil)
	if _, err := manager.PlanWildcardDNS(context.Background()); err == nil {
		t.Fatal("expected quick tunnel rejection")
	}

	manager.Config = &config.OverlayConfig{Domain: "example.com", Subdomain: "openclaw", TunnelID: "abc", TunnelMode: "account"}
	if _, err := manager.PlanWildcardDNS(context.Background()); err == nil {
		t.Fatal("expected credential rejection")
	}
}

func TestUpdateLastSeenFromLogs(t *testing.T) {
	ctx := context.Background()
	store := testStore(t)
	upsertRoute(t, store, state.Route{ID: "r1", Username: "alice", Kind: state.RouteKindGateway, LocalPort: 18789, Hostname: "alice.example.com", Enabled: true})
	manager := NewManager(store, &shell.MockExecutor{}, config.Defaults(), &fakeDNS{}, nil)
	if err := manager.UpdateLastSeenFromLogs(ctx, "2026-04-25T10:00:00Z host=alice.example.com\n"); err != nil {
		t.Fatalf("UpdateLastSeenFromLogs: %v", err)
	}
	routes, err := store.ListRoutes(ctx)
	if err != nil {
		t.Fatalf("ListRoutes: %v", err)
	}
	want := time.Date(2026, 4, 25, 10, 0, 0, 0, time.UTC)
	if !routes[0].LastSeenValue.Equal(want) {
		t.Fatalf("got %s want %s", routes[0].LastSeenValue, want)
	}
}

func TestSnapshotReturnsStoreError(t *testing.T) {
	manager := NewManager(failingStore{}, &shell.MockExecutor{}, config.Defaults(), &fakeDNS{}, nil)
	if _, err := manager.Snapshot(context.Background()); err == nil {
		t.Fatal("expected store error")
	}
}

func TestInspectFailuresProduceWarnings(t *testing.T) {
	ctx := context.Background()
	store := testStore(t)
	runErr := errors.New("missing command")
	exec := &shell.MockExecutor{
		Responses: []shell.ExecResult{{Stderr: "missing", ExitCode: 127}},
		Errors:    []error{runErr},
	}
	manager := NewManager(store, exec, config.Defaults(), &fakeDNS{}, nil)
	report, err := manager.Snapshot(ctx)
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	if report.Tailscale.Status != StatusWarn || report.Cloudflare.Status != StatusWarn ||
		report.UFW.Status != StatusWarn || report.Ports[0].Status != StatusWarn {
		t.Fatalf("expected warnings, got %+v", report)
	}
}

type failingStore struct{}

func (failingStore) ListRoutes(context.Context) ([]state.Route, error) {
	return nil, errors.New("store unavailable")
}

func (failingStore) SetRouteLastSeen(context.Context, string, time.Time) error {
	return nil
}

func testStore(t *testing.T) *state.Store {
	t.Helper()
	store, err := state.Open(context.Background(), t.TempDir()+"/state.db")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func upsertRoute(t *testing.T, store *state.Store, route state.Route) {
	t.Helper()
	if err := store.UpsertUser(context.Background(), state.User{Username: route.Username, Status: state.UserStatusActive}); err != nil {
		t.Fatalf("UpsertUser: %v", err)
	}
	if err := store.UpsertRoute(context.Background(), route); err != nil {
		t.Fatalf("UpsertRoute: %v", err)
	}
}
