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
