// Package maintenance provides the host-wide startup fence for a read-only cutover.
// It does not fence older binaries: operators must quiesce every old backend,
// worker and consumer before relying on this mode.
package maintenance

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

const ModeEnv = "WARMBLY_MAINTENANCE_MODE"
const ScopeEnv = "WARMBLY_MAINTENANCE_SCOPE"

// Parse accepts only the explicit host-wide health-only mode. An org-scoped
// setting cannot safely fence shared schedulers and is rejected, not ignored.
func Parse(mode, scope string) (bool, error) {
	if mode == "" && scope == "" {
		return false, nil
	}
	if mode == "health-only" && scope == "host" {
		return true, nil
	}
	return false, fmt.Errorf("invalid maintenance mode/scope: require %s=health-only and %s=host", ModeEnv, ScopeEnv)
}

func FromEnv() (bool, error) { return Parse(os.Getenv(ModeEnv), os.Getenv(ScopeEnv)) }

// HealthOnlyHandler exposes no application routes, task webhook, or probes
// that use a provider. GET/HEAD /health reports only process liveness.
func HealthOnlyHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" || (r.Method != http.MethodGet && r.Method != http.MethodHead) {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte("maintenance: health-only\n"))
		}
	})
}

func ServeHealthOnly(addr string) error {
	if addr == "" {
		addr = "0.0.0.0:8080"
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	srv := &http.Server{Addr: addr, Handler: HealthOnlyHandler(), ReadHeaderTimeout: 5 * time.Second}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()
	err := srv.ListenAndServe()
	if err == http.ErrServerClosed {
		return nil
	}
	return err
}

// WaitQuiescent keeps a fenced worker/consumer alive without opening a bus
// subscription or acknowledging any queued message.
func WaitQuiescent() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()
}
