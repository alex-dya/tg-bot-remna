package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	BotToken    string
	AdminIDs    map[int64]struct{}
	DatabaseURL string

	RemnawaveURL       string
	RemnawaveToken     string
	RemnawaveSquadUUID string

	SubDays      int
	SubDevices   int
	SubTrafficGB int64

	// Опциональная ссылка, которая отправляется сотруднику вместе с подпиской —
	// страница с инструкцией: как установить клиент, как добавить ссылку и т.д.
	SubscriptionInfoURL string
}

func Load() (*Config, error) {
	cfg := &Config{
		BotToken:           must("BOT_TOKEN"),
		DatabaseURL:        must("DATABASE_URL"),
		RemnawaveURL:       strings.TrimRight(must("REMNAWAVE_URL"), "/"),
		RemnawaveToken:     must("REMNAWAVE_TOKEN"),
		RemnawaveSquadUUID: must("REMNAWAVE_SQUAD_UUID"),
		SubDays:             getInt("SUB_DAYS", 365),
		SubDevices:          getInt("SUB_DEVICES", 5),
		SubTrafficGB:        int64(getInt("SUB_TRAFFIC_GB", 0)),
		SubscriptionInfoURL: strings.TrimSpace(os.Getenv("SUBSCRIPTION_INFO_URL")),
	}

	adminRaw := must("ADMIN_IDS")
	cfg.AdminIDs = make(map[int64]struct{})
	for _, p := range strings.Split(adminRaw, ",") {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		id, err := strconv.ParseInt(p, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid ADMIN_IDS entry %q: %w", p, err)
		}
		cfg.AdminIDs[id] = struct{}{}
	}
	if len(cfg.AdminIDs) == 0 {
		return nil, fmt.Errorf("ADMIN_IDS is empty")
	}

	return cfg, nil
}

func (c *Config) IsAdmin(id int64) bool {
	_, ok := c.AdminIDs[id]
	return ok
}

func must(key string) string {
	v := os.Getenv(key)
	if v == "" {
		panic("missing required env: " + key)
	}
	return v
}

func getInt(key string, def int) int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}
