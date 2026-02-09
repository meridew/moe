package intune

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"strings"

	"github.com/dan/moe/internal/provider"
)

// policyEndpoint defines a Graph API collection to fetch policies from.
type policyEndpoint struct {
	Category string // Display category for grouping
	Path     string // Graph API path (relative to deviceManagement/ unless FullPath set)
	FullPath string // If set, used as-is instead of deviceManagement/{Path}
	Beta     bool   // If true, use the beta endpoint instead of v1.0
	Settings bool   // If true, fetch /settings sub-resource per item (Settings Catalog)
}

// policyEndpoints is the list of Intune policy collection endpoints.
// Discovered via Graph $metadata NavigationProperty inspection on deviceManagement.
var policyEndpoints = []policyEndpoint{
	// ── Compliance ──
	{Category: "Compliance Policies", Path: "deviceCompliancePolicies"},
	{Category: "Compliance Policies (Settings Catalog)", Path: "compliancePolicies", Beta: true, Settings: true},
	{Category: "Compliance Scripts", Path: "deviceComplianceScripts", Beta: true},

	// ── Configuration ──
	{Category: "Configuration Profiles", Path: "deviceConfigurations"},
	{Category: "Settings Catalog", Path: "configurationPolicies", Beta: true, Settings: true},
	{Category: "Group Policy (Admin Templates)", Path: "groupPolicyConfigurations", Beta: true},

	// ── Endpoint Security ──
	{Category: "Endpoint Security", Path: "intents", Beta: true},
	{Category: "Security Baselines", Path: "templates", Beta: true},

	// ── App Protection ──
	{Category: "App Protection", FullPath: "deviceAppManagement/managedAppPolicies", Beta: true},

	// ── Scripts ──
	{Category: "PowerShell Scripts", Path: "deviceManagementScripts", Beta: true},
	{Category: "Shell Scripts (macOS)", Path: "deviceShellScripts", Beta: true},
	{Category: "Health Scripts (Remediations)", Path: "deviceHealthScripts", Beta: true},
	{Category: "Custom Attribute Scripts", Path: "deviceCustomAttributeShellScripts", Beta: true},

	// ── Enrollment ──
	{Category: "Enrollment Configurations", Path: "deviceEnrollmentConfigurations"},
	{Category: "Autopilot Profiles", Path: "windowsAutopilotDeploymentProfiles", Beta: true},

	// ── Updates ──
	{Category: "Windows Update Policies", Path: "windowsQualityUpdatePolicies", Beta: true},

	// ── Hardware ──
	{Category: "Hardware Configurations", Path: "hardwareConfigurations", Beta: true},

	// ── Reusable ──
	{Category: "Reusable Policy Settings", Path: "reusablePolicySettings", Beta: true},
}

// SyncPolicies implements provider.PolicyProvider. It first attempts to use the
// UTCM (Unified Tenant Configuration Management) APIs for a comprehensive
// snapshot, and falls back to the legacy per-endpoint approach if UTCM is
// unavailable (missing permissions, service principal not configured, etc.).
func (p *Provider) SyncPolicies(ctx context.Context, progress func(category string, count int)) ([]provider.SyncPolicy, error) {
	// Try UTCM first — broader coverage, single async operation
	policies, err := p.SyncPoliciesUTCM(ctx, progress)
	if err == nil && len(policies) > 0 {
		return policies, nil
	}
	if err != nil {
		log.Printf("[intune:%s] UTCM snapshot failed, falling back to legacy endpoints: %v", p.config.Name, err)
		if progress != nil {
			progress("UTCM unavailable — using legacy sync", 0)
		}
	}

	// Fall back to legacy per-endpoint approach
	return p.syncPoliciesLegacy(ctx, progress)
}

// syncPoliciesLegacy is the original per-endpoint approach: iterates through
// known Intune/Graph policy endpoints, fetches all items with pagination, and
// returns them as a flat slice of SyncPolicy.
func (p *Provider) syncPoliciesLegacy(ctx context.Context, progress func(category string, count int)) ([]provider.SyncPolicy, error) {
	var all []provider.SyncPolicy

	for _, ep := range policyEndpoints {
		items, err := p.fetchPolicyEndpoint(ctx, ep)
		if err != nil {
			// Log and continue — some endpoints may not be licensed or accessible
			log.Printf("[intune:%s] warning: could not fetch %s: %v", p.config.Name, ep.Path, err)
			continue
		}

		all = append(all, items...)

		if progress != nil {
			progress(ep.Category, len(all))
		}

		log.Printf("[intune:%s] fetched %s: %d items", p.config.Name, ep.Category, len(items))
	}

	return all, nil
}

// fetchPolicyEndpoint fetches all items from a single Graph policy collection,
// following @odata.nextLink for pagination.
func (p *Provider) fetchPolicyEndpoint(ctx context.Context, ep policyEndpoint) ([]provider.SyncPolicy, error) {
	apiVersion := "v1.0"
	if ep.Beta {
		apiVersion = "beta"
	}

	// Build the collection URL
	var url string
	if ep.FullPath != "" {
		url = fmt.Sprintf("https://graph.microsoft.com/%s/%s", apiVersion, ep.FullPath)
	} else {
		url = fmt.Sprintf("https://graph.microsoft.com/%s/deviceManagement/%s", apiVersion, ep.Path)
	}

	var policies []provider.SyncPolicy

	for url != "" {
		body, err := p.graphGet(ctx, url)
		if err != nil {
			return policies, fmt.Errorf("fetch %s: %w", ep.Path, err)
		}

		var resp graphCollectionResponse
		if err := json.Unmarshal(body, &resp); err != nil {
			return policies, fmt.Errorf("parse %s: %w", ep.Path, err)
		}

		for _, raw := range resp.Value {
			sp, err := parsePolicyItem(raw, ep.Category)
			if err != nil {
				log.Printf("[intune] warning: skipping item in %s: %v", ep.Path, err)
				continue
			}

			// For Settings Catalog policies, fetch the /settings sub-resource
			// which contains the actual configuration values.
			if ep.Settings && sp.SourceID != "" {
				settings, err := p.fetchPolicySettings(ctx, apiVersion, ep, sp.SourceID)
				if err != nil {
					log.Printf("[intune] warning: could not fetch settings for %s/%s: %v", ep.Path, sp.SourceID, err)
				} else if settings != "" {
					sp.SettingsJSON = mergeSettingsJSON(sp.SettingsJSON, settings)
				}
			}

			policies = append(policies, sp)
		}

		url = resp.NextLink
	}

	return policies, nil
}

// fetchPolicySettings fetches the /settings sub-resource for a Settings Catalog
// or Compliance v2 policy, which contains the actual configured values.
func (p *Provider) fetchPolicySettings(ctx context.Context, apiVersion string, ep policyEndpoint, policyID string) (string, error) {
	var base string
	if ep.FullPath != "" {
		base = fmt.Sprintf("https://graph.microsoft.com/%s/%s/%s/settings", apiVersion, ep.FullPath, policyID)
	} else {
		base = fmt.Sprintf("https://graph.microsoft.com/%s/deviceManagement/%s/%s/settings", apiVersion, ep.Path, policyID)
	}

	var allSettings []json.RawMessage
	url := base

	for url != "" {
		body, err := p.graphGet(ctx, url)
		if err != nil {
			return "", err
		}

		var resp graphCollectionResponse
		if err := json.Unmarshal(body, &resp); err != nil {
			return "", err
		}
		allSettings = append(allSettings, resp.Value...)
		url = resp.NextLink
	}

	if len(allSettings) == 0 {
		return "", nil
	}

	b, err := json.Marshal(allSettings)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// mergeSettingsJSON adds a "_settings" key into the existing settings JSON
// so the sub-fetched settings are stored alongside the policy metadata.
func mergeSettingsJSON(existingJSON, settingsJSON string) string {
	var m map[string]any
	if err := json.Unmarshal([]byte(existingJSON), &m); err != nil {
		return existingJSON
	}
	var s any
	if err := json.Unmarshal([]byte(settingsJSON), &s); err != nil {
		return existingJSON
	}
	m["_settings"] = s
	b, err := json.Marshal(m)
	if err != nil {
		return existingJSON
	}
	return string(b)
}

// ── Graph response parsing ──────────────────────────────────────────────

// graphCollectionResponse is a generic OData collection response.
type graphCollectionResponse struct {
	Value    []json.RawMessage `json:"value"`
	NextLink string            `json:"@odata.nextLink"`
}

// parsePolicyItem extracts a SyncPolicy from a raw Graph JSON object.
// It reads common fields (id, displayName, description, @odata.type) and stores
// the full JSON blob as settings.
func parsePolicyItem(raw json.RawMessage, category string) (provider.SyncPolicy, error) {
	// Parse the common fields we need for indexing
	var common struct {
		ID          string `json:"id"`
		DisplayName string `json:"displayName"`
		Description string `json:"description"`
		ODataType   string `json:"@odata.type"`
		// Alternative name fields used by different endpoints
		Name         string `json:"name"`
		Platforms    string `json:"platforms"`    // Settings Catalog: "windows10", "iOS", etc.
		PlatformType string `json:"platformType"` // Security Baselines/templates: "android", "iOS", etc.
	}
	if err := json.Unmarshal(raw, &common); err != nil {
		return provider.SyncPolicy{}, err
	}

	name := common.DisplayName
	if name == "" {
		name = common.Name
	}
	if name == "" {
		name = common.ID
	}

	// Extract platform — prefer the explicit platforms field (Settings Catalog),
	// then platformType (Security Baselines/templates),
	// fall back to OData type guessing
	platform := guessPlatformFromField(common.Platforms)
	if platform == "" {
		platform = guessPlatformFromField(common.PlatformType)
	}
	if platform == "" {
		platform = guessPlatform(common.ODataType, category)
	}

	// Build a clean settings JSON: parse the full object, remove
	// navigation-only fields, and store the flattened properties.
	settingsJSON := buildSettingsJSON(raw)

	return provider.SyncPolicy{
		Category:     category,
		SourceID:     common.ID,
		PolicyName:   name,
		PolicyType:   cleanODataType(common.ODataType),
		Platform:     platform,
		Description:  common.Description,
		SettingsJSON: settingsJSON,
	}, nil
}

// buildSettingsJSON takes a raw Graph policy object and produces a cleaned-up
// JSON string of its settings/properties, excluding OData metadata and
// navigation-property keys that are just IDs.
func buildSettingsJSON(raw json.RawMessage) string {
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return string(raw)
	}

	// Remove OData metadata and non-setting fields
	skipPrefixes := []string{"@odata", "@microsoft"}
	skipKeys := map[string]bool{
		"id": true, "createdDateTime": true, "lastModifiedDateTime": true,
		"version": true, "roleScopeTagIds": true, "creationSource": true,
	}

	clean := make(map[string]any)
	for k, v := range m {
		skip := false
		for _, prefix := range skipPrefixes {
			if strings.HasPrefix(k, prefix) {
				skip = true
				break
			}
		}
		if skip || skipKeys[k] {
			continue
		}
		clean[k] = v
	}

	b, err := json.Marshal(clean)
	if err != nil {
		return string(raw)
	}
	return string(b)
}

// guessPlatformFromField maps the explicit "platforms" enum field from
// Settings Catalog / Compliance v2 policies to a display name.
func guessPlatformFromField(platforms string) string {
	switch strings.ToLower(platforms) {
	case "windows10", "windows10x", "windows10andlater":
		return "Windows"
	case "ios":
		return "iOS"
	case "macos":
		return "macOS"
	case "android", "androidenterprise", "androidopensourceproject":
		return "Android"
	case "linux":
		return "Linux"
	default:
		return ""
	}
}

// guessPlatform attempts to extract the target platform from the OData type string.
func guessPlatform(odataType, category string) string {
	lower := strings.ToLower(odataType)
	switch {
	case strings.Contains(lower, "windows") || strings.Contains(lower, "win32"):
		return "Windows"
	case strings.Contains(lower, "ios") || strings.Contains(lower, "iphone"):
		return "iOS"
	case strings.Contains(lower, "macos") || strings.Contains(lower, "mac"):
		return "macOS"
	case strings.Contains(lower, "android"):
		return "Android"
	default:
		return ""
	}
}

// cleanODataType strips the namespace prefix from an OData type string.
// e.g. "#microsoft.graph.windowsCompliancePolicy" → "windowsCompliancePolicy"
func cleanODataType(t string) string {
	if t == "" {
		return ""
	}
	parts := strings.Split(t, ".")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return t
}

// ── Settings flattening for display ─────────────────────────────────────

// FlattenSettings takes a settings_json string and returns flattened key/value
// pairs suitable for display. Nested objects are rendered as JSON strings.
func FlattenSettings(settingsJSON string) []provider.SyncPolicySetting {
	var m map[string]any
	if err := json.Unmarshal([]byte(settingsJSON), &m); err != nil {
		return nil
	}

	var settings []provider.SyncPolicySetting
	for k, v := range m {
		settings = append(settings, provider.SyncPolicySetting{
			Name:  k,
			Value: formatValue(v),
		})
	}

	// Sort by key name for stable display
	sort.Slice(settings, func(i, j int) bool {
		return settings[i].Name < settings[j].Name
	})
	return settings
}

// formatValue converts a value to a display string.
func formatValue(v any) string {
	switch val := v.(type) {
	case nil:
		return ""
	case bool:
		if val {
			return "true"
		}
		return "false"
	case float64:
		if val == float64(int64(val)) {
			return fmt.Sprintf("%d", int64(val))
		}
		return fmt.Sprintf("%g", val)
	case string:
		return val
	default:
		b, _ := json.MarshalIndent(val, "", "  ")
		return string(b)
	}
}
