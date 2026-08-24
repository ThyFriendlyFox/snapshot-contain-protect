package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"
)

// Serve runs the daemon until ctx is cancelled. It refuses any address that
// is not loopback: the service takes no authentication, so it must not be
// reachable from the network.
func Serve(ctx context.Context, svc *Service, log *slog.Logger) error {
	if err := checkLoopback(svc.cfg.Addr); err != nil {
		return err
	}
	srv := &http.Server{
		Addr:              svc.cfg.Addr,
		Handler:           svc.Handler(log),
		ReadHeaderTimeout: 5 * time.Second,
	}
	ln, err := net.Listen("tcp", svc.cfg.Addr)
	if err != nil {
		return err
	}
	log.Info("listening", "addr", ln.Addr().String(), "backend", svc.Backend(), "data_dir", svc.cfg.DataDir)

	done := make(chan error, 1)
	go func() {
		err := srv.Serve(ln)
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
		done <- err
	}()

	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return srv.Shutdown(shutdown)
	}
}

func checkLoopback(addr string) error {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("address %q needs the form host:port", addr)
	}
	if host == "localhost" || strings.EqualFold(host, "ip6-localhost") {
		return nil
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return fmt.Errorf("address %q is not loopback; the daemon has no authentication", addr)
	}
	return nil
}
