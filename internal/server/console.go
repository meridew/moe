package server

import (
	"fmt"
	"net/http"
)

// ── Template data ───────────────────────────────────────────────────────

type consoleData struct {
	Nav      string
	Statuses map[string]*ProviderStatus
	Events   []ActivityEvent
	Seq      int64
}

// handleConsole renders the full console page.
func (s *Server) handleConsole(w http.ResponseWriter, r *http.Request) {
	s.render.render(w, "console.html", consoleData{
		Nav:      "console",
		Statuses: s.status.All(),
		Events:   s.activity.Recent(100),
		Seq:      s.activity.Seq(),
	})
}

// handleConsoleEvents returns just the activity log rows as an HTML fragment,
// for htmx polling. It returns 204 No Content if nothing has changed (htmx
// will skip swapping).
func (s *Server) handleConsoleEvents(w http.ResponseWriter, r *http.Request) {
	// htmx sends the last known seq as a query param.
	lastSeq := r.URL.Query().Get("seq")
	currentSeq := s.activity.Seq()

	if lastSeq == fmt.Sprintf("%d", currentSeq) {
		w.WriteHeader(http.StatusNoContent) // 204 — htmx skips swap
		return
	}

	s.render.renderBlock(w, "console.html", "event-rows", struct {
		Events []ActivityEvent
		Seq    int64
	}{
		Events: s.activity.Recent(100),
		Seq:    currentSeq,
	})
}

// handleConsoleStatuses returns just the provider status cards as an HTML
// fragment for htmx polling.
func (s *Server) handleConsoleStatuses(w http.ResponseWriter, r *http.Request) {
	s.render.renderBlock(w, "console.html", "status-cards-inner", struct {
		Statuses map[string]*ProviderStatus
	}{
		Statuses: s.status.All(),
	})
}

// handleProviderTest triggers an immediate connection test for a provider
// and redirects back. POST /providers/{id}/test
func (s *Server) handleProviderTest(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	cfg, err := s.providerConfigs.GetByID(id)
	if err != nil || cfg == nil {
		http.Error(w, "Provider not found", http.StatusNotFound)
		return
	}

	// Run the check synchronously (fast — just an auth call).
	s.CheckProviderNow(cfg.Name, cfg.Type)

	status := s.status.Get(cfg.Name)
	if status != nil && status.Status == "connected" {
		http.Redirect(w, r, "/providers?flash="+cfg.Name+"+connected+successfully&flash_type=success", http.StatusSeeOther)
	} else {
		errMsg := "unknown error"
		if status != nil {
			errMsg = status.Error
		}
		http.Redirect(w, r, "/providers?flash="+cfg.Name+": "+errMsg+"&flash_type=error", http.StatusSeeOther)
	}
}
