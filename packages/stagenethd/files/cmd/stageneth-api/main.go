// stageneth-api serves the StageNeth management API on a unix socket,
// intended to sit behind nginx (TLS termination + static UI).
package main

import (
	"context"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"os/user"
	"strconv"
	"syscall"
	"time"

	"stageneth/internal/api"
	"stageneth/internal/auth"
	"stageneth/internal/mon"
)

func socketPath() string {
	if p := os.Getenv("STAGENETH_SOCKET"); p != "" {
		return p
	}
	return "/var/run/stageneth-api.sock"
}

func configPath() string {
	if p := os.Getenv("STAGENETHD_CONFIG"); p != "" {
		return p
	}
	return "/etc/config/stagenethd"
}

func main() {
	sock := socketPath()
	os.Remove(sock) // stale socket from previous run
	l, err := net.Listen("unix", sock)
	if err != nil {
		log.Fatal("listen:", err)
	}
	if err := os.Chmod(sock, 0660); err != nil {
		log.Fatal("chmod:", err)
	}
	// nginx workers are not root — hand the socket to their group so the
	// proxy can connect. Try known candidates; on a root-run nginx the
	// group change is harmless anyway.
	for _, gname := range []string{"nginx", "nogroup", "nobody", "www-data"} {
		if g, err := user.LookupGroup(gname); err == nil {
			gid, _ := strconv.Atoi(g.Gid)
			if err := os.Chown(sock, -1, gid); err == nil {
				break
			}
		}
	}

	api.Init(auth.NewUsers(configPath()), auth.NewStore())
	api.InitMetrics(configPath())

	collector := mon.NewCollector(5*time.Second, 30*time.Second)
	go collector.Run()
	api.InitMonitor(collector)

	mux := http.NewServeMux()
	api.Register(mux)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, 8<<20)
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Cache-Control", "no-store")
		mux.ServeHTTP(w, r)
	})

	srv := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      120 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		srv.Shutdown(shutdownCtx)
	}()

	log.Println("StageNeth API listening on unix:" + sock)
	if err := srv.Serve(l); err != http.ErrServerClosed {
		log.Fatal(err)
	}
}
