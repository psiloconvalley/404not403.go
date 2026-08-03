// Package shared provides utilities shared across handler packages.
package shared

import (
	"errors"
	"net/http"

	"github.com/psiloconvalley/404not403/internal/domain"
)

// DomainErrStatus maps domain errors to HTTP status codes.
func DomainErrStatus(err error) int {
	switch {
	case errors.Is(err, domain.ErrUnauthorized):
		return http.StatusForbidden
	case errors.Is(err, domain.ErrInvalidTransition):
		return http.StatusUnprocessableEntity
	case errors.Is(err, domain.ErrInvalidPriority):
		return http.StatusBadRequest
	case errors.Is(err, domain.ErrInvalidStatus):
		return http.StatusBadRequest
	case errors.Is(err, domain.ErrTicketClosed):
		return http.StatusUnprocessableEntity
	default:
		return http.StatusInternalServerError
	}
}
