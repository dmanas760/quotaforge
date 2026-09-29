-- +goose Up
CREATE TABLE service_api_keys (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id  UUID NOT NULL REFERENCES tenants(id),
    key_prefix TEXT NOT NULL UNIQUE,          -- first 8 chars, safe to log/display
    key_hash   TEXT NOT NULL,                 -- argon2id hash, never returned in API responses
    status     TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'revoked')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    revoked_at TIMESTAMPTZ
);

CREATE INDEX idx_service_api_keys_prefix ON service_api_keys(key_prefix);
CREATE INDEX idx_service_api_keys_tenant ON service_api_keys(tenant_id);

-- +goose Down
DROP TABLE service_api_keys;
