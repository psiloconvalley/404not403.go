package org

import (
	"testing"
)

func TestRoleAtLeast_OwnerMeetsAll(t *testing.T) {
	roles := []string{"viewer", "agent", "admin", "owner"}
	for _, required := range roles {
		if !roleAtLeast("owner", required) {
			t.Errorf("owner should meet %q", required)
		}
	}
}

func TestRoleAtLeast_ViewerMeetsOnlyViewer(t *testing.T) {
	if !roleAtLeast("viewer", "viewer") {
		t.Error("viewer should meet viewer")
	}
	higher := []string{"agent", "admin", "owner"}
	for _, required := range higher {
		if roleAtLeast("viewer", required) {
			t.Errorf("viewer should NOT meet %q", required)
		}
	}
}

func TestRoleAtLeast_AgentMeetsAgentAndViewer(t *testing.T) {
	if !roleAtLeast("agent", "agent") {
		t.Error("agent should meet agent")
	}
	if !roleAtLeast("agent", "viewer") {
		t.Error("agent should meet viewer")
	}
	if roleAtLeast("agent", "admin") {
		t.Error("agent should NOT meet admin")
	}
	if roleAtLeast("agent", "owner") {
		t.Error("agent should NOT meet owner")
	}
}

func TestRoleAtLeast_AdminMeetsAllExceptOwner(t *testing.T) {
	meets := []string{"viewer", "agent", "admin"}
	for _, required := range meets {
		if !roleAtLeast("admin", required) {
			t.Errorf("admin should meet %q", required)
		}
	}
	if roleAtLeast("admin", "owner") {
		t.Error("admin should NOT meet owner")
	}
}

func TestRoleAtLeast_UnknownRole_ReturnsFalse(t *testing.T) {
	if roleAtLeast("hacker", "viewer") {
		t.Error("unknown role should not meet any requirement")
	}
	if roleAtLeast("viewer", "superadmin") {
		t.Error("unknown required role should not be met")
	}
}
