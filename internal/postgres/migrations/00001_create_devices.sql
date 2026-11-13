-- +goose Up
CREATE TYPE device_state AS ENUM ('available', 'in-use', 'inactive');

CREATE TABLE devices (
    id         uuid         PRIMARY KEY,
    name       text         NOT NULL,
    brand      text         NOT NULL,
    state      device_state NOT NULL,
    created_at timestamptz  NOT NULL DEFAULT now(),
    updated_at timestamptz,
    deleted_at timestamptz
);

CREATE INDEX devices_active_id_idx ON devices (id) WHERE deleted_at IS NULL;
CREATE INDEX devices_active_brand_idx ON devices (brand, id) WHERE deleted_at IS NULL;
CREATE INDEX devices_active_state_idx ON devices (state, id) WHERE deleted_at IS NULL;

-- +goose Down
DROP TABLE devices;
DROP TYPE device_state;
