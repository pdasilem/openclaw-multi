// Package doctor provides structured health checks for the overlay.
package doctor

import (
	"sort"
)

// Status is the normalized outcome of a single health check.
type Status string

const (
	// StatusOK means the check passed.
	StatusOK Status = "ok"
	// StatusWarn means the check found drift that may need attention.
	StatusWarn Status = "warn"
	// StatusFail means the check failed or found a broken runtime condition.
	StatusFail Status = "fail"
	// StatusSkipped means the check is not applicable in the current state.
	StatusSkipped Status = "skipped"
)

// CheckResult is one stable, renderable health-check result.
type CheckResult struct {
	ID       string
	Category string
	Target   string
	Status   Status
	Message  string
	Details  map[string]string
	Fixable  bool
	FixID    string
}

// Report is a complete doctor report.
type Report struct {
	Results []CheckResult
}

// Summary counts results by status.
type Summary struct {
	OK      int
	Warn    int
	Fail    int
	Skipped int
}

// Fix describes one approved auto-fix operation.
type Fix struct {
	ID      string
	Target  string
	Message string
}

// FixPlan is the reviewed set of auto-fixes to apply.
type FixPlan struct {
	Fixes []Fix
}

// Summary returns status counts for the report.
func (r Report) Summary() Summary {
	var s Summary
	for _, result := range r.Results {
		switch result.Status {
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

// Add appends one check result to the report.
func (r *Report) Add(result CheckResult) {
	r.Results = append(r.Results, result)
}

// Sort orders results deterministically by category, target, and ID.
func (r *Report) Sort() {
	sort.SliceStable(r.Results, func(i, j int) bool {
		a, b := r.Results[i], r.Results[j]
		if a.Category != b.Category {
			return categoryRank(a.Category) < categoryRank(b.Category)
		}
		if a.Target != b.Target {
			return a.Target < b.Target
		}
		return a.ID < b.ID
	})
}

func categoryRank(category string) int {
	switch category {
	case "system":
		return 0
	case "services":
		return 1
	case "users":
		return 2
	case "filesystem":
		return 3
	case "network":
		return 4
	case "openclaw":
		return 5
	default:
		return 99
	}
}
