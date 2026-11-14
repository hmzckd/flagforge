-- +goose Up
ALTER TABLE flag_settings
    ADD COLUMN rules       jsonb NOT NULL DEFAULT '[]',
    ADD COLUMN fallthrough jsonb NOT NULL DEFAULT '{"value": true}';

-- +goose Down
ALTER TABLE flag_settings
    DROP COLUMN rules,
    DROP COLUMN fallthrough;
