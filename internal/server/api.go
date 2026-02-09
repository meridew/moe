package server

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/dan/moe/internal/models"
	"github.com/dan/moe/internal/provider"
)

// ── JSON helpers ────────────────────────────────────────────────────────

// apiResponse is the standard envelope for all API responses.
type apiResponse struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
	Data  any    `json:"data,omitempty"`
}

func jsonOK(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(apiResponse{OK: true, Data: data})
}

func jsonError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(apiResponse{OK: false, Error: msg})
}

// ── Devices ─────────────────────────────────────────────────────────────

// GET /api/v1/devices?provider=&os=&compliance=&q=&limit=&offset=
func (s *Server) apiListDevices(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := models.DeviceFilter{
		ProviderName: q.Get("provider"),
		OS:           q.Get("os"),
		Compliance:   q.Get("compliance"),
		Search:       q.Get("q"),
		Limit:        queryInt(q, "limit", 200),
		Offset:       queryInt(q, "offset", 0),
	}

	devices, total, err := s.devices.List(f)
	if err != nil {
		log.Printf("[api] list devices error: %v", err)
		jsonError(w, http.StatusInternalServerError, "failed to list devices")
		return
	}

	jsonOK(w, map[string]any{
		"devices": devices,
		"total":   total,
		"limit":   f.Limit,
		"offset":  f.Offset,
	})
}

// GET /api/v1/devices/{id}
func (s *Server) apiGetDevice(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	device, err := s.devices.GetByID(id)
	if err != nil {
		log.Printf("[api] get device error: %v", err)
		jsonError(w, http.StatusInternalServerError, "failed to get device")
		return
	}
	if device == nil {
		jsonError(w, http.StatusNotFound, "device not found")
		return
	}
	jsonOK(w, device)
}

// ── Providers ───────────────────────────────────────────────────────────

// GET /api/v1/providers
func (s *Server) apiListProviders(w http.ResponseWriter, r *http.Request) {
	providers, err := s.providerConfigs.ListAll()
	if err != nil {
		log.Printf("[api] list providers error: %v", err)
		jsonError(w, http.StatusInternalServerError, "failed to list providers")
		return
	}
	jsonOK(w, providers)
}

// ── Policy snapshots ────────────────────────────────────────────────────

// GET /api/v1/policies/snapshots
func (s *Server) apiListSnapshots(w http.ResponseWriter, r *http.Request) {
	snapshots, err := s.policies.ListSnapshots()
	if err != nil {
		log.Printf("[api] list snapshots error: %v", err)
		jsonError(w, http.StatusInternalServerError, "failed to list snapshots")
		return
	}
	jsonOK(w, snapshots)
}

// GET /api/v1/policies/snapshots/{id}
func (s *Server) apiGetSnapshot(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	snap, err := s.policies.GetSnapshot(id)
	if err != nil {
		log.Printf("[api] get snapshot error: %v", err)
		jsonError(w, http.StatusInternalServerError, "failed to get snapshot")
		return
	}
	if snap == nil {
		jsonError(w, http.StatusNotFound, "snapshot not found")
		return
	}

	categories, _ := s.policies.DistinctCategories(id)

	jsonOK(w, map[string]any{
		"snapshot":   snap,
		"categories": categories,
	})
}

// GET /api/v1/policies/snapshots/{id}/items?category=&q=
func (s *Server) apiListSnapshotItems(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	q := r.URL.Query()

	snap, err := s.policies.GetSnapshot(id)
	if err != nil || snap == nil {
		jsonError(w, http.StatusNotFound, "snapshot not found")
		return
	}

	items, err := s.policies.ListItems(id, q.Get("category"), q.Get("q"))
	if err != nil {
		log.Printf("[api] list snapshot items error: %v", err)
		jsonError(w, http.StatusInternalServerError, "failed to list items")
		return
	}

	jsonOK(w, map[string]any{
		"snapshot_id": id,
		"count":       len(items),
		"items":       items,
	})
}

// ── Policy comparison ───────────────────────────────────────────────────

// apiCompareResult is the JSON shape returned by the compare endpoint.
type apiCompareResult struct {
	Left   *models.PolicySnapshot `json:"left"`
	Right  *models.PolicySnapshot `json:"right"`
	Filter string                 `json:"filter,omitempty"`
	Stats  CompareStats           `json:"stats"`
	Diffs  []PolicyDiff           `json:"diffs"`
}

// GET /api/v1/policies/compare?left={id}&right={id}&filter=
func (s *Server) apiCompareSnapshots(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	leftID := q.Get("left")
	rightID := q.Get("right")
	filter := q.Get("filter")

	if leftID == "" || rightID == "" {
		jsonError(w, http.StatusBadRequest, "both 'left' and 'right' snapshot IDs are required")
		return
	}

	leftSnap, err := s.policies.GetSnapshot(leftID)
	if err != nil || leftSnap == nil {
		jsonError(w, http.StatusNotFound, "left snapshot not found")
		return
	}
	rightSnap, err := s.policies.GetSnapshot(rightID)
	if err != nil || rightSnap == nil {
		jsonError(w, http.StatusNotFound, "right snapshot not found")
		return
	}

	leftItems, err := s.policies.ListItems(leftID, "", "")
	if err != nil {
		log.Printf("[api] compare left items error: %v", err)
		jsonError(w, http.StatusInternalServerError, "failed to load left snapshot items")
		return
	}
	rightItems, err := s.policies.ListItems(rightID, "", "")
	if err != nil {
		log.Printf("[api] compare right items error: %v", err)
		jsonError(w, http.StatusInternalServerError, "failed to load right snapshot items")
		return
	}

	stats, diffs := computeDiff(leftItems, rightItems, filter)

	jsonOK(w, apiCompareResult{
		Left:   leftSnap,
		Right:  rightSnap,
		Filter: filter,
		Stats:  stats,
		Diffs:  diffs,
	})
}

// ── Snapshot creation ────────────────────────────────────────────────────

// apiCreateSnapshot triggers a policy snapshot for the given provider.
// POST /api/v1/policies/snapshots  {"provider_id": "..."}
func (s *Server) apiCreateSnapshot(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ProviderID string `json:"provider_id"`
		Label      string `json:"label"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if body.ProviderID == "" {
		jsonError(w, http.StatusBadRequest, "provider_id is required")
		return
	}

	cfg, err := s.providerConfigs.GetByID(body.ProviderID)
	if err != nil || cfg == nil {
		jsonError(w, http.StatusNotFound, "provider not found")
		return
	}

	p, err := s.buildProvider(cfg)
	if err != nil {
		log.Printf("[api] build provider error: %v", err)
		s.activity.Logf(cfg.Name, "error", "API snapshot failed — could not init provider: %s", err)
		jsonError(w, http.StatusInternalServerError, "failed to initialise provider")
		return
	}

	pp, ok := p.(provider.PolicyProvider)
	if !ok {
		jsonError(w, http.StatusBadRequest, "provider does not support policy sync")
		return
	}

	s.activity.Logf(cfg.Name, "info", "API policy snapshot started…")

	snapshotID := newID()
	snap := &models.PolicySnapshot{
		ID:           snapshotID,
		ProviderName: cfg.Name,
		ProviderType: cfg.Type,
		Label:        body.Label,
		TakenAt:      time.Now().UTC(),
	}
	if err := s.policies.CreateSnapshot(snap); err != nil {
		log.Printf("[api] create snapshot error: %v", err)
		jsonError(w, http.StatusInternalServerError, "failed to create snapshot record")
		return
	}

	syncPolicies, err := pp.SyncPolicies(r.Context(), func(category string, count int) {
		s.activity.Logf(cfg.Name, "info", "API snapshot: fetched %s (%d total so far)", category, count)
	})
	if err != nil {
		log.Printf("[api] sync error for %s: %v", cfg.Name, err)
		s.activity.Logf(cfg.Name, "error", "API snapshot error: %s", err)
		_ = s.policies.DeleteSnapshot(snapshotID)
		jsonError(w, http.StatusInternalServerError, fmt.Sprintf("policy sync failed: %v", err))
		return
	}

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
			log.Printf("[api] insert item error: %v", err)
		}
	}

	_ = s.policies.UpdateSnapshotCounts(snapshotID)
	_ = s.policies.DeleteOldSnapshots(10)

	// Re-read the snapshot to get the updated counts
	snap, _ = s.policies.GetSnapshot(snapshotID)

	s.activity.Logf(cfg.Name, "success", "API snapshot complete — %d policies captured", len(syncPolicies))

	w.WriteHeader(http.StatusCreated)
	jsonOK(w, snap)
}

// ── Export / Import ──────────────────────────────────────────────────────

// snapshotExport is the JSON shape for a portable snapshot.
type snapshotExport struct {
	Version    int                   `json:"version"`
	ExportedAt time.Time             `json:"exported_at"`
	Snapshot   models.PolicySnapshot `json:"snapshot"`
	Items      []models.PolicyItem   `json:"items"`
}

// GET /api/v1/policies/snapshots/{id}/export — full JSON export
func (s *Server) apiExportSnapshot(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	snap, err := s.policies.GetSnapshot(id)
	if err != nil || snap == nil {
		jsonError(w, http.StatusNotFound, "snapshot not found")
		return
	}
	items, err := s.policies.ListItems(id, "", "")
	if err != nil {
		log.Printf("[api] export items error: %v", err)
		jsonError(w, http.StatusInternalServerError, "failed to load snapshot items")
		return
	}

	export := snapshotExport{
		Version:    1,
		ExportedAt: time.Now().UTC(),
		Snapshot:   *snap,
		Items:      items,
	}

	fname := fmt.Sprintf("moe-snapshot-%s-%s.json", snap.ProviderName, snap.TakenAt.Format("20060102-150405"))
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, fname))
	json.NewEncoder(w).Encode(export)
}

// POST /api/v1/policies/snapshots/import — import a previously exported snapshot
func (s *Server) apiImportSnapshot(w http.ResponseWriter, r *http.Request) {
	var imp snapshotExport
	if err := json.NewDecoder(r.Body).Decode(&imp); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if imp.Snapshot.ProviderName == "" {
		jsonError(w, http.StatusBadRequest, "snapshot.provider_name is required")
		return
	}

	// Create a new snapshot with a fresh ID
	newSnapID := newID()
	label := imp.Snapshot.Label
	if label == "" {
		label = imp.Snapshot.DisplayName() + " (imported)"
	}
	snap := &models.PolicySnapshot{
		ID:           newSnapID,
		ProviderName: imp.Snapshot.ProviderName,
		ProviderType: imp.Snapshot.ProviderType,
		Label:        label,
		TakenAt:      imp.Snapshot.TakenAt,
	}
	if err := s.policies.CreateSnapshot(snap); err != nil {
		log.Printf("[api] import create snapshot error: %v", err)
		jsonError(w, http.StatusInternalServerError, "failed to create snapshot")
		return
	}

	inserted := 0
	for _, item := range imp.Items {
		newItem := &models.PolicyItem{
			ID:           newID(),
			SnapshotID:   newSnapID,
			Category:     item.Category,
			SourceID:     item.SourceID,
			PolicyName:   item.PolicyName,
			PolicyType:   item.PolicyType,
			Platform:     item.Platform,
			Description:  item.Description,
			SettingsJSON: item.SettingsJSON,
		}
		if err := s.policies.InsertItem(newItem); err != nil {
			log.Printf("[api] import insert item error: %v", err)
			continue
		}
		inserted++
	}
	_ = s.policies.UpdateSnapshotCounts(newSnapID)

	snap, _ = s.policies.GetSnapshot(newSnapID)
	s.activity.Logf(snap.ProviderName, "success", "Imported snapshot with %d policies", inserted)

	w.WriteHeader(http.StatusCreated)
	jsonOK(w, snap)
}

// GET /api/v1/policies/snapshots/{id}/export/csv — flattened CSV export
func (s *Server) apiExportSnapshotCSV(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	snap, err := s.policies.GetSnapshot(id)
	if err != nil || snap == nil {
		jsonError(w, http.StatusNotFound, "snapshot not found")
		return
	}
	items, err := s.policies.ListItems(id, "", "")
	if err != nil {
		log.Printf("[api] export csv items error: %v", err)
		jsonError(w, http.StatusInternalServerError, "failed to load snapshot items")
		return
	}

	fname := fmt.Sprintf("moe-snapshot-%s-%s.csv", snap.ProviderName, snap.TakenAt.Format("20060102-150405"))
	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, fname))

	cw := csv.NewWriter(w)
	defer cw.Flush()

	// Header row
	cw.Write([]string{"Category", "PolicyName", "PolicyType", "Platform", "Description", "SettingsJSON"})

	for _, item := range items {
		cw.Write([]string{
			item.Category,
			item.PolicyName,
			item.PolicyType,
			item.Platform,
			item.Description,
			item.SettingsJSON,
		})
	}
}

// ── Helpers ─────────────────────────────────────────────────────────────

func queryInt(q map[string][]string, key string, fallback int) int {
	v := q[key]
	if len(v) == 0 || v[0] == "" {
		return fallback
	}
	n, err := strconv.Atoi(v[0])
	if err != nil || n <= 0 {
		return fallback
	}
	return n
}
