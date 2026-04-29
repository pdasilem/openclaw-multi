package doctor

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

func parseDF(stdout string) (string, Status) {
	lines := nonEmptyLines(stdout)
	if len(lines) < 2 {
		return "disk output unavailable", StatusWarn
	}
	fields := strings.Fields(lines[1])
	if len(fields) < 5 {
		return "disk output malformed", StatusWarn
	}
	usedPct := strings.TrimSuffix(fields[4], "%")
	used, err := strconv.Atoi(usedPct)
	if err != nil {
		return "disk usage malformed", StatusWarn
	}
	msg := fmt.Sprintf("disk %s used, %s KB available", fields[4], fields[3])
	if used >= 90 {
		return msg, StatusWarn
	}
	return msg, StatusOK
}

func parseLoad(data string) (string, Status) {
	fields := strings.Fields(data)
	if len(fields) < 3 {
		return "load average unavailable", StatusWarn
	}
	return "load average " + strings.Join(fields[:3], ", "), StatusOK
}

func parseMemInfo(data string) (string, Status) {
	var total, available int
	for _, line := range strings.Split(data, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		n, err := strconv.Atoi(fields[1])
		if err != nil {
			continue
		}
		switch strings.TrimSuffix(fields[0], ":") {
		case "MemTotal":
			total = n
		case "MemAvailable":
			available = n
		}
	}
	if total == 0 {
		return "memory information unavailable", StatusWarn
	}
	msg := fmt.Sprintf("memory available %d MB / %d MB", available/1024, total/1024)
	if available*100/total < 10 {
		return msg, StatusWarn
	}
	return msg, StatusOK
}

func parseHidepid(mounts string) (string, Status) {
	for _, line := range strings.Split(mounts, "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 4 && fields[1] == "/proc" {
			if strings.Contains(fields[3], "hidepid=2") {
				return "/proc hidepid=2", StatusOK
			}
			return "/proc hidepid is not 2", StatusWarn
		}
	}
	return "/proc mount not found", StatusWarn
}

func parsePtraceScope(data string) (string, Status) {
	value := strings.TrimSpace(data)
	if value == "2" {
		return "ptrace_scope=2", StatusOK
	}
	if value == "" {
		return "ptrace_scope unavailable", StatusWarn
	}
	return "ptrace_scope=" + value + ", recommended 2", StatusWarn
}

func parseSSForPort(stdout string, port int) (string, Status) {
	needle := ":" + strconv.Itoa(port)
	for _, line := range strings.Split(stdout, "\n") {
		if !strings.Contains(line, needle) {
			continue
		}
		if strings.Contains(line, "0.0.0.0"+needle) || strings.Contains(line, "[::]"+needle) {
			return fmt.Sprintf("gateway port %d listens on a public address", port), StatusWarn
		}
		return fmt.Sprintf("gateway port %d is listening", port), StatusOK
	}
	return fmt.Sprintf("gateway port %d is not listening", port), StatusFail
}

func parseDoctorOutput(username, stdout string) []CheckResult {
	var doc struct {
		Checks []struct {
			ID      string `json:"id"`
			Status  string `json:"status"`
			Message string `json:"message"`
		} `json:"checks"`
	}
	if err := json.Unmarshal([]byte(stdout), &doc); err == nil && len(doc.Checks) > 0 {
		results := make([]CheckResult, 0, len(doc.Checks))
		for _, check := range doc.Checks {
			id := check.ID
			if id == "" {
				id = "openclaw-doctor"
			}
			results = append(results, CheckResult{
				ID:       "openclaw-" + username + "-" + id,
				Category: "openclaw",
				Target:   username,
				Status:   normalizeStatus(check.Status),
				Message:  nonEmpty(check.Message, id),
			})
		}
		return results
	}

	status := StatusOK
	for _, line := range nonEmptyLines(stdout) {
		lower := strings.ToLower(line)
		if strings.Contains(lower, "error") || strings.Contains(lower, "fail") {
			status = StatusFail
			break
		}
		if strings.Contains(lower, "warn") {
			status = StatusWarn
		}
	}
	msg := "openclaw doctor passed"
	if strings.TrimSpace(stdout) != "" {
		msg = strings.TrimSpace(nonEmptyLines(stdout)[0])
	}
	return []CheckResult{{
		ID:       "openclaw-" + username + "-doctor",
		Category: "openclaw",
		Target:   username,
		Status:   status,
		Message:  msg,
	}}
}

func normalizeStatus(s string) Status {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "ok", "pass", "passed", "success":
		return StatusOK
	case "warn", "warning":
		return StatusWarn
	case "fail", "failed", "error":
		return StatusFail
	case "skipped", "skip":
		return StatusSkipped
	default:
		return StatusWarn
	}
}

func nonEmptyLines(s string) []string {
	var lines []string
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

func nonEmpty(value, defaultValue string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return defaultValue
	}
	return value
}
