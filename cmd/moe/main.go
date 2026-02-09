package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/dan/moe/internal/db"
	"github.com/dan/moe/internal/server"
)

func main() {
	addr := flag.String("addr", ":8080", "HTTP listen address")
	dbPath := flag.String("db", "moe.db", "path to SQLite database file")
	flag.Parse()

	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)
	log.Println("starting MOE — Mobile Operations Engine")

	// ── Database ────────────────────────────────────────────────────────
	database, err := db.New(*dbPath)
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer database.Close()

	if err := database.Migrate(); err != nil {
		log.Fatalf("migrations: %v", err)
	}

	// ── HTTP Server ─────────────────────────────────────────────────────
	srv, err := server.New(database, *addr)
	if err != nil {
		log.Fatalf("server: %v", err)
	}

	// Start server in a goroutine so we can listen for shutdown signals.
	srv.StartBackgroundJobs()
	go func() {
		if err := srv.Start(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server: %v", err)
		}
	}()

	// ── Graceful Shutdown ───────────────────────────────────────────────
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit
	log.Printf("received %s, shutting down...", sig)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("shutdown: %v", err)
	}

	log.Println("shutdown complete")
}
