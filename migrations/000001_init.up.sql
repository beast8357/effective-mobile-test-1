CREATE TABLE IF NOT EXISTS subscriptions (
    id           UUID PRIMARY KEY,
    service_name TEXT NOT NULL,
    price        INTEGER NOT NULL CHECK (price >= 0),
    user_id      UUID NOT NULL,
    start_date   DATE NOT NULL,
    end_date     DATE,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK (end_date IS NULL OR end_date >= start_date)
);

CREATE INDEX IF NOT EXISTS idx_subscriptions_user_id      ON subscriptions(user_id);
CREATE INDEX IF NOT EXISTS idx_subscriptions_service_name ON subscriptions(service_name);
CREATE INDEX IF NOT EXISTS idx_subscriptions_period       ON subscriptions(start_date, end_date);
