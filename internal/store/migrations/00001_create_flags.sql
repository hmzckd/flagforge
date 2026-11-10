-- +goose Up
CREATE TABLE flags (
    id          bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    key         text        NOT NULL UNIQUE CHECK (key ~ '^[a-z0-9][a-z0-9-]{0,63}$'),
    name        text        NOT NULL,
    description text        NOT NULL DEFAULT '',
    created_at  timestamptz NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE flags;
