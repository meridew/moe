package server

import "net/http"

// handleCampaigns renders the campaigns placeholder page.
func (s *Server) handleCampaigns(w http.ResponseWriter, r *http.Request) {
	s.render.render(w, "campaigns.html", struct{ Nav string }{Nav: "campaigns"})
}

// handleAuditLog renders the audit log placeholder page.
func (s *Server) handleAuditLog(w http.ResponseWriter, r *http.Request) {
	s.render.render(w, "audit.html", struct{ Nav string }{Nav: "audit"})
}

// handleNotFound renders a styled 404 page for unmatched routes.
func (s *Server) handleNotFound(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNotFound)
	s.render.render(w, "not_found.html", struct{ Nav string }{Nav: ""})
}
