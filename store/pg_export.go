package store

// Public surface for packages outside `store` to register their own
// pgTypeHandlers. The `queue` and `webapi` types live outside this package, so
// they cannot reach the unexported helpers directly. Generic type aliases
// (Go 1.24+) keep the underlying handler type internal while letting callers
// name it without the underscore-prefixed receiver dance.

// PgFieldSpec is the public alias for pgFieldSpec — one column-mapping entry
// passed to BuildSimpleHandler.
type PgFieldSpec = pgFieldSpec

// PgFieldCodec is the public alias for pgFieldCodec — implementations plug
// custom encode/scan for non-trivial column types (JSONB, BYTEA-encoded
// blobs, value-converted strings, etc.).
type PgFieldCodec = pgFieldCodec

// PgTypeHandler is the public alias for the per-type handler returned by
// BuildSimpleHandler and consumed by RegisterPgType. Treat as opaque.
type PgTypeHandler[T any] = pgTypeHandler[T]

// BuildSimpleHandler builds a reflection-driven handler for a struct type
// whose fields map cleanly onto SQL columns. See the unexported helper for
// the heavy lifting; this wrapper only exists so external packages can call
// in without touching the lowercase names.
func BuildSimpleHandler[T any](table string, specs []PgFieldSpec) *PgTypeHandler[T] {
	return buildSimpleHandler[T](table, specs)
}

// RegisterPgType records the handler for type T in both registries (generic
// + runtime CRUD adapter). Call from an init() in the package that owns T.
func RegisterPgType[T any](h *PgTypeHandler[T]) {
	registerPgType(h)
}
