package store

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/dan/moe/internal/models"
)

// DeviceStore handles persistence for Device records.
type DeviceStore struct {
	db *sql.DB
}

// NewDeviceStore creates a DeviceStore backed by the given database connection.
func NewDeviceStore(db *sql.DB) *DeviceStore {
	return &DeviceStore{db: db}
}

// Create inserts a new device record.
func (s *DeviceStore) Create(d *models.Device) error {
	now := time.Now().UTC()
	d.CreatedAt = now
	d.UpdatedAt = now

	_, err := s.db.Exec(`
		INSERT INTO devices (
			id, provider_name, provider_type, source_id,
			device_name, os, os_version, model,
			user_name, user_email, compliance,
			last_seen, last_synced_at, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		d.ID, d.ProviderName, d.ProviderType, d.SourceID,
		d.DeviceName, d.OS, d.OSVersion, d.Model,
		d.UserName, d.UserEmail, d.Compliance,
		d.LastSeen, d.LastSyncedAt, d.CreatedAt, d.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert device: %w", err)
	}
	return nil
}

// Upsert inserts or updates a device keyed by (provider_name, source_id).
// Used by the sync engine to refresh cached data.
func (s *DeviceStore) Upsert(d *models.Device) error {
	now := time.Now().UTC()
	d.UpdatedAt = now

	_, err := s.db.Exec(`
		INSERT INTO devices (
			id, provider_name, provider_type, source_id,
			device_name, os, os_version, model,
			user_name, user_email, compliance,
			last_seen, last_synced_at, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(provider_name, source_id) DO UPDATE SET
			device_name    = excluded.device_name,
			os             = excluded.os,
			os_version     = excluded.os_version,
			model          = excluded.model,
			user_name      = excluded.user_name,
			user_email     = excluded.user_email,
			compliance     = excluded.compliance,
			last_seen      = excluded.last_seen,
			last_synced_at = excluded.last_synced_at,
			updated_at     = excluded.updated_at`,
		d.ID, d.ProviderName, d.ProviderType, d.SourceID,
		d.DeviceName, d.OS, d.OSVersion, d.Model,
		d.UserName, d.UserEmail, d.Compliance,
		d.LastSeen, d.LastSyncedAt, d.CreatedAt, d.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("upsert device: %w", err)
	}
	return nil
}

// GetByID returns a single device by its MOE internal ID.
func (s *DeviceStore) GetByID(id string) (*models.Device, error) {
	d := &models.Device{}
	err := s.db.QueryRow(`
		SELECT id, provider_name, provider_type, source_id,
			device_name, os, os_version, model,
			user_name, user_email, compliance,
			last_seen, last_synced_at, created_at, updated_at
		FROM devices WHERE id = ?`, id,
	).Scan(
		&d.ID, &d.ProviderName, &d.ProviderType, &d.SourceID,
		&d.DeviceName, &d.OS, &d.OSVersion, &d.Model,
		&d.UserName, &d.UserEmail, &d.Compliance,
		&d.LastSeen, &d.LastSyncedAt, &d.CreatedAt, &d.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get device: %w", err)
	}
	return d, nil
}

// Update modifies an existing device by ID.
func (s *DeviceStore) Update(d *models.Device) error {
	d.UpdatedAt = time.Now().UTC()

	res, err := s.db.Exec(`
		UPDATE devices SET
			provider_name = ?, provider_type = ?, source_id = ?,
			device_name = ?, os = ?, os_version = ?, model = ?,
			user_name = ?, user_email = ?, compliance = ?,
			last_seen = ?, last_synced_at = ?, updated_at = ?
		WHERE id = ?`,
		d.ProviderName, d.ProviderType, d.SourceID,
		d.DeviceName, d.OS, d.OSVersion, d.Model,
		d.UserName, d.UserEmail, d.Compliance,
		d.LastSeen, d.LastSyncedAt, d.UpdatedAt,
		d.ID,
	)
	if err != nil {
		return fmt.Errorf("update device: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("device not found: %s", d.ID)
	}
	return nil
}

// Delete removes a device by ID.
func (s *DeviceStore) Delete(id string) error {
	res, err := s.db.Exec("DELETE FROM devices WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete device: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("device not found: %s", id)
	}
	return nil
}

// List returns devices matching the given filter criteria.
func (s *DeviceStore) List(f models.DeviceFilter) ([]models.Device, int, error) {
	var (
		where []string
		args  []any
	)

	if f.ProviderName != "" {
		where = append(where, "provider_name = ?")
		args = append(args, f.ProviderName)
	}
	if f.ProviderType != "" {
		where = append(where, "provider_type = ?")
		args = append(args, f.ProviderType)
	}
	if f.OS != "" {
		where = append(where, "os = ?")
		args = append(args, f.OS)
	}
	if f.Compliance != "" {
		where = append(where, "compliance = ?")
		args = append(args, f.Compliance)
	}
	if f.Search != "" {
		where = append(where, "(device_name LIKE ? OR user_name LIKE ? OR user_email LIKE ? OR model LIKE ?)")
		q := "%" + f.Search + "%"
		args = append(args, q, q, q, q)
	}

	whereClause := ""
	if len(where) > 0 {
		whereClause = "WHERE " + strings.Join(where, " AND ")
	}

	// Count total matches.
	var total int
	countSQL := "SELECT COUNT(*) FROM devices " + whereClause
	if err := s.db.QueryRow(countSQL, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count devices: %w", err)
	}

	// Apply pagination defaults.
	limit := f.Limit
	if limit <= 0 {
		limit = 50
	}
	offset := f.Offset
	if offset < 0 {
		offset = 0
	}

	querySQL := fmt.Sprintf(`
		SELECT id, provider_name, provider_type, source_id,
			device_name, os, os_version, model,
			user_name, user_email, compliance,
			last_seen, last_synced_at, created_at, updated_at
		FROM devices %s
		ORDER BY updated_at DESC
		LIMIT ? OFFSET ?`, whereClause)

	queryArgs := append(args, limit, offset)
	rows, err := s.db.Query(querySQL, queryArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("list devices: %w", err)
	}
	defer rows.Close()

	var devices []models.Device
	for rows.Next() {
		var d models.Device
		if err := rows.Scan(
			&d.ID, &d.ProviderName, &d.ProviderType, &d.SourceID,
			&d.DeviceName, &d.OS, &d.OSVersion, &d.Model,
			&d.UserName, &d.UserEmail, &d.Compliance,
			&d.LastSeen, &d.LastSyncedAt, &d.CreatedAt, &d.UpdatedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("scan device: %w", err)
		}
		devices = append(devices, d)
	}

	return devices, total, rows.Err()
}

// Count returns the total number of devices.
func (s *DeviceStore) Count() (int, error) {
	var count int
	err := s.db.QueryRow("SELECT COUNT(*) FROM devices").Scan(&count)
	return count, err
}

// CountByProvider returns device counts grouped by provider_name.
func (s *DeviceStore) CountByProvider() (map[string]int, error) {
	rows, err := s.db.Query("SELECT provider_name, COUNT(*) FROM devices GROUP BY provider_name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]int)
	for rows.Next() {
		var name string
		var count int
		if err := rows.Scan(&name, &count); err != nil {
			return nil, err
		}
		result[name] = count
	}
	return result, rows.Err()
}

// DistinctProviders returns the list of distinct provider names that have devices.
func (s *DeviceStore) DistinctProviders() ([]string, error) {
	rows, err := s.db.Query("SELECT DISTINCT provider_name FROM devices ORDER BY provider_name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var names []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, err
		}
		names = append(names, n)
	}
	return names, rows.Err()
}

// DistinctOS returns the list of distinct OS values in the devices table.
func (s *DeviceStore) DistinctOS() ([]string, error) {
	rows, err := s.db.Query("SELECT DISTINCT os FROM devices WHERE os != '' ORDER BY os")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var values []string
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		values = append(values, v)
	}
	return values, rows.Err()
}

// LastSyncByProvider returns the most recent last_synced_at per provider_name.
func (s *DeviceStore) LastSyncByProvider() (map[string]time.Time, error) {
	rows, err := s.db.Query("SELECT provider_name, MAX(last_synced_at) FROM devices WHERE last_synced_at IS NOT NULL GROUP BY provider_name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]time.Time)
	for rows.Next() {
		var name string
		var t time.Time
		if err := rows.Scan(&name, &t); err != nil {
			return nil, err
		}
		result[name] = t
	}
	return result, rows.Err()
}
