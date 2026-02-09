package server

// routes registers all HTTP handlers on the server's mux.
// New routes are added here as the application grows.
func (s *Server) routes() {
	// Dashboard
	s.router.HandleFunc("GET /{$}", s.handleDashboard)
	s.router.HandleFunc("GET /health", s.handleHealth)

	// Devices
	s.router.HandleFunc("GET /devices", s.handleDeviceList)
	s.router.HandleFunc("GET /devices/rows", s.handleDeviceRows)
	s.router.HandleFunc("GET /devices/new", s.handleDeviceNew)
	s.router.HandleFunc("POST /devices", s.handleDeviceCreate)
	s.router.HandleFunc("GET /devices/{id}/edit", s.handleDeviceEdit)
	s.router.HandleFunc("POST /devices/{id}", s.handleDeviceUpdate)
	s.router.HandleFunc("POST /devices/{id}/delete", s.handleDeviceDelete)

	// Providers
	s.router.HandleFunc("GET /providers", s.handleProviderList)
	s.router.HandleFunc("GET /providers/new", s.handleProviderNew)
	s.router.HandleFunc("POST /providers", s.handleProviderCreate)
	s.router.HandleFunc("GET /providers/{id}/edit", s.handleProviderEdit)
	s.router.HandleFunc("POST /providers/{id}", s.handleProviderUpdate)
	s.router.HandleFunc("POST /providers/{id}/delete", s.handleProviderDelete)
	s.router.HandleFunc("POST /providers/{id}/sync", s.handleProviderSync)
	s.router.HandleFunc("POST /providers/{id}/test", s.handleProviderTest)
	s.router.HandleFunc("POST /providers/{id}/toggle", s.handleProviderToggle)

	// Console (live activity feed)
	s.router.HandleFunc("GET /console", s.handleConsole)
	s.router.HandleFunc("GET /console/events", s.handleConsoleEvents)
	s.router.HandleFunc("GET /console/statuses", s.handleConsoleStatuses)

	// Policies
	s.router.HandleFunc("GET /policies", s.handlePolicies)
	s.router.HandleFunc("POST /policies/snapshot", s.handlePolicySnapshotCreate)
	s.router.HandleFunc("GET /policies/compare", s.handlePolicyCompare)
	s.router.HandleFunc("GET /policies/snapshots/{id}", s.handlePolicySnapshot)
	s.router.HandleFunc("POST /policies/snapshots/{id}/delete", s.handlePolicySnapshotDelete)

	// Placeholder pages (coming soon)
	s.router.HandleFunc("GET /campaigns", s.handleCampaigns)
	s.router.HandleFunc("GET /audit", s.handleAuditLog)

	// ── JSON API (read-only) ────────────────────────────────────────────
	s.router.HandleFunc("GET /api/v1/devices", s.apiListDevices)
	s.router.HandleFunc("GET /api/v1/devices/{id}", s.apiGetDevice)
	s.router.HandleFunc("GET /api/v1/providers", s.apiListProviders)
	s.router.HandleFunc("GET /api/v1/policies/snapshots", s.apiListSnapshots)
	s.router.HandleFunc("POST /api/v1/policies/snapshots", s.apiCreateSnapshot)
	s.router.HandleFunc("GET /api/v1/policies/snapshots/{id}", s.apiGetSnapshot)
	s.router.HandleFunc("GET /api/v1/policies/snapshots/{id}/items", s.apiListSnapshotItems)
	s.router.HandleFunc("GET /api/v1/policies/snapshots/{id}/export", s.apiExportSnapshot)
	s.router.HandleFunc("GET /api/v1/policies/snapshots/{id}/export/csv", s.apiExportSnapshotCSV)
	s.router.HandleFunc("POST /api/v1/policies/snapshots/import", s.apiImportSnapshot)
	s.router.HandleFunc("GET /api/v1/policies/compare", s.apiCompareSnapshots)
}
