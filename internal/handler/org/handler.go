// Package org implements HTTP handlers for organization management.
//
// Responsibilities:
//   - Parse HTTP requests
//   - Extract identity from JWT
//   - Call the org service
//   - Write JSON responses
//
// No SQL. No business rules. No direct store calls.
// All logic lives in internal/service/org.
package org

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/psiloconvalley/404not403/internal/app"
	"github.com/psiloconvalley/404not403/internal/domain"
	"github.com/psiloconvalley/404not403/internal/middleware"
	"github.com/psiloconvalley/404not403/internal/store"
	orgsvc "github.com/psiloconvalley/404not403/internal/service/org"
)

// Handler holds handler dependencies.
type Handler struct {
	app *app.App
	svc *orgsvc.Service
}

// New returns a new org Handler.
func New(a *app.App) *Handler {
	return &Handler{
		app: a,
		svc: orgsvc.New(a.DB),
	}
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func domainErrStatus(err error) int {
	if errors.Is(err, domain.ErrUnauthorized) {
		return http.StatusForbidden
	}
	return http.StatusInternalServerError
}

// orgIDFromPath extracts org ID from /api/orgs/{orgID}/...
func orgIDFromPath(r *http.Request) string {
	path := strings.TrimPrefix(r.URL.Path, "/api/orgs/")
	parts := strings.SplitN(path, "/", 2)
	if len(parts) == 0 {
		return ""
	}
	return parts[0]
}

// ── Create ────────────────────────────────────────────────────────────────────

// Create handles POST /api/orgs
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "use POST")
		return
	}

	userID := middleware.GetUserID(r)
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	var input struct {
		Name string `json:"name"`
		Slug string `json:"slug"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	org, err := h.svc.Create(r.Context(), orgsvc.CreateInput{
		Name:        input.Name,
		Slug:        input.Slug,
		OwnerUserID: userID,
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, org)
}

// ── Get ───────────────────────────────────────────────────────────────────────

// Get handles GET /api/orgs/{orgID}
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "use GET")
		return
	}

	userID := middleware.GetUserID(r)
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	orgID := orgIDFromPath(r)
	if orgID == "" {
		writeError(w, http.StatusBadRequest, "org_id is required")
		return
	}

	ctx, err := h.svc.Get(r.Context(), orgID, userID)
	if err != nil {
		writeError(w, domainErrStatus(err), err.Error())
		return
	}

	writeJSON(w, http.StatusOK, ctx)
}

// ── List user orgs ────────────────────────────────────────────────────────────

// ListMine handles GET /api/orgs/me
// Returns all orgs the authenticated user belongs to.
func (h *Handler) ListMine(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "use GET")
		return
	}

	userID := middleware.GetUserID(r)
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	orgs, err := h.svc.GetForUser(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load orgs")
		return
	}

	if orgs == nil {
		orgs = []store.OrgMemberDetail{}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"orgs":  orgs,
		"count": len(orgs),
	})
}

// ── Invite member ─────────────────────────────────────────────────────────────

// Invite handles POST /api/orgs/{orgID}/members
func (h *Handler) Invite(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "use POST")
		return
	}

	userID := middleware.GetUserID(r)
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	orgID := orgIDFromPath(r)
	if orgID == "" {
		writeError(w, http.StatusBadRequest, "org_id is required")
		return
	}

	var input struct {
		TargetUserID string `json:"user_id"`
		Role         string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if input.TargetUserID == "" {
		writeError(w, http.StatusBadRequest, "user_id is required")
		return
	}
	if input.Role == "" {
		input.Role = "agent"
	}

	if err := h.svc.Invite(r.Context(), orgsvc.InviteInput{
		OrgID:            orgID,
		RequestingUserID: userID,
		TargetUserID:     input.TargetUserID,
		Role:             input.Role,
	}); err != nil {
		writeError(w, domainErrStatus(err), err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"status":  "invited",
		"user_id": input.TargetUserID,
		"role":    input.Role,
	})
}

// ── Update role ───────────────────────────────────────────────────────────────

// UpdateRole handles POST /api/orgs/{orgID}/members/{userID}/role
func (h *Handler) UpdateRole(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "use POST")
		return
	}

	userID := middleware.GetUserID(r)
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	// Path: /api/orgs/{orgID}/members/{targetUserID}/role
	path := strings.TrimPrefix(r.URL.Path, "/api/orgs/")
	path = strings.TrimSuffix(path, "/role")
	parts := strings.SplitN(path, "/members/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		writeError(w, http.StatusBadRequest, "invalid path")
		return
	}
	orgID := parts[0]
	targetUserID := parts[1]

	var input struct {
		Role string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if input.Role == "" {
		writeError(w, http.StatusBadRequest, "role is required")
		return
	}

	if err := h.svc.UpdateRole(r.Context(), orgsvc.UpdateRoleInput{
		OrgID:            orgID,
		RequestingUserID: userID,
		TargetUserID:     targetUserID,
		NewRole:          input.Role,
	}); err != nil {
		writeError(w, domainErrStatus(err), err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"status":  "updated",
		"user_id": targetUserID,
		"role":    input.Role,
	})
}

// ── Remove member ─────────────────────────────────────────────────────────────

// RemoveMember handles DELETE /api/orgs/{orgID}/members/{userID}
func (h *Handler) RemoveMember(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		writeError(w, http.StatusMethodNotAllowed, "use DELETE")
		return
	}

	userID := middleware.GetUserID(r)
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	// Path: /api/orgs/{orgID}/members/{targetUserID}
	path := strings.TrimPrefix(r.URL.Path, "/api/orgs/")
	parts := strings.SplitN(path, "/members/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		writeError(w, http.StatusBadRequest, "invalid path")
		return
	}
	orgID := parts[0]
	targetUserID := parts[1]

	if err := h.svc.RemoveMember(r.Context(), orgID, userID, targetUserID); err != nil {
		writeError(w, domainErrStatus(err), err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"status":  "removed",
		"user_id": targetUserID,
	})
}

// ── Update Settings ──────────────────────────────────────────────────────────

// UpdateSettings handles POST /api/orgs/{orgID}/settings
func (h *Handler) UpdateSettings(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "use POST")
		return
	}

	userID := middleware.GetUserID(r)
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	orgID := orgIDFromPath(r)
	if orgID == "" {
		writeError(w, http.StatusBadRequest, "org_id is required")
		return
	}

	var input struct {
		Name         *string `json:"name"`
		Domain       *string `json:"domain"`
		InboundEmail *string `json:"inbound_email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	org, err := h.svc.UpdateOrg(r.Context(), orgsvc.UpdateOrgInput{
		OrgID:            orgID,
		RequestingUserID: userID,
		Name:             input.Name,
		Domain:           input.Domain,
		InboundEmail:     input.InboundEmail,
	})
	if err != nil {
		writeError(w, domainErrStatus(err), err.Error())
		return
	}

	writeJSON(w, http.StatusOK, org)
}
