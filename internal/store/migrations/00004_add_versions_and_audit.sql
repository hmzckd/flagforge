-- +goose Up
ALTER TABLE flag_settings ADD COLUMN version bigint NOT NULL DEFAULT 1;

CREATE TABLE audit_entries (
    id             bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    flag_id        bigint      NOT NULL REFERENCES flags (id) ON DELETE CASCADE,
    environment_id bigint      REFERENCES environments (id),
    actor          text        NOT NULL,
    action         text        NOT NULL,
    before         jsonb,
    after          jsonb       NOT NULL,
    created_at     timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX audit_entries_flag_id_idx ON audit_entries (flag_id, id DESC);

-- +goose Down
DROP TABLE audit_entries;
ALTER TABLE flag_settings DROP COLUMN version;
