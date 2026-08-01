package domain

import "testing"

func TestPriority_IsValid(t *testing.T) {
	valid := []Priority{PriorityP0, PriorityP1, PriorityP2, PriorityP3}
	for _, p := range valid {
		if !p.IsValid() {
			t.Errorf("expected %q to be valid", p)
		}
	}

	invalid := []Priority{"P4", "high", "critical", ""}
	for _, p := range invalid {
		if p.IsValid() {
			t.Errorf("expected %q to be invalid", p)
		}
	}
}

func TestPriority_Severity_Order(t *testing.T) {
	if PriorityP0.Severity() >= PriorityP1.Severity() {
		t.Error("P0 should be more severe than P1")
	}
	if PriorityP1.Severity() >= PriorityP2.Severity() {
		t.Error("P1 should be more severe than P2")
	}
	if PriorityP2.Severity() >= PriorityP3.Severity() {
		t.Error("P2 should be more severe than P3")
	}
}

func TestPriority_MoreSevereThan(t *testing.T) {
	if !PriorityP0.MoreSevereThan(PriorityP1) {
		t.Error("P0 should be more severe than P1")
	}
	if !PriorityP1.MoreSevereThan(PriorityP3) {
		t.Error("P1 should be more severe than P3")
	}
	if PriorityP3.MoreSevereThan(PriorityP0) {
		t.Error("P3 should not be more severe than P0")
	}
	if PriorityP2.MoreSevereThan(PriorityP2) {
		t.Error("P2 should not be more severe than itself")
	}
}

func TestPriority_IsPageWorthy(t *testing.T) {
	if !PriorityP0.IsPageWorthy() {
		t.Error("P0 should be page-worthy")
	}
	if !PriorityP1.IsPageWorthy() {
		t.Error("P1 should be page-worthy")
	}
	if PriorityP2.IsPageWorthy() {
		t.Error("P2 should not be page-worthy")
	}
	if PriorityP3.IsPageWorthy() {
		t.Error("P3 should not be page-worthy")
	}
}

func TestPriority_IsUrgent(t *testing.T) {
	if !PriorityP0.IsUrgent() {
		t.Error("P0 should be urgent")
	}
	if PriorityP1.IsUrgent() {
		t.Error("P1 should not be urgent — page-worthy, not urgent")
	}
}

func TestParsePriority_Valid(t *testing.T) {
	p, err := ParsePriority("P0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p != PriorityP0 {
		t.Errorf("expected P0, got %q", p)
	}
}

func TestParsePriority_Invalid(t *testing.T) {
	_, err := ParsePriority("high")
	if err == nil {
		t.Error("expected error for invalid priority")
	}
}

func TestDefaultPriority(t *testing.T) {
	if DefaultPriority() != PriorityP2 {
		t.Errorf("expected default P2, got %q", DefaultPriority())
	}
}
