package domain

import "fmt"

// EventType identifies what happened in the audit log.
type EventType string

const (
	// Ticket lifecycle
	EventTicketCreated      EventType = "ticket.created"
	EventTicketAssigned     EventType = "ticket.assigned"
	EventTicketUnassigned   EventType = "ticket.unassigned"
	EventTicketStatusChange EventType = "ticket.status_changed"
	EventTicketPriorityChange EventType = "ticket.priority_changed"
	EventTicketResolved     EventType = "ticket.resolved"
	EventTicketReopened     EventType = "ticket.reopened"
	EventTicketClosed       EventType = "ticket.closed"

	// Comments
	EventCommentAdded       EventType = "comment.added"
	EventCommentInternal    EventType = "comment.internal" // Agent-only note

	// AI
	EventAIClassified       EventType = "ai.classified"
	EventAIDraftGenerated   EventType = "ai.draft_generated"
	EventAIDraftAccepted    EventType = "ai.draft_accepted"
	EventAIDraftRejected    EventType = "ai.draft_rejected"
	EventAIAutoResolved     EventType = "ai.auto_resolved"

	// SLA
	EventSLAWarning         EventType = "sla.warning"
	EventSLABreached        EventType = "sla.breached"

	// Config Items
	EventCILinked           EventType = "ci.linked"
	EventCIUnlinked         EventType = "ci.unlinked"

	// Customer
	EventCustomerResponded  EventType = "customer.responded"
	EventCustomerEscalated  EventType = "customer.escalated" // Rejected AI resolution
)

// AllEventTypes returns all valid event types.
func AllEventTypes() []EventType {
	return []EventType{
		EventTicketCreated,
		EventTicketAssigned,
		EventTicketUnassigned,
		EventTicketStatusChange,
		EventTicketPriorityChange,
		EventTicketResolved,
		EventTicketReopened,
		EventTicketClosed,
		EventCommentAdded,
		EventCommentInternal,
		EventAIClassified,
		EventAIDraftGenerated,
		EventAIDraftAccepted,
		EventAIDraftRejected,
		EventAIAutoResolved,
		EventSLAWarning,
		EventSLABreached,
		EventCILinked,
		EventCIUnlinked,
		EventCustomerResponded,
		EventCustomerEscalated,
	}
}

// IsValid returns true if the event type is known.
func (e EventType) IsValid() bool {
	for _, valid := range AllEventTypes() {
		if e == valid {
			return true
		}
	}
	return false
}

// ParseEventType converts a string to an EventType.
func ParseEventType(s string) (EventType, error) {
	e := EventType(s)
	if !e.IsValid() {
		return "", fmt.Errorf("invalid event type: %q", s)
	}
	return e, nil
}

// ActorType identifies who or what performed an action.
type ActorType string

const (
	ActorUser    ActorType = "user"    // Human agent
	ActorSystem  ActorType = "system"  // Automated system process
	ActorAI      ActorType = "ai"      // AI classification or response
	ActorWebhook ActorType = "webhook" // External system via webhook
)

// IsValid returns true if the actor type is known.
func (a ActorType) IsValid() bool {
	switch a {
	case ActorUser, ActorSystem, ActorAI, ActorWebhook:
		return true
	}
	return false
}
