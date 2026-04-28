package api

import "time"

type ErrorResponse struct {
	Error string `json:"error"`
}

type HealthResponse struct {
	OK bool `json:"ok"`
}

type RouteRequest struct {
	PluginID     string `json:"plugin_id,omitempty"`
	HostnameHint string `json:"hostname_hint,omitempty"`
	LocalPort    int    `json:"local_port"`
}

type RouteResponse struct {
	RouteID   string     `json:"route_id"`
	Username  string     `json:"username"`
	Kind      string     `json:"kind"`
	PluginID  string     `json:"plugin_id,omitempty"`
	Hostname  string     `json:"hostname"`
	URL       string     `json:"url"`
	LocalPort int        `json:"local_port"`
	Enabled   bool       `json:"enabled"`
	LastSeen  *time.Time `json:"last_seen,omitempty"`
}

type LastSeenResponse struct {
	RouteID  string     `json:"route_id"`
	LastSeen *time.Time `json:"last_seen"`
}
