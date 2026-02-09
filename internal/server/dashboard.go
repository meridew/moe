package server

import "net/http"

// dashboardData is the template data for the dashboard page.
type dashboardData struct {
	Nav       string
	Stats     dashboardStats
	Providers []providerStat
}

type dashboardStats struct {
	Devices    int
	Providers  int
	Campaigns  int
	Migrations int
}

type providerStat struct {
	Name    string
	Type    string
	Devices int
	Enabled bool
}

// handleDashboard renders the main dashboard overview page.
func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	migrations, _ := s.db.MigrationCount()
	deviceCount, _ := s.devices.Count()
	providers, _ := s.providerConfigs.ListAll()
	deviceCounts, _ := s.devices.CountByProvider()

	var pstats []providerStat
	for _, p := range providers {
		pstats = append(pstats, providerStat{
			Name:    p.Name,
			Type:    p.Type,
			Devices: deviceCounts[p.Name],
			Enabled: p.Enabled,
		})
	}

	data := dashboardData{
		Nav: "dashboard",
		Stats: dashboardStats{
			Devices:    deviceCount,
			Providers:  len(providers),
			Campaigns:  0, // Populated in Phase 5
			Migrations: migrations,
		},
		Providers: pstats,
	}

	s.render.render(w, "dashboard.html", data)
}
