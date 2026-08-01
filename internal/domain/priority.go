// Package domain defines the canonical business vocabulary for the ticketing system.
// This package has zero external dependencies. It is the source of truth.
package domain

import "fmt"

// Priority represents ticket urgency and impact.
// P0 is most severe. P3 is least severe.
type Priority string

const (
	PriorityP0 Priority = "P0" // Critical — system down, business stopped
	PriorityP1 Priority = "P1" // High — user blocked, core function impaired
	PriorityP2 Priority = "P2" // Medium — degraded but workaround exists
	PriorityP3 Priority = "P3" // Low — minor issue, no urgency
)

// AllPriorities returns all valid priorities in severity order (most severe first).
func AllPriorities() []Priority {
	return []Priority{PriorityP0, PriorityP1, PriorityP2, PriorityP3}
}

// IsValid returns true if the priority is a known value.
func (p Priority) IsValid() bool {
	switch p {
	case PriorityP0, PriorityP1, PriorityP2, PriorityP3:
		return true
	}
	return false
}

// Severity returns a numeric value for comparison.
// Lower number = more severe. P0 = 0, P3 = 3.
func (p Priority) Severity() int {
	switch p {
	case PriorityP0:
		return 0
	case PriorityP1:
		return 1
	case PriorityP2:
		return 2
	case PriorityP3:
		return 3
	}
	return 99 // unknown priority sorts last
}

// IsPageWorthy returns true if this priority should trigger immediate paging.
func (p Priority) IsPageWorthy() bool {
	return p == PriorityP0 || p == PriorityP1
}

// IsUrgent returns true if this priority requires expedited handling.
func (p Priority) IsUrgent() bool {
	return p == PriorityP0
}

// MoreSevereThan returns true if p is more severe than other.
func (p Priority) MoreSevereThan(other Priority) bool {
	return p.Severity() < other.Severity()
}

// ParsePriority converts a string to a Priority.
// Returns an error if the string is not a valid priority.
func ParsePriority(s string) (Priority, error) {
	p := Priority(s)
	if !p.IsValid() {
		return "", fmt.Errorf("invalid priority: %q", s)
	}
	return p, nil
}

// DefaultPriority returns the priority assigned to new tickets
// before AI classification or manual assignment.
func DefaultPriority() Priority {
	return PriorityP2
}
