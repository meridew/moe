package models

import "time"

// Device represents a managed device synced from a UEM or Intune tenant.
type Device struct {
	ID           string     `json:"id"`
	ProviderName string     `json:"provider_name"` // e.g. "uem-anz", "intune-corp"
	ProviderType string     `json:"provider_type"` // "uem" or "intune"
	SourceID     string     `json:"source_id"`     // ID within the source system
	DeviceName   string     `json:"device_name"`
	OS           string     `json:"os"`         // "iOS", "Android", "Windows", "macOS"
	OSVersion    string     `json:"os_version"` // e.g. "17.2.1"
	Model        string     `json:"model"`      // e.g. "iPhone 15 Pro"
	UserName     string     `json:"user_name"`
	UserEmail    string     `json:"user_email"`
	Compliance   string     `json:"compliance"` // "compliant", "non-compliant", "unknown"
	IsEncrypted  bool       `json:"is_encrypted"`
	JailBroken   string     `json:"jail_broken"` // "True", "False", "Unknown", or ""
	IsSupervised bool       `json:"is_supervised"`
	ThreatState  string     `json:"threat_state"` // "activated", "secured", "compromised", etc.
	LastSeen     *time.Time `json:"last_seen,omitempty"`
	LastSyncedAt *time.Time `json:"last_synced_at,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

// DeviceFilter contains optional filter criteria for querying devices.
type DeviceFilter struct {
	ProviderName string
	ProviderType string
	OS           string
	Compliance   string
	Search       string // free-text search across name, email, device name
	Limit        int
	Offset       int
}

// ProviderConfig represents a configured MDM tenant connection.
type ProviderConfig struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`          // unique display name: "uem-anz"
	Type         string    `json:"type"`          // "uem" or "intune"
	BaseURL      string    `json:"base_url"`      // API endpoint
	TenantID     string    `json:"tenant_id"`     // Intune: Azure AD tenant ID; UEM: SRP ID
	ClientID     string    `json:"client_id"`     // Intune: OAuth application/client ID
	ClientSecret string    `json:"-"`             // Intune: OAuth client secret (never serialised)
	Username     string    `json:"username"`      // UEM: admin username
	Password     string    `json:"-"`             // UEM: admin password (never serialised)
	SyncInterval string    `json:"sync_interval"` // e.g. "15m"
	Enabled      bool      `json:"enabled"`
	LastCheckAt  time.Time `json:"last_check_at"`  // last health check time
	LastCheckOK  bool      `json:"last_check_ok"`  // true if last check succeeded
	LastCheckErr string    `json:"last_check_err"` // error message from last failed check
	LastSyncAt   time.Time `json:"last_sync_at"`   // last successful sync time
	ConsecFails  int       `json:"consec_fails"`   // consecutive health check failures
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// PolicySnapshot represents a point-in-time capture of all policies from a provider.
type PolicySnapshot struct {
	ID            string    `json:"id"`
	ProviderName  string    `json:"provider_name"`
	ProviderType  string    `json:"provider_type"`
	Label         string    `json:"label"`
	TakenAt       time.Time `json:"taken_at"`
	PolicyCount   int       `json:"policy_count"`
	CategoryCount int       `json:"category_count"`
	Status        string    `json:"status"`         // "capturing", "complete", "error"
	StatusMessage string    `json:"status_message"` // error detail when status=error
}

// Snapshot status constants.
const (
	SnapshotStatusCapturing = "capturing"
	SnapshotStatusComplete  = "complete"
	SnapshotStatusError     = "error"
)

// DisplayName returns the label if set, otherwise the provider name.
func (s PolicySnapshot) DisplayName() string {
	if s.Label != "" {
		return s.Label
	}
	return s.ProviderName
}

// PolicyItem represents a single policy within a snapshot.
type PolicyItem struct {
	ID           string `json:"id"`
	SnapshotID   string `json:"snapshot_id"`
	Category     string `json:"category"`  // "compliance", "configuration", "app-protection", etc.
	SourceID     string `json:"source_id"` // ID within the source system
	PolicyName   string `json:"policy_name"`
	PolicyType   string `json:"policy_type"` // OData type or classification
	Platform     string `json:"platform"`    // "Windows", "iOS", "Android", "All", ""
	Description  string `json:"description"`
	SettingsJSON string `json:"settings_json"` // full JSON blob of settings
}
