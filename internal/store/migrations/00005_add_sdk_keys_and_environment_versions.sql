-- +goose Up
ALTER TABLE environments ADD COLUMN version bigint NOT NULL DEFAULT 1;

CREATE TABLE sdk_keys (
    id             bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    environment_id bigint      NOT NULL REFERENCES environments (id),
    key_hash       bytea       NOT NULL UNIQUE,
    prefix         text        NOT NULL,
    created_at     timestamptz NOT NULL DEFAULT now(),
    revoked_at     timestamptz
);

-- +goose Down
DROP TABLE sdk_keys;
ALTER TABLE environments DROP COLUMN version;
