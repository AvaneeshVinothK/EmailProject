CREATE TABLE users (
    id         SERIAL PRIMARY KEY,
    email      TEXT UNIQUE NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE email_accounts (
    id                    SERIAL PRIMARY KEY,
    user_id               INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    provider              TEXT NOT NULL DEFAULT 'gmail',
    email_address         TEXT NOT NULL,
    refresh_token         TEXT,
    last_history_id       TEXT,
    backfill_completed_at TIMESTAMPTZ,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (user_id, email_address)
);

CREATE TABLE emails (
    id                SERIAL PRIMARY KEY,
    email_account_id  INTEGER NOT NULL REFERENCES email_accounts(id) ON DELETE CASCADE,
    gmail_message_id  TEXT NOT NULL,
    sender            TEXT NOT NULL DEFAULT '',
    subject           TEXT NOT NULL DEFAULT '',
    body              TEXT NOT NULL DEFAULT '',
    received_at       TIMESTAMPTZ NOT NULL,
    UNIQUE (email_account_id, gmail_message_id)
);

CREATE INDEX idx_emails_account_received ON emails (email_account_id, received_at DESC);

CREATE TABLE categories (
    id   SERIAL PRIMARY KEY,
    name TEXT UNIQUE NOT NULL
);

INSERT INTO categories (name) VALUES
    ('confirmation'),
    ('next_steps'),
    ('recruiter_reach_out'),
    ('online_assessment'),
    ('interview');