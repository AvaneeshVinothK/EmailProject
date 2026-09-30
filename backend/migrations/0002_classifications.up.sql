-- 0002_classifications.up.sql
CREATE TABLE classifications (
    id            SERIAL PRIMARY KEY,
    email_id      INTEGER NOT NULL REFERENCES emails(id) ON DELETE CASCADE,
    category_id   INTEGER NOT NULL REFERENCES categories(id),
    confidence    REAL NOT NULL,
    model_version TEXT NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (email_id)
);