package tui

import (
	"context"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	doctorops "github.com/pdasilem/openclaw-multi/internal/doctor"
)

type doctorService interface {
	Run(ctx context.Context) (doctorops.Report, error)
	RunOpenClawDoctor(ctx context.Context) (doctorops.Report, error)
	PlanFixes(report doctorops.Report) doctorops.FixPlan
	ApplyFixes(ctx context.Context, plan doctorops.FixPlan) error
}

type doctorMode int

const (
	doctorModeReport doctorMode = iota
	doctorModeFixReview
)

type doctorModel struct {
	service doctorService
	report  doctorops.Report
	fixes   doctorops.FixPlan
	mode    doctorMode
	status  string
	errText string
	loading string
}

type doctorLoadedMsg struct {
	report doctorops.Report
	err    error
	label  string
}

type doctorFixDoneMsg struct {
	err error
}

func newDoctor(service doctorService) doctorModel {
	return doctorModel{service: service, status: "Press r to run health checks."}
}

func (m doctorModel) Init() tea.Cmd { return nil }

func (m doctorModel) Update(msg tea.Msg) (doctorModel, tea.Cmd) {
	switch msg := msg.(type) {
	case doctorLoadedMsg:
		m.loading = ""
		m.report = msg.report
		m.errText = errString(msg.err)
		if msg.err == nil {
			m.status = msg.label + " completed"
		}
		return m, nil
	case doctorFixDoneMsg:
		m.loading = ""
		m.errText = errString(msg.err)
		if msg.err == nil {
			m.status = "fixes applied"
			m.mode = doctorModeReport
		}
		return m, nil
	case tea.KeyMsg:
		if m.mode == doctorModeFixReview {
			return m.updateFixReview(msg)
		}
		return m.updateReport(msg)
	}
	return m, nil
}

func (m doctorModel) View() string {
	var b strings.Builder
	b.WriteString("Health Check / Doctor\n\n")
	if m.loading != "" {
		b.WriteString("Running: " + m.loading + "\n\n")
	}
	if m.status != "" {
		b.WriteString(m.status + "\n")
	}
	if m.errText != "" {
		b.WriteString("Error: " + m.errText + "\n")
	}
	if m.mode == doctorModeFixReview {
		b.WriteString("\nFix review:\n")
		if len(m.fixes.Fixes) == 0 {
			b.WriteString("  No approved fixes available.\n")
		}
		for _, fix := range m.fixes.Fixes {
			fmt.Fprintf(&b, "  [%s] %s: %s\n", fix.ID, fix.Target, fix.Message)
		}
		b.WriteString("\nEnter apply   Esc cancel")
		return b.String()
	}

	summary := m.report.Summary()
	if len(m.report.Results) > 0 {
		fmt.Fprintf(&b, "\nSummary: ok=%d warn=%d fail=%d skipped=%d\n\n", summary.OK, summary.Warn, summary.Fail, summary.Skipped)
		current := ""
		for _, result := range m.report.Results {
			if result.Category != current {
				current = result.Category
				b.WriteString(titleCase(current) + ":\n")
			}
			fix := ""
			if result.Fixable {
				fix = " [fixable]"
			}
			fmt.Fprintf(&b, "  [%s] %s: %s%s\n", result.Status, result.Target, result.Message, fix)
		}
	}
	b.WriteString("\nr run checks   d run openclaw doctor   f review fixes   q back")
	return b.String()
}

func titleCase(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func (m doctorModel) updateReport(msg tea.KeyMsg) (doctorModel, tea.Cmd) {
	switch msg.String() {
	case "q", "esc":
		return m, func() tea.Msg { return backMsg{} }
	case "r":
		m.loading = "health checks"
		return m, m.runCmd()
	case "d":
		m.loading = "openclaw doctor"
		return m, m.doctorCmd()
	case "f":
		if m.service == nil {
			m.errText = "doctor service unavailable"
			return m, nil
		}
		m.fixes = m.service.PlanFixes(m.report)
		m.mode = doctorModeFixReview
		return m, nil
	}
	return m, nil
}

func (m doctorModel) updateFixReview(msg tea.KeyMsg) (doctorModel, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.mode = doctorModeReport
		return m, nil
	case "enter":
		if len(m.fixes.Fixes) == 0 {
			m.mode = doctorModeReport
			return m, nil
		}
		m.loading = "apply fixes"
		return m, m.applyFixesCmd(m.fixes)
	}
	return m, nil
}

func (m doctorModel) runCmd() tea.Cmd {
	return func() tea.Msg {
		if m.service == nil {
			return doctorLoadedMsg{err: fmt.Errorf("doctor service unavailable"), label: "health checks"}
		}
		report, err := m.service.Run(context.Background())
		return doctorLoadedMsg{report: report, err: err, label: "health checks"}
	}
}

func (m doctorModel) doctorCmd() tea.Cmd {
	return func() tea.Msg {
		if m.service == nil {
			return doctorLoadedMsg{err: fmt.Errorf("doctor service unavailable"), label: "openclaw doctor"}
		}
		report, err := m.service.RunOpenClawDoctor(context.Background())
		return doctorLoadedMsg{report: report, err: err, label: "openclaw doctor"}
	}
}

func (m doctorModel) applyFixesCmd(plan doctorops.FixPlan) tea.Cmd {
	return func() tea.Msg {
		if m.service == nil {
			return doctorFixDoneMsg{err: fmt.Errorf("doctor service unavailable")}
		}
		return doctorFixDoneMsg{err: m.service.ApplyFixes(context.Background(), plan)}
	}
}
