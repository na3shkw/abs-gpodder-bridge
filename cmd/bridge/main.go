// Command bridge は gPodder 互換クライアントと Audiobookshelf の再生状況を同期する HTTP サーバー。
package main

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strconv"

	"abs-gpodder-bridge/internal/bridge"
)

func main() {
	cfg, err := bridge.LoadConfig(os.Getenv)
	if err != nil {
		slog.New(slog.NewTextHandler(os.Stdout, nil)).Error("invalid configuration", "error", err)
		os.Exit(1)
	}
	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: cfg.LogLevel,
		// ログの時刻は TZ で出す。
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if a.Key == slog.TimeKey && len(groups) == 0 {
				a.Value = slog.TimeValue(a.Value.Time().In(cfg.Location))
			}
			return a
		},
	}))

	ln, err := net.Listen("tcp", ":"+strconv.Itoa(cfg.ListenPort))
	if err != nil {
		log.Error("listen failed", "error", err)
		os.Exit(1)
	}
	srv := bridge.NewServer(cfg, log)
	go srv.RunIndex(context.Background())

	log.Info("listening", "port", cfg.ListenPort)
	if err := http.Serve(ln, srv); err != nil {
		log.Error("server stopped", "error", err)
		os.Exit(1)
	}
}
