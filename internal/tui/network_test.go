package tui

import (
	"context"
	"testing"

	netops "github.com/pdasilem/openclaw-multi/internal/network"
)

type fakeNetworkService struct {
	report       netops.Report
	probes       []netops.ProbeResult
	dnsPlan      netops.DNSPlan
	ufwPlan      netops.UFWPlan
	appliedDNS   bool
	appliedUFW   bool
	snapshotCall bool
}

func (f *fakeNetworkService) Snapshot(context.Context) (netops.Report, error) {
	f.snapshotCall = true
	return f.report, nil
}

func (f *fakeNetworkService) Probe(context.Context) ([]netops.ProbeResult, error) {
	return f.probes, nil
}

func (f *fakeNetworkService) PlanWildcardDNS(context.Context) (netops.DNSPlan, error) {
	return f.dnsPlan, nil
}

func (f *fakeNetworkService) ApplyWildcardDNS(context.Context, netops.DNSPlan) error {
	f.appliedDNS = true
	return nil
}

func (f *fakeNetworkService) PlanUFW(context.Context) (netops.UFWPlan, error) {
	return f.ufwPlan, nil
}

func (f *fakeNetworkService) ApplyUFW(context.Context, netops.UFWPlan) error {
	f.appliedUFW = true
	return nil
}

func TestNetworkScreenRefreshAndProbe(t *testing.T) {
	svc := &fakeNetworkService{
		report: netops.Report{
			Tailscale:  netops.TailscaleInfo{Status: netops.StatusOK, Message: "ok"},
			Cloudflare: netops.CloudflareInfo{Status: netops.StatusOK, Message: "ok"},
			UFW:        netops.UFWInfo{Status: netops.StatusOK, Message: "ok"},
			Summary:    netops.Summary{OK: 3},
		},
		probes: []netops.ProbeResult{{Hostname: "alice.example.com", Status: netops.StatusOK, Message: "gateway responded"}},
	}
	m := newNetwork(svc)
	updated, cmd := m.Update(keyText("r"))
	m = updated
	msg := cmd()
	updated, _ = m.Update(msg)
	m = updated
	if !svc.snapshotCall || m.report.Tailscale.Status != netops.StatusOK {
		t.Fatalf("refresh failed: %+v", m)
	}
	updated, cmd = m.Update(keyText("p"))
	m = updated
	msg = cmd()
	updated, _ = m.Update(msg)
	m = updated
	if len(m.report.Probes) != 1 {
		t.Fatalf("probe result missing: %+v", m.report.Probes)
	}
}

func TestNetworkProbeViewWrapsLongErrors(t *testing.T) {
	m := newNetwork(&fakeNetworkService{})
	m.report = netops.Report{
		Tailscale:  netops.TailscaleInfo{Status: netops.StatusOK, Message: "ok"},
		Cloudflare: netops.CloudflareInfo{Status: netops.StatusOK, Message: "ok"},
		UFW:        netops.UFWInfo{Status: netops.StatusOK, Message: "ok"},
		Probes: []netops.ProbeResult{{
			Hostname: "gateway-pdasilem.oc.defiharbor.top",
			Status:   netops.StatusFail,
			Message:  "curl: (35) OpenSSL/3.0.13: error:0A000410:SSL routines::sslv3 alert handshake failure",
		}},
	}

	view := m.View()
	if !contains(view, "routines::sslv3 alert handshake failure") || !contains(view, "\n                                             routines::sslv3") {
		t.Fatalf("expected wrapped full probe error, got %q", view)
	}
}

func TestNetworkScreenDNSReviewApply(t *testing.T) {
	svc := &fakeNetworkService{dnsPlan: netops.DNSPlan{
		Action: netops.DNSActionCreate,
		Record: netops.DNSRecord{Type: "CNAME", Name: "*.openclaw.example.com", Content: "abc.cfargotunnel.com"},
	}}
	m := newNetwork(svc)
	updated, cmd := m.Update(keyText("d"))
	m = updated
	msg := cmd()
	updated, _ = m.Update(msg)
	m = updated
	if m.mode != networkModeDNSReview {
		t.Fatalf("expected dns review, got %d", m.mode)
	}
	updated, cmd = m.Update(keyText("enter"))
	m = updated
	msg = cmd()
	updated, _ = m.Update(msg)
	m = updated
	if !svc.appliedDNS || m.mode != networkModeReport {
		t.Fatalf("dns apply failed: %+v", m)
	}
}

func TestNetworkScreenUFWReviewApply(t *testing.T) {
	svc := &fakeNetworkService{ufwPlan: netops.UFWPlan{RequiredPorts: []int{18789}, MissingPorts: []int{18789}}}
	m := newNetwork(svc)
	updated, cmd := m.Update(keyText("u"))
	m = updated
	msg := cmd()
	updated, _ = m.Update(msg)
	m = updated
	if m.mode != networkModeUFWReview {
		t.Fatalf("expected ufw review, got %d", m.mode)
	}
	updated, cmd = m.Update(keyText("enter"))
	m = updated
	msg = cmd()
	_, _ = m.Update(msg)
	if !svc.appliedUFW {
		t.Fatal("expected UFW apply")
	}
}

func TestModelMenuActionRoutesToNetwork(t *testing.T) {
	m := testModel("host")
	m.networkService = &fakeNetworkService{}
	updated, _ := m.Update(MenuActionMsg{ItemID: 5})
	um := updated.(Model)
	if um.screen != screenNetwork {
		t.Fatalf("expected screenNetwork, got %d", um.screen)
	}
}
