package network

import (
	"testing"
	"time"
)

func TestParseTailscaleStatusJSONRunning(t *testing.T) {
	info := parseTailscaleStatusJSON(`{
		"BackendState": "Running",
		"TailscaleIPs": ["100.64.0.1"],
		"Self": {"HostName": "vps", "UserID": 1},
		"User": {"1": {"LoginName": "admin@example.com"}}
	}`)
	if info.Status != StatusOK || !info.LoggedIn || info.IP != "100.64.0.1" {
		t.Fatalf("unexpected tailscale info: %+v", info)
	}
	if info.User != "admin@example.com" {
		t.Fatalf("unexpected user: %q", info.User)
	}
}

func TestParseTailscaleStatusJSONInvalid(t *testing.T) {
	info := parseTailscaleStatusJSON("{")
	if info.Status != StatusWarn || !info.Installed {
		t.Fatalf("expected installed warning, got %+v", info)
	}
}

func TestParseUFWStatusFindsMissingPorts(t *testing.T) {
	info := parseUFWStatus(`Status: active
Default: deny (incoming), allow (outgoing), disabled (routed)

To                         Action      From
--                         ------      ----
22/tcp                     ALLOW       Anywhere
18789/tcp                  ALLOW       Anywhere
`, []int{18789, 18809})
	if info.Status != StatusWarn {
		t.Fatalf("expected warning, got %+v", info)
	}
	if len(info.MissingPorts) != 1 || info.MissingPorts[0] != 18809 {
		t.Fatalf("unexpected missing ports: %+v", info.MissingPorts)
	}
}

func TestParseSSMarksExpectedPublicPortWarning(t *testing.T) {
	ports := parseSS(`State  Recv-Q Send-Q Local Address:Port Peer Address:PortProcess
LISTEN 0      4096       127.0.0.1:18789      0.0.0.0:* users:(("node",pid=1,fd=2))
LISTEN 0      4096       0.0.0.0:18809        0.0.0.0:* users:(("node",pid=2,fd=2))
`, map[int]bool{18789: true, 18809: true})
	if len(ports) != 2 {
		t.Fatalf("expected two ports, got %+v", ports)
	}
	if ports[0].Status != StatusOK {
		t.Fatalf("expected local port ok, got %+v", ports[0])
	}
	if ports[1].Status != StatusWarn {
		t.Fatalf("expected public expected port warning, got %+v", ports[1])
	}
}

func TestParseLastSeenLogs(t *testing.T) {
	seen := parseLastSeenLogs(
		"2026-04-25T10:00:00Z request host alice.example.com\n",
		map[string]string{"alice.example.com": "route-alice"},
	)
	got := seen["route-alice"]
	want := time.Date(2026, 4, 25, 10, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("got %s want %s", got, want)
	}
}

func TestParseCloudflaredVersion(t *testing.T) {
	if got := parseCloudflaredVersion("cloudflared version 2026.4.0"); got != "2026.4.0" {
		t.Fatalf("unexpected version: %q", got)
	}
	if got := parseCloudflaredVersion("cloudflared 2026.4.1"); got != "2026.4.1" {
		t.Fatalf("unexpected short version: %q", got)
	}
}
