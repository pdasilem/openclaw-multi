// Package network provides read-only network diagnostics and explicit network actions.
package network

import "time"

// Status is the normalized health state for a network component.
type Status string

const (
	StatusOK      Status = "ok"
	StatusWarn    Status = "warn"
	StatusFail    Status = "fail"
	StatusSkipped Status = "skipped"
)

// Report is a complete Phase 5 network snapshot.
type Report struct {
	Tailscale  TailscaleInfo
	Cloudflare CloudflareInfo
	UFW        UFWInfo
	Ports      []PortInfo
	Probes     []ProbeResult
	Summary    Summary
}

// Summary counts normalized component states.
type Summary struct {
	OK      int
	Warn    int
	Fail    int
	Skipped int
}

// TailscaleInfo describes tailscaled state from `tailscale status --json`.
type TailscaleInfo struct {
	Status       Status
	Message      string
	Installed    bool
	Running      bool
	LoggedIn     bool
	BackendState string
	IP           string
	Hostname     string
	User         string
}

// CloudflareInfo describes cloudflared and known route state.
type CloudflareInfo struct {
	Status     Status
	Message    string
	Installed  bool
	Active     bool
	Version    string
	TunnelID   string
	TunnelMode string
	Domain     string
	Subdomain  string
	Wildcard   string
	Routes     []RouteInfo
}

// RouteInfo is the network-facing projection of a persisted route.
type RouteInfo struct {
	ID        string
	Username  string
	Kind      string
	Hostname  string
	LocalPort int
	Enabled   bool
	LastSeen  time.Time
}

// UFWInfo describes UFW status without mutating firewall rules.
type UFWInfo struct {
	Status        Status
	Message       string
	Active        bool
	DenyIncoming  bool
	AllowOutgoing bool
	AllowedPorts  []int
	MissingPorts  []int
}

// PortInfo describes a listening TCP port discovered from ss.
type PortInfo struct {
	Status   Status
	Message  string
	Proto    string
	Address  string
	Port     int
	Process  string
	Expected bool
	Public   bool
}

// ProbeResult describes an explicit external gateway probe.
type ProbeResult struct {
	URL        string
	Hostname   string
	Status     Status
	Message    string
	DurationMs int64
}

// DNSRecord is the subset of Cloudflare DNS data Phase 5 needs.
type DNSRecord struct {
	ID      string
	Type    string
	Name    string
	Content string
	TTL     int
	Proxied bool
}

// DNSAction is an explicit DNS operation to review before applying.
type DNSAction string

const (
	DNSActionNoop   DNSAction = "noop"
	DNSActionCreate DNSAction = "create"
	DNSActionUpdate DNSAction = "update"
)

// DNSPlan is a reviewable Cloudflare DNS wildcard plan.
type DNSPlan struct {
	Action  DNSAction
	Record  DNSRecord
	Current *DNSRecord
	Message string
}

// UFWPlan is a reviewable firewall plan.
type UFWPlan struct {
	RequiredPorts []int
	MissingPorts  []int
	Message       string
}
