package server

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sort"
	"time"

	"github.com/dan/moe/internal/models"
	"github.com/dan/moe/internal/provider"
	"github.com/dan/moe/internal/provider/intune"
)

// ── Template data ───────────────────────────────────────────────────────

// PolicySnapshotSummary is the list-level view of a snapshot (no full policy data).
type PolicySnapshotSummary struct {
	ID            string
	ProviderName  string
	ProviderType  string
	Label         string
	DisplayName   string // Label if set, otherwise ProviderName
	TakenAt       time.Time
	PolicyCount   int
	CategoryCount int
}

// PolicySetting is a single key/value setting within a policy.
type PolicySetting struct {
	Name  string `json:"Name"`
	Value string `json:"Value"`
}

// PolicyItem represents one policy within a snapshot.
type PolicyItem struct {
	ID           string          `json:"ID"`
	Category     string          `json:"Category"`
	PolicyName   string          `json:"PolicyName"`
	PolicyType   string          `json:"PolicyType"`
	Platform     string          `json:"Platform"`
	Description  string          `json:"Description"`
	SettingCount int             `json:"SettingCount"`
	Settings     []PolicySetting `json:"Settings"`
}

// PolicyCategoryGroup is a set of policies grouped by category for display.
type PolicyCategoryGroup struct {
	Category string
	Policies []PolicyItem
}

// policiesPageData is the data for the /policies list page.
type policiesPageData struct {
	Nav       string
	Providers []models.ProviderConfig
	Snapshots []PolicySnapshotSummary
}

// policySnapshotPageData is the data for the /policies/snapshots/{id} detail page.
type policySnapshotPageData struct {
	Nav          string
	Snapshot     PolicySnapshotSummary
	Categories   []string
	Platforms    []string
	Items        []PolicyItem
	GroupedItems []PolicyCategoryGroup
}

// CompareStats holds summary counts for a comparison.
type CompareStats struct {
	Matching  int `json:"Matching"`
	Different int `json:"Different"`
	LeftOnly  int `json:"LeftOnly"`
	RightOnly int `json:"RightOnly"`
}

// SettingDiff represents one setting row in a side-by-side comparison.
type SettingDiff struct {
	Name       string `json:"Name"`
	LeftValue  string `json:"LeftValue"`
	RightValue string `json:"RightValue"`
	Changed    bool   `json:"Changed"`
}

// PolicyDiff represents one policy's comparison result.
type PolicyDiff struct {
	PolicyName   string          `json:"PolicyName"`
	Category     string          `json:"Category"`
	Platform     string          `json:"Platform"`
	Status       string          `json:"Status"`
	SettingDiffs []SettingDiff   `json:"SettingDiffs"`
	Settings     []PolicySetting `json:"Settings"`
}

// policyComparePageData is the data for the /policies/compare page.
type policyComparePageData struct {
	Nav        string
	Snapshots  []PolicySnapshotSummary
	LeftID     string
	RightID    string
	LeftName   string
	RightName  string
	HasResults bool
	Stats      CompareStats
	Diffs      []PolicyDiff
	Platforms  []string // distinct platforms across all diffs
	Categories []string // distinct categories across all diffs
	TotalCount int      // total policy count (for alignment %)
}

// ── Handlers ────────────────────────────────────────────────────────────

// handlePolicies serves the main policies page with snapshot list.
func (s *Server) handlePolicies(w http.ResponseWriter, r *http.Request) {
	providers, _ := s.providerConfigs.ListAll()

	snapshots, err := s.policies.ListSnapshots()
	if err != nil {
		log.Printf("[policies] list snapshots error: %v", err)
	}

	summaries := make([]PolicySnapshotSummary, len(snapshots))
	for i, snap := range snapshots {
		summaries[i] = snapshotToSummary(snap)
	}

	s.render.render(w, "policies.html", policiesPageData{
		Nav:       "policies",
		Providers: providers,
		Snapshots: summaries,
	})
}

// handlePolicySnapshot serves the snapshot detail/browse page.
func (s *Server) handlePolicySnapshot(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	snap, err := s.policies.GetSnapshot(id)
	if err != nil {
		log.Printf("[policies] get snapshot error: %v", err)
		http.Redirect(w, r, "/policies?flash=Error+loading+snapshot&flash_type=error", http.StatusSeeOther)
		return
	}
	if snap == nil {
		http.Redirect(w, r, "/policies?flash=Snapshot+not+found&flash_type=error", http.StatusSeeOther)
		return
	}

	categories, err := s.policies.DistinctCategories(id)
	if err != nil {
		log.Printf("[policies] categories error: %v", err)
	}
	items, err := s.policies.ListItems(id, "", "")
	if err != nil {
		log.Printf("[policies] list items error: %v", err)
	}

	viewItems, grouped := buildPolicyView(items)

	// Extract unique platforms for tabs
	platSet := map[string]bool{}
	for _, item := range items {
		p := item.Platform
		if p == "" {
			p = "Other"
		}
		platSet[p] = true
	}
	platforms := make([]string, 0, len(platSet))
	for p := range platSet {
		platforms = append(platforms, p)
	}
	sort.Strings(platforms)

	s.render.render(w, "policy_snapshot.html", policySnapshotPageData{
		Nav:          "policies",
		Snapshot:     snapshotToSummary(*snap),
		Categories:   categories,
		Platforms:    platforms,
		Items:        viewItems,
		GroupedItems: grouped,
	})
}

// handlePolicySnapshotCreate takes a new policy snapshot from a provider.
func (s *Server) handlePolicySnapshotCreate(w http.ResponseWriter, r *http.Request) {
	providerID := r.FormValue("provider_id")
	label := r.FormValue("label")
	if providerID == "" {
		http.Redirect(w, r, "/policies?flash=Select+a+provider&flash_type=error", http.StatusSeeOther)
		return
	}

	cfg, err := s.providerConfigs.GetByID(providerID)
	if err != nil || cfg == nil {
		http.Redirect(w, r, "/policies?flash=Provider+not+found&flash_type=error", http.StatusSeeOther)
		return
	}

	// Build the provider
	p, err := s.buildProvider(cfg)
	if err != nil {
		s.activity.Logf(cfg.Name, "error", "Policy snapshot failed — could not init provider: %s", err)
		http.Redirect(w, r, "/policies?flash=Failed+to+init+provider&flash_type=error", http.StatusSeeOther)
		return
	}

	// Check if provider supports policies
	pp, ok := p.(provider.PolicyProvider)
	if !ok {
		http.Redirect(w, r, "/policies?flash=Provider+does+not+support+policy+sync&flash_type=error", http.StatusSeeOther)
		return
	}

	s.activity.Logf(cfg.Name, "info", "Policy snapshot started…")

	// Create the snapshot record
	snapshotID := newID()
	snap := &models.PolicySnapshot{
		ID:           snapshotID,
		ProviderName: cfg.Name,
		ProviderType: cfg.Type,
		Label:        label,
		TakenAt:      time.Now().UTC(),
	}
	if err := s.policies.CreateSnapshot(snap); err != nil {
		log.Printf("[policies] create snapshot error: %v", err)
		http.Redirect(w, r, "/policies?flash=Failed+to+create+snapshot&flash_type=error", http.StatusSeeOther)
		return
	}

	// Fetch all policies from provider
	syncPolicies, err := pp.SyncPolicies(r.Context(), func(category string, count int) {
		s.activity.Logf(cfg.Name, "info", "Policy snapshot: fetched %s (%d total so far)", category, count)
	})
	if err != nil {
		log.Printf("[policies] sync error for %s: %v", cfg.Name, err)
		s.activity.Logf(cfg.Name, "error", "Policy snapshot error: %s", err)
		// Clean up the snapshot since we failed
		_ = s.policies.DeleteSnapshot(snapshotID)
		http.Redirect(w, r, "/policies?flash=Snapshot+failed&flash_type=error", http.StatusSeeOther)
		return
	}

	// Store all policy items
	for _, sp := range syncPolicies {
		item := &models.PolicyItem{
			ID:           newID(),
			SnapshotID:   snapshotID,
			Category:     sp.Category,
			SourceID:     sp.SourceID,
			PolicyName:   sp.PolicyName,
			PolicyType:   sp.PolicyType,
			Platform:     sp.Platform,
			Description:  sp.Description,
			SettingsJSON: sp.SettingsJSON,
		}
		if err := s.policies.InsertItem(item); err != nil {
			log.Printf("[policies] insert item error: %v", err)
		}
	}

	// Update denormalised counts
	_ = s.policies.UpdateSnapshotCounts(snapshotID)

	// Keep only 10 snapshots per provider
	_ = s.policies.DeleteOldSnapshots(10)

	s.activity.Logf(cfg.Name, "success", "Policy snapshot complete — %d policies captured", len(syncPolicies))
	http.Redirect(w, r, fmt.Sprintf("/policies?flash=Snapshot+taken%%3A+%d+policies+from+%s&flash_type=success", len(syncPolicies), cfg.Name), http.StatusSeeOther)
}

// handlePolicySnapshotDelete deletes a snapshot.
func (s *Server) handlePolicySnapshotDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	// Look up snapshot name for logging before we delete it
	snap, _ := s.policies.GetSnapshot(id)
	snapshotLabel := id
	if snap != nil {
		snapshotLabel = snap.ProviderName
	}

	if err := s.policies.DeleteSnapshot(id); err != nil {
		log.Printf("[policies] delete snapshot error: %v", err)
		s.activity.Logf(snapshotLabel, "error", "Failed to delete policy snapshot: %s", err)
		http.Redirect(w, r, "/policies?flash=Delete+failed&flash_type=error", http.StatusSeeOther)
		return
	}

	log.Printf("[policies] deleted snapshot %s (%s)", id, snapshotLabel)
	s.activity.Logf(snapshotLabel, "info", "Policy snapshot deleted")
	http.Redirect(w, r, "/policies?flash=Snapshot+deleted&flash_type=success", http.StatusSeeOther)
}

// handlePolicyCompare serves the compare page with side-by-side diff.
func (s *Server) handlePolicyCompare(w http.ResponseWriter, r *http.Request) {
	leftID := r.URL.Query().Get("left")
	rightID := r.URL.Query().Get("right")

	// Load all snapshots for the picker dropdowns
	snapshots, _ := s.policies.ListSnapshots()
	summaries := make([]PolicySnapshotSummary, len(snapshots))
	for i, snap := range snapshots {
		summaries[i] = snapshotToSummary(snap)
	}

	data := policyComparePageData{
		Nav:       "policies",
		Snapshots: summaries,
		LeftID:    leftID,
		RightID:   rightID,
	}

	// Only compute results if both snapshots selected
	if leftID != "" && rightID != "" {
		leftSnap, _ := s.policies.GetSnapshot(leftID)
		rightSnap, _ := s.policies.GetSnapshot(rightID)
		if leftSnap != nil && rightSnap != nil {
			data.HasResults = true
			data.LeftName = leftSnap.ProviderName
			data.RightName = rightSnap.ProviderName

			leftItems, _ := s.policies.ListItems(leftID, "", "")
			rightItems, _ := s.policies.ListItems(rightID, "", "")

			// Always pass ALL diffs — client-side Alpine handles filtering
			data.Stats, data.Diffs = computeDiff(leftItems, rightItems, "")
			data.TotalCount = data.Stats.Matching + data.Stats.Different + data.Stats.LeftOnly + data.Stats.RightOnly
			data.Platforms, data.Categories = extractDimensions(data.Diffs)
		}
	}

	s.render.render(w, "policy_compare.html", data)
}

// ── Helpers ─────────────────────────────────────────────────────────────

// snapshotToSummary converts a DB model to a template view model.
func snapshotToSummary(snap models.PolicySnapshot) PolicySnapshotSummary {
	return PolicySnapshotSummary{
		ID:            snap.ID,
		ProviderName:  snap.ProviderName,
		ProviderType:  snap.ProviderType,
		Label:         snap.Label,
		DisplayName:   snap.DisplayName(),
		TakenAt:       snap.TakenAt,
		PolicyCount:   snap.PolicyCount,
		CategoryCount: snap.CategoryCount,
	}
}

// buildPolicyView converts DB models into view models with flattened settings.
func buildPolicyView(items []models.PolicyItem) ([]PolicyItem, []PolicyCategoryGroup) {
	viewItems := make([]PolicyItem, len(items))
	grouped := map[string][]PolicyItem{}

	for i, item := range items {
		settings := intune.FlattenSettings(item.SettingsJSON)
		policySettings := make([]PolicySetting, len(settings))
		for j, s := range settings {
			policySettings[j] = PolicySetting{Name: s.Name, Value: s.Value}
		}

		vi := PolicyItem{
			ID:           item.ID,
			Category:     item.Category,
			PolicyName:   item.PolicyName,
			PolicyType:   item.PolicyType,
			Platform:     item.Platform,
			Description:  item.Description,
			SettingCount: len(policySettings),
			Settings:     policySettings,
		}
		viewItems[i] = vi
		grouped[item.Category] = append(grouped[item.Category], vi)
	}

	// Sort categories
	var cats []string
	for c := range grouped {
		cats = append(cats, c)
	}
	sort.Strings(cats)

	groups := make([]PolicyCategoryGroup, len(cats))
	for i, c := range cats {
		groups[i] = PolicyCategoryGroup{
			Category: c,
			Policies: grouped[c],
		}
	}

	return viewItems, groups
}

// ── Comparison logic ────────────────────────────────────────────────────

// computeDiff compares two sets of policy items and produces diffs.
// Policies are matched by PolicyName + Category + PolicyType + Platform
// to handle cases where multiple policies share the same display name
// (e.g., Enrollment Configurations or cross-platform Security Baselines).
func computeDiff(leftItems, rightItems []models.PolicyItem, filter string) (CompareStats, []PolicyDiff) {
	type policyKey struct {
		Name       string
		Category   string
		PolicyType string
		Platform   string
	}

	// Index right items by key
	rightIndex := make(map[policyKey]models.PolicyItem)
	for _, item := range rightItems {
		key := policyKey{Name: item.PolicyName, Category: item.Category, PolicyType: item.PolicyType, Platform: item.Platform}
		rightIndex[key] = item
	}

	// Track which right items were matched
	matched := make(map[policyKey]bool)

	var stats CompareStats
	var diffs []PolicyDiff

	// Compare left items against right
	for _, left := range leftItems {
		key := policyKey{Name: left.PolicyName, Category: left.Category, PolicyType: left.PolicyType, Platform: left.Platform}
		right, found := rightIndex[key]
		matched[key] = true

		if !found {
			stats.LeftOnly++
			diff := PolicyDiff{
				PolicyName: left.PolicyName,
				Category:   left.Category,
				Platform:   left.Platform,
				Status:     "left-only",
				Settings:   flattenToViewSettings(left.SettingsJSON),
			}
			if filter == "" || filter == "left-only" {
				diffs = append(diffs, diff)
			}
			continue
		}

		// Both exist — compare settings
		settingDiffs, allMatch := diffSettings(left.SettingsJSON, right.SettingsJSON)

		if allMatch {
			stats.Matching++
			diff := PolicyDiff{
				PolicyName:   left.PolicyName,
				Category:     left.Category,
				Platform:     left.Platform,
				Status:       "matching",
				SettingDiffs: settingDiffs,
			}
			if filter == "" || filter == "matching" {
				diffs = append(diffs, diff)
			}
		} else {
			stats.Different++
			diff := PolicyDiff{
				PolicyName:   left.PolicyName,
				Category:     left.Category,
				Platform:     left.Platform,
				Status:       "different",
				SettingDiffs: settingDiffs,
			}
			if filter == "" || filter == "different" {
				diffs = append(diffs, diff)
			}
		}
	}

	// Find right-only items
	for _, right := range rightItems {
		key := policyKey{Name: right.PolicyName, Category: right.Category, PolicyType: right.PolicyType, Platform: right.Platform}
		if matched[key] {
			continue
		}
		stats.RightOnly++
		diff := PolicyDiff{
			PolicyName: right.PolicyName,
			Category:   right.Category,
			Platform:   right.Platform,
			Status:     "right-only",
			Settings:   flattenToViewSettings(right.SettingsJSON),
		}
		if filter == "" || filter == "right-only" {
			diffs = append(diffs, diff)
		}
	}

	// Sort diffs: different first, then left-only, right-only, matching
	statusOrder := map[string]int{"different": 0, "left-only": 1, "right-only": 2, "matching": 3}
	sort.Slice(diffs, func(i, j int) bool {
		oi, oj := statusOrder[diffs[i].Status], statusOrder[diffs[j].Status]
		if oi != oj {
			return oi < oj
		}
		return diffs[i].PolicyName < diffs[j].PolicyName
	})

	return stats, diffs
}

// diffSettings compares two JSON settings blobs and returns per-setting diffs.
func diffSettings(leftJSON, rightJSON string) ([]SettingDiff, bool) {
	leftMap := parseSettingsMap(leftJSON)
	rightMap := parseSettingsMap(rightJSON)

	// Collect all keys from both sides
	allKeys := make(map[string]bool)
	for k := range leftMap {
		allKeys[k] = true
	}
	for k := range rightMap {
		allKeys[k] = true
	}

	var keys []string
	for k := range allKeys {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var diffs []SettingDiff
	allMatch := true

	for _, k := range keys {
		lv := formatSettingValue(leftMap[k])
		rv := formatSettingValue(rightMap[k])
		changed := lv != rv
		if changed {
			allMatch = false
		}
		diffs = append(diffs, SettingDiff{
			Name:       k,
			LeftValue:  lv,
			RightValue: rv,
			Changed:    changed,
		})
	}

	return diffs, allMatch
}

// parseSettingsMap parses a JSON string into a map of settings.
func parseSettingsMap(jsonStr string) map[string]any {
	var m map[string]any
	if err := json.Unmarshal([]byte(jsonStr), &m); err != nil {
		return map[string]any{}
	}
	return m
}

// formatSettingValue converts a value into a string for comparison/display.
func formatSettingValue(v any) string {
	if v == nil {
		return ""
	}
	switch val := v.(type) {
	case string:
		return val
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
	default:
		b, _ := json.Marshal(val)
		return string(b)
	}
}

// flattenToViewSettings converts a JSON blob into PolicySetting view models.
func flattenToViewSettings(settingsJSON string) []PolicySetting {
	settings := intune.FlattenSettings(settingsJSON)
	ps := make([]PolicySetting, len(settings))
	for i, s := range settings {
		ps[i] = PolicySetting{Name: s.Name, Value: s.Value}
	}
	return ps
}

// extractDimensions returns sorted unique platforms and categories from diffs.
func extractDimensions(diffs []PolicyDiff) ([]string, []string) {
	platSet := map[string]bool{}
	catSet := map[string]bool{}
	for _, d := range diffs {
		p := d.Platform
		if p == "" {
			p = "Other"
		}
		platSet[p] = true
		catSet[d.Category] = true
	}
	platforms := make([]string, 0, len(platSet))
	for p := range platSet {
		platforms = append(platforms, p)
	}
	sort.Strings(platforms)
	categories := make([]string, 0, len(catSet))
	for c := range catSet {
		categories = append(categories, c)
	}
	sort.Strings(categories)
	return platforms, categories
}
