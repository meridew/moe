package store

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/dan/moe/internal/models"
)

// ProviderConfigStore handles persistence for ProviderConfig records.
type ProviderConfigStore struct {
	db *sql.DB
}

// NewProviderConfigStore creates a ProviderConfigStore.
func NewProviderConfigStore(db *sql.DB) *ProviderConfigStore {
	return &ProviderConfigStore{db: db}
}

// column list shared by all SELECT queries.
const providerCols = `id, name, type, base_url, tenant_id, client_id, client_secret,
	username, password, sync_interval, enabled,
	last_check_at, last_check_ok, last_check_err, last_sync_at, consec_fails,
	created_at, updated_at`

// scanProvider scans a full row into a ProviderConfig.
func scanProvider(sc interface{ Scan(...any) error }) (*models.ProviderConfig, error) {
	p := &models.ProviderConfig{}
	var lastCheckAt, lastSyncAt string
	err := sc.Scan(
		&p.ID, &p.Name, &p.Type, &p.BaseURL, &p.TenantID, &p.ClientID, &p.ClientSecret,
		&p.Username, &p.Password, &p.SyncInterval, &p.Enabled,
		&lastCheckAt, &p.LastCheckOK, &p.LastCheckErr, &lastSyncAt, &p.ConsecFails,
		&p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	if lastCheckAt != "" {
		p.LastCheckAt, _ = time.Parse(time.RFC3339, lastCheckAt)
	}
	if lastSyncAt != "" {
		p.LastSyncAt, _ = time.Parse(time.RFC3339, lastSyncAt)
	}
	return p, nil
}

// Create inserts a new provider config.
func (s *ProviderConfigStore) Create(p *models.ProviderConfig) error {
	now := time.Now().UTC()
	p.CreatedAt = now
	p.UpdatedAt = now

	_, err := s.db.Exec(`
		INSERT INTO provider_configs (id, name, type, base_url, tenant_id, client_id, client_secret, username, password, sync_interval, enabled, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.ID, p.Name, p.Type, p.BaseURL, p.TenantID, p.ClientID, p.ClientSecret, p.Username, p.Password, p.SyncInterval, p.Enabled, p.CreatedAt, p.UpdatedAt,
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint") {
			return fmt.Errorf("a provider named %q already exists", p.Name)
		}
		return fmt.Errorf("insert provider config: %w", err)
	}
	return nil
}

// GetByID returns a provider config by ID.
func (s *ProviderConfigStore) GetByID(id string) (*models.ProviderConfig, error) {
	row := s.db.QueryRow(`SELECT `+providerCols+` FROM provider_configs WHERE id = ?`, id)
	p, err := scanProvider(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get provider config: %w", err)
	}
	return p, nil
}

// GetByName returns a provider config by unique name.
func (s *ProviderConfigStore) GetByName(name string) (*models.ProviderConfig, error) {
	row := s.db.QueryRow(`SELECT `+providerCols+` FROM provider_configs WHERE name = ?`, name)
	p, err := scanProvider(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get provider config by name: %w", err)
	}
	return p, nil
}

// Update modifies an existing provider config.
func (s *ProviderConfigStore) Update(p *models.ProviderConfig) error {
	p.UpdatedAt = time.Now().UTC()

	res, err := s.db.Exec(`
		UPDATE provider_configs SET
			name = ?, type = ?, base_url = ?, tenant_id = ?,
			client_id = ?, client_secret = ?,
			username = ?, password = ?,
			sync_interval = ?, enabled = ?, updated_at = ?
		WHERE id = ?`,
		p.Name, p.Type, p.BaseURL, p.TenantID,
		p.ClientID, p.ClientSecret,
		p.Username, p.Password,
		p.SyncInterval, p.Enabled, p.UpdatedAt, p.ID,
	)
	if err != nil {
		return fmt.Errorf("update provider config: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("provider config not found: %s", p.ID)
	}
	return nil
}

// SetEnabled toggles a provider's enabled flag.
func (s *ProviderConfigStore) SetEnabled(id string, enabled bool) error {
	res, err := s.db.Exec(
		`UPDATE provider_configs SET enabled = ?, updated_at = ? WHERE id = ?`,
		enabled, time.Now().UTC(), id,
	)
	if err != nil {
		return fmt.Errorf("set enabled: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("provider config not found: %s", id)
	}
	return nil
}

// RecordCheckResult persists the outcome of a health check.
func (s *ProviderConfigStore) RecordCheckResult(name string, ok bool, errMsg string, consecFails int) error {
	_, err := s.db.Exec(`
		UPDATE provider_configs SET
			last_check_at = ?, last_check_ok = ?, last_check_err = ?,
			consec_fails = ?, updated_at = ?
		WHERE name = ?`,
		time.Now().UTC().Format(time.RFC3339), ok, errMsg, consecFails, time.Now().UTC(), name,
	)
	if err != nil {
		return fmt.Errorf("record check result: %w", err)
	}
	return nil
}

// RecordSyncSuccess persists the time of a successful sync and resets failure count.
func (s *ProviderConfigStore) RecordSyncSuccess(name string) error {
	_, err := s.db.Exec(`
		UPDATE provider_configs SET
			last_sync_at = ?, consec_fails = 0, updated_at = ?
		WHERE name = ?`,
		time.Now().UTC().Format(time.RFC3339), time.Now().UTC(), name,
	)
	if err != nil {
		return fmt.Errorf("record sync success: %w", err)
	}
	return nil
}

// Delete removes a provider config by ID.
func (s *ProviderConfigStore) Delete(id string) error {
	res, err := s.db.Exec("DELETE FROM provider_configs WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete provider config: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("provider config not found: %s", id)
	}
	return nil
}

// ListAll returns all provider configs ordered by enabled (desc) then name.
func (s *ProviderConfigStore) ListAll() ([]models.ProviderConfig, error) {
	rows, err := s.db.Query(`SELECT ` + providerCols + ` FROM provider_configs ORDER BY enabled DESC, name`)
	if err != nil {
		return nil, fmt.Errorf("list provider configs: %w", err)
	}
	defer rows.Close()

	var configs []models.ProviderConfig
	for rows.Next() {
		p, err := scanProvider(rows)
		if err != nil {
			return nil, fmt.Errorf("scan provider config: %w", err)
		}
		configs = append(configs, *p)
	}
	return configs, rows.Err()
}

// ListEnabled returns only enabled provider configs.
func (s *ProviderConfigStore) ListEnabled() ([]models.ProviderConfig, error) {
	rows, err := s.db.Query(`SELECT ` + providerCols + ` FROM provider_configs WHERE enabled = 1 ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("list enabled provider configs: %w", err)
	}
	defer rows.Close()

	var configs []models.ProviderConfig
	for rows.Next() {
		p, err := scanProvider(rows)
		if err != nil {
			return nil, fmt.Errorf("scan provider config: %w", err)
		}
		configs = append(configs, *p)
	}
	return configs, rows.Err()
}

// ProviderNames returns just the names for use in dropdowns etc.
func (s *ProviderConfigStore) ProviderNames() ([]string, error) {
	rows, err := s.db.Query("SELECT name FROM provider_configs ORDER BY name")
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
