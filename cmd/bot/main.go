package main

import (
	"context"
	"errors"
	_ "embed"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"corp-vpn-bot/internal/config"
	"corp-vpn-bot/internal/db"
	"corp-vpn-bot/internal/handlers"
	"corp-vpn-bot/internal/remnawave"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

//go:embed schema.sql
var schemaSQL string

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	cfg, err := config.Load()
	if err != nil {
		logger.Error("config", "err", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	database, err := db.New(ctx, cfg.DatabaseURL)
	if err != nil {
		logger.Error("db connect", "err", err)
		os.Exit(1)
	}
	defer database.Close()

	if err := database.Migrate(ctx, schemaSQL); err != nil {
		logger.Error("migrate", "err", err)
		os.Exit(1)
	}

	rw := remnawave.New(cfg.RemnawaveURL, cfg.RemnawaveToken, cfg.RemnawaveSquadUUID)
	h := handlers.New(cfg, database, rw, logger)

	opts := []bot.Option{
		bot.WithDefaultHandler(func(ctx context.Context, b *bot.Bot, u *models.Update) {
			h.Fallback(ctx, b, u)
		}),
	}
	b, err := bot.New(cfg.BotToken, opts...)
	if err != nil {
		logger.Error("bot init", "err", err)
		os.Exit(1)
	}
	h.Register(b)

	// Меню команд: для админов — полный набор, для остальных — только /start
	if err := h.SetupMenus(ctx, b); err != nil {
		logger.Warn("setup menus", "err", err)
	}

	logger.Info("bot started", "admins", len(cfg.AdminIDs))
	b.Start(ctx)

	if ctx.Err() != nil && !errors.Is(ctx.Err(), context.Canceled) {
		logger.Error("ctx", "err", ctx.Err())
	}
	logger.Info("shutdown")
}
