package network

import (
	"encoding/json"
	"fmt"
	"net"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

func parseTailscaleStatusJSON(stdout string) TailscaleInfo {
	var raw struct {
		BackendState string   `json:"BackendState"`
		TailscaleIPs []string `json:"TailscaleIPs"`
		Self         struct {
			HostName string `json:"HostName"`
			DNSName  string `json:"DNSName"`
			UserID   int    `json:"UserID"`
			Online   bool   `json:"Online"`
		} `json:"Self"`
		User map[string]struct {
			LoginName string `json:"LoginName"`
		} `json:"User"`
	}
	if err := json.Unmarshal([]byte(stdout), &raw); err != nil {
		return TailscaleInfo{Status: StatusWarn, Installed: true, Message: "tailscale returned invalid JSON"}
	}

	info := TailscaleInfo{
		Installed:    true,
		BackendState: raw.BackendState,
		Running:      raw.BackendState == "Running",
		LoggedIn:     raw.BackendState == "Running" && len(raw.TailscaleIPs) > 0,
		Hostname:     firstNonEmpty(raw.Self.HostName, strings.TrimSuffix(raw.Self.DNSName, ".")),
	}
	if len(raw.TailscaleIPs) > 0 {
		info.IP = raw.TailscaleIPs[0]
	}
	if raw.Self.UserID != 0 {
		if u, ok := raw.User[strconv.Itoa(raw.Self.UserID)]; ok {
			info.User = u.LoginName
		}
	}
	switch {
	case !info.Running:
		info.Status = StatusWarn
		info.Message = "tailscale backend is not running"
	case !info.LoggedIn:
		info.Status = StatusWarn
		info.Message = "tailscale is running but not logged in"
	default:
		info.Status = StatusOK
		info.Message = "tailscale is running and logged in"
	}
	return info
}

func parseUFWStatus(stdout string, required []int) UFWInfo {
	info := UFWInfo{AllowedPorts: []int{}, MissingPorts: []int{}}
	lower := strings.ToLower(stdout)
	info.Active = strings.Contains(lower, "status: active")
	info.DenyIncoming = strings.Contains(lower, "default: deny (incoming)")
	info.AllowOutgoing = strings.Contains(lower, "allow (outgoing)")

	portRe := regexp.MustCompile(`(?m)^\s*(\d+)(?:/tcp)?\s+ALLOW`)
	seen := map[int]bool{}
	for _, match := range portRe.FindAllStringSubmatch(stdout, -1) {
		port, err := strconv.Atoi(match[1])
		if err == nil && !seen[port] {
			info.AllowedPorts = append(info.AllowedPorts, port)
			seen[port] = true
		}
	}
	sort.Ints(info.AllowedPorts)
	for _, port := range required {
		if !seen[port] {
			info.MissingPorts = append(info.MissingPorts, port)
		}
	}

	switch {
	case !info.Active:
		info.Status = StatusWarn
		info.Message = "ufw is inactive"
	case !info.DenyIncoming:
		info.Status = StatusWarn
		info.Message = "ufw default incoming policy is not deny"
	case len(info.MissingPorts) > 0:
		info.Status = StatusWarn
		info.Message = "ufw is missing required allow rules"
	default:
		info.Status = StatusOK
		info.Message = "ufw policy looks correct"
	}
	return info
}

func parseSS(stdout string, expected map[int]bool) []PortInfo {
	var ports []PortInfo
	for _, line := range strings.Split(stdout, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 5 || fields[0] == "State" {
			continue
		}
		addr, port, ok := splitAddrPort(fields[3])
		if !ok {
			continue
		}
		process := ""
		if len(fields) > 5 {
			process = strings.Join(fields[5:], " ")
		}
		public := addr == "0.0.0.0" || addr == "::" || addr == "*" || addr == "[::]"
		info := PortInfo{
			Proto:    fields[0],
			Address:  addr,
			Port:     port,
			Process:  process,
			Expected: expected[port],
			Public:   public,
		}
		switch {
		case info.Expected && public:
			info.Status = StatusWarn
			info.Message = "expected user port listens on a public address"
		case info.Expected:
			info.Status = StatusOK
			info.Message = "expected user port is listening"
		case public:
			info.Status = StatusWarn
			info.Message = "unexpected public listener"
		default:
			info.Status = StatusOK
			info.Message = "local listener"
		}
		ports = append(ports, info)
	}
	sort.Slice(ports, func(i, j int) bool {
		if ports[i].Port == ports[j].Port {
			return ports[i].Address < ports[j].Address
		}
		return ports[i].Port < ports[j].Port
	})
	return ports
}

func splitAddrPort(value string) (string, int, bool) {
	host, portText, err := net.SplitHostPort(value)
	if err != nil {
		idx := strings.LastIndex(value, ":")
		if idx < 0 {
			return "", 0, false
		}
		host, portText = value[:idx], value[idx+1:]
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		return "", 0, false
	}
	host = strings.Trim(host, "[]")
	if host == "" {
		host = "*"
	}
	return host, port, true
}

func parseCloudflaredVersion(stdout string) string {
	fields := strings.Fields(strings.TrimSpace(stdout))
	for i, field := range fields {
		if field == "version" && i+1 < len(fields) {
			return fields[i+1]
		}
	}
	if len(fields) >= 2 {
		return fields[1]
	}
	return strings.TrimSpace(stdout)
}

func parseLastSeenLogs(logs string, knownHosts map[string]string) map[string]time.Time {
	seen := map[string]time.Time{}
	for _, line := range strings.Split(logs, "\n") {
		t, ok := parseLogTime(line)
		if !ok {
			continue
		}
		for host, routeID := range knownHosts {
			if strings.Contains(line, host) {
				seen[routeID] = t
			}
		}
	}
	return seen
}

func parseLogTime(line string) (time.Time, bool) {
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return time.Time{}, false
	}
	candidates := []string{fields[0]}
	if len(fields) > 1 {
		candidates = append(candidates, fields[0]+"T"+fields[1])
	}
	layouts := []string{time.RFC3339, "2006-01-02T15:04:05", "2006-01-02T15:04:05.000Z07:00"}
	for _, candidate := range candidates {
		candidate = strings.Trim(candidate, "[]")
		for _, layout := range layouts {
			if t, err := time.Parse(layout, candidate); err == nil {
				return t.UTC(), true
			}
		}
	}
	return time.Time{}, false
}

func summarize(statuses ...Status) Summary {
	var s Summary
	for _, status := range statuses {
		switch status {
		case StatusOK:
			s.OK++
		case StatusWarn:
			s.Warn++
		case StatusFail:
			s.Fail++
		case StatusSkipped:
			s.Skipped++
		}
	}
	return s
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func requiredPortMap(ports []int) map[int]bool {
	out := make(map[int]bool, len(ports))
	for _, port := range ports {
		out[port] = true
	}
	return out
}

func formatPorts(ports []int) string {
	if len(ports) == 0 {
		return "none"
	}
	parts := make([]string, 0, len(ports))
	for _, port := range ports {
		parts = append(parts, fmt.Sprint(port))
	}
	return strings.Join(parts, ", ")
}
