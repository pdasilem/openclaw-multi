// Package preflight verifies host prerequisites before installation.
// All checks use injectable interfaces so tests never touch real system state.
package preflight

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"

	"github.com/pdasilem/openclaw-multi/internal/shell"
)

// Level indicates the severity of a check result.
const (
	LevelOK   = "ok"
	LevelWarn = "warn"
	LevelFail = "fail"
)

// Result is a single preflight check outcome.
type Result struct {
	Name   string
	Detail string
	Level  string // LevelOK | LevelWarn | LevelFail
}

func (r Result) OK() bool { return r.Level == LevelOK }

// PortConflict describes a process already listening on a port in our range.
type PortConflict struct {
	Port    int
	HexPort string
}

// Checker runs preflight checks.
type Checker struct {
	Exec          shell.Executor
	OSReleasePath string  // default: /etc/os-release
	MeminfoPath   string  // default: /proc/meminfo
	ProcNetTCP    string  // default: /proc/net/tcp
	MinDiskGB     float64 // default: 5
	MinRAMGB      float64 // default: 1
	PortRangeMin  int     // default: 18789
	PortRangeMax  int     // default: 19999
}

// NewChecker returns a Checker with production defaults.
func NewChecker(exec shell.Executor) *Checker {
	return &Checker{
		Exec:          exec,
		OSReleasePath: "/etc/os-release",
		MeminfoPath:   "/proc/meminfo",
		ProcNetTCP:    "/proc/net/tcp",
		MinDiskGB:     5,
		MinRAMGB:      1,
		PortRangeMin:  18789,
		PortRangeMax:  19999,
	}
}

// CheckAll runs all preflight checks and returns a slice of results.
// It never short-circuits — all checks always run.
func (c *Checker) CheckAll(ctx context.Context) ([]Result, error) {
	var results []Result
	results = append(results, c.checkDistro())
	results = append(results, c.checkDisk())
	results = append(results, c.checkRAM())
	results = append(results, c.checkTools(ctx))
	conflicts, err := c.CheckPorts()
	switch {
	case err != nil:
		results = append(results, Result{Name: "port-scan", Detail: err.Error(), Level: LevelWarn})
	case len(conflicts) > 0:
		ports := make([]string, len(conflicts))
		for i, p := range conflicts {
			ports[i] = strconv.Itoa(p.Port)
		}
		results = append(results, Result{
			Name:   "port-conflicts",
			Detail: fmt.Sprintf("ports in use: %s", strings.Join(ports, ", ")),
			Level:  LevelWarn,
		})
	default:
		results = append(results, Result{Name: "port-conflicts", Detail: "no conflicts", Level: LevelOK})
	}
	return results, nil
}

func (c *Checker) checkDistro() Result {
	data, err := os.ReadFile(c.OSReleasePath)
	if err != nil {
		return Result{Name: "distro", Detail: fmt.Sprintf("cannot read %s: %v", c.OSReleasePath, err), Level: LevelWarn}
	}
	info := parseOSRelease(data)
	id := strings.ToLower(info["ID"])
	versionID := info["VERSION_ID"]

	switch id {
	case "ubuntu":
		major := majorVersion(versionID)
		if major >= 22 {
			return Result{Name: "distro", Detail: fmt.Sprintf("Ubuntu %s ✓", versionID), Level: LevelOK}
		}
		return Result{Name: "distro", Detail: fmt.Sprintf("Ubuntu %s < 22.04 (unsupported)", versionID), Level: LevelWarn}
	case "debian":
		major := majorVersion(versionID)
		if major >= 12 {
			return Result{Name: "distro", Detail: fmt.Sprintf("Debian %s ✓", versionID), Level: LevelOK}
		}
		return Result{Name: "distro", Detail: fmt.Sprintf("Debian %s < 12 (unsupported)", versionID), Level: LevelWarn}
	default:
		return Result{Name: "distro", Detail: fmt.Sprintf("%s %s — untested, proceed with caution", id, versionID), Level: LevelWarn}
	}
}

func (c *Checker) checkDisk() Result {
	var stat syscall.Statfs_t
	if err := syscall.Statfs("/", &stat); err != nil {
		return Result{Name: "disk", Detail: fmt.Sprintf("statfs /: %v", err), Level: LevelWarn}
	}
	freeGB := float64(stat.Bavail) * float64(stat.Bsize) / 1e9
	if freeGB >= c.MinDiskGB {
		return Result{Name: "disk", Detail: fmt.Sprintf("%.1f GB free ✓", freeGB), Level: LevelOK}
	}
	return Result{Name: "disk", Detail: fmt.Sprintf("only %.1f GB free (need %.0f GB)", freeGB, c.MinDiskGB), Level: LevelWarn}
}

func (c *Checker) checkRAM() Result {
	data, err := os.ReadFile(c.MeminfoPath)
	if err != nil {
		return Result{Name: "ram", Detail: fmt.Sprintf("cannot read %s: %v", c.MeminfoPath, err), Level: LevelWarn}
	}
	totalKB := parseMeminfoTotal(data)
	totalGB := float64(totalKB) / 1e6
	if totalGB >= c.MinRAMGB {
		return Result{Name: "ram", Detail: fmt.Sprintf("%.1f GB total ✓", totalGB), Level: LevelOK}
	}
	return Result{Name: "ram", Detail: fmt.Sprintf("only %.1f GB RAM (need %.0f GB)", totalGB, c.MinRAMGB), Level: LevelWarn}
}

func (c *Checker) checkTools(ctx context.Context) Result {
	tools := []string{"systemctl", "useradd", "loginctl", "bash"}
	var missing []string
	for _, tool := range tools {
		res, _ := c.Exec.Run(ctx, shell.ExecOpts{Cmd: []string{"which", tool}})
		if res.ExitCode != 0 || strings.TrimSpace(res.Stdout) == "" {
			missing = append(missing, tool)
		}
	}
	if len(missing) == 0 {
		return Result{Name: "tools", Detail: "systemctl useradd loginctl bash ✓", Level: LevelOK}
	}
	return Result{Name: "tools", Detail: fmt.Sprintf("missing: %s", strings.Join(missing, ", ")), Level: LevelFail}
}

// CheckPorts returns PortConflicts for ports in [PortRangeMin, PortRangeMax]
// that are already in use, by parsing /proc/net/tcp.
func (c *Checker) CheckPorts() ([]PortConflict, error) {
	data, err := os.ReadFile(c.ProcNetTCP)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", c.ProcNetTCP, err)
	}
	return parseProcNetTCP(data, c.PortRangeMin, c.PortRangeMax), nil
}

// parseProcNetTCP extracts listening ports in [min, max] from /proc/net/tcp.
func parseProcNetTCP(data []byte, min, max int) []PortConflict {
	var conflicts []PortConflict
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Scan() // skip header
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 4 {
			continue
		}
		// state == 0A means LISTEN
		if fields[3] != "0A" {
			continue
		}
		// local_address is hex ip:port (little-endian)
		parts := strings.Split(fields[1], ":")
		if len(parts) != 2 {
			continue
		}
		portHex := parts[1]
		portBytes, err := hex.DecodeString(portHex)
		if err != nil || len(portBytes) != 2 {
			continue
		}
		port := int(binary.BigEndian.Uint16(portBytes))
		if port >= min && port <= max {
			conflicts = append(conflicts, PortConflict{Port: port, HexPort: portHex})
		}
	}
	return conflicts
}

func parseOSRelease(data []byte) map[string]string {
	result := make(map[string]string)
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := scanner.Text()
		if idx := strings.Index(line, "="); idx > 0 {
			k := line[:idx]
			v := strings.Trim(line[idx+1:], `"`)
			result[k] = v
		}
	}
	return result
}

func parseMeminfoTotal(data []byte) int64 {
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "MemTotal:") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				v, _ := strconv.ParseInt(fields[1], 10, 64)
				return v
			}
		}
	}
	return 0
}

func majorVersion(s string) int {
	parts := strings.SplitN(s, ".", 2)
	v, _ := strconv.Atoi(parts[0])
	return v
}
