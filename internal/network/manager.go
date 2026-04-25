package network

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/pdasilem/openclaw-multi/internal/audit"
	"github.com/pdasilem/openclaw-multi/internal/config"
	"github.com/pdasilem/openclaw-multi/internal/deps"
	"github.com/pdasilem/openclaw-multi/internal/shell"
	"github.com/pdasilem/openclaw-multi/internal/state"
)

// Auditor is the audit interface used by Manager.
type Auditor interface {
	Emit(e audit.Event) error
}

// Store is the state subset used by Manager.
type Store interface {
	ListRoutes(ctx context.Context) ([]state.Route, error)
	SetRouteLastSeen(ctx context.Context, id string, seenAt time.Time) error
}

// Manager provides network diagnostics and explicit network actions.
type Manager struct {
	Store  Store
	Exec   shell.Executor
	Config *config.OverlayConfig
	DNS    DNSClient
	Logger Auditor
	Actor  string
}

// NewManager returns a network manager.
func NewManager(store Store, exec shell.Executor, cfg *config.OverlayConfig, dns DNSClient, logger Auditor) *Manager {
	if dns == nil {
		dns = NewHTTPDNSClient(nil)
	}
	return &Manager{Store: store, Exec: exec, Config: cfg, DNS: dns, Logger: logger, Actor: "system"}
}

// Snapshot builds a read-only network report.
func (m *Manager) Snapshot(ctx context.Context) (Report, error) {
	start := time.Now()
	routes, err := m.routes(ctx)
	if err != nil {
		m.emit(audit.ActionNetworkRefresh, "network", audit.ResultError, err, start, nil)
		return Report{}, err
	}
	requiredPorts := requiredPorts(routes)
	report := Report{
		Tailscale:  m.inspectTailscale(ctx),
		Cloudflare: m.inspectCloudflare(ctx, routes),
		UFW:        m.inspectUFW(ctx, requiredPorts),
		Ports:      m.inspectPorts(ctx, requiredPorts),
	}
	portStatuses := make([]Status, 0, len(report.Ports))
	for _, port := range report.Ports {
		portStatuses = append(portStatuses, port.Status)
	}
	report.Summary = summarize(append([]Status{
		report.Tailscale.Status,
		report.Cloudflare.Status,
		report.UFW.Status,
	}, portStatuses...)...)
	m.emit(audit.ActionNetworkRefresh, "network", audit.ResultOk, nil, start, map[string]any{
		"routes": len(routes),
		"ports":  len(report.Ports),
	})
	return report, nil
}

// Probe checks enabled gateway hostnames through HTTPS.
func (m *Manager) Probe(ctx context.Context) ([]ProbeResult, error) {
	start := time.Now()
	routes, err := m.routes(ctx)
	if err != nil {
		m.emit(audit.ActionNetworkProbe, "gateway", audit.ResultError, err, start, nil)
		return nil, err
	}
	var probes []ProbeResult
	for _, route := range routes {
		if !route.Enabled || route.Hostname == "" {
			continue
		}
		url := "https://" + route.Hostname
		probeStart := time.Now()
		res, runErr := m.Exec.Run(ctx, shell.ExecOpts{
			Cmd:     []string{"curl", "-fsS", "--max-time", "5", url},
			Timeout: 8 * time.Second,
		})
		probe := ProbeResult{URL: url, Hostname: route.Hostname, DurationMs: time.Since(probeStart).Milliseconds()}
		if runErr != nil {
			probe.Status = StatusFail
			probe.Message = firstNonEmpty(strings.TrimSpace(res.Stderr), runErr.Error())
		} else {
			probe.Status = StatusOK
			probe.Message = "gateway responded"
		}
		probes = append(probes, probe)
	}
	m.emit(audit.ActionNetworkProbe, "gateway", audit.ResultOk, nil, start, map[string]any{"probes": len(probes)})
	return probes, nil
}

// PlanWildcardDNS returns a reviewable Cloudflare wildcard CNAME plan.
func (m *Manager) PlanWildcardDNS(ctx context.Context) (DNSPlan, error) {
	cfg := m.config()
	if cfg.TunnelMode == deps.TunnelModeQuick {
		return DNSPlan{}, errors.New("cloudflare DNS management requires account tunnel mode")
	}
	if cfg.Domain == "" || cfg.Subdomain == "" || cfg.TunnelID == "" {
		return DNSPlan{}, errors.New("domain, subdomain and tunnel_id are required")
	}
	if cfg.CloudflareZoneID == "" || cfg.CloudflareAPIToken == "" {
		return DNSPlan{}, errors.New("cloudflare_zone_id and cloudflare_api_token are required")
	}
	record := DNSRecord{
		Type:    "CNAME",
		Name:    "*." + cfg.Subdomain + "." + cfg.Domain,
		Content: cfg.TunnelID + ".cfargotunnel.com",
		TTL:     1,
		Proxied: true,
	}
	current, err := m.DNS.ListRecords(ctx, cfg.CloudflareZoneID, cfg.CloudflareAPIToken, record.Type, record.Name)
	if err != nil {
		return DNSPlan{}, err
	}
	switch {
	case len(current) == 0:
		return DNSPlan{Action: DNSActionCreate, Record: record, Message: "create missing wildcard CNAME"}, nil
	case current[0].Content == record.Content && current[0].Proxied == record.Proxied:
		return DNSPlan{Action: DNSActionNoop, Record: record, Current: &current[0], Message: "wildcard CNAME is already correct"}, nil
	default:
		record.ID = current[0].ID
		return DNSPlan{Action: DNSActionUpdate, Record: record, Current: &current[0], Message: "update wildcard CNAME target"}, nil
	}
}

// ApplyWildcardDNS applies a reviewed Cloudflare wildcard DNS plan.
func (m *Manager) ApplyWildcardDNS(ctx context.Context, plan DNSPlan) error {
	start := time.Now()
	cfg := m.config()
	var err error
	switch plan.Action {
	case DNSActionNoop:
		err = nil
	case DNSActionCreate:
		_, err = m.DNS.CreateRecord(ctx, cfg.CloudflareZoneID, cfg.CloudflareAPIToken, plan.Record)
	case DNSActionUpdate:
		_, err = m.DNS.UpdateRecord(ctx, cfg.CloudflareZoneID, cfg.CloudflareAPIToken, plan.Record.ID, plan.Record)
	default:
		err = fmt.Errorf("unsupported dns action %q", plan.Action)
	}
	result := audit.ResultOk
	if err != nil {
		result = audit.ResultError
	}
	m.emit(audit.ActionDNSUpdate, plan.Record.Name, result, err, start, map[string]any{"action": plan.Action})
	return err
}

// PlanUFW returns a reviewable UFW plan for current route ports.
func (m *Manager) PlanUFW(ctx context.Context) (UFWPlan, error) {
	routes, err := m.routes(ctx)
	if err != nil {
		return UFWPlan{}, err
	}
	required := requiredPorts(routes)
	info := m.inspectUFW(ctx, required)
	return UFWPlan{
		RequiredPorts: required,
		MissingPorts:  info.MissingPorts,
		Message:       "required route ports: " + formatPorts(required),
	}, nil
}

// ApplyUFW applies a reviewed UFW plan through the existing idempotent dependency helper.
func (m *Manager) ApplyUFW(ctx context.Context, plan UFWPlan) error {
	start := time.Now()
	_, err := deps.EnsureUFW(ctx, m.Exec, plan.RequiredPorts)
	result := audit.ResultOk
	if err != nil {
		result = audit.ResultError
	}
	m.emit(audit.ActionUFWUpdate, "ufw", result, err, start, map[string]any{"ports": plan.RequiredPorts})
	return err
}

// UpdateLastSeenFromLogs caches last_seen values from cloudflared logs.
func (m *Manager) UpdateLastSeenFromLogs(ctx context.Context, logs string) error {
	routes, err := m.routes(ctx)
	if err != nil {
		return err
	}
	known := map[string]string{}
	for _, route := range routes {
		if route.Hostname != "" {
			known[route.Hostname] = route.ID
		}
	}
	for id, seenAt := range parseLastSeenLogs(logs, known) {
		if err := m.Store.SetRouteLastSeen(ctx, id, seenAt); err != nil {
			return err
		}
	}
	return nil
}

func (m *Manager) inspectTailscale(ctx context.Context) TailscaleInfo {
	res, err := m.Exec.Run(ctx, shell.ExecOpts{Cmd: []string{"tailscale", "status", "--json"}, Timeout: 5 * time.Second})
	if err != nil {
		return TailscaleInfo{Status: StatusWarn, Message: "tailscale status failed", Installed: false}
	}
	return parseTailscaleStatusJSON(res.Stdout)
}

func (m *Manager) inspectCloudflare(ctx context.Context, routes []state.Route) CloudflareInfo {
	cfg := m.config()
	info := CloudflareInfo{
		TunnelID:   cfg.TunnelID,
		TunnelMode: cfg.TunnelMode,
		Domain:     cfg.Domain,
		Subdomain:  cfg.Subdomain,
		Wildcard:   "*." + cfg.Subdomain + "." + cfg.Domain,
		Routes:     routeInfos(routes),
	}
	res, err := m.Exec.Run(ctx, shell.ExecOpts{Cmd: []string{"cloudflared", "--version"}, Timeout: 5 * time.Second})
	if err != nil {
		info.Status = StatusWarn
		info.Message = "cloudflared is not installed or not executable"
		return info
	}
	info.Installed = true
	info.Version = parseCloudflaredVersion(res.Stdout)
	active, err := m.Exec.Run(ctx, shell.ExecOpts{Cmd: []string{"systemctl", "is-active", "cloudflared"}, Timeout: 5 * time.Second})
	info.Active = err == nil && strings.TrimSpace(active.Stdout) == "active"
	switch {
	case cfg.TunnelMode == deps.TunnelModeQuick:
		info.Status = StatusSkipped
		info.Message = "quick tunnel mode does not support managed wildcard DNS"
	case cfg.Domain == "" || cfg.Subdomain == "":
		info.Status = StatusWarn
		info.Message = "domain and subdomain are required for public gateway URLs"
	case !info.Active:
		info.Status = StatusWarn
		info.Message = "cloudflared service is not active"
	default:
		info.Status = StatusOK
		info.Message = "cloudflared is installed and active"
	}
	return info
}

func (m *Manager) inspectUFW(ctx context.Context, required []int) UFWInfo {
	res, err := m.Exec.Run(ctx, shell.ExecOpts{Cmd: []string{"ufw", "status", "verbose"}, Sudo: true, Timeout: 5 * time.Second})
	if err != nil {
		return UFWInfo{Status: StatusWarn, Message: "ufw status failed", MissingPorts: required}
	}
	return parseUFWStatus(res.Stdout, required)
}

func (m *Manager) inspectPorts(ctx context.Context, required []int) []PortInfo {
	res, err := m.Exec.Run(ctx, shell.ExecOpts{Cmd: []string{"ss", "-ltnp"}, Timeout: 5 * time.Second})
	if err != nil {
		return []PortInfo{{Status: StatusWarn, Message: "ss port scan failed"}}
	}
	return parseSS(res.Stdout, requiredPortMap(required))
}

func (m *Manager) routes(ctx context.Context) ([]state.Route, error) {
	if m.Store == nil {
		return nil, errors.New("network store is not configured")
	}
	return m.Store.ListRoutes(ctx)
}

func (m *Manager) config() *config.OverlayConfig {
	if m.Config == nil {
		return config.Defaults()
	}
	return m.Config
}

func (m *Manager) emit(
	action audit.ActionType,
	target string,
	result audit.Result,
	err error,
	start time.Time,
	details map[string]any,
) {
	if m.Logger == nil {
		return
	}
	event := audit.Event{
		Actor:      firstNonEmpty(m.Actor, "system"),
		Action:     action,
		Target:     target,
		Result:     result,
		Details:    details,
		DurationMs: time.Since(start).Milliseconds(),
	}
	if err != nil {
		event.ErrorMessage = err.Error()
	}
	_ = m.Logger.Emit(event)
}

func requiredPorts(routes []state.Route) []int {
	seen := map[int]bool{}
	for _, route := range routes {
		if route.Enabled && route.LocalPort > 0 {
			seen[route.LocalPort] = true
		}
	}
	out := make([]int, 0, len(seen))
	for port := range seen {
		out = append(out, port)
	}
	sort.Ints(out)
	return out
}

func routeInfos(routes []state.Route) []RouteInfo {
	out := make([]RouteInfo, 0, len(routes))
	for _, route := range routes {
		out = append(out, RouteInfo{
			ID:        route.ID,
			Username:  route.Username,
			Kind:      string(route.Kind),
			Hostname:  route.Hostname,
			LocalPort: route.LocalPort,
			Enabled:   route.Enabled,
			LastSeen:  route.LastSeenValue,
		})
	}
	return out
}
