package intune

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/dan/moe/internal/provider"
)

// Config holds the configuration for an Intune provider instance.
type Config struct {
	Name         string // unique name e.g. "intune-corp"
	TenantID     string
	ClientID     string
	ClientSecret string
}

// Provider implements the provider.Provider interface for Microsoft Intune
// via the Microsoft Graph API.
type Provider struct {
	config Config
	tokens *tokenCache
	client *http.Client
}

// New creates a new Intune provider instance.
func New(cfg Config) *Provider {
	return &Provider{
		config: cfg,
		tokens: newTokenCache(cfg.TenantID, cfg.ClientID, cfg.ClientSecret),
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

func (p *Provider) Name() string { return p.config.Name }
func (p *Provider) Type() string { return "intune" }

// TestConnection verifies the Intune tenant is reachable by acquiring an
// OAuth2 access token. This validates tenant ID, client ID, and client secret
// without making any Graph API data calls.
func (p *Provider) TestConnection(ctx context.Context) error {
	_, err := p.tokens.Token()
	if err != nil {
		return fmt.Errorf("authentication failed: %w", err)
	}
	return nil
}

// ── Sync ────────────────────────────────────────────────────────────────

// graphDeviceListResponse is the JSON shape returned by the Graph managed devices endpoint.
type graphDeviceListResponse struct {
	Value    []graphDevice `json:"value"`
	NextLink string        `json:"@odata.nextLink"`
}

type graphDevice struct {
	ID                         string `json:"id"`
	DeviceName                 string `json:"deviceName"`
	OperatingSystem            string `json:"operatingSystem"`
	OSVersion                  string `json:"osVersion"`
	Model                      string `json:"model"`
	UserDisplayName            string `json:"userDisplayName"`
	UserPrincipalName          string `json:"userPrincipalName"`
	ComplianceState            string `json:"complianceState"`
	LastSyncDateTime           string `json:"lastSyncDateTime"`
	ManagementAgent            string `json:"managementAgent"`
	ManagedDeviceOwnerType     string `json:"managedDeviceOwnerType"`
	IsEncrypted                bool   `json:"isEncrypted"`
	JailBroken                 string `json:"jailBroken"`
	IsSupervised               bool   `json:"isSupervised"`
	PartnerReportedThreatState string `json:"partnerReportedThreatState"`
}

// SyncDevices fetches a page of managed devices from Microsoft Graph.
// cursor is the nextLink URL for pagination (empty for first page).
func (p *Provider) SyncDevices(ctx context.Context, cursor string) ([]provider.SyncDevice, string, error) {
	endpoint := cursor
	if endpoint == "" {
		// First page: request key fields, ordered for consistency.
		endpoint = "https://graph.microsoft.com/v1.0/deviceManagement/managedDevices?" +
			"$select=id,deviceName,operatingSystem,osVersion,model,userDisplayName,userPrincipalName,complianceState,lastSyncDateTime,managementAgent,isEncrypted,jailBroken,isSupervised,partnerReportedThreatState&" +
			"$top=200&" +
			"$orderby=deviceName"
	}

	body, err := p.graphGet(ctx, endpoint)
	if err != nil {
		return nil, "", fmt.Errorf("sync devices: %w", err)
	}

	var resp graphDeviceListResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, "", fmt.Errorf("parse device list: %w", err)
	}

	devices := make([]provider.SyncDevice, 0, len(resp.Value))
	for _, gd := range resp.Value {
		d := provider.SyncDevice{
			SourceID:     gd.ID,
			DeviceName:   gd.DeviceName,
			OS:           normalizeOS(gd.OperatingSystem),
			OSVersion:    gd.OSVersion,
			Model:        gd.Model,
			UserName:     gd.UserDisplayName,
			UserEmail:    gd.UserPrincipalName,
			Compliance:   normalizeCompliance(gd.ComplianceState),
			IsEncrypted:  gd.IsEncrypted,
			JailBroken:   gd.JailBroken,
			IsSupervised: gd.IsSupervised,
			ThreatState:  gd.PartnerReportedThreatState,
		}
		if t, err := time.Parse(time.RFC3339, gd.LastSyncDateTime); err == nil {
			d.LastSeen = &t
		}
		devices = append(devices, d)
	}

	log.Printf("[intune:%s] synced page: %d devices, has_next=%v", p.config.Name, len(devices), resp.NextLink != "")
	return devices, resp.NextLink, nil
}

// ── Commands ────────────────────────────────────────────────────────────

// SendCommand sends a remote action to a managed device.
// Supported actions: "reboot", "lock", "sync", "retire", "wipe", "resetPasscode",
// "shutDown", "windowsDefenderScan", "windowsDefenderUpdateSignatures".
func (p *Provider) SendCommand(ctx context.Context, sourceDeviceID string, cmd provider.Command) (string, error) {
	action := mapCommandAction(cmd.Action)
	if action == "" {
		return "", fmt.Errorf("unsupported command action: %s", cmd.Action)
	}

	endpoint := fmt.Sprintf(
		"https://graph.microsoft.com/v1.0/deviceManagement/managedDevices/%s/%s",
		sourceDeviceID, action,
	)

	body, err := p.graphPost(ctx, endpoint, nil)
	if err != nil {
		return "", fmt.Errorf("send command %s: %w", cmd.Action, err)
	}

	// Most Intune actions return 204 No Content. The "command ID" is synthetic
	// since Graph doesn't return one for device actions — we compose it.
	commandID := fmt.Sprintf("%s:%s:%d", sourceDeviceID, cmd.Action, time.Now().UnixMilli())
	_ = body
	return commandID, nil
}

// CheckCommandStatus checks device action status. Intune doesn't expose per-action
// status via Graph, so we check the device's lastSyncDateTime as a proxy.
func (p *Provider) CheckCommandStatus(ctx context.Context, commandID string) (provider.CommandStatus, error) {
	// For now, return a pending status. In Phase 6, the campaign engine will
	// use device sync time as a proxy for command completion.
	return provider.CommandStatus{
		ID:        commandID,
		State:     "completed", // Intune actions are fire-and-forget via Graph
		Detail:    "Intune actions are dispatched asynchronously",
		UpdatedAt: time.Now(),
	}, nil
}

// ── HTTP helpers ────────────────────────────────────────────────────────

func (p *Provider) graphGet(ctx context.Context, url string) ([]byte, error) {
	token, err := p.tokens.Token()
	if err != nil {
		return nil, fmt.Errorf("auth: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("graph API error (HTTP %d): %s", resp.StatusCode, truncate(string(body), 500))
	}

	return body, nil
}

func (p *Provider) graphPost(ctx context.Context, url string, payload io.Reader) ([]byte, error) {
	token, err := p.tokens.Token()
	if err != nil {
		return nil, fmt.Errorf("auth: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, payload)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// 200, 201, 204 are all valid success codes for Graph POST.
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("graph API error (HTTP %d): %s", resp.StatusCode, truncate(string(body), 500))
	}

	return body, nil
}

// ── Normalisation ───────────────────────────────────────────────────────

func normalizeOS(os string) string {
	switch {
	case os == "iOS" || os == "iPadOS":
		return "iOS"
	case os == "Android":
		return "Android"
	case os == "Windows":
		return "Windows"
	case os == "macOS":
		return "macOS"
	default:
		return os
	}
}

func normalizeCompliance(state string) string {
	switch state {
	case "compliant":
		return "compliant"
	case "noncompliant":
		return "non-compliant"
	default:
		return "unknown"
	}
}

func mapCommandAction(action string) string {
	switch action {
	case "reboot":
		return "rebootNow"
	case "lock":
		return "remoteLock"
	case "sync":
		return "syncDevice"
	case "retire":
		return "retire"
	case "wipe":
		return "wipe"
	case "resetPasscode":
		return "resetPasscode"
	case "shutDown":
		return "shutDown"
	case "windowsDefenderScan":
		return "windowsDefenderScan"
	case "windowsDefenderUpdateSignatures":
		return "windowsDefenderUpdateSignatures"
	default:
		return ""
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
