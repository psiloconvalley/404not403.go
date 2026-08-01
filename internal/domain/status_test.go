package domain

import "testing"

func TestStatus_IsValid(t *testing.T) {
	for _, s := range AllStatuses() {
		if !s.IsValid() {
			t.Errorf("expected %q to be valid", s)
		}
	}

	invalid := []Status{"deleted", "archived", "pending", ""}
	for _, s := range invalid {
		if s.IsValid() {
			t.Errorf("expected %q to be invalid", s)
		}
	}
}

func TestStatus_IsTerminal(t *testing.T) {
	if !StatusClosed.IsTerminal() {
		t.Error("closed should be terminal")
	}

	nonTerminal := []Status{
		StatusOpen, StatusAssigned, StatusInProgress,
		StatusPendingCustomer, StatusPendingVendor,
		StatusResolved, StatusReopened,
	}
	for _, s := range nonTerminal {
		if s.IsTerminal() {
			t.Errorf("%q should not be terminal", s)
		}
	}
}

func TestStatus_IsResolved(t *testing.T) {
	if !StatusResolved.IsResolved() {
		t.Error("resolved should be resolved")
	}
	if !StatusClosed.IsResolved() {
		t.Error("closed should be resolved")
	}
	if StatusOpen.IsResolved() {
		t.Error("open should not be resolved")
	}
}

func TestStatus_IsWaiting(t *testing.T) {
	if !StatusPendingCustomer.IsWaiting() {
		t.Error("pending_customer should be waiting")
	}
	if !StatusPendingVendor.IsWaiting() {
		t.Error("pending_vendor should be waiting")
	}
	if StatusInProgress.IsWaiting() {
		t.Error("in_progress should not be waiting")
	}
}

func TestStatus_IsActive(t *testing.T) {
	active := []Status{StatusOpen, StatusAssigned, StatusInProgress, StatusReopened}
	for _, s := range active {
		if !s.IsActive() {
			t.Errorf("%q should be active", s)
		}
	}

	inactive := []Status{StatusPendingCustomer, StatusPendingVendor, StatusResolved, StatusClosed}
	for _, s := range inactive {
		if s.IsActive() {
			t.Errorf("%q should not be active", s)
		}
	}
}

func TestStatus_SLAClockRunning(t *testing.T) {
	running := []Status{StatusOpen, StatusAssigned, StatusInProgress, StatusReopened}
	for _, s := range running {
		if !s.SLAClockRunning() {
			t.Errorf("SLA clock should be running for %q", s)
		}
	}

	paused := []Status{StatusPendingCustomer, StatusPendingVendor, StatusResolved, StatusClosed}
	for _, s := range paused {
		if s.SLAClockRunning() {
			t.Errorf("SLA clock should not be running for %q", s)
		}
	}
}

// ── State Machine Tests ───────────────────────────────────────────────────────

func TestTransition_OpenToAssigned(t *testing.T) {
	if !StatusOpen.CanTransitionTo(StatusAssigned) {
		t.Error("open → assigned should be valid")
	}
}

func TestTransition_OpenToClosed(t *testing.T) {
	if !StatusOpen.CanTransitionTo(StatusClosed) {
		t.Error("open → closed should be valid (duplicate/spam)")
	}
}

func TestTransition_OpenToInProgress_Invalid(t *testing.T) {
	if StatusOpen.CanTransitionTo(StatusInProgress) {
		t.Error("open → in_progress should be invalid (must assign first)")
	}
}

func TestTransition_AssignedToInProgress(t *testing.T) {
	if !StatusAssigned.CanTransitionTo(StatusInProgress) {
		t.Error("assigned → in_progress should be valid")
	}
}

func TestTransition_AssignedToOpen(t *testing.T) {
	if !StatusAssigned.CanTransitionTo(StatusOpen) {
		t.Error("assigned → open should be valid (unassign)")
	}
}

func TestTransition_InProgressToResolved(t *testing.T) {
	if !StatusInProgress.CanTransitionTo(StatusResolved) {
		t.Error("in_progress → resolved should be valid")
	}
}

func TestTransition_InProgressToPendingCustomer(t *testing.T) {
	if !StatusInProgress.CanTransitionTo(StatusPendingCustomer) {
		t.Error("in_progress → pending_customer should be valid")
	}
}

func TestTransition_InProgressToPendingVendor(t *testing.T) {
	if !StatusInProgress.CanTransitionTo(StatusPendingVendor) {
		t.Error("in_progress → pending_vendor should be valid")
	}
}

func TestTransition_PendingCustomerToInProgress(t *testing.T) {
	if !StatusPendingCustomer.CanTransitionTo(StatusInProgress) {
		t.Error("pending_customer → in_progress should be valid (customer responded)")
	}
}

func TestTransition_ResolvedToReopened(t *testing.T) {
	if !StatusResolved.CanTransitionTo(StatusReopened) {
		t.Error("resolved → reopened should be valid")
	}
}

func TestTransition_ResolvedToClosed(t *testing.T) {
	if !StatusResolved.CanTransitionTo(StatusClosed) {
		t.Error("resolved → closed should be valid")
	}
}

func TestTransition_ClosedToAnything_Invalid(t *testing.T) {
	for _, s := range AllStatuses() {
		if StatusClosed.CanTransitionTo(s) {
			t.Errorf("closed → %q should be invalid (terminal state)", s)
		}
	}
}

func TestTransition_ReopenedToAssigned(t *testing.T) {
	if !StatusReopened.CanTransitionTo(StatusAssigned) {
		t.Error("reopened → assigned should be valid")
	}
}

func TestTransition_ReopenedToResolved(t *testing.T) {
	if !StatusReopened.CanTransitionTo(StatusResolved) {
		t.Error("reopened → resolved should be valid")
	}
}

func TestTransition_InvalidStatusHasNoTransitions(t *testing.T) {
	invalid := Status("deleted")
	if invalid.CanTransitionTo(StatusOpen) {
		t.Error("invalid status should have no valid transitions")
	}
}

func TestValidTransitions_ReturnsCorrectSet(t *testing.T) {
	valid := StatusOpen.ValidTransitions()
	if len(valid) != 2 {
		t.Errorf("expected 2 valid transitions from open, got %d", len(valid))
	}

	// Verify the two valid transitions are assigned and closed
	found := map[Status]bool{}
	for _, s := range valid {
		found[s] = true
	}
	if !found[StatusAssigned] {
		t.Error("open should be able to transition to assigned")
	}
	if !found[StatusClosed] {
		t.Error("open should be able to transition to closed")
	}
}
