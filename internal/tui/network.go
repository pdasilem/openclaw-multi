package tui

import (
	"context"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	netops "github.com/pdasilem/openclaw-multi/internal/network"
)

type networkService interface {
	Snapshot(ctx context.Context) (netops.Report, error)
	Probe(ctx context.Context) ([]netops.ProbeResult, error)
	PlanWildcardDNS(ctx context.Context) (netops.DNSPlan, error)
	ApplyWildcardDNS(ctx context.Context, plan netops.DNSPlan) error
	PlanUFW(ctx context.Context) (netops.UFWPlan, error)
	ApplyUFW(ctx context.Context, plan netops.UFWPlan) error
}

type networkMode int

const (
	networkModeReport networkMode = iota
	networkModeDNSReview
	networkModeUFWReview
)

type networkModel struct {
	service networkService
	report  netops.Report
	dnsPlan netops.DNSPlan
	ufwPlan netops.UFWPlan
	mode    networkMode
	status  string
	errText string
	loading string
}

type networkLoadedMsg struct {
	report netops.Report
	err    error
}

type networkProbeMsg struct {
	probes []netops.ProbeResult
	err    error
}

type networkDNSPlanMsg struct {
	plan netops.DNSPlan
	err  error
}

type networkUFWPlanMsg struct {
	plan netops.UFWPlan
	err  error
}

type networkApplyMsg struct {
	label string
	err   error
}

func newNetwork(service networkService) networkModel {
	return networkModel{service: service, status: "Press r to refresh network status."}
}

func (m networkModel) Init() tea.Cmd { return nil }

func (m networkModel) Update(msg tea.Msg) (networkModel, tea.Cmd) {
	switch msg := msg.(type) {
	case networkLoadedMsg:
		m.loading = ""
		m.errText = errString(msg.err)
		if msg.err == nil {
			m.report = msg.report
			m.status = "network status refreshed"
		}
		return m, nil
	case networkProbeMsg:
		m.loading = ""
		m.errText = errString(msg.err)
		if msg.err == nil {
			m.report.Probes = msg.probes
			m.status = "gateway probes completed"
		}
		return m, nil
	case networkDNSPlanMsg:
		m.loading = ""
		m.errText = errString(msg.err)
		if msg.err == nil {
			m.dnsPlan = msg.plan
			m.mode = networkModeDNSReview
		}
		return m, nil
	case networkUFWPlanMsg:
		m.loading = ""
		m.errText = errString(msg.err)
		if msg.err == nil {
			m.ufwPlan = msg.plan
			m.mode = networkModeUFWReview
		}
		return m, nil
	case networkApplyMsg:
		m.loading = ""
		m.errText = errString(msg.err)
		if msg.err == nil {
			m.status = msg.label + " applied"
			m.mode = networkModeReport
		}
		return m, nil
	case tea.KeyMsg:
		if m.mode == networkModeDNSReview {
			return m.updateDNSReview(msg)
		}
		if m.mode == networkModeUFWReview {
			return m.updateUFWReview(msg)
		}
		return m.updateReport(msg)
	}
	return m, nil
}

func (m networkModel) View() string {
	var b strings.Builder
	b.WriteString("Network and firewall\n\n")
	if m.loading != "" {
		b.WriteString("Running: " + m.loading + "\n\n")
	}
	if m.status != "" {
		b.WriteString(m.status + "\n")
	}
	if m.errText != "" {
		b.WriteString("Error: " + m.errText + "\n")
	}
	switch m.mode {
	case networkModeDNSReview:
		return m.renderDNSReview(&b)
	case networkModeUFWReview:
		return m.renderUFWReview(&b)
	default:
		return m.renderReport(&b)
	}
}

func (m networkModel) renderReport(b *strings.Builder) string {
	if m.report.Tailscale.Message != "" {
		fmt.Fprintf(b, "\nSummary: ok=%d warn=%d fail=%d skipped=%d\n\n",
			m.report.Summary.OK, m.report.Summary.Warn, m.report.Summary.Fail, m.report.Summary.Skipped)
		fmt.Fprintf(b, "Tailscale: [%s] %s", m.report.Tailscale.Status, m.report.Tailscale.Message)
		if m.report.Tailscale.IP != "" {
			fmt.Fprintf(b, " (%s)", m.report.Tailscale.IP)
		}
		b.WriteByte('\n')
		fmt.Fprintf(b, "Cloudflare: [%s] %s\n", m.report.Cloudflare.Status, m.report.Cloudflare.Message)
		fmt.Fprintf(b, "UFW: [%s] %s\n", m.report.UFW.Status, m.report.UFW.Message)
		if len(m.report.Cloudflare.Routes) > 0 {
			b.WriteString("\nRoutes:\n")
			for _, route := range m.report.Cloudflare.Routes {
				enabled := "disabled"
				if route.Enabled {
					enabled = "enabled"
				}
				fmt.Fprintf(b, "  %s -> localhost:%d (%s)\n", route.Hostname, route.LocalPort, enabled)
			}
		}
		if len(m.report.Ports) > 0 {
			b.WriteString("\nListening ports:\n")
			for _, port := range m.report.Ports {
				fmt.Fprintf(b, "  [%s] %s:%d %s\n", port.Status, port.Address, port.Port, port.Message)
			}
		}
		if len(m.report.Probes) > 0 {
			b.WriteString("\nGateway probes:\n")
			for _, probe := range m.report.Probes {
				fmt.Fprintf(b, "  [%s] %s: %s\n", probe.Status, probe.Hostname, probe.Message)
			}
		}
	}
	b.WriteString("\nr refresh   p probe gateways   d review DNS   u review UFW   q back")
	return b.String()
}

func (m networkModel) renderDNSReview(b *strings.Builder) string {
	b.WriteString("\nCloudflare DNS review:\n")
	fmt.Fprintf(b, "  Action: %s\n", m.dnsPlan.Action)
	fmt.Fprintf(b, "  Record: %s %s -> %s\n", m.dnsPlan.Record.Type, m.dnsPlan.Record.Name, m.dnsPlan.Record.Content)
	if m.dnsPlan.Current != nil {
		fmt.Fprintf(b, "  Current: %s\n", m.dnsPlan.Current.Content)
	}
	fmt.Fprintf(b, "  Note: %s\n", m.dnsPlan.Message)
	b.WriteString("\nEnter apply   Esc cancel")
	return b.String()
}

func (m networkModel) renderUFWReview(b *strings.Builder) string {
	b.WriteString("\nUFW review:\n")
	fmt.Fprintf(b, "  Required ports: %s\n", intList(m.ufwPlan.RequiredPorts))
	fmt.Fprintf(b, "  Missing ports: %s\n", intList(m.ufwPlan.MissingPorts))
	fmt.Fprintf(b, "  Note: %s\n", m.ufwPlan.Message)
	b.WriteString("\nEnter apply   Esc cancel")
	return b.String()
}

func (m networkModel) updateReport(msg tea.KeyMsg) (networkModel, tea.Cmd) {
	switch msg.String() {
	case "q", "esc":
		return m, func() tea.Msg { return backMsg{} }
	case "r":
		m.loading = "network status"
		return m, m.snapshotCmd()
	case "p":
		m.loading = "gateway probes"
		return m, m.probeCmd()
	case "d":
		m.loading = "dns plan"
		return m, m.dnsPlanCmd()
	case "u":
		m.loading = "ufw plan"
		return m, m.ufwPlanCmd()
	}
	return m, nil
}

func (m networkModel) updateDNSReview(msg tea.KeyMsg) (networkModel, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.mode = networkModeReport
		return m, nil
	case "enter":
		m.loading = "apply dns plan"
		return m, m.applyDNSCmd(m.dnsPlan)
	}
	return m, nil
}

func (m networkModel) updateUFWReview(msg tea.KeyMsg) (networkModel, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.mode = networkModeReport
		return m, nil
	case "enter":
		m.loading = "apply ufw plan"
		return m, m.applyUFWCmd(m.ufwPlan)
	}
	return m, nil
}

func (m networkModel) snapshotCmd() tea.Cmd {
	return func() tea.Msg {
		if m.service == nil {
			return networkLoadedMsg{err: fmt.Errorf("network service unavailable")}
		}
		report, err := m.service.Snapshot(context.Background())
		return networkLoadedMsg{report: report, err: err}
	}
}

func (m networkModel) probeCmd() tea.Cmd {
	return func() tea.Msg {
		if m.service == nil {
			return networkProbeMsg{err: fmt.Errorf("network service unavailable")}
		}
		probes, err := m.service.Probe(context.Background())
		return networkProbeMsg{probes: probes, err: err}
	}
}

func (m networkModel) dnsPlanCmd() tea.Cmd {
	return func() tea.Msg {
		if m.service == nil {
			return networkDNSPlanMsg{err: fmt.Errorf("network service unavailable")}
		}
		plan, err := m.service.PlanWildcardDNS(context.Background())
		return networkDNSPlanMsg{plan: plan, err: err}
	}
}

func (m networkModel) ufwPlanCmd() tea.Cmd {
	return func() tea.Msg {
		if m.service == nil {
			return networkUFWPlanMsg{err: fmt.Errorf("network service unavailable")}
		}
		plan, err := m.service.PlanUFW(context.Background())
		return networkUFWPlanMsg{plan: plan, err: err}
	}
}

func (m networkModel) applyDNSCmd(plan netops.DNSPlan) tea.Cmd {
	return func() tea.Msg {
		if m.service == nil {
			return networkApplyMsg{label: "dns plan", err: fmt.Errorf("network service unavailable")}
		}
		return networkApplyMsg{label: "dns plan", err: m.service.ApplyWildcardDNS(context.Background(), plan)}
	}
}

func (m networkModel) applyUFWCmd(plan netops.UFWPlan) tea.Cmd {
	return func() tea.Msg {
		if m.service == nil {
			return networkApplyMsg{label: "ufw plan", err: fmt.Errorf("network service unavailable")}
		}
		return networkApplyMsg{label: "ufw plan", err: m.service.ApplyUFW(context.Background(), plan)}
	}
}

func intList(values []int) string {
	if len(values) == 0 {
		return "none"
	}
	parts := make([]string, 0, len(values))
	for _, value := range values {
		parts = append(parts, fmt.Sprint(value))
	}
	return strings.Join(parts, ", ")
}
