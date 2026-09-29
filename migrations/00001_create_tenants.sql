-- +goose Up
CREATE TABLE tenants (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name       TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Seed a default tenant so the API works immediately
INSERT INTO tenants (id, name) VALUES ('00000000-0000-0000-0000-000000000001', 'default');

-- +goose Down
DROP TABLE tenants;
