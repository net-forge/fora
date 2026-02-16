package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"fora/internal/api"
	"fora/internal/db"
)

const serverVersion = "0.1.0-dev"

func main() {
	if len(os.Args) > 1 && os.Args[1] == "import" {
		if err := runImport(os.Args[2:]); err != nil {
			log.Fatalf("import failed: %v", err)
		}
		return
	}

	var (
		port        = flag.String("port", "8080", "HTTP listen port")
		dbPath      = flag.String("db", "./fora.db", "path to SQLite database")
		adminKeyOut = flag.String("admin-key-out", "", "write bootstrap admin API key to this file if no admin exists")
	)
	flag.Parse()

	database, err := db.Open(*dbPath)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer database.Close()

	if err := db.ApplyMigrations(database); err != nil {
		log.Fatalf("apply migrations: %v", err)
	}

	if *adminKeyOut != "" {
		adminName, err := db.EnsureBootstrapAdmin(database, *adminKeyOut)
		if err != nil {
			log.Fatalf("bootstrap admin: %v", err)
		}
		if adminName != "" {
			log.Printf("bootstrap admin %q created", adminName)
		}
	}

	mux := api.NewRouter(database, serverVersion)

	server := &http.Server{
		Addr:         ":" + *port,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 0,
		IdleTimeout:  60 * time.Second,
	}

	shutdownDone := make(chan struct{})
	go func() {
		defer close(shutdownDone)
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
		<-sigCh

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(ctx); err != nil {
			log.Printf("graceful shutdown failed: %v", err)
		}
	}()

	log.Printf("fora-server listening on %s", server.Addr)
	err = server.ListenAndServe()
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("server error: %v", err)
	}
	<-shutdownDone
}

func runImport(args []string) error {
	fs := flag.NewFlagSet("import", flag.ContinueOnError)
	fromPath := fs.String("from", "", "path to json export file or markdown export directory")
	dbPath := fs.String("db", "./fora.db", "path to SQLite database")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *fromPath == "" {
		return errors.New("missing --from")
	}

	database, err := db.Open(*dbPath)
	if err != nil {
		return err
	}
	defer database.Close()

	if err := db.ApplyMigrations(database); err != nil {
		return err
	}
	if err := db.ImportFromPath(context.Background(), database, *fromPath); err != nil {
		return err
	}
	log.Printf("import complete from %s into %s", *fromPath, *dbPath)
	return nil
}
