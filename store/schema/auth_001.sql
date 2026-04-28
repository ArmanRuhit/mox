-- Schema for the auth database (component "auth", version 1).
-- Mirrors store.AuthDBTypes: TLSPublicKey, LoginAttempt, LoginAttemptState, AccountRemove.
-- Runner sets search_path before executing this file.

CREATE TABLE IF NOT EXISTS schema_version (
    component TEXT NOT NULL,
    version INTEGER NOT NULL,
    applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (component, version)
);

CREATE TABLE IF NOT EXISTS tlspublickey (
    fingerprint TEXT PRIMARY KEY,
    created TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    type TEXT NOT NULL,
    name TEXT NOT NULL,
    no_imap_preauth BOOLEAN NOT NULL DEFAULT FALSE,
    cert_der BYTEA NOT NULL,
    account TEXT NOT NULL,
    login_address TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS tlspublickey_account_idx ON tlspublickey (account);

CREATE TABLE IF NOT EXISTS loginattempt (
    key BYTEA PRIMARY KEY,
    last TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    first TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    count BIGINT NOT NULL DEFAULT 0,
    account_name TEXT NOT NULL,
    login_address TEXT NOT NULL DEFAULT '',
    remote_ip TEXT NOT NULL DEFAULT '',
    local_ip TEXT NOT NULL DEFAULT '',
    tls TEXT NOT NULL DEFAULT '',
    tls_pubkey_fingerprint TEXT NOT NULL DEFAULT '',
    protocol TEXT NOT NULL DEFAULT '',
    user_agent TEXT NOT NULL DEFAULT '',
    auth_mech TEXT NOT NULL DEFAULT '',
    result TEXT NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS loginattempt_last_idx ON loginattempt (last);
CREATE INDEX IF NOT EXISTS loginattempt_accountname_last_idx ON loginattempt (account_name, last);

CREATE TABLE IF NOT EXISTS loginattemptstate (
    account_name TEXT PRIMARY KEY,
    records_failed INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS accountremove (
    account_name TEXT PRIMARY KEY
);
