package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/WaltzForLovers/Slate/internal/apperr"
	"github.com/WaltzForLovers/Slate/internal/auth"
	"github.com/WaltzForLovers/Slate/internal/catalog"
	"github.com/WaltzForLovers/Slate/internal/httpapi"
	"github.com/WaltzForLovers/Slate/internal/queue"
	"github.com/WaltzForLovers/Slate/internal/storage"
	"github.com/WaltzForLovers/Slate/web"
)

func main() {
	log.SetFlags(0)
	addr := os.Getenv("SLATE_ADDR")
	if addr == "" {
		addr = "127.0.0.1:8080"
	}
	dbPath := os.Getenv("SLATE_DB")
	if dbPath == "" {
		exe, err := os.Executable()
		if err != nil {
			fmt.Fprintln(os.Stderr, apperr.Internal(err).Message)
			log.Printf("%v", err)
			os.Exit(1)
		}
		dbPath = filepath.Join(filepath.Dir(exe), "queue.db")
	}

	db, err := storage.Open(dbPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, apperr.From(err).Message)
		log.Printf("%v", err)
		os.Exit(1)
	}
	defer db.Close()

	catalogURL := os.Getenv("SLATE_CATALOG")
	if catalogURL == "" {
		catalogURL = "https://api.tvmaze.com"
	}
	handler := httpapi.New(
		auth.New(db),
		catalog.New(db, catalog.NewHTTPClient(catalogURL)),
		queue.New(db),
		os.Stderr,
		web.Files,
	)
	if err := httpapi.ListenAndServe(addr, handler); err != nil {
		fmt.Fprintln(os.Stderr, apperr.From(err).Message)
		if apperr.From(err).Code == apperr.CodeInternal {
			log.Printf("%v", err)
		}
		os.Exit(1)
	}
}
