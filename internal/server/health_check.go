package server

import (
	"context"
	"log"
	"sync"
	"time"
)

const healthCheckInterval = 2 * time.Minute
const healthCheckTimeout = 15 * time.Second

// healthPoller runs in a goroutine and periodically checks all enabled
// providers in parallel, updating the status tracker and activity log.
func (s *Server) healthPoller() {
	// Run an initial check immediately after startup.
	s.checkAllProviders()

	ticker := time.NewTicker(healthCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopHealth:
			log.Println("[health] poller stopped")
			return
		case <-ticker.C:
			s.checkAllProviders()
		}
	}
}

// checkAllProviders tests connectivity to every enabled provider in parallel.
func (s *Server) checkAllProviders() {
	configs, err := s.providerConfigs.ListEnabled()
	if err != nil {
		log.Printf("[health] failed to list providers: %v", err)
		return
	}

	if len(configs) == 0 {
		return
	}

	log.Printf("[health] checking %d provider(s)…", len(configs))
	s.activity.Logf("system", "info", "Health check started for %d provider(s)", len(configs))

	var wg sync.WaitGroup
	for _, cfg := range configs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.checkProvider(cfg.Name, cfg.Type)
		}()
	}
	wg.Wait()

	s.activity.Logf("system", "info", "Health check complete")
}

// checkProvider tests a single provider and updates the status tracker.
func (s *Server) checkProvider(name, providerType string) {
	// Mark as checking.
	s.status.Set(&ProviderStatus{
		Name:      name,
		Type:      providerType,
		Status:    "checking",
		CheckedAt: time.Now().UTC(),
	})

	cfg, err := s.providerConfigs.GetByName(name)
	if err != nil || cfg == nil {
		s.status.Set(&ProviderStatus{
			Name:      name,
			Type:      providerType,
			Status:    "error",
			Error:     "provider config not found",
			CheckedAt: time.Now().UTC(),
		})
		s.activity.Logf(name, "error", "Config not found")
		return
	}

	p, err := s.buildProvider(cfg)
	if err != nil {
		fails := cfg.ConsecFails + 1
		s.status.Set(&ProviderStatus{
			Name:        name,
			Type:        providerType,
			Status:      "error",
			Error:       err.Error(),
			CheckedAt:   time.Now().UTC(),
			ConsecFails: fails,
		})
		_ = s.providerConfigs.RecordCheckResult(name, false, err.Error(), fails)
		s.activity.Logf(name, "error", "Build failed: %s", err)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), healthCheckTimeout)
	defer cancel()

	start := time.Now()
	checkErr := p.TestConnection(ctx)
	latency := time.Since(start)

	if checkErr != nil {
		fails := cfg.ConsecFails + 1
		s.status.Set(&ProviderStatus{
			Name:        name,
			Type:        providerType,
			Status:      "error",
			Error:       checkErr.Error(),
			CheckedAt:   time.Now().UTC(),
			Latency:     latency,
			ConsecFails: fails,
		})
		_ = s.providerConfigs.RecordCheckResult(name, false, checkErr.Error(), fails)
		s.activity.Logf(name, "error", "Connection failed (%s): %s", latency.Round(time.Millisecond), checkErr)
		log.Printf("[health] %s: FAIL (%s) — %v", name, latency.Round(time.Millisecond), checkErr)
	} else {
		s.status.Set(&ProviderStatus{
			Name:      name,
			Type:      providerType,
			Status:    "connected",
			CheckedAt: time.Now().UTC(),
			Latency:   latency,
		})
		_ = s.providerConfigs.RecordCheckResult(name, true, "", 0)
		s.activity.Logf(name, "success", "Connected (%s)", latency.Round(time.Millisecond))
		log.Printf("[health] %s: OK (%s)", name, latency.Round(time.Millisecond))
	}
}

// CheckProviderNow runs an immediate health check for a single provider
// (used by the "Test Connection" button).
func (s *Server) CheckProviderNow(name, providerType string) {
	s.activity.Logf(name, "info", "Manual connection test…")
	s.checkProvider(name, providerType)
}
