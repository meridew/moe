package server

import (
	"context"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/dan/moe/internal/db"
	"github.com/dan/moe/internal/store"
	"github.com/dan/moe/web"
)

// Server holds the HTTP server and its dependencies.
type Server struct {
	db              *db.DB
	devices         *store.DeviceStore
	providerConfigs *store.ProviderConfigStore
	policies        *store.PolicyStore
	render          *renderer
	router          *http.ServeMux
	http            *http.Server
	status          *statusTracker
	activity        *activityLog
	stopHealth      chan struct{} // signals the health poller to stop
	shutdownCtx     context.Context
	shutdownCancel  context.CancelFunc
	bgWg            sync.WaitGroup // tracks in-flight background goroutines
}

// New creates a new Server wired to the given database. It sets up routes and
// middleware but does not start listening.
func New(database *db.DB, addr string) (*Server, error) {
	mux := http.NewServeMux()

	rn, err := newRenderer()
	if err != nil {
		return nil, fmt.Errorf("init renderer: %w", err)
	}

	shutdownCtx, shutdownCancel := context.WithCancel(context.Background())

	s := &Server{
		db:              database,
		devices:         store.NewDeviceStore(database.Conn),
		providerConfigs: store.NewProviderConfigStore(database.Conn),
		policies:        store.NewPolicyStore(database.Conn),
		render:          rn,
		router:          mux,
		status:          newStatusTracker(),
		activity:        newActivityLog(200),
		stopHealth:      make(chan struct{}),
		shutdownCtx:     shutdownCtx,
		shutdownCancel:  shutdownCancel,
		http: &http.Server{
			Addr:         addr,
			Handler:      mux,
			ReadTimeout:  15 * time.Second,
			WriteTimeout: 120 * time.Second, // generous for policy sync (9 endpoints)
			IdleTimeout:  60 * time.Second,
		},
	}

	s.routes()
	s.staticFiles()

	// Custom 404 handler for unmatched routes.
	notFoundHandler := http.HandlerFunc(s.handleNotFound)
	handler := notFound(mux, notFoundHandler)

	// Wrap with middleware (outermost runs first).
	s.http.Handler = logging(recovery(handler))

	return s, nil
}

// Start begins listening. It blocks until the server is shut down.
func (s *Server) Start() error {
	log.Printf("server listening on %s", s.http.Addr)
	return s.http.ListenAndServe()
}

// StartBackgroundJobs launches the health poller and any other recurring work.
// Call this before Start().
func (s *Server) StartBackgroundJobs() {
	// Mark any snapshots left in "capturing" state from a previous crash.
	recovered, err := s.policies.RecoverStaleCapturing("interrupted — server was stopped")
	if err != nil {
		log.Printf("[startup] recover stale snapshots: %v", err)
	} else if recovered > 0 {
		log.Printf("[startup] marked %d stale capturing snapshot(s) as error", recovered)
		s.activity.Logf("system", "warning", "Marked %d interrupted baseline capture(s) as failed", recovered)
	}

	go s.healthPoller()
	s.activity.Logf("system", "info", "MOE started — background health checks active")
}

// Shutdown gracefully shuts down the HTTP server and background jobs.
// It cancels any in-flight background tasks and waits for them to finish.
func (s *Server) Shutdown(ctx context.Context) error {
	close(s.stopHealth)

	// Signal all background goroutines to stop.
	s.shutdownCancel()

	// Wait for in-flight background work (snapshot captures, etc.) to finish
	// or the caller's context to expire.
	done := make(chan struct{})
	go func() {
		s.bgWg.Wait()
		close(done)
	}()

	select {
	case <-done:
		log.Println("[shutdown] all background tasks finished")
	case <-ctx.Done():
		log.Println("[shutdown] timed out waiting for background tasks")
	}

	return s.http.Shutdown(ctx)
}

// staticFiles registers the handler for serving embedded static assets.
func (s *Server) staticFiles() {
	// Sub into the "static" directory so URLs map as /static/css/style.css etc.
	sub, err := fs.Sub(web.StaticFS, "static")
	if err != nil {
		log.Fatalf("static fs: %v", err)
	}
	s.router.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(sub))))
}
