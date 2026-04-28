package store

// Step 9: per-type pgTypeHandler registrations.
//
// Each registered type maps a Go struct (declared elsewhere in the store
// package) to a PG table whose schema is in store/schema/account_001.sql,
// auth_001.sql, or queue_001.sql. Most registrations use buildSimpleHandler
// from pg_helpers.go, which derives INSERT/UPDATE/SELECT/DELETE SQL via
// reflection from a list of field-to-column specs.
//
// Types with embedded structs (Mailbox: SpecialUse + MailboxCounts;
// Message: Flags), JSONB columns, BYTEA-encoded fields (Password,
// LoginSession.[16]byte tokens), or array element types not natively
// supported by pgx are deferred to follow-up batches and use custom Codecs
// or hand-written handlers.

func init() {
	registerPgType(buildSimpleHandler[NextUIDValidity]("nextuidvalidity", []pgFieldSpec{
		{Field: "ID", Column: "id", PK: true},
		{Field: "Next", Column: "next"},
	}))

	registerPgType(buildSimpleHandler[SyncState]("syncstate", []pgFieldSpec{
		{Field: "ID", Column: "id", PK: true},
		{Field: "LastModSeq", Column: "last_mod_seq"},
		{Field: "HighestDeletedModSeq", Column: "highest_deleted_mod_seq"},
	}))

	registerPgType(buildSimpleHandler[DiskUsage]("diskusage", []pgFieldSpec{
		{Field: "ID", Column: "id", PK: true},
		{Field: "MessageSize", Column: "message_size"},
	}))

	registerPgType(buildSimpleHandler[Settings]("settings", []pgFieldSpec{
		{Field: "ID", Column: "id", PK: true},
		{Field: "Signature", Column: "signature"},
		{Field: "Quoting", Column: "quoting"},
		{Field: "ShowAddressSecurity", Column: "show_address_security"},
		{Field: "ShowHTML", Column: "show_html"},
		{Field: "NoShowShortcuts", Column: "no_show_shortcuts"},
		{Field: "ShowHeaders", Column: "show_headers"},
	}))

	registerPgType(buildSimpleHandler[Upgrade]("upgrade", []pgFieldSpec{
		{Field: "ID", Column: "id", PK: true},
		{Field: "Threads", Column: "threads"},
		{Field: "MailboxModSeq", Column: "mailbox_mod_seq"},
		{Field: "MailboxParentID", Column: "mailbox_parent_id"},
		{Field: "MailboxCounts", Column: "mailbox_counts"},
		{Field: "MessageParseVersion", Column: "message_parse_version"},
	}))

	registerPgType(buildSimpleHandler[Subjectpass]("subjectpass", []pgFieldSpec{
		{Field: "Email", Column: "email", PK: true},
		{Field: "Key", Column: "key"},
	}))

	registerPgType(buildSimpleHandler[RecipientDomainTLS]("recipientdomaintls", []pgFieldSpec{
		{Field: "Domain", Column: "domain", PK: true},
		{Field: "Updated", Column: "updated"},
		{Field: "STARTTLS", Column: "starttls"},
		{Field: "RequireTLS", Column: "require_tls"},
	}))

	registerPgType(buildSimpleHandler[FromAddressSettings]("fromaddresssettings", []pgFieldSpec{
		{Field: "FromAddress", Column: "from_address", PK: true},
		{Field: "ViewMode", Column: "view_mode"},
	}))

	registerPgType(buildSimpleHandler[Subscription]("subscription", []pgFieldSpec{
		{Field: "Name", Column: "name", PK: true},
	}))

	registerPgType(buildSimpleHandler[Outgoing]("outgoing", []pgFieldSpec{
		{Field: "ID", Column: "id", PK: true},
		{Field: "Recipient", Column: "recipient"},
		{Field: "Submitted", Column: "submitted"},
	}))

	registerPgType(buildSimpleHandler[MessageErase]("messageerase", []pgFieldSpec{
		{Field: "ID", Column: "id", PK: true},
		{Field: "SkipUpdateDiskUsage", Column: "skip_update_disk_usage"},
	}))

	registerPgType(buildSimpleHandler[RulesetNoListID]("rulesetnolistid", []pgFieldSpec{
		{Field: "ID", Column: "id", PK: true},
		{Field: "RcptToAddress", Column: "rcpt_to_address"},
		{Field: "ListID", Column: "list_id"},
		{Field: "ToInbox", Column: "to_inbox"},
	}))

	registerPgType(buildSimpleHandler[RulesetNoMsgFrom]("rulesetnomsgfrom", []pgFieldSpec{
		{Field: "ID", Column: "id", PK: true},
		{Field: "RcptToAddress", Column: "rcpt_to_address"},
		{Field: "MsgFromAddress", Column: "msg_from_address"},
		{Field: "ToInbox", Column: "to_inbox"},
	}))

	registerPgType(buildSimpleHandler[RulesetNoMailbox]("rulesetnomailbox", []pgFieldSpec{
		{Field: "ID", Column: "id", PK: true},
		{Field: "MailboxID", Column: "mailbox_id"},
		{Field: "ToMailbox", Column: "to_mailbox"},
	}))

	registerPgType(buildSimpleHandler[Recipient]("recipient", []pgFieldSpec{
		{Field: "ID", Column: "id", PK: true},
		{Field: "MessageID", Column: "message_id"},
		{Field: "Localpart", Column: "localpart"},
		{Field: "Domain", Column: "domain"},
		{Field: "OrgDomain", Column: "org_domain"},
		{Field: "Sent", Column: "sent"},
	}))

	registerPgType(buildSimpleHandler[Annotation]("annotation", []pgFieldSpec{
		{Field: "ID", Column: "id", PK: true},
		{Field: "CreateSeq", Column: "create_seq"},
		{Field: "ModSeq", Column: "mod_seq"},
		{Field: "Expunged", Column: "expunged"},
		{Field: "MailboxID", Column: "mailbox_id"},
		{Field: "Key", Column: "key"},
		{Field: "IsString", Column: "is_string"},
		{Field: "Value", Column: "value"},
	}))

	// Password: CRAMMD5 has its own MarshalBinary/UnmarshalBinary; SCRAM is
	// a plain struct with []byte + int fields, gob-encoded for round-trip.
	registerPgType(buildSimpleHandler[Password]("password", []pgFieldSpec{
		{Field: "Hash", Column: "hash", PK: true},
		{Field: "CRAMMD5", Column: "crammd5", Codec: binaryMarshalCodec{}},
		{Field: "SCRAMSHA1", Column: "scramsha1", Codec: gobCodec{}},
		{Field: "SCRAMSHA256", Column: "scramsha256", Codec: gobCodec{}},
	}))

	// LoginSession's two token fields are [16]byte; pgx talks []byte for BYTEA
	// so a small copying codec bridges them.
	registerPgType(buildSimpleHandler[LoginSession]("loginsession", []pgFieldSpec{
		{Field: "ID", Column: "id", PK: true},
		{Field: "Created", Column: "created"},
		{Field: "Expires", Column: "expires"},
		{Field: "SessionTokenBinary", Column: "session_token_binary", Codec: bytes16Codec{}},
		{Field: "CSRFTokenBinary", Column: "csrf_token_binary", Codec: bytes16Codec{}},
		{Field: "AccountName", Column: "account_name"},
		{Field: "LoginAddress", Column: "login_address"},
	}))

	// Mailbox: SpecialUse fields embed at the top level and keep their bare
	// names; MailboxCounts is also embedded but its columns are prefixed
	// counts_*. The dotted Field path resolves the inner struct.
	registerPgType(buildSimpleHandler[Mailbox]("mailbox", []pgFieldSpec{
		{Field: "ID", Column: "id", PK: true},
		{Field: "CreateSeq", Column: "create_seq"},
		{Field: "ModSeq", Column: "mod_seq"},
		{Field: "Expunged", Column: "expunged"},
		{Field: "ParentID", Column: "parent_id"},
		{Field: "Name", Column: "name"},
		{Field: "UIDValidity", Column: "uid_validity"},
		{Field: "UIDNext", Column: "uid_next"},
		{Field: "SpecialUse.Archive", Column: "archive"},
		{Field: "SpecialUse.Draft", Column: "draft"},
		{Field: "SpecialUse.Junk", Column: "junk"},
		{Field: "SpecialUse.Sent", Column: "sent"},
		{Field: "SpecialUse.Trash", Column: "trash"},
		{Field: "Keywords", Column: "keywords"},
		{Field: "HaveCounts", Column: "have_counts"},
		{Field: "MailboxCounts.Total", Column: "counts_total"},
		{Field: "MailboxCounts.Deleted", Column: "counts_deleted"},
		{Field: "MailboxCounts.Unread", Column: "counts_unread"},
		{Field: "MailboxCounts.Unseen", Column: "counts_unseen"},
		{Field: "MailboxCounts.Size", Column: "counts_size"},
	}))

	// Auth DB types live in a separate PG schema (auth_001.sql) but share the
	// same handler registry — the pool's search_path resolves the table.

	registerPgType(buildSimpleHandler[TLSPublicKey]("tlspublickey", []pgFieldSpec{
		{Field: "Fingerprint", Column: "fingerprint", PK: true},
		{Field: "Created", Column: "created"},
		{Field: "Type", Column: "type"},
		{Field: "Name", Column: "name"},
		{Field: "NoIMAPPreauth", Column: "no_imap_preauth"},
		{Field: "CertDER", Column: "cert_der"},
		{Field: "Account", Column: "account"},
		{Field: "LoginAddress", Column: "login_address"},
	}))

	registerPgType(buildSimpleHandler[LoginAttempt]("loginattempt", []pgFieldSpec{
		{Field: "Key", Column: "key", PK: true},
		{Field: "Last", Column: "last"},
		{Field: "First", Column: "first"},
		{Field: "Count", Column: "count"},
		{Field: "AccountName", Column: "account_name"},
		{Field: "LoginAddress", Column: "login_address"},
		{Field: "RemoteIP", Column: "remote_ip"},
		{Field: "LocalIP", Column: "local_ip"},
		{Field: "TLS", Column: "tls"},
		{Field: "TLSPubKeyFingerprint", Column: "tls_pubkey_fingerprint"},
		{Field: "Protocol", Column: "protocol"},
		{Field: "UserAgent", Column: "user_agent"},
		{Field: "AuthMech", Column: "auth_mech"},
		{Field: "Result", Column: "result"},
	}))

	registerPgType(buildSimpleHandler[LoginAttemptState]("loginattemptstate", []pgFieldSpec{
		{Field: "AccountName", Column: "account_name", PK: true},
		{Field: "RecordsFailed", Column: "records_failed"},
	}))

	registerPgType(buildSimpleHandler[AccountRemove]("accountremove", []pgFieldSpec{
		{Field: "AccountName", Column: "account_name", PK: true},
	}))

	// Message: largest type. Embedded Flags fields keep bare column names
	// (seen, answered, ...). All scalar/array fields map natively via pgx.
	registerPgType(buildSimpleHandler[Message]("message", []pgFieldSpec{
		{Field: "ID", Column: "id", PK: true},
		{Field: "UID", Column: "uid"},
		{Field: "MailboxID", Column: "mailbox_id"},
		{Field: "ModSeq", Column: "mod_seq"},
		{Field: "CreateSeq", Column: "create_seq"},
		{Field: "Expunged", Column: "expunged"},
		{Field: "IsReject", Column: "is_reject"},
		{Field: "IsForward", Column: "is_forward"},
		{Field: "MailboxOrigID", Column: "mailbox_orig_id"},
		{Field: "MailboxDestinedID", Column: "mailbox_destined_id"},
		{Field: "Received", Column: "received"},
		{Field: "SaveDate", Column: "save_date"},
		{Field: "RemoteIP", Column: "remote_ip"},
		{Field: "RemoteIPMasked1", Column: "remote_ip_masked1"},
		{Field: "RemoteIPMasked2", Column: "remote_ip_masked2"},
		{Field: "RemoteIPMasked3", Column: "remote_ip_masked3"},
		{Field: "EHLODomain", Column: "ehlo_domain"},
		{Field: "MailFrom", Column: "mail_from"},
		{Field: "MailFromLocalpart", Column: "mail_from_localpart"},
		{Field: "MailFromDomain", Column: "mail_from_domain"},
		{Field: "RcptToLocalpart", Column: "rcpt_to_localpart"},
		{Field: "RcptToDomain", Column: "rcpt_to_domain"},
		{Field: "MsgFromLocalpart", Column: "msg_from_localpart"},
		{Field: "MsgFromDomain", Column: "msg_from_domain"},
		{Field: "MsgFromOrgDomain", Column: "msg_from_org_domain"},
		{Field: "EHLOValidated", Column: "ehlo_validated"},
		{Field: "MailFromValidated", Column: "mail_from_validated"},
		{Field: "MsgFromValidated", Column: "msg_from_validated"},
		{Field: "EHLOValidation", Column: "ehlo_validation"},
		{Field: "MailFromValidation", Column: "mail_from_validation"},
		{Field: "MsgFromValidation", Column: "msg_from_validation"},
		{Field: "DKIMDomains", Column: "dkim_domains"},
		{Field: "OrigEHLODomain", Column: "orig_ehlo_domain"},
		{Field: "OrigDKIMDomains", Column: "orig_dkim_domains"},
		{Field: "MessageID", Column: "message_id"},
		{Field: "SubjectBase", Column: "subject_base"},
		{Field: "MessageHash", Column: "message_hash"},
		{Field: "ThreadID", Column: "thread_id"},
		{Field: "ThreadParentIDs", Column: "thread_parent_ids"},
		{Field: "ThreadMissingLink", Column: "thread_missing_link"},
		{Field: "ThreadMuted", Column: "thread_muted"},
		{Field: "ThreadCollapsed", Column: "thread_collapsed"},
		{Field: "IsMailingList", Column: "is_mailing_list"},
		{Field: "DSN", Column: "dsn"},
		{Field: "ReceivedTLSVersion", Column: "received_tls_version"},
		{Field: "ReceivedTLSCipherSuite", Column: "received_tls_cipher_suite"},
		{Field: "ReceivedRequireTLS", Column: "received_require_tls"},
		{Field: "Flags.Seen", Column: "seen"},
		{Field: "Flags.Answered", Column: "answered"},
		{Field: "Flags.Flagged", Column: "flagged"},
		{Field: "Flags.Forwarded", Column: "forwarded"},
		{Field: "Flags.Junk", Column: "junk"},
		{Field: "Flags.Notjunk", Column: "notjunk"},
		{Field: "Flags.Deleted", Column: "deleted"},
		{Field: "Flags.Draft", Column: "draft"},
		{Field: "Flags.Phishing", Column: "phishing"},
		{Field: "Flags.MDNSent", Column: "mdn_sent"},
		{Field: "Keywords", Column: "keywords"},
		{Field: "Size", Column: "size"},
		{Field: "TrainedJunk", Column: "trained_junk"},
		{Field: "MsgPrefix", Column: "msg_prefix"},
		{Field: "Preview", Column: "preview"},
		{Field: "ParsedBuf", Column: "parsed_buf"},
	}))
}
