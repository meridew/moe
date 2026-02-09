package intune

// utcm_parse.go — Parse UTCM snapshot results into []provider.SyncPolicy.

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/dan/moe/internal/provider"
)

// SyncPoliciesUTCM captures an Intune configuration snapshot using the UTCM API,
// waits for completion, downloads the results, and maps them to SyncPolicy.
// Falls back to the legacy per-endpoint approach if UTCM fails.
func (p *Provider) SyncPoliciesUTCM(ctx context.Context, progress func(category string, count int)) ([]provider.SyncPolicy, error) {
	label := sanitiseSnapshotLabel(fmt.Sprintf("MOE %s %d", p.config.Name, nowUnixMilli()))
	total := 0

	// 1. Create snapshot job
	if progress != nil {
		progress("UTCM: creating snapshot", 0)
	}
	job, err := p.utcmCreateSnapshot(ctx, label)
	if err != nil {
		return nil, fmt.Errorf("UTCM create snapshot: %w", err)
	}
	jobID := job.ID

	// 2. Poll until completion
	waitStart := time.Now()
	job, err = p.utcmWaitForSnapshot(ctx, jobID, func(status string) {
		if progress != nil {
			elapsed := time.Since(waitStart).Round(time.Second)
			progress(fmt.Sprintf("UTCM: %s (%v)", status, elapsed), total)
		}
	})
	if err != nil {
		// Clean up the failed job
		_ = p.utcmDeleteSnapshotJob(context.Background(), jobID)
		return nil, fmt.Errorf("UTCM wait: %w", err)
	}

	// 3. Download results
	if progress != nil {
		progress("UTCM: downloading results", total)
	}
	result, err := p.utcmDownloadSnapshot(ctx, job.ResourceLocation)
	if err != nil {
		// Clean up
		_ = p.utcmDeleteSnapshotJob(context.Background(), job.ID)
		return nil, fmt.Errorf("UTCM download: %w", err)
	}

	// 4. Parse into SyncPolicy
	policies := utcmResultToSyncPolicies(result)
	total = len(policies)

	if progress != nil {
		progress("UTCM: parsing complete", total)
	}

	log.Printf("[utcm:%s] snapshot complete: %d policies from %d resource groups",
		p.config.Name, total, len(result.Resources))

	// 5. Clean up the snapshot job (they count towards the 12-job quota)
	go func() {
		_ = p.utcmDeleteSnapshotJob(context.Background(), job.ID)
	}()

	return policies, nil
}

// utcmResultToSyncPolicies converts downloaded UTCM snapshot results into
// normalised SyncPolicy structs for storage in MOE's database.
func utcmResultToSyncPolicies(result *utcmSnapshotResult) []provider.SyncPolicy {
	if result == nil {
		return nil
	}

	var policies []provider.SyncPolicy

	for _, group := range result.Resources {
		meta, ok := utcmResourceIndex[group.ResourceType]
		if !ok {
			// Try matching without the "microsoft.intune." prefix
			meta = utcmResource{
				ResourceType: group.ResourceType,
				Category:     guessUTCMCategory(group.ResourceType),
				Platform:     "",
			}
		}

		for _, instance := range group.Instances {
			sp := utcmInstanceToSyncPolicy(instance, meta)
			policies = append(policies, sp)
		}
	}

	// Sort by category then name for consistent output
	sort.Slice(policies, func(i, j int) bool {
		if policies[i].Category != policies[j].Category {
			return policies[i].Category < policies[j].Category
		}
		return policies[i].PolicyName < policies[j].PolicyName
	})

	return policies
}

// utcmInstanceToSyncPolicy maps a single UTCM resource instance to a SyncPolicy.
func utcmInstanceToSyncPolicy(instance map[string]interface{}, meta utcmResource) provider.SyncPolicy {
	sp := provider.SyncPolicy{
		Category: meta.Category,
		Platform: meta.Platform,
	}

	// Extract standard fields: DisplayName, Description, Identity, Id
	if v, ok := stringField(instance, "DisplayName"); ok {
		sp.PolicyName = v
	} else if v, ok := stringField(instance, "displayName"); ok {
		sp.PolicyName = v
	} else if v, ok := stringField(instance, "Name"); ok {
		sp.PolicyName = v
	}

	if v, ok := stringField(instance, "Description"); ok {
		sp.Description = v
	} else if v, ok := stringField(instance, "description"); ok {
		sp.Description = v
	}

	if v, ok := stringField(instance, "Identity"); ok {
		sp.SourceID = v
	} else if v, ok := stringField(instance, "Id"); ok {
		sp.SourceID = v
	} else if v, ok := stringField(instance, "id"); ok {
		sp.SourceID = v
	}

	// PolicyType = the UTCM resource type name (short form)
	sp.PolicyType = shortResourceType(meta.ResourceType)

	// Platform override: check if instance has a Platform field
	if v, ok := stringField(instance, "Platform"); ok {
		guessed := guessPlatformFromUTCM(v)
		if guessed != "" {
			sp.Platform = guessed
		}
	}
	if v, ok := stringField(instance, "Platforms"); ok {
		guessed := guessPlatformFromUTCM(v)
		if guessed != "" {
			sp.Platform = guessed
		}
	}

	// Build settings JSON: everything except the extracted fields
	sp.SettingsJSON = buildUTCMSettingsJSON(instance)

	return sp
}

// buildUTCMSettingsJSON serialises the instance properties as a clean JSON blob.
// Strips internal/meta fields that aren't useful for comparison.
func buildUTCMSettingsJSON(instance map[string]interface{}) string {
	// Copy, removing fields we've already extracted as top-level SyncPolicy fields
	clean := make(map[string]interface{}, len(instance))
	for k, v := range instance {
		// Skip internal fields
		switch k {
		case "@odata.type", "@odata.context", "Ensure":
			continue
		}
		clean[k] = v
	}

	data, err := json.MarshalIndent(clean, "", "  ")
	if err != nil {
		// Fallback: dump as-is
		raw, _ := json.Marshal(instance)
		return string(raw)
	}
	return string(data)
}

// shortResourceType strips the "microsoft.intune." prefix for display.
func shortResourceType(rt string) string {
	if after, found := strings.CutPrefix(rt, "microsoft.intune."); found {
		return after
	}
	return rt
}

// guessUTCMCategory infers a MOE category from an unknown resource type name.
func guessUTCMCategory(resourceType string) string {
	rt := strings.ToLower(resourceType)

	switch {
	case strings.Contains(rt, "compliance"):
		return "Compliance"
	case strings.Contains(rt, "deviceconfiguration"):
		return "Configuration Profiles"
	case strings.Contains(rt, "antivirus"), strings.Contains(rt, "attacksurface"),
		strings.Contains(rt, "endpointdetection"), strings.Contains(rt, "exploit"),
		strings.Contains(rt, "accountprotection"), strings.Contains(rt, "applicationcontrol"):
		return "Endpoint Security"
	case strings.Contains(rt, "appprotection"):
		return "App Protection"
	case strings.Contains(rt, "appconfiguration"):
		return "App Configuration"
	case strings.Contains(rt, "settingcatalog"):
		return "Settings Catalog"
	case strings.Contains(rt, "enrollment"):
		return "Enrollment"
	case strings.Contains(rt, "windowsupdate"):
		return "Windows Update"
	case strings.Contains(rt, "autopilot"):
		return "Autopilot"
	case strings.Contains(rt, "wifi"):
		return "Configuration Profiles"
	case strings.Contains(rt, "role"):
		return "Roles"
	default:
		return "Other"
	}
}

// guessPlatformFromUTCM maps UTCM platform strings to MOE's normalised form.
func guessPlatformFromUTCM(platform string) string {
	p := strings.ToLower(platform)
	switch {
	case strings.Contains(p, "windows"):
		return "Windows"
	case strings.Contains(p, "ios"):
		return "iOS"
	case strings.Contains(p, "macos"):
		return "macOS"
	case strings.Contains(p, "android"):
		return "Android"
	case strings.Contains(p, "linux"):
		return "Linux"
	case strings.Contains(p, "none"), strings.Contains(p, "all"):
		return "All"
	default:
		return ""
	}
}

// stringField safely extracts a string value from a map.
func stringField(m map[string]interface{}, key string) (string, bool) {
	v, ok := m[key]
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	if !ok || s == "" {
		return "", false
	}
	return s, true
}

// nowUnixMilli returns the current unix timestamp in milliseconds.
// Used to generate unique snapshot labels.
func nowUnixMilli() int64 {
	return time.Now().UnixMilli()
}

// sanitiseSnapshotLabel strips characters that UTCM rejects.
// Only alphabets, numbers, and spaces are allowed.
func sanitiseSnapshotLabel(s string) string {
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == ' ' {
			b.WriteRune(r)
		} else {
			b.WriteRune(' ')
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

// newDeleteRequest builds an authenticated DELETE request.
func newDeleteRequest(ctx context.Context, url, token string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, "DELETE", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	return req, nil
}
