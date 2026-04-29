package watcher

import (
	"errors"
	"testing"
)

func TestParseConfigDiscoversCallbacks(t *testing.T) {
	cfg, err := ParseConfig([]byte(`{
		"plugins": {
			"entries": {
				"calendar": {
					"config": {"callbackPort": 19000, "callbackHostnameHint": "oauth"},
					"ignored": true
				},
				"plain": {"config": {"enabled": true}}
			}
		}
	}`))
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}
	routes, err := DiscoverCallbacks(cfg)
	if err != nil {
		t.Fatalf("DiscoverCallbacks: %v", err)
	}
	if len(routes) != 1 {
		t.Fatalf("expected 1 route, got %d", len(routes))
	}
	route := routes[0]
	if route.PluginID != "calendar" || route.HostnameHint != "oauth" || route.LocalPort != 19000 {
		t.Fatalf("unexpected route: %+v", route)
	}
	if route.URLConfigKey != "plugins.entries.calendar.config.callbackUrl" {
		t.Fatalf("unexpected url key: %s", route.URLConfigKey)
	}
	if route.PortConfigKey != "plugins.entries.calendar.config.callbackPort" {
		t.Fatalf("unexpected port key: %s", route.PortConfigKey)
	}
}

func TestParseConfigRejectsMalformedJSON(t *testing.T) {
	_, err := ParseConfig([]byte(`{"plugins":`))
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestParseConfigEmptyIsValid(t *testing.T) {
	cfg, err := ParseConfig(nil)
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}
	if len(cfg.Plugins.Entries) != 0 {
		t.Fatalf("unexpected entries: %+v", cfg.Plugins.Entries)
	}
}

func TestDiscoverCallbacksReturnsNoCallbacks(t *testing.T) {
	cfg, err := ParseConfig([]byte(`{"plugins":{"entries":{"plain":{"config":{}}}}}`))
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}
	_, err = DiscoverCallbacks(cfg)
	if !errors.Is(err, ErrNoCallbackRoutes) {
		t.Fatalf("expected ErrNoCallbackRoutes, got %v", err)
	}
}

func TestDiscoverCallbacksRejectsInvalidPort(t *testing.T) {
	cfg, err := ParseConfig([]byte(`{"plugins":{"entries":{"bad":{"config":{"callbackPort":70000}}}}}`))
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}
	_, err = DiscoverCallbacks(cfg)
	if err == nil {
		t.Fatal("expected invalid port error")
	}
}

func TestDiscoverCallbacksUsesExplicitOverlayContract(t *testing.T) {
	cfg, err := ParseConfig([]byte(`{
		"plugins": {
			"entries": {
				"webhook": {
					"overlay": {
						"callback": {
							"local_port": 19100,
							"hostname_hint": "webhook",
							"url_config_key": "plugins.entries.webhook.config.url",
							"port_config_key": "plugins.entries.webhook.config.port"
						}
					}
				}
			}
		}
	}`))
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}
	routes, err := DiscoverCallbacks(cfg)
	if err != nil {
		t.Fatalf("DiscoverCallbacks: %v", err)
	}
	if routes[0].URLConfigKey != "plugins.entries.webhook.config.url" || routes[0].PortConfigKey != "plugins.entries.webhook.config.port" {
		t.Fatalf("unexpected config keys: %+v", routes[0])
	}
}

func TestDiscoverCallbacksParsesStringPort(t *testing.T) {
	cfg, err := ParseConfig([]byte(`{"plugins":{"entries":{"calendar":{"config":{"callbackPort":"19000"}}}}}`))
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}
	routes, err := DiscoverCallbacks(cfg)
	if err != nil {
		t.Fatalf("DiscoverCallbacks: %v", err)
	}
	if routes[0].LocalPort != 19000 {
		t.Fatalf("unexpected port: %d", routes[0].LocalPort)
	}
}
