package queue

// Step 9b: per-type pgTypeHandler registrations for the queue DB.
//
// Mirrors the schema in store/schema/queue_001.sql. Three custom codecs
// bridge the structured Go fields to TEXT/JSONB:
//
//   - store.JSONBCodec — for map[string][]net.IP, map[string]string,
//     []MsgResult, []HookResult.
//   - ipDomainCodec    — for dns.IPDomain (TEXT, parsed back via
//     net.ParseIP / dns.ParseDomain).
//   - domainCodec      — for dns.Domain (TEXT, parsed back via
//     dns.ParseDomain).
//
// webapi.Suppression is registered here too, since `queue` already depends
// on `webapi` and `store` does not.

import (
	"fmt"
	"net"
	"reflect"

	"github.com/mjl-/mox/dns"
	"github.com/mjl-/mox/store"
	"github.com/mjl-/mox/webapi"
)

// ipDomainCodec serialises a dns.IPDomain value to its canonical XString
// (UTF-8) form and parses it back. The string is unambiguous: a parseable
// net.IP is taken as the IP arm, otherwise the value is parsed as a domain.
type ipDomainCodec struct{}

func (ipDomainCodec) Encode(field reflect.Value) any {
	d := field.Interface().(dns.IPDomain)
	if d.IsZero() {
		return ""
	}
	return d.XString(true)
}

func (ipDomainCodec) NewScanTarget() any {
	var s string
	return &s
}

func (ipDomainCodec) WriteScanned(target any, field reflect.Value) {
	s := *target.(*string)
	var d dns.IPDomain
	if s == "" {
		field.Set(reflect.ValueOf(d))
		return
	}
	if ip := net.ParseIP(s); ip != nil {
		d.IP = ip
	} else {
		dom, err := dns.ParseDomain(s)
		if err != nil {
			panic(fmt.Sprintf("pg ipdomain decode %q: %v", s, err))
		}
		d.Domain = dom
	}
	field.Set(reflect.ValueOf(d))
}

// domainCodec stores a dns.Domain by its (Unicode) name, restoring it via
// dns.ParseDomain.
type domainCodec struct{}

func (domainCodec) Encode(field reflect.Value) any {
	d := field.Interface().(dns.Domain)
	if d.IsZero() {
		return ""
	}
	return d.Name()
}

func (domainCodec) NewScanTarget() any {
	var s string
	return &s
}

func (domainCodec) WriteScanned(target any, field reflect.Value) {
	s := *target.(*string)
	if s == "" {
		field.Set(reflect.Zero(field.Type()))
		return
	}
	dom, err := dns.ParseDomain(s)
	if err != nil {
		panic(fmt.Sprintf("pg domain decode %q: %v", s, err))
	}
	field.Set(reflect.ValueOf(dom))
}

func init() {
	store.RegisterPgType(store.BuildSimpleHandler[Msg]("msg", []store.PgFieldSpec{
		{Field: "ID", Column: "id", PK: true},
		{Field: "BaseID", Column: "base_id"},
		{Field: "Queued", Column: "queued"},
		{Field: "Hold", Column: "hold"},
		{Field: "SenderAccount", Column: "sender_account"},
		{Field: "SenderLocalpart", Column: "sender_localpart"},
		{Field: "SenderDomain", Column: "sender_domain", Codec: ipDomainCodec{}},
		{Field: "SenderDomainStr", Column: "sender_domain_str"},
		{Field: "FromID", Column: "from_id"},
		{Field: "RecipientLocalpart", Column: "recipient_localpart"},
		{Field: "RecipientDomain", Column: "recipient_domain", Codec: ipDomainCodec{}},
		{Field: "RecipientDomainStr", Column: "recipient_domain_str"},
		{Field: "Attempts", Column: "attempts"},
		{Field: "MaxAttempts", Column: "max_attempts"},
		{Field: "DialedIPs", Column: "dialed_ips", Codec: store.JSONBCodec{}},
		{Field: "NextAttempt", Column: "next_attempt"},
		{Field: "LastAttempt", Column: "last_attempt"},
		{Field: "Results", Column: "results", Codec: store.JSONBCodec{}},
		{Field: "Has8bit", Column: "has_8bit"},
		{Field: "SMTPUTF8", Column: "smtputf8"},
		{Field: "IsDMARCReport", Column: "is_dmarc_report"},
		{Field: "IsTLSReport", Column: "is_tls_report"},
		{Field: "Size", Column: "size"},
		{Field: "MessageID", Column: "message_id"},
		{Field: "MsgPrefix", Column: "msg_prefix"},
		{Field: "Subject", Column: "subject"},
		{Field: "DSNUTF8", Column: "dsn_utf8"},
		{Field: "Transport", Column: "transport"},
		{Field: "RequireTLS", Column: "require_tls"},
		{Field: "FutureReleaseRequest", Column: "future_release_request"},
		{Field: "Extra", Column: "extra", Codec: store.JSONBCodec{}},
	}))

	store.RegisterPgType(store.BuildSimpleHandler[HoldRule]("holdrule", []store.PgFieldSpec{
		{Field: "ID", Column: "id", PK: true},
		{Field: "Account", Column: "account"},
		{Field: "SenderDomain", Column: "sender_domain", Codec: domainCodec{}},
		{Field: "RecipientDomain", Column: "recipient_domain", Codec: domainCodec{}},
		{Field: "SenderDomainStr", Column: "sender_domain_str"},
		{Field: "RecipientDomainStr", Column: "recipient_domain_str"},
	}))

	store.RegisterPgType(store.BuildSimpleHandler[MsgRetired]("msgretired", []store.PgFieldSpec{
		{Field: "ID", Column: "id", PK: true},
		{Field: "BaseID", Column: "base_id"},
		{Field: "Queued", Column: "queued"},
		{Field: "SenderAccount", Column: "sender_account"},
		{Field: "SenderLocalpart", Column: "sender_localpart"},
		{Field: "SenderDomainStr", Column: "sender_domain_str"},
		{Field: "FromID", Column: "from_id"},
		{Field: "RecipientLocalpart", Column: "recipient_localpart"},
		{Field: "RecipientDomain", Column: "recipient_domain", Codec: ipDomainCodec{}},
		{Field: "RecipientDomainStr", Column: "recipient_domain_str"},
		{Field: "Attempts", Column: "attempts"},
		{Field: "MaxAttempts", Column: "max_attempts"},
		{Field: "DialedIPs", Column: "dialed_ips", Codec: store.JSONBCodec{}},
		{Field: "LastAttempt", Column: "last_attempt"},
		{Field: "Results", Column: "results", Codec: store.JSONBCodec{}},
		{Field: "Has8bit", Column: "has_8bit"},
		{Field: "SMTPUTF8", Column: "smtputf8"},
		{Field: "IsDMARCReport", Column: "is_dmarc_report"},
		{Field: "IsTLSReport", Column: "is_tls_report"},
		{Field: "Size", Column: "size"},
		{Field: "MessageID", Column: "message_id"},
		{Field: "Subject", Column: "subject"},
		{Field: "Transport", Column: "transport"},
		{Field: "RequireTLS", Column: "require_tls"},
		{Field: "FutureReleaseRequest", Column: "future_release_request"},
		{Field: "Extra", Column: "extra", Codec: store.JSONBCodec{}},
		{Field: "LastActivity", Column: "last_activity"},
		{Field: "RecipientAddress", Column: "recipient_address"},
		{Field: "Success", Column: "success"},
		{Field: "KeepUntil", Column: "keep_until"},
	}))

	store.RegisterPgType(store.BuildSimpleHandler[Hook]("hook", []store.PgFieldSpec{
		{Field: "ID", Column: "id", PK: true},
		{Field: "QueueMsgID", Column: "queue_msg_id"},
		{Field: "FromID", Column: "from_id"},
		{Field: "MessageID", Column: "message_id"},
		{Field: "Subject", Column: "subject"},
		{Field: "Extra", Column: "extra", Codec: store.JSONBCodec{}},
		{Field: "Account", Column: "account"},
		{Field: "URL", Column: "url"},
		{Field: "Authorization", Column: "authorization"},
		{Field: "IsIncoming", Column: "is_incoming"},
		{Field: "OutgoingEvent", Column: "outgoing_event"},
		{Field: "Payload", Column: "payload"},
		{Field: "Submitted", Column: "submitted"},
		{Field: "Attempts", Column: "attempts"},
		{Field: "NextAttempt", Column: "next_attempt"},
		{Field: "Results", Column: "results", Codec: store.JSONBCodec{}},
	}))

	// HookRetired.Authorization is BOOL (just whether auth was present),
	// distinct from Hook.Authorization which holds the literal header value.
	store.RegisterPgType(store.BuildSimpleHandler[HookRetired]("hookretired", []store.PgFieldSpec{
		{Field: "ID", Column: "id", PK: true},
		{Field: "QueueMsgID", Column: "queue_msg_id"},
		{Field: "FromID", Column: "from_id"},
		{Field: "MessageID", Column: "message_id"},
		{Field: "Subject", Column: "subject"},
		{Field: "Extra", Column: "extra", Codec: store.JSONBCodec{}},
		{Field: "Account", Column: "account"},
		{Field: "URL", Column: "url"},
		{Field: "Authorization", Column: "authorization"},
		{Field: "IsIncoming", Column: "is_incoming"},
		{Field: "OutgoingEvent", Column: "outgoing_event"},
		{Field: "Payload", Column: "payload"},
		{Field: "Submitted", Column: "submitted"},
		{Field: "SupersededByID", Column: "superseded_by_id"},
		{Field: "Attempts", Column: "attempts"},
		{Field: "Results", Column: "results", Codec: store.JSONBCodec{}},
		{Field: "Success", Column: "success"},
		{Field: "LastActivity", Column: "last_activity"},
		{Field: "KeepUntil", Column: "keep_until"},
	}))

	store.RegisterPgType(store.BuildSimpleHandler[webapi.Suppression]("suppression", []store.PgFieldSpec{
		{Field: "ID", Column: "id", PK: true},
		{Field: "Created", Column: "created"},
		{Field: "Account", Column: "account"},
		{Field: "BaseAddress", Column: "base_address"},
		{Field: "OriginalAddress", Column: "original_address"},
		{Field: "Manual", Column: "manual"},
		{Field: "Reason", Column: "reason"},
	}))
}
