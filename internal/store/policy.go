package store

import (
	"database/sql"
	"fmt"

	"github.com/dan/moe/internal/models"
)

// PolicyStore handles persistence for policy snapshots and items.
type PolicyStore struct {
	db *sql.DB
}

// NewPolicyStore creates a PolicyStore backed by the given database connection.
func NewPolicyStore(db *sql.DB) *PolicyStore {
	return &PolicyStore{db: db}
}

// CreateSnapshot inserts a new snapshot record.
func (s *PolicyStore) CreateSnapshot(snap *models.PolicySnapshot) error {
	_, err := s.db.Exec(`
		INSERT INTO policy_snapshots (id, provider_name, provider_type, label, taken_at, policy_count, category_count)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		snap.ID, snap.ProviderName, snap.ProviderType, snap.Label, snap.TakenAt, snap.PolicyCount, snap.CategoryCount,
	)
	if err != nil {
		return fmt.Errorf("insert snapshot: %w", err)
	}
	return nil
}

// UpdateSnapshotCounts updates the denormalised counts on a snapshot.
func (s *PolicyStore) UpdateSnapshotCounts(id string) error {
	_, err := s.db.Exec(`
		UPDATE policy_snapshots SET
			policy_count = (SELECT COUNT(*) FROM policy_items WHERE snapshot_id = ?),
			category_count = (SELECT COUNT(DISTINCT category) FROM policy_items WHERE snapshot_id = ?)
		WHERE id = ?`, id, id, id)
	return err
}

// InsertItem inserts a single policy item into a snapshot.
func (s *PolicyStore) InsertItem(item *models.PolicyItem) error {
	_, err := s.db.Exec(`
		INSERT INTO policy_items (id, snapshot_id, category, source_id, policy_name, policy_type, platform, description, settings_json)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		item.ID, item.SnapshotID, item.Category, item.SourceID,
		item.PolicyName, item.PolicyType, item.Platform,
		item.Description, item.SettingsJSON,
	)
	if err != nil {
		return fmt.Errorf("insert policy item: %w", err)
	}
	return nil
}

// ListSnapshots returns all snapshots ordered by most recent first.
func (s *PolicyStore) ListSnapshots() ([]models.PolicySnapshot, error) {
	rows, err := s.db.Query(`
		SELECT id, provider_name, provider_type, label, taken_at, policy_count, category_count
		FROM policy_snapshots ORDER BY taken_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("list snapshots: %w", err)
	}
	defer rows.Close()

	var snapshots []models.PolicySnapshot
	for rows.Next() {
		var snap models.PolicySnapshot
		if err := rows.Scan(&snap.ID, &snap.ProviderName, &snap.ProviderType,
			&snap.Label, &snap.TakenAt, &snap.PolicyCount, &snap.CategoryCount); err != nil {
			return nil, fmt.Errorf("scan snapshot: %w", err)
		}
		snapshots = append(snapshots, snap)
	}
	if snapshots == nil {
		snapshots = []models.PolicySnapshot{}
	}
	return snapshots, rows.Err()
}

// GetSnapshot returns a single snapshot by ID.
func (s *PolicyStore) GetSnapshot(id string) (*models.PolicySnapshot, error) {
	var snap models.PolicySnapshot
	err := s.db.QueryRow(`
		SELECT id, provider_name, provider_type, label, taken_at, policy_count, category_count
		FROM policy_snapshots WHERE id = ?`, id,
	).Scan(&snap.ID, &snap.ProviderName, &snap.ProviderType,
		&snap.Label, &snap.TakenAt, &snap.PolicyCount, &snap.CategoryCount)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get snapshot: %w", err)
	}
	return &snap, nil
}

// DeleteSnapshot removes a snapshot and all its items (via CASCADE).
func (s *PolicyStore) DeleteSnapshot(id string) error {
	// SQLite foreign key CASCADE should handle items, but be explicit
	if _, err := s.db.Exec("DELETE FROM policy_items WHERE snapshot_id = ?", id); err != nil {
		return fmt.Errorf("delete policy items: %w", err)
	}
	if _, err := s.db.Exec("DELETE FROM policy_snapshots WHERE id = ?", id); err != nil {
		return fmt.Errorf("delete snapshot: %w", err)
	}
	return nil
}

// ListItems returns all policy items for a snapshot, optionally filtered.
func (s *PolicyStore) ListItems(snapshotID, category, search string) ([]models.PolicyItem, error) {
	query := "SELECT id, snapshot_id, category, source_id, policy_name, policy_type, platform, description, settings_json FROM policy_items WHERE snapshot_id = ?"
	args := []any{snapshotID}

	if category != "" {
		query += " AND category = ?"
		args = append(args, category)
	}
	if search != "" {
		query += " AND (policy_name LIKE ? OR description LIKE ? OR policy_type LIKE ?)"
		q := "%" + search + "%"
		args = append(args, q, q, q)
	}

	query += " ORDER BY category, policy_name"

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("list policy items: %w", err)
	}
	defer rows.Close()

	var items []models.PolicyItem
	for rows.Next() {
		var item models.PolicyItem
		if err := rows.Scan(&item.ID, &item.SnapshotID, &item.Category, &item.SourceID,
			&item.PolicyName, &item.PolicyType, &item.Platform,
			&item.Description, &item.SettingsJSON); err != nil {
			return nil, fmt.Errorf("scan policy item: %w", err)
		}
		items = append(items, item)
	}
	if items == nil {
		items = []models.PolicyItem{}
	}
	return items, rows.Err()
}

// DistinctCategories returns the unique categories in a snapshot.
func (s *PolicyStore) DistinctCategories(snapshotID string) ([]string, error) {
	rows, err := s.db.Query(
		"SELECT DISTINCT category FROM policy_items WHERE snapshot_id = ? ORDER BY category",
		snapshotID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cats []string
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			return nil, err
		}
		cats = append(cats, c)
	}
	if cats == nil {
		cats = []string{}
	}
	return cats, rows.Err()
}

// SnapshotExists checks if a snapshot with given ID exists.
func (s *PolicyStore) SnapshotExists(id string) (bool, error) {
	var exists bool
	err := s.db.QueryRow("SELECT EXISTS(SELECT 1 FROM policy_snapshots WHERE id = ?)", id).Scan(&exists)
	return exists, err
}

// DeleteOldSnapshots keeps only the N most recent snapshots per provider and deletes older ones.
func (s *PolicyStore) DeleteOldSnapshots(keepPerProvider int) error {
	// Get all provider names that have snapshots
	rows, err := s.db.Query("SELECT DISTINCT provider_name FROM policy_snapshots")
	if err != nil {
		return err
	}
	defer rows.Close()

	var providers []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return err
		}
		providers = append(providers, name)
	}

	for _, prov := range providers {
		_, err := s.db.Exec(`
			DELETE FROM policy_items WHERE snapshot_id IN (
				SELECT id FROM policy_snapshots
				WHERE provider_name = ?
				ORDER BY taken_at DESC
				LIMIT -1 OFFSET ?
			)`, prov, keepPerProvider)
		if err != nil {
			return err
		}
		_, err = s.db.Exec(`
			DELETE FROM policy_snapshots
			WHERE provider_name = ?
			AND id NOT IN (
				SELECT id FROM policy_snapshots
				WHERE provider_name = ?
				ORDER BY taken_at DESC
				LIMIT ?
			)`, prov, prov, keepPerProvider)
		if err != nil {
			return err
		}
	}
	return nil
}
