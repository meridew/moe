package server

import (
	"encoding/json"
	"net/http"
)

// healthResponse is the JSON shape returned by the health endpoint.
type healthResponse struct {
	Status     string `json:"status"`
	DB         string `json:"db"`
	Migrations int    `json:"migrations_applied"`
}

// handleHealth reports whether the server and database are operational.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	resp := healthResponse{
		Status: "ok",
		DB:     "connected",
	}

	if err := s.db.Ping(); err != nil {
		resp.Status = "degraded"
		resp.DB = "error: " + err.Error()
		w.WriteHeader(http.StatusServiceUnavailable)
	}

	count, err := s.db.MigrationCount()
	if err == nil {
		resp.Migrations = count
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}
