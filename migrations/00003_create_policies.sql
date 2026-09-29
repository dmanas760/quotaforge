-- +goose Up
CREATE TABLE policies (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id      UUID NOT NULL REFERENCES tenants(id),
    name           TEXT NOT NULL,
    algorithm      TEXT NOT NULL CHECK (algorithm IN ('token_bucket', 'sliding_window')),
    capacity       BIGINT NOT NULL CHECK (capacity > 0),
    refill_rate    DOUBLE PRECISION NOT NULL DEFAULT 0,   -- tokens per second (token bucket)
    window_seconds BIGINT NOT NULL DEFAULT 0,             -- interval (sliding window)
    burst_capacity BIGINT NOT NULL DEFAULT 0,
    enabled        BOOLEAN NOT NULL DEFAULT TRUE,
    version        INT NOT NULL DEFAULT 1,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_policies_tenant ON policies(tenant_id);

-- +goose Down
DROP TABLE policies;
