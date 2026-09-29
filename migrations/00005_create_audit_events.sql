-- +goose Up
CREATE TABLE audit_events (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id     UUID NOT NULL REFERENCES tenants(id),
    actor_key_id  UUID REFERENCES service_api_keys(id),
    action        TEXT NOT NULL,         -- e.g. "policy.create"
    resource_type TEXT NOT NULL,         -- e.g. "policy"
    resource_id   UUID NOT NULL,
    before_json   JSONB,
    after_json    JSONB,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_audit_events_tenant_time ON audit_events(tenant_id, created_at DESC);

-- +goose Down
DROP TABLE audit_events;
