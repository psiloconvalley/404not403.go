package domain

import "fmt"

// SourceType identifies where a ticket or comment originated.
type SourceType string

const (
	SourceEmail   SourceType = "email"   // Inbound email
	SourceSlack   SourceType = "slack"   // Slack message or thread
	SourceWeb     SourceType = "web"     // Web form submission
	SourceAPI     SourceType = "api"     // Direct API call
	SourceApp     SourceType = "app"     // Agent dashboard (internal)
	SourceSystem  SourceType = "system"  // Automated system action
)

// AllSourceTypes returns all valid source types.
func AllSourceTypes() []SourceType {
	return []SourceType{
		SourceEmail,
		SourceSlack,
		SourceWeb,
		SourceAPI,
		SourceApp,
		SourceSystem,
	}
}

// IsValid returns true if the source type is known.
func (s SourceType) IsValid() bool {
	switch s {
	case SourceEmail, SourceSlack, SourceWeb, SourceAPI, SourceApp, SourceSystem:
		return true
	}
	return false
}

// IsExternal returns true if the source is from outside the system.
// External sources: email, slack, web, api.
// Internal sources: app (agent action), system (automated).
func (s SourceType) IsExternal() bool {
	return s == SourceEmail || s == SourceSlack || s == SourceWeb || s == SourceAPI
}

// ParseSourceType converts a string to a SourceType.
func ParseSourceType(s string) (SourceType, error) {
	st := SourceType(s)
	if !st.IsValid() {
		return "", fmt.Errorf("invalid source type: %q", s)
	}
	return st, nil
}
