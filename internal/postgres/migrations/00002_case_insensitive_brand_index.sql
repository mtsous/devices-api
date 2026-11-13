-- +goose Up
DROP INDEX devices_active_brand_idx;
CREATE INDEX devices_active_brand_lower_idx ON devices (lower(brand), id) WHERE deleted_at IS NULL;

-- +goose Down
DROP INDEX devices_active_brand_lower_idx;
CREATE INDEX devices_active_brand_idx ON devices (brand, id) WHERE deleted_at IS NULL;
