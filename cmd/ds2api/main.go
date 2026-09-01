// ds2api is a slim OpenAI-compatible gateway for DeepSeek.
package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"ds2api/internal/config"
	"ds2api/internal/server"
)

func main() {
	if err := run(); err != nil {
		log.Fatalf("ds2api: %v", err)
	}
}

func run() error {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	store := config.LoadStore()
	_ = store

	app := server.NewApp(ctx, store)
	srv := &http.Server{
		Addr:              listenAddr(),
		Handler:           app.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Printf("ds2api: listening on %s", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	hup := make(chan os.Signal, 1)
	signal.Notify(hup, syscall.SIGHUP)
	go func() {
		for range hup {
			log.Print("ds2api: SIGHUP received, reloading config")
			if err := store.ReloadFromFile(); err != nil {
				log.Printf("ds2api: reload failed: %v", err)
			}
		}
	}()

	if os.Getenv("DS2API_HOT_RELOAD") == "true" && !store.IsEnvBacked() {
		go watchConfigFile(hup)
	}

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		log.Print("ds2api: shutdown signal received")
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	return srv.Shutdown(shutdownCtx)
}

func listenAddr() string {
	if v := os.Getenv("DS2API_LISTEN"); v != "" {
		return v
	}
	return ":8080"
}

// watchConfigFile polls the config file mtime every 5s and sends a SIGHUP-equivalent
// reload signal on change. DS2API_HOT_RELOAD=true must be set.
// The watcher only fires when the file's mtime advances; it is best-effort and
// does not attempt to parse the file (ReloadFromFile handles validation).
func watchConfigFile(hup chan<- os.Signal) {
	const interval = 5 * time.Second
	cfgPath := os.Getenv("DS2API_CONFIG_PATH")
	if cfgPath == "" {
		return
	}
	var lastMod time.Time
	for {
		time.Sleep(interval)
		info, err := os.Stat(cfgPath)
		if err != nil {
			continue
		}
		mod := info.ModTime()
		if !lastMod.IsZero() && mod.After(lastMod) {
			hup <- syscall.SIGHUP
		}
		lastMod = mod
	}
}
