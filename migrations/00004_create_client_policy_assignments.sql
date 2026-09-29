-- +goose Up
CREATE TABLE client_policy_assignments (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id     UUID NOT NULL REFERENCES tenants(id),
    client_id     TEXT NOT NULL,
    policy_id     UUID NOT NULL REFERENCES policies(id),
    override_json JSONB,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (tenant_id, client_id)
);

CREATE INDEX idx_assignments_tenant_client ON client_policy_assignments(tenant_id, client_id);

-- +goose Down
DROP TABLE client_policy_assignments;
