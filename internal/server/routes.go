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

	// Placeholder pages (coming soon)
	s.router.HandleFunc("GET /campaigns", s.handleCampaigns)
	s.router.HandleFunc("GET /audit", s.handleAuditLog)
}
