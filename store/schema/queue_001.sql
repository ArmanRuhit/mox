-- Schema for the queue database (component "queue", version 1).
-- Mirrors queue.DBTypes: Msg, HoldRule, MsgRetired, webapi.Suppression, Hook, HookRetired.
-- Slice/map fields (Results, DialedIPs, Extra) are stored as JSONB.
-- IPDomain values are stored as TEXT (their canonical XString form).

CREATE TABLE IF NOT EXISTS schema_version (
    component TEXT NOT NULL,
    version INTEGER NOT NULL,
    applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (component, version)
);

CREATE TABLE IF NOT EXISTS msg (
    id BIGINT PRIMARY KEY,
    base_id BIGINT NOT NULL DEFAULT 0,
    queued TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    hold BOOLEAN NOT NULL DEFAULT FALSE,
    sender_account TEXT NOT NULL DEFAULT '',
    sender_localpart TEXT NOT NULL DEFAULT '',
    sender_domain TEXT NOT NULL DEFAULT '',
    sender_domain_str TEXT NOT NULL DEFAULT '',
    from_id TEXT NOT NULL DEFAULT '',
    recipient_localpart TEXT NOT NULL DEFAULT '',
    recipient_domain TEXT NOT NULL DEFAULT '',
    recipient_domain_str TEXT NOT NULL DEFAULT '',
    attempts INTEGER NOT NULL DEFAULT 0,
    max_attempts INTEGER NOT NULL DEFAULT 0,
    dialed_ips JSONB NOT NULL DEFAULT '{}'::jsonb,
    next_attempt TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_attempt TIMESTAMPTZ NULL,
    results JSONB NOT NULL DEFAULT '[]'::jsonb,
    has_8bit BOOLEAN NOT NULL DEFAULT FALSE,
    smtputf8 BOOLEAN NOT NULL DEFAULT FALSE,
    is_dmarc_report BOOLEAN NOT NULL DEFAULT FALSE,
    is_tls_report BOOLEAN NOT NULL DEFAULT FALSE,
    size BIGINT NOT NULL DEFAULT 0,
    message_id TEXT NOT NULL DEFAULT '',
    msg_prefix BYTEA NOT NULL DEFAULT '\x'::bytea,
    subject TEXT NOT NULL DEFAULT '',
    dsn_utf8 BYTEA NULL,
    transport TEXT NOT NULL DEFAULT '',
    require_tls BOOLEAN NULL,
    future_release_request TEXT NOT NULL DEFAULT '',
    extra JSONB NOT NULL DEFAULT '{}'::jsonb
);

CREATE INDEX IF NOT EXISTS msg_baseid_idx ON msg (base_id);

CREATE TABLE IF NOT EXISTS holdrule (
    id BIGINT PRIMARY KEY,
    account TEXT NOT NULL DEFAULT '',
    sender_domain TEXT NOT NULL DEFAULT '',
    recipient_domain TEXT NOT NULL DEFAULT '',
    sender_domain_str TEXT NOT NULL DEFAULT '',
    recipient_domain_str TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS msgretired (
    id BIGINT PRIMARY KEY,
    base_id BIGINT NOT NULL DEFAULT 0,
    queued TIMESTAMPTZ NOT NULL,
    sender_account TEXT NOT NULL DEFAULT '',
    sender_localpart TEXT NOT NULL DEFAULT '',
    sender_domain_str TEXT NOT NULL DEFAULT '',
    from_id TEXT NOT NULL DEFAULT '',
    recipient_localpart TEXT NOT NULL DEFAULT '',
    recipient_domain TEXT NOT NULL DEFAULT '',
    recipient_domain_str TEXT NOT NULL DEFAULT '',
    attempts INTEGER NOT NULL DEFAULT 0,
    max_attempts INTEGER NOT NULL DEFAULT 0,
    dialed_ips JSONB NOT NULL DEFAULT '{}'::jsonb,
    last_attempt TIMESTAMPTZ NULL,
    results JSONB NOT NULL DEFAULT '[]'::jsonb,
    has_8bit BOOLEAN NOT NULL DEFAULT FALSE,
    smtputf8 BOOLEAN NOT NULL DEFAULT FALSE,
    is_dmarc_report BOOLEAN NOT NULL DEFAULT FALSE,
    is_tls_report BOOLEAN NOT NULL DEFAULT FALSE,
    size BIGINT NOT NULL DEFAULT 0,
    message_id TEXT NOT NULL DEFAULT '',
    subject TEXT NOT NULL DEFAULT '',
    transport TEXT NOT NULL DEFAULT '',
    require_tls BOOLEAN NULL,
    future_release_request TEXT NOT NULL DEFAULT '',
    extra JSONB NOT NULL DEFAULT '{}'::jsonb,
    last_activity TIMESTAMPTZ NOT NULL,
    recipient_address TEXT NOT NULL DEFAULT '',
    success BOOLEAN NOT NULL DEFAULT FALSE,
    keep_until TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS msgretired_fromid_idx ON msgretired (from_id);
CREATE INDEX IF NOT EXISTS msgretired_lastactivity_idx ON msgretired (last_activity);
CREATE INDEX IF NOT EXISTS msgretired_recipient_lastactivity_idx ON msgretired (recipient_address, last_activity);
CREATE INDEX IF NOT EXISTS msgretired_keepuntil_idx ON msgretired (keep_until);

CREATE TABLE IF NOT EXISTS suppression (
    id BIGINT PRIMARY KEY,
    created TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    account TEXT NOT NULL,
    base_address TEXT NOT NULL,
    original_address TEXT NOT NULL,
    manual BOOLEAN NOT NULL DEFAULT FALSE,
    reason TEXT NOT NULL DEFAULT '',
    UNIQUE (account, base_address)
);

CREATE TABLE IF NOT EXISTS hook (
    id BIGINT PRIMARY KEY,
    queue_msg_id BIGINT NOT NULL DEFAULT 0,
    from_id TEXT NOT NULL DEFAULT '',
    message_id TEXT NOT NULL DEFAULT '',
    subject TEXT NOT NULL DEFAULT '',
    extra JSONB NOT NULL DEFAULT '{}'::jsonb,
    account TEXT NOT NULL,
    url TEXT NOT NULL,
    "authorization" TEXT NOT NULL DEFAULT '',
    is_incoming BOOLEAN NOT NULL DEFAULT FALSE,
    outgoing_event TEXT NOT NULL DEFAULT '',
    payload TEXT NOT NULL DEFAULT '',
    submitted TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    attempts INTEGER NOT NULL DEFAULT 0,
    next_attempt TIMESTAMPTZ NOT NULL,
    results JSONB NOT NULL DEFAULT '[]'::jsonb
);

CREATE INDEX IF NOT EXISTS hook_queuemsgid_idx ON hook (queue_msg_id);

CREATE INDEX IF NOT EXISTS hook_submitted_idx ON hook (submitted);
CREATE INDEX IF NOT EXISTS hook_nextattempt_idx ON hook (next_attempt);

CREATE TABLE IF NOT EXISTS hookretired (
    id BIGINT PRIMARY KEY,
    queue_msg_id BIGINT NOT NULL DEFAULT 0,
    from_id TEXT NOT NULL DEFAULT '',
    message_id TEXT NOT NULL DEFAULT '',
    subject TEXT NOT NULL DEFAULT '',
    extra JSONB NOT NULL DEFAULT '{}'::jsonb,
    account TEXT NOT NULL,
    url TEXT NOT NULL,
    "authorization" BOOLEAN NOT NULL DEFAULT FALSE,
    is_incoming BOOLEAN NOT NULL DEFAULT FALSE,
    outgoing_event TEXT NOT NULL DEFAULT '',
    payload TEXT NOT NULL DEFAULT '',
    submitted TIMESTAMPTZ NOT NULL,
    superseded_by_id BIGINT NOT NULL DEFAULT 0,
    attempts INTEGER NOT NULL DEFAULT 0,
    results JSONB NOT NULL DEFAULT '[]'::jsonb,
    success BOOLEAN NOT NULL DEFAULT FALSE,
    last_activity TIMESTAMPTZ NOT NULL,
    keep_until TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS hookretired_account_lastactivity_idx ON hookretired (account, last_activity);
CREATE INDEX IF NOT EXISTS hookretired_lastactivity_idx ON hookretired (last_activity);
CREATE INDEX IF NOT EXISTS hookretired_keepuntil_idx ON hookretired (keep_until);
