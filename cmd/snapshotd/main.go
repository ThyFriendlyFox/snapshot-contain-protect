// Command snapshotd is the local snapshot service. It listens on loopback and
// answers the five verbs an agent needs: snapshot, list, diff, restore, prune.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/ThyFriendlyFox/snapshot-contain-protect/internal/api"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "snapshotd: "+err.Error())
		os.Exit(1)
	}
}

func run() error {
	cfg := api.DefaultConfig()
	flag.StringVar(&cfg.Addr, "addr", envOr("SNAPSHOT_ADDR", cfg.Addr), "loopback address to listen on")
	flag.StringVar(&cfg.DataDir, "data-dir", cfg.DataDir, "directory for snapshots/ and snapshot.db")
	flag.StringVar(&cfg.Backend, "backend", envOr("SNAPSHOT_BACKEND", cfg.Backend), "auto, btrfs, copy, apfs or vss")
	flag.IntVar(&cfg.AutoKeep, "auto-keep", cfg.AutoKeep, "auto snapshots kept per workset")
	flag.DurationVar(&cfg.PruneInterval, "prune-interval", cfg.PruneInterval, "how often retention runs; 0 turns it off")
	verbose := flag.Bool("verbose", false, "log every request at debug level")
	flag.Parse()

	level := slog.LevelInfo
	if *verbose {
		level = slog.LevelDebug
	}
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	svc, err := api.NewService(ctx, cfg)
	if err != nil {
		return err
	}
	defer svc.Close()

	return api.Serve(ctx, svc, log)
}

func envOr(name, fallback string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return fallback
}
