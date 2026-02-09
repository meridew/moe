package provider

import (
	"context"
	"time"
)

// Provider is the interface every MDM backend must implement.
// A single Provider instance represents one tenant connection (e.g. "intune-corp").
type Provider interface {
	// Identity
	Name() string // Unique name matching ProviderConfig.Name, e.g. "intune-corp"
	Type() string // "uem" or "intune"

	// ── Connectivity ────────────────────────────────────────────────────
	// TestConnection performs a lightweight connectivity check (e.g. acquire
	// an OAuth token or hit an auth endpoint). Returns nil if reachable.
	TestConnection(ctx context.Context) error

	// ── Sync (pull data INTO MOE) ───────────────────────────────────────
	// SyncDevices fetches a page of devices. Pass empty cursor for first page.
	// Returns devices, next cursor (empty if done), and error.
	SyncDevices(ctx context.Context, cursor string) ([]SyncDevice, string, error)

	// ── Commands (push actions OUT to devices) ──────────────────────────
	// SendCommand sends a management command to a device.
	SendCommand(ctx context.Context, sourceDeviceID string, cmd Command) (string, error)

	// CheckCommandStatus checks whether a previously sent command has completed.
	CheckCommandStatus(ctx context.Context, commandID string) (CommandStatus, error)
}

// SyncDevice is the normalised device record returned by a provider during sync.
// The sync engine maps this to the internal Device model.
type SyncDevice struct {
	SourceID   string
	DeviceName string
	OS         string
	OSVersion  string
	Model      string
	UserName   string
	UserEmail  string
	Compliance string // "compliant", "non-compliant", "unknown"
	LastSeen   *time.Time
}

// Command represents an action to send to a device.
type Command struct {
	Action string            // e.g. "reboot", "lock", "wipe", "sync", "retire"
	Params map[string]string // action-specific parameters
}

// CommandStatus represents the current state of a previously sent command.
type CommandStatus struct {
	ID        string
	State     string // "pending", "running", "completed", "failed"
	Detail    string // human-readable detail or error message
	UpdatedAt time.Time
}
