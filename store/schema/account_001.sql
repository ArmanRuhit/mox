-- Schema for a per-account database (component "account", version 1).
-- Mirrors store.DBTypes. Embedded structs (Flags, SpecialUse, MailboxCounts) are
-- flattened: Flags fields and SpecialUse fields keep their bare names; MailboxCounts
-- fields are prefixed counts_*. Slice fields are stored as PG arrays where the
-- original element type is scalar; complex slices (ParsedBuf, etc.) are BYTEA/JSONB.
-- Runner sets search_path before executing this file.

CREATE TABLE IF NOT EXISTS schema_version (
    component TEXT NOT NULL,
    version INTEGER NOT NULL,
    applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (component, version)
);

CREATE TABLE IF NOT EXISTS nextuidvalidity (
    id INTEGER PRIMARY KEY,
    next BIGINT NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS syncstate (
    id INTEGER PRIMARY KEY,
    last_mod_seq BIGINT NOT NULL,
    highest_deleted_mod_seq BIGINT NOT NULL DEFAULT -1
);

CREATE TABLE IF NOT EXISTS diskusage (
    id BIGINT PRIMARY KEY,
    message_size BIGINT NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS settings (
    id SMALLINT PRIMARY KEY,
    signature TEXT NOT NULL DEFAULT '',
    quoting TEXT NOT NULL DEFAULT '',
    show_address_security BOOLEAN NOT NULL DEFAULT FALSE,
    show_html BOOLEAN NOT NULL DEFAULT FALSE,
    no_show_shortcuts BOOLEAN NOT NULL DEFAULT FALSE,
    show_headers TEXT[] NOT NULL DEFAULT '{}'::text[]
);

CREATE TABLE IF NOT EXISTS upgrade (
    id SMALLINT PRIMARY KEY,
    threads SMALLINT NOT NULL DEFAULT 0,
    mailbox_mod_seq BOOLEAN NOT NULL DEFAULT FALSE,
    mailbox_parent_id BOOLEAN NOT NULL DEFAULT FALSE,
    mailbox_counts BOOLEAN NOT NULL DEFAULT FALSE,
    message_parse_version INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS subjectpass (
    email TEXT PRIMARY KEY,
    key TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS password (
    hash TEXT PRIMARY KEY,
    crammd5 BYTEA NOT NULL,
    scramsha1 BYTEA NOT NULL,
    scramsha256 BYTEA NOT NULL
);

CREATE TABLE IF NOT EXISTS recipientdomaintls (
    domain TEXT PRIMARY KEY,
    updated TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    starttls BOOLEAN NOT NULL DEFAULT FALSE,
    require_tls BOOLEAN NOT NULL DEFAULT FALSE
);

CREATE TABLE IF NOT EXISTS fromaddresssettings (
    from_address TEXT PRIMARY KEY,
    view_mode TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS subscription (
    name TEXT PRIMARY KEY
);

CREATE TABLE IF NOT EXISTS mailbox (
    id BIGINT PRIMARY KEY,
    create_seq BIGINT NOT NULL DEFAULT 0,
    mod_seq BIGINT NOT NULL DEFAULT 0,
    expunged BOOLEAN NOT NULL DEFAULT FALSE,
    parent_id BIGINT NULL REFERENCES mailbox(id) DEFERRABLE INITIALLY DEFERRED,
    name TEXT NOT NULL,
    uid_validity BIGINT NOT NULL DEFAULT 0,
    uid_next BIGINT NOT NULL DEFAULT 0,
    archive BOOLEAN NOT NULL DEFAULT FALSE,
    draft BOOLEAN NOT NULL DEFAULT FALSE,
    junk BOOLEAN NOT NULL DEFAULT FALSE,
    sent BOOLEAN NOT NULL DEFAULT FALSE,
    trash BOOLEAN NOT NULL DEFAULT FALSE,
    keywords TEXT[] NOT NULL DEFAULT '{}'::text[],
    have_counts BOOLEAN NOT NULL DEFAULT FALSE,
    counts_total BIGINT NOT NULL DEFAULT 0,
    counts_deleted BIGINT NOT NULL DEFAULT 0,
    counts_unread BIGINT NOT NULL DEFAULT 0,
    counts_unseen BIGINT NOT NULL DEFAULT 0,
    counts_size BIGINT NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS mailbox_modseq_idx ON mailbox (mod_seq);
CREATE INDEX IF NOT EXISTS mailbox_parentid_idx ON mailbox (parent_id);

CREATE TABLE IF NOT EXISTS annotation (
    id BIGINT PRIMARY KEY,
    create_seq BIGINT NOT NULL DEFAULT 0,
    mod_seq BIGINT NOT NULL DEFAULT 0,
    expunged BOOLEAN NOT NULL DEFAULT FALSE,
    mailbox_id BIGINT NOT NULL DEFAULT 0 REFERENCES mailbox(id) DEFERRABLE INITIALLY DEFERRED,
    key TEXT NOT NULL,
    is_string BOOLEAN NOT NULL DEFAULT FALSE,
    value BYTEA NULL
);

CREATE INDEX IF NOT EXISTS annotation_modseq_idx ON annotation (mod_seq);
CREATE INDEX IF NOT EXISTS annotation_mailboxid_key_idx ON annotation (mailbox_id, key);

CREATE TABLE IF NOT EXISTS message (
    id BIGINT PRIMARY KEY,
    uid BIGINT NOT NULL,
    mailbox_id BIGINT NOT NULL REFERENCES mailbox(id) DEFERRABLE INITIALLY DEFERRED,
    mod_seq BIGINT NOT NULL DEFAULT 0,
    create_seq BIGINT NOT NULL DEFAULT 0,
    expunged BOOLEAN NOT NULL DEFAULT FALSE,
    is_reject BOOLEAN NOT NULL DEFAULT FALSE,
    is_forward BOOLEAN NOT NULL DEFAULT FALSE,
    mailbox_orig_id BIGINT NOT NULL DEFAULT 0,
    mailbox_destined_id BIGINT NOT NULL DEFAULT 0,
    received TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    save_date TIMESTAMPTZ NULL DEFAULT NOW(),
    remote_ip TEXT NOT NULL DEFAULT '',
    remote_ip_masked1 TEXT NOT NULL DEFAULT '',
    remote_ip_masked2 TEXT NOT NULL DEFAULT '',
    remote_ip_masked3 TEXT NOT NULL DEFAULT '',
    ehlo_domain TEXT NOT NULL DEFAULT '',
    mail_from TEXT NOT NULL DEFAULT '',
    mail_from_localpart TEXT NOT NULL DEFAULT '',
    mail_from_domain TEXT NOT NULL DEFAULT '',
    rcpt_to_localpart TEXT NOT NULL DEFAULT '',
    rcpt_to_domain TEXT NOT NULL DEFAULT '',
    msg_from_localpart TEXT NOT NULL DEFAULT '',
    msg_from_domain TEXT NOT NULL DEFAULT '',
    msg_from_org_domain TEXT NOT NULL DEFAULT '',
    ehlo_validated BOOLEAN NOT NULL DEFAULT FALSE,
    mail_from_validated BOOLEAN NOT NULL DEFAULT FALSE,
    msg_from_validated BOOLEAN NOT NULL DEFAULT FALSE,
    ehlo_validation SMALLINT NOT NULL DEFAULT 0,
    mail_from_validation SMALLINT NOT NULL DEFAULT 0,
    msg_from_validation SMALLINT NOT NULL DEFAULT 0,
    dkim_domains TEXT[] NOT NULL DEFAULT '{}'::text[],
    orig_ehlo_domain TEXT NOT NULL DEFAULT '',
    orig_dkim_domains TEXT[] NOT NULL DEFAULT '{}'::text[],
    message_id TEXT NOT NULL DEFAULT '',
    subject_base TEXT NOT NULL DEFAULT '',
    message_hash BYTEA NULL,
    thread_id BIGINT NOT NULL DEFAULT 0,
    thread_parent_ids BIGINT[] NOT NULL DEFAULT '{}'::bigint[],
    thread_missing_link BOOLEAN NOT NULL DEFAULT FALSE,
    thread_muted BOOLEAN NOT NULL DEFAULT FALSE,
    thread_collapsed BOOLEAN NOT NULL DEFAULT FALSE,
    is_mailing_list BOOLEAN NOT NULL DEFAULT FALSE,
    dsn BOOLEAN NOT NULL DEFAULT FALSE,
    received_tls_version INTEGER NOT NULL DEFAULT 0,
    received_tls_cipher_suite INTEGER NOT NULL DEFAULT 0,
    received_require_tls BOOLEAN NOT NULL DEFAULT FALSE,
    seen BOOLEAN NOT NULL DEFAULT FALSE,
    answered BOOLEAN NOT NULL DEFAULT FALSE,
    flagged BOOLEAN NOT NULL DEFAULT FALSE,
    forwarded BOOLEAN NOT NULL DEFAULT FALSE,
    junk BOOLEAN NOT NULL DEFAULT FALSE,
    notjunk BOOLEAN NOT NULL DEFAULT FALSE,
    deleted BOOLEAN NOT NULL DEFAULT FALSE,
    draft BOOLEAN NOT NULL DEFAULT FALSE,
    phishing BOOLEAN NOT NULL DEFAULT FALSE,
    mdn_sent BOOLEAN NOT NULL DEFAULT FALSE,
    keywords TEXT[] NOT NULL DEFAULT '{}'::text[],
    size BIGINT NOT NULL DEFAULT 0,
    trained_junk BOOLEAN NULL,
    msg_prefix BYTEA NOT NULL DEFAULT '\x'::bytea,
    preview TEXT NULL,
    parsed_buf BYTEA NULL,
    UNIQUE (mailbox_id, uid)
);

CREATE INDEX IF NOT EXISTS message_mailboxid_received_idx ON message (mailbox_id, received);
CREATE INDEX IF NOT EXISTS message_mailboxid_modseq_idx ON message (mailbox_id, mod_seq);
CREATE INDEX IF NOT EXISTS message_modseq_idx ON message (mod_seq);
CREATE INDEX IF NOT EXISTS message_createseq_idx ON message (create_seq);
CREATE INDEX IF NOT EXISTS message_received_idx ON message (received);
CREATE INDEX IF NOT EXISTS message_remoteip1_received_idx ON message (remote_ip_masked1, received);
CREATE INDEX IF NOT EXISTS message_remoteip2_received_idx ON message (remote_ip_masked2, received);
CREATE INDEX IF NOT EXISTS message_remoteip3_received_idx ON message (remote_ip_masked3, received);
CREATE INDEX IF NOT EXISTS message_ehlodomain_received_idx ON message (ehlo_domain, received);
CREATE INDEX IF NOT EXISTS message_mailfromdomain_received_idx ON message (mail_from_domain, received);
CREATE INDEX IF NOT EXISTS message_msgfromdomain_received_idx ON message (msg_from_domain, received);
CREATE INDEX IF NOT EXISTS message_msgfromorgdomain_received_idx ON message (msg_from_org_domain, received);
CREATE INDEX IF NOT EXISTS message_messageid_idx ON message (message_id);
CREATE INDEX IF NOT EXISTS message_subjectbase_idx ON message (subject_base);
CREATE INDEX IF NOT EXISTS message_threadid_idx ON message (thread_id);
CREATE INDEX IF NOT EXISTS message_dkimdomains_received_gin_idx ON message USING GIN (dkim_domains);
CREATE INDEX IF NOT EXISTS message_keywords_gin_idx ON message USING GIN (keywords);

CREATE TABLE IF NOT EXISTS recipient (
    id BIGINT PRIMARY KEY,
    message_id BIGINT NOT NULL REFERENCES message(id) DEFERRABLE INITIALLY DEFERRED,
    localpart TEXT NOT NULL,
    domain TEXT NOT NULL,
    org_domain TEXT NOT NULL,
    sent TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS recipient_messageid_idx ON recipient (message_id);
CREATE INDEX IF NOT EXISTS recipient_domain_localpart_idx ON recipient (domain, localpart);
CREATE INDEX IF NOT EXISTS recipient_orgdomain_idx ON recipient (org_domain);

CREATE TABLE IF NOT EXISTS outgoing (
    id BIGINT PRIMARY KEY,
    recipient TEXT NOT NULL,
    submitted TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS outgoing_recipient_idx ON outgoing (recipient);

CREATE TABLE IF NOT EXISTS rulesetnolistid (
    id BIGINT PRIMARY KEY,
    rcpt_to_address TEXT NOT NULL,
    list_id TEXT NOT NULL,
    to_inbox BOOLEAN NOT NULL DEFAULT FALSE
);

CREATE TABLE IF NOT EXISTS rulesetnomsgfrom (
    id BIGINT PRIMARY KEY,
    rcpt_to_address TEXT NOT NULL,
    msg_from_address TEXT NOT NULL,
    to_inbox BOOLEAN NOT NULL DEFAULT FALSE
);

CREATE TABLE IF NOT EXISTS rulesetnomailbox (
    id BIGINT PRIMARY KEY,
    mailbox_id BIGINT NOT NULL,
    to_mailbox BOOLEAN NOT NULL DEFAULT FALSE
);

CREATE TABLE IF NOT EXISTS messageerase (
    id BIGINT PRIMARY KEY,
    skip_update_disk_usage BOOLEAN NOT NULL DEFAULT FALSE
);

CREATE TABLE IF NOT EXISTS loginsession (
    id BIGINT PRIMARY KEY,
    created TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires TIMESTAMPTZ NOT NULL,
    session_token_binary BYTEA NOT NULL,
    csrf_token_binary BYTEA NOT NULL,
    account_name TEXT NOT NULL,
    login_address TEXT NOT NULL
);
