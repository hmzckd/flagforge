-- +goose Up
CREATE TABLE environments (
    id   bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    key  text NOT NULL UNIQUE,
    name text NOT NULL
);

INSERT INTO environments (key, name) VALUES ('staging', 'Staging'), ('production', 'Production');

CREATE TABLE flag_settings (
    flag_id        bigint      NOT NULL REFERENCES flags (id) ON DELETE CASCADE,
    environment_id bigint      NOT NULL REFERENCES environments (id),
    enabled        boolean     NOT NULL DEFAULT false,
    updated_at     timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (flag_id, environment_id)
);

INSERT INTO flag_settings (flag_id, environment_id)
SELECT f.id, e.id FROM flags f CROSS JOIN environments e;

-- +goose Down
DROP TABLE flag_settings;
DROP TABLE environments;
