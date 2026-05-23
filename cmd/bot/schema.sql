-- Сотрудники, которым админ выдал доступ.
-- username хранится в нижнем регистре (Telegram usernames регистронезависимы).
CREATE TABLE IF NOT EXISTS bot_employees (
    username         TEXT PRIMARY KEY,
    remnawave_uuid   TEXT NOT NULL,
    subscription_url TEXT NOT NULL,
    telegram_id      BIGINT,
    activated_at     TIMESTAMPTZ,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_by       BIGINT NOT NULL,
    expires_at       TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_bot_employees_tg ON bot_employees(telegram_id)
    WHERE telegram_id IS NOT NULL;
