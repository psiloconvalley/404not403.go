package domain

import "fmt"

// Status represents a ticket's position in its lifecycle.
type Status string

const (
	StatusOpen            Status = "open"             // New ticket, not yet assigned
	StatusAssigned        Status = "assigned"         // Assigned to an agent
	StatusInProgress      Status = "in_progress"      // Agent actively working
	StatusPendingCustomer Status = "pending_customer" // Waiting for customer response
	StatusPendingVendor   Status = "pending_vendor"   // Waiting for external vendor
	StatusResolved        Status = "resolved"         // Solution delivered
	StatusClosed          Status = "closed"           // Confirmed complete, no further action
	StatusReopened        Status = "reopened"         // Customer rejected resolution
)

// AllStatuses returns all valid statuses.
func AllStatuses() []Status {
	return []Status{
		StatusOpen,
		StatusAssigned,
		StatusInProgress,
		StatusPendingCustomer,
		StatusPendingVendor,
		StatusResolved,
		StatusClosed,
		StatusReopened,
	}
}

// IsValid returns true if the status is a known value.
func (s Status) IsValid() bool {
	switch s {
	case StatusOpen, StatusAssigned, StatusInProgress,
		StatusPendingCustomer, StatusPendingVendor,
		StatusResolved, StatusClosed, StatusReopened:
		return true
	}
	return false
}

// IsTerminal returns true if no further work is expected.
func (s Status) IsTerminal() bool {
	return s == StatusClosed
}

// IsResolved returns true if the ticket has been resolved (but may reopen).
func (s Status) IsResolved() bool {
	return s == StatusResolved || s == StatusClosed
}

// IsWaiting returns true if the ticket is blocked on external response.
func (s Status) IsWaiting() bool {
	return s == StatusPendingCustomer || s == StatusPendingVendor
}

// IsActive returns true if the ticket requires agent attention.
func (s Status) IsActive() bool {
	return s == StatusOpen || s == StatusAssigned ||
		s == StatusInProgress || s == StatusReopened
}

// SLAClockRunning returns true if time-to-resolution SLA is counting.
// Clock pauses when waiting on customer/vendor.
func (s Status) SLAClockRunning() bool {
	return !s.IsWaiting() && !s.IsResolved()
}

// ParseStatus converts a string to a Status.
func ParseStatus(s string) (Status, error) {
	st := Status(s)
	if !st.IsValid() {
		return "", fmt.Errorf("invalid status: %q", s)
	}
	return st, nil
}

// DefaultStatus returns the status assigned to new tickets.
func DefaultStatus() Status {
	return StatusOpen
}

// transitions defines the valid state machine.
// Key = current status, Value = list of valid next statuses.
var transitions = map[Status][]Status{
	StatusOpen: {
		StatusAssigned,
		StatusClosed, // Can close without working (duplicate, spam, etc.)
	},
	StatusAssigned: {
		StatusOpen,            // Unassign
		StatusInProgress,
		StatusPendingCustomer,
		StatusPendingVendor,
		StatusResolved,
		StatusClosed,
	},
	StatusInProgress: {
		StatusAssigned,        // Hand off to another agent
		StatusPendingCustomer,
		StatusPendingVendor,
		StatusResolved,
		StatusClosed,
	},
	StatusPendingCustomer: {
		StatusInProgress, // Customer responded
		StatusResolved,   // No response, auto-resolve
		StatusClosed,     // No response, close
	},
	StatusPendingVendor: {
		StatusInProgress, // Vendor responded
		StatusResolved,
		StatusClosed,
	},
	StatusResolved: {
		StatusReopened, // Customer says "not fixed"
		StatusClosed,   // Confirmed fixed
	},
	StatusClosed: {
		// Terminal. No valid transitions out.
		// If truly needed, create a new ticket.
	},
	StatusReopened: {
		StatusAssigned,
		StatusInProgress,
		StatusResolved,
		StatusClosed,
	},
}

// CanTransitionTo returns true if moving from current status to next is valid.
func (s Status) CanTransitionTo(next Status) bool {
	valid, ok := transitions[s]
	if !ok {
		return false
	}
	for _, v := range valid {
		if v == next {
			return true
		}
	}
	return false
}

// ValidTransitions returns all statuses reachable from the current status.
func (s Status) ValidTransitions() []Status {
	return transitions[s]
}
