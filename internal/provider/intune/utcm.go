package intune

// UTCM — Unified Tenant Configuration Management integration.
//
// Uses the Microsoft Graph beta UTCM APIs to capture a full Intune configuration
// snapshot in a single async operation, replacing the legacy per-endpoint approach.
//
// Flow:
//   1. POST  /admin/configurationManagement/configurationSnapshots/createSnapshot
//   2. Poll  GET  /admin/configurationManagement/configurationSnapshotJobs/{id}
//   3. Download the snapshot file from the resourceLocation URL
//   4. Parse results into []SyncPolicy
//
// Requires permission: ConfigurationMonitoring.ReadWrite.All on the app registration,
// AND the UTCM service principal must be added to the tenant.
// See: https://learn.microsoft.com/en-us/graph/utcm-authentication-setup

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"
)

// ── UTCM resource definitions ───────────────────────────────────────────

// utcmResource maps a UTCM Intune resource type name to MOE category/platform.
type utcmResource struct {
	ResourceType string // e.g. "microsoft.intune.deviceCompliancePolicyWindows10"
	Category     string // MOE category: "Compliance", "Configuration Profiles", etc.
	Platform     string // "Windows", "iOS", "Android", "macOS", "All", ""
}

// utcmIntuneResources is the list of all Intune UTCM resource types we request
// in a snapshot. Derived from:
// https://learn.microsoft.com/en-us/graph/utcm-intune-resources
var utcmIntuneResources = []utcmResource{
	// ── Compliance ──
	{ResourceType: "microsoft.intune.deviceCompliancePolicyWindows10", Category: "Compliance", Platform: "Windows"},
	{ResourceType: "microsoft.intune.deviceCompliancePolicyAndroid", Category: "Compliance", Platform: "Android"},
	{ResourceType: "microsoft.intune.deviceCompliancePolicyAndroidDeviceOwner", Category: "Compliance", Platform: "Android"},
	{ResourceType: "microsoft.intune.deviceCompliancePolicyAndroidWorkProfile", Category: "Compliance", Platform: "Android"},
	{ResourceType: "microsoft.intune.deviceCompliancePolicymacOS", Category: "Compliance", Platform: "macOS"},
	{ResourceType: "microsoft.intune.deviceCompliancePolicyiOS", Category: "Compliance", Platform: "iOS"},

	// ── Configuration Profiles ──
	{ResourceType: "microsoft.intune.deviceConfigurationPolicyWindows10", Category: "Configuration Profiles", Platform: "Windows"},
	{ResourceType: "microsoft.intune.deviceConfigurationPolicyAndroidDeviceOwner", Category: "Configuration Profiles", Platform: "Android"},
	{ResourceType: "microsoft.intune.deviceConfigurationPolicyAndroidWorkProfile", Category: "Configuration Profiles", Platform: "Android"},
	{ResourceType: "microsoft.intune.deviceConfigurationPolicyAndroidOpenSourceProject", Category: "Configuration Profiles", Platform: "Android"},
	{ResourceType: "microsoft.intune.deviceConfigurationPolicymacOS", Category: "Configuration Profiles", Platform: "macOS"},
	{ResourceType: "microsoft.intune.deviceConfigurationPolicyiOS", Category: "Configuration Profiles", Platform: "iOS"},
	{ResourceType: "microsoft.intune.deviceConfigurationPolicyAndroidDeviceAdministrator", Category: "Configuration Profiles", Platform: "Android"},
	{ResourceType: "microsoft.intune.deviceConfigurationAdministrativeTemplatePolicyWindows10", Category: "Configuration Profiles", Platform: "Windows"},
	{ResourceType: "microsoft.intune.deviceConfigurationCustomPolicyWindows10", Category: "Configuration Profiles", Platform: "Windows"},
	{ResourceType: "microsoft.intune.deviceConfigurationEndpointProtectionPolicyWindows10", Category: "Configuration Profiles", Platform: "Windows"},
	{ResourceType: "microsoft.intune.deviceConfigurationIdentityProtectionPolicyWindows10", Category: "Configuration Profiles", Platform: "Windows"},
	{ResourceType: "microsoft.intune.deviceConfigurationDeliveryOptimizationPolicyWindows10", Category: "Configuration Profiles", Platform: "Windows"},
	{ResourceType: "microsoft.intune.deviceConfigurationFirmwareInterfacePolicyWindows10", Category: "Configuration Profiles", Platform: "Windows"},
	{ResourceType: "microsoft.intune.deviceConfigurationDomainJoinPolicyWindows10", Category: "Configuration Profiles", Platform: "Windows"},
	{ResourceType: "microsoft.intune.deviceConfigurationHealthMonitoringConfigurationPolicyWindows10", Category: "Configuration Profiles", Platform: "Windows"},
	{ResourceType: "microsoft.intune.deviceConfigurationKioskPolicyWindows10", Category: "Configuration Profiles", Platform: "Windows"},
	{ResourceType: "microsoft.intune.deviceConfigurationNetworkBoundaryPolicyWindows10", Category: "Configuration Profiles", Platform: "Windows"},
	{ResourceType: "microsoft.intune.deviceConfigurationSecureAssessmentPolicyWindows10", Category: "Configuration Profiles", Platform: "Windows"},
	{ResourceType: "microsoft.intune.deviceConfigurationSharedMultiDevicePolicyWindows10", Category: "Configuration Profiles", Platform: "Windows"},
	{ResourceType: "microsoft.intune.deviceConfigurationWindowsTeamPolicyWindows10", Category: "Configuration Profiles", Platform: "Windows"},
	{ResourceType: "microsoft.intune.deviceConfigurationDefenderForEndpointOnboardingPolicyWindows10", Category: "Configuration Profiles", Platform: "Windows"},
	{ResourceType: "microsoft.intune.deviceConfigurationEmailProfilePolicyWindows10", Category: "Configuration Profiles", Platform: "Windows"},
	{ResourceType: "microsoft.intune.deviceConfigurationVpnPolicyWindows10", Category: "Configuration Profiles", Platform: "Windows"},
	{ResourceType: "microsoft.intune.deviceConfigurationWiredNetworkPolicyWindows10", Category: "Configuration Profiles", Platform: "Windows"},

	// ── Certificates ──
	{ResourceType: "microsoft.intune.deviceConfigurationTrustedCertificatePolicyWindows10", Category: "Configuration Profiles", Platform: "Windows"},
	{ResourceType: "microsoft.intune.deviceConfigurationPkcsCertificatePolicyWindows10", Category: "Configuration Profiles", Platform: "Windows"},
	{ResourceType: "microsoft.intune.deviceConfigurationScepCertificatePolicyWindows10", Category: "Configuration Profiles", Platform: "Windows"},
	{ResourceType: "microsoft.intune.deviceConfigurationImportedPfxCertificatePolicyWindows10", Category: "Configuration Profiles", Platform: "Windows"},

	// ── Endpoint Security ──
	{ResourceType: "microsoft.intune.antivirusPolicyWindows10SettingCatalog", Category: "Endpoint Security", Platform: "Windows"},
	{ResourceType: "microsoft.intune.attackSurfaceReductionRulesPolicyWindows10ConfigManager", Category: "Endpoint Security", Platform: "Windows"},
	{ResourceType: "microsoft.intune.settingCatalogAsrRulesPolicyWindows10", Category: "Endpoint Security", Platform: "Windows"},
	{ResourceType: "microsoft.intune.endpointDetectionAndResponsePolicyWindows10", Category: "Endpoint Security", Platform: "Windows"},
	{ResourceType: "microsoft.intune.exploitProtectionPolicyWindows10SettingCatalog", Category: "Endpoint Security", Platform: "Windows"},
	{ResourceType: "microsoft.intune.accountProtectionPolicy", Category: "Endpoint Security", Platform: "Windows"},
	{ResourceType: "microsoft.intune.accountProtectionLocalUserGroupMembershipPolicy", Category: "Endpoint Security", Platform: "Windows"},

	// ── App Protection ──
	{ResourceType: "microsoft.intune.appProtectionPolicyAndroid", Category: "App Protection", Platform: "Android"},
	{ResourceType: "microsoft.intune.appProtectionPolicyiOS", Category: "App Protection", Platform: "iOS"},

	// ── App Configuration ──
	{ResourceType: "microsoft.intune.appConfigurationPolicy", Category: "App Configuration", Platform: "All"},

	// ── Settings Catalog ──
	{ResourceType: "microsoft.intune.settingCatalogCustomPolicyWindows10", Category: "Settings Catalog", Platform: "Windows"},

	// ── Enrollment ──
	{ResourceType: "microsoft.intune.deviceEnrollmentPlatformRestriction", Category: "Enrollment", Platform: "All"},
	{ResourceType: "microsoft.intune.deviceEnrollmentStatusPageWindows10", Category: "Enrollment", Platform: "Windows"},

	// ── Windows Update ──
	{ResourceType: "microsoft.intune.windowsUpdateForBusinessFeatureUpdateProfileWindows10", Category: "Windows Update", Platform: "Windows"},
	{ResourceType: "microsoft.intune.windowsUpdateForBusinessRingUpdateProfileWindows10", Category: "Windows Update", Platform: "Windows"},

	// ── Autopilot ──
	{ResourceType: "microsoft.intune.windowsAutopilotDeploymentProfileAzureADJoined", Category: "Autopilot", Platform: "Windows"},
	{ResourceType: "microsoft.intune.windowsAutopilotDeploymentProfileAzureADHybridJoined", Category: "Autopilot", Platform: "Windows"},

	// ── Windows Information Protection ──
	{ResourceType: "microsoft.intune.windowsInformationProtectionPolicyWindows10MdmEnrolled", Category: "Information Protection", Platform: "Windows"},

	// ── WiFi ──
	{ResourceType: "microsoft.intune.wifiConfigurationPolicyWindows10", Category: "Configuration Profiles", Platform: "Windows"},
	{ResourceType: "microsoft.intune.wifiConfigurationPolicymacOS", Category: "Configuration Profiles", Platform: "macOS"},
	{ResourceType: "microsoft.intune.wifiConfigurationPolicyAndroidDeviceAdministrator", Category: "Configuration Profiles", Platform: "Android"},
	{ResourceType: "microsoft.intune.wifiConfigurationPolicyAndroidEnterpriseDeviceOwner", Category: "Configuration Profiles", Platform: "Android"},
	{ResourceType: "microsoft.intune.wifiConfigurationPolicyAndroidEnterpriseWorkProfile", Category: "Configuration Profiles", Platform: "Android"},
	{ResourceType: "microsoft.intune.wifiConfigurationPolicyAndroidForWork", Category: "Configuration Profiles", Platform: "Android"},
	{ResourceType: "microsoft.intune.wifiConfigurationPolicyAndroidOpenSourceProject", Category: "Configuration Profiles", Platform: "Android"},

	// ── Roles ──
	{ResourceType: "microsoft.intune.roleDefinition", Category: "Roles", Platform: ""},
	{ResourceType: "microsoft.intune.roleAssignment", Category: "Roles", Platform: ""},

	// ── Policy Sets ──
	{ResourceType: "microsoft.intune.policySets", Category: "Policy Sets", Platform: "All"},

	// ── Filters ──
	{ResourceType: "microsoft.intune.deviceAndAppManagementAssignmentFilter", Category: "Assignment Filters", Platform: "All"},
	{ResourceType: "microsoft.intune.deviceCategory", Category: "Device Categories", Platform: "All"},

	// ── Application Control ──
	{ResourceType: "microsoft.intune.applicationControlPolicyWindows10", Category: "Endpoint Security", Platform: "Windows"},
}

// utcmResourceIndex keyed by resource type name for quick lookup.
var utcmResourceIndex map[string]utcmResource

func init() {
	utcmResourceIndex = make(map[string]utcmResource, len(utcmIntuneResources))
	for _, r := range utcmIntuneResources {
		utcmResourceIndex[r.ResourceType] = r
	}
}

// allUTCMResourceNames returns the resource type strings for the snapshot request.
func allUTCMResourceNames() []string {
	names := make([]string, len(utcmIntuneResources))
	for i, r := range utcmIntuneResources {
		names[i] = r.ResourceType
	}
	return names
}

// ── UTCM API types ──────────────────────────────────────────────────────

const (
	utcmBaseURL           = "https://graph.microsoft.com/beta/admin/configurationManagement"
	utcmCreateSnapshotURL = utcmBaseURL + "/configurationSnapshots/createSnapshot"
)

// utcmSnapshotRequest is the POST body for createSnapshot.
type utcmSnapshotRequest struct {
	DisplayName string   `json:"displayName"`
	Description string   `json:"description,omitempty"`
	Resources   []string `json:"resources"`
}

// utcmSnapshotJob mirrors the configurationSnapshotJob resource type.
type utcmSnapshotJob struct {
	ID                string    `json:"id"`
	DisplayName       string    `json:"displayName"`
	Description       string    `json:"description"`
	TenantID          string    `json:"tenantId"`
	Status            string    `json:"status"` // notStarted, running, succeeded, failed, partiallySuccessful
	Resources         []string  `json:"resources"`
	CreatedDateTime   time.Time `json:"createdDateTime"`
	CompletedDateTime time.Time `json:"completedDateTime"`
	ResourceLocation  string    `json:"resourceLocation"`
	ErrorDetails      []string  `json:"errorDetails"`
}

// utcmSnapshotResult represents the downloaded snapshot content.
// The exact structure is inferred from the UTCM resource schema.
// Each resource type produces instances with key/value properties.
type utcmSnapshotResult struct {
	Resources []utcmSnapshotResourceGroup `json:"resources"`
}

// utcmSnapshotResourceGroup is a set of instances for a single resource type
// within a snapshot download.
type utcmSnapshotResourceGroup struct {
	ResourceType string                   `json:"resourceType"`
	Instances    []map[string]interface{} `json:"instances"`
}

// ── UTCM API methods on Provider ────────────────────────────────────────

// utcmCreateSnapshot submits a snapshot job to the UTCM API.
func (p *Provider) utcmCreateSnapshot(ctx context.Context, label string) (*utcmSnapshotJob, error) {
	reqBody := utcmSnapshotRequest{
		DisplayName: label,
		Description: fmt.Sprintf("MOE snapshot: %s", label),
		Resources:   allUTCMResourceNames(),
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal snapshot request: %w", err)
	}

	respBytes, err := p.graphPost(ctx, utcmCreateSnapshotURL, strings.NewReader(string(bodyBytes)))
	if err != nil {
		return nil, fmt.Errorf("create snapshot job: %w", err)
	}

	var job utcmSnapshotJob
	if err := json.Unmarshal(respBytes, &job); err != nil {
		return nil, fmt.Errorf("parse snapshot job response: %w", err)
	}

	log.Printf("[utcm:%s] snapshot job created: id=%s status=%s resources=%d",
		p.config.Name, job.ID, job.Status, len(job.Resources))
	return &job, nil
}

// utcmGetSnapshotJob retrieves the current state of a snapshot job.
func (p *Provider) utcmGetSnapshotJob(ctx context.Context, jobID string) (*utcmSnapshotJob, error) {
	url := fmt.Sprintf("%s/configurationSnapshotJobs/%s", utcmBaseURL, jobID)

	respBytes, err := p.graphGet(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("get snapshot job: %w", err)
	}

	var job utcmSnapshotJob
	if err := json.Unmarshal(respBytes, &job); err != nil {
		return nil, fmt.Errorf("parse snapshot job: %w", err)
	}

	return &job, nil
}

// utcmWaitForSnapshot polls a snapshot job until it completes or context expires.
// Returns the completed job with resourceLocation populated.
func (p *Provider) utcmWaitForSnapshot(ctx context.Context, jobID string, progress func(status string)) (*utcmSnapshotJob, error) {
	const pollInterval = 5 * time.Second
	const maxWait = 10 * time.Minute

	deadline := time.Now().Add(maxWait)
	for {
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("snapshot job timed out after %v", maxWait)
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(pollInterval):
		}

		job, err := p.utcmGetSnapshotJob(ctx, jobID)
		if err != nil {
			return nil, err
		}

		if progress != nil {
			progress(job.Status)
		}

		switch job.Status {
		case "succeeded", "partiallySuccessful":
			log.Printf("[utcm:%s] snapshot completed: status=%s", p.config.Name, job.Status)
			if len(job.ErrorDetails) > 0 {
				log.Printf("[utcm:%s] snapshot warnings: %v", p.config.Name, job.ErrorDetails)
			}
			return job, nil
		case "failed":
			errMsg := "snapshot job failed"
			if len(job.ErrorDetails) > 0 {
				errMsg = fmt.Sprintf("snapshot job failed: %s", strings.Join(job.ErrorDetails, "; "))
			}
			return nil, fmt.Errorf(errMsg)
		case "notStarted", "running":
			log.Printf("[utcm:%s] snapshot in progress: status=%s", p.config.Name, job.Status)
			continue
		default:
			log.Printf("[utcm:%s] unknown snapshot status: %s", p.config.Name, job.Status)
			continue
		}
	}
}

// utcmDownloadSnapshot downloads and parses the snapshot results from the
// resourceLocation URL.
func (p *Provider) utcmDownloadSnapshot(ctx context.Context, resourceLocation string) (*utcmSnapshotResult, error) {
	if resourceLocation == "" {
		return nil, fmt.Errorf("empty resource location — snapshot may not have produced results")
	}

	respBytes, err := p.graphGet(ctx, resourceLocation)
	if err != nil {
		return nil, fmt.Errorf("download snapshot: %w", err)
	}

	// Try parsing as our expected structure first.
	var result utcmSnapshotResult
	if err := json.Unmarshal(respBytes, &result); err != nil {
		// If that fails, try parsing as a raw JSON object/array and wrap it.
		result, parseErr := parseRawSnapshotJSON(respBytes)
		if parseErr != nil {
			return nil, fmt.Errorf("parse snapshot (tried structured + raw): structured=%w, raw=%v", err, parseErr)
		}
		return result, nil
	}

	return &result, nil
}

// parseRawSnapshotJSON attempts to interpret an unknown snapshot format.
// UTCM may return the data in different shapes depending on API version.
func parseRawSnapshotJSON(data []byte) (*utcmSnapshotResult, error) {
	// Attempt 1: top-level array of resource groups
	var groups []utcmSnapshotResourceGroup
	if err := json.Unmarshal(data, &groups); err == nil && len(groups) > 0 {
		return &utcmSnapshotResult{Resources: groups}, nil
	}

	// Attempt 2: map of resource type → instances
	var resourceMap map[string][]map[string]interface{}
	if err := json.Unmarshal(data, &resourceMap); err == nil && len(resourceMap) > 0 {
		result := &utcmSnapshotResult{}
		for resType, instances := range resourceMap {
			result.Resources = append(result.Resources, utcmSnapshotResourceGroup{
				ResourceType: resType,
				Instances:    instances,
			})
		}
		return result, nil
	}

	// Attempt 3: single object with a 'value' array (Graph collection pattern)
	var collectionResp struct {
		Value []map[string]interface{} `json:"value"`
	}
	if err := json.Unmarshal(data, &collectionResp); err == nil && len(collectionResp.Value) > 0 {
		// Group by @odata.type or resourceType field if present
		grouped := make(map[string][]map[string]interface{})
		for _, item := range collectionResp.Value {
			resType := "unknown"
			if rt, ok := item["resourceType"].(string); ok {
				resType = rt
			} else if rt, ok := item["@odata.type"].(string); ok {
				resType = rt
			}
			grouped[resType] = append(grouped[resType], item)
		}
		result := &utcmSnapshotResult{}
		for resType, instances := range grouped {
			result.Resources = append(result.Resources, utcmSnapshotResourceGroup{
				ResourceType: resType,
				Instances:    instances,
			})
		}
		return result, nil
	}

	return nil, fmt.Errorf("unrecognised snapshot format (len=%d)", len(data))
}

// utcmDeleteSnapshotJob deletes a completed snapshot job to free up quota.
// UTCM allows max 12 visible snapshot jobs.
func (p *Provider) utcmDeleteSnapshotJob(ctx context.Context, jobID string) error {
	url := fmt.Sprintf("%s/configurationSnapshotJobs/%s", utcmBaseURL, jobID)

	token, err := p.tokens.Token()
	if err != nil {
		return fmt.Errorf("auth: %w", err)
	}

	req, err := newDeleteRequest(ctx, url, token)
	if err != nil {
		return err
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("delete snapshot job: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return fmt.Errorf("delete snapshot job: HTTP %d", resp.StatusCode)
	}

	log.Printf("[utcm:%s] deleted snapshot job %s", p.config.Name, jobID)
	return nil
}
