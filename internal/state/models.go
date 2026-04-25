// Package state provides SQLite-backed persistent storage for the overlay.
package state

import "time"

// Admin represents the configured overlay administrator.
type Admin struct {
	Username string
	UID      int
	SetAt    time.Time
	SetBy    string
}

// UserStatus is the lifecycle state of a managed OpenClaw Linux user.
type UserStatus string

const (
	// UserStatusActive means the user's services and routes should be running.
	UserStatusActive UserStatus = "active"
	// UserStatusPaused means the user exists but services/routes are disabled.
	UserStatusPaused UserStatus = "paused"
)

// RouteKind identifies what a tunnel route points to.
type RouteKind string

const (
	// RouteKindGateway is the per-user Control UI gateway route.
	RouteKindGateway RouteKind = "gateway"
	// RouteKindPlugin is reserved for per-plugin callback routes.
	RouteKindPlugin RouteKind = "plugin"
)

// User represents an OpenClaw user managed by the overlay.
type User struct {
	Username   string
	UID        int
	Port       int
	Status     UserStatus
	Linger     bool
	GatewayURL string
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// Route represents a Cloudflare Tunnel ingress route recorded in state.
type Route struct {
	ID               string
	Username         string
	Kind             RouteKind
	PluginID         string
	LocalPort        int
	Hostname         string
	Enabled          bool
	CreatedAt        time.Time
	LastSeenCachedAt time.Time
	LastSeenValue    time.Time
}
