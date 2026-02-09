package server

import "net/http"

// dashboardData is the template data for the dashboard page.
type dashboardData struct {
	Nav   string
	Stats dashboardStats
}

type dashboardStats struct {
	Devices    int
	Providers  int
	Campaigns  int
	Migrations int
}

// handleDashboard renders the main dashboard overview page.
func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	migrations, _ := s.db.MigrationCount()
	deviceCount, _ := s.devices.Count()
	providers, _ := s.providerConfigs.ListAll()

	data := dashboardData{
		Nav: "dashboard",
		Stats: dashboardStats{
			Devices:    deviceCount,
			Providers:  len(providers),
			Campaigns:  0, // Populated in Phase 5
			Migrations: migrations,
		},
	}

	s.render.render(w, "dashboard.html", data)
}
