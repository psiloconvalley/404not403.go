package domain

import "errors"

// Domain-level errors.
// These are not store errors (SQL failures).
// These are business rule violations.

var (
	ErrInvalidTransition = errors.New("invalid status transition")
	ErrInvalidPriority   = errors.New("invalid priority")
	ErrInvalidStatus     = errors.New("invalid status")
	ErrInvalidSource     = errors.New("invalid source type")
	ErrTicketClosed      = errors.New("ticket is closed and cannot be modified")
	ErrNotAssigned       = errors.New("ticket is not assigned")
	ErrAlreadyAssigned   = errors.New("ticket is already assigned")
	ErrSelfAssign        = errors.New("cannot assign ticket to yourself")
	ErrUnauthorized      = errors.New("not authorized to perform this action")
)
