package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// Exercise the actual entrypoints: the fence must run before configuration,
// background jobs and the bus, not merely exist as an unused helper.
func TestMaintenanceStartupFence(t *testing.T) {
	for _, role := range []string{"backend", "worker", "consumer"} {
		t.Run(role, func(t *testing.T) {
			binary := filepath.Join(t.TempDir(), role)
			build := exec.Command("go", "build", "-o", binary, "./cmd/"+role)
			build.Dir = "../.."
			if output, err := build.CombinedOutput(); err != nil {
				t.Fatalf("build %s: %v: %s", role, err, output)
			}

			addr := ""
			if role == "backend" {
				ln, err := net.Listen("tcp", "127.0.0.1:0")
				if err != nil {
					t.Fatal(err)
				}
				addr = ln.Addr().String()
				_ = ln.Close()
			}
			for restart := 0; restart < 2; restart++ {
				t.Run(fmt.Sprintf("boot-%d", restart), func(t *testing.T) {
					ctx, cancel := context.WithCancel(context.Background())
					defer cancel()
					cmd := exec.CommandContext(ctx, binary)
					cmd.Env = append(os.Environ(), "WARMBLY_MAINTENANCE_MODE=health-only", "WARMBLY_MAINTENANCE_SCOPE=host", "API_HOST="+addr, "NATS_URL=nats://127.0.0.1:1", "PRIMARY_DB=postgres://127.0.0.1:1/invalid")
					if err := cmd.Start(); err != nil {
						t.Fatal(err)
					}
					defer func() { cancel(); _ = cmd.Wait() }()
					if role == "backend" {
						client := &http.Client{Timeout: 100 * time.Millisecond}
						deadline := time.Now().Add(5 * time.Second)
						for {
							req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+addr+"/health", nil)
							if err != nil {
								t.Fatal(err)
							}
							resp, err := client.Do(req)
							if err == nil {
								_ = resp.Body.Close()
								if resp.StatusCode != http.StatusOK {
									t.Fatalf("health: %d", resp.StatusCode)
								}
								break
							}
							if time.Now().After(deadline) {
								t.Fatalf("health endpoint never started: %v", err)
							}
							time.Sleep(20 * time.Millisecond)
						}
						for _, path := range []string{"/api/v1/tasks/email", "/api/v1/emails", "/metrics"} {
							req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://"+addr+path, nil)
							if err != nil {
								t.Fatal(err)
							}
							resp, err := client.Do(req)
							if err != nil {
								t.Fatal(err)
							}
							_ = resp.Body.Close()
							if resp.StatusCode != http.StatusNotFound {
								t.Fatalf("%s exposed: %d", path, resp.StatusCode)
							}
						}
					} else {
						time.Sleep(200 * time.Millisecond)
						if err := cmd.Process.Signal(syscall.Signal(0)); err != nil {
							t.Fatalf("%s exited before signal: %v", role, err)
						}
					}
				})
			}
		})
	}
}
