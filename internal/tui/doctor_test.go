package tui

import (
	"context"
	"testing"

	doctorops "github.com/pdasilem/openclaw-multi/internal/doctor"
)

type fakeDoctorService struct {
	report  doctorops.Report
	doctor  doctorops.Report
	planned doctorops.FixPlan
	applied bool
}

func (f *fakeDoctorService) Run(context.Context) (doctorops.Report, error) {
	return f.report, nil
}

func (f *fakeDoctorService) RunOpenClawDoctor(context.Context) (doctorops.Report, error) {
	return f.doctor, nil
}

func (f *fakeDoctorService) PlanFixes(doctorops.Report) doctorops.FixPlan {
	return f.planned
}

func (f *fakeDoctorService) ApplyFixes(context.Context, doctorops.FixPlan) error {
	f.applied = true
	return nil
}

func TestDoctorScreenRunChecks(t *testing.T) {
	svc := &fakeDoctorService{report: doctorops.Report{Results: []doctorops.CheckResult{{
		ID:       "system-disk",
		Category: "system",
		Target:   "disk",
		Status:   doctorops.StatusOK,
		Message:  "disk ok",
	}}}}
	m := newDoctor(svc)
	m, cmd := m.Update(keyText("r"))
	if cmd == nil {
		t.Fatal("expected run command")
	}
	loaded := cmd().(doctorLoadedMsg)
	m, _ = m.Update(loaded)
	view := m.View()
	if !contains(view, "disk ok") || !contains(view, "ok=1") {
		t.Fatalf("expected report in view, got %q", view)
	}
}

func TestDoctorScreenRunOpenClawDoctor(t *testing.T) {
	svc := &fakeDoctorService{doctor: doctorops.Report{Results: []doctorops.CheckResult{{
		ID:       "openclaw-alice",
		Category: "openclaw",
		Target:   "alice",
		Status:   doctorops.StatusWarn,
		Message:  "plugin warning",
	}}}}
	m := newDoctor(svc)
	_, cmd := m.Update(keyText("d"))
	if cmd == nil {
		t.Fatal("expected doctor command")
	}
	m, _ = m.Update(cmd().(doctorLoadedMsg))
	if !contains(m.View(), "plugin warning") {
		t.Fatalf("expected doctor output in view, got %q", m.View())
	}
}

func TestDoctorViewWrapsLongMessages(t *testing.T) {
	m := newDoctor(&fakeDoctorService{})
	m.report = doctorops.Report{Results: []doctorops.CheckResult{{
		ID:       "user-pdasilem-gateway",
		Category: "users",
		Target:   "pdasilem",
		Status:   doctorops.StatusFail,
		Message:  "gateway not active: failed to connect to systemd user bus and returned a long diagnostic message",
	}}}

	view := m.View()
	if !contains(view, "failed to connect to systemd") || !contains(view, "\n                   diagnostic message") {
		t.Fatalf("expected wrapped doctor message, got %q", view)
	}
}

func TestDoctorScreenFixReviewAndApply(t *testing.T) {
	svc := &fakeDoctorService{
		report: doctorops.Report{Results: []doctorops.CheckResult{{
			ID:       "filesystem-alice-openclaw.json",
			Category: "filesystem",
			Target:   "alice",
			Status:   doctorops.StatusWarn,
			Message:  "mode wrong",
			Fixable:  true,
			FixID:    "chmod-openclaw-config",
		}}},
		planned: doctorops.FixPlan{Fixes: []doctorops.Fix{{ID: "chmod-openclaw-config", Target: "alice", Message: "mode wrong"}}},
	}
	m := newDoctor(svc)
	m.report = svc.report
	m, _ = m.Update(keyText("f"))
	if !contains(m.View(), "Fix review") || !contains(m.View(), "chmod-openclaw-config") {
		t.Fatalf("expected fix review, got %q", m.View())
	}
	_, cmd := m.Update(keyText("enter"))
	if cmd == nil {
		t.Fatal("expected apply fixes command")
	}
	m, _ = m.Update(cmd().(doctorFixDoneMsg))
	if !svc.applied || !contains(m.View(), "fixes applied") {
		t.Fatalf("expected fixes applied, view=%q", m.View())
	}
}
