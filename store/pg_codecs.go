package store

import (
	"bytes"
	"encoding"
	"encoding/gob"
	"encoding/json"
	"fmt"
	"reflect"
)

// JSONBCodec stores arbitrary JSON-encodable Go values (maps, slices of
// custom structs) into a PG JSONB column. Round-trip uses encoding/json,
// matching how bstore historically stored map/slice fields. The codec is
// exported so external packages (queue, webapi) can use it from their own
// pg_types.go init().
type JSONBCodec struct{}

func (JSONBCodec) Encode(field reflect.Value) any {
	raw, err := json.Marshal(field.Interface())
	if err != nil {
		panic(fmt.Sprintf("pg jsonb encode %s: %v", field.Type(), err))
	}
	// json.RawMessage is recognised by pgx as JSON/JSONB on the wire, so
	// the column type is inferred without an explicit cast in SQL.
	return json.RawMessage(raw)
}

func (JSONBCodec) NewScanTarget() any {
	var rm json.RawMessage
	return &rm
}

func (JSONBCodec) WriteScanned(target any, field reflect.Value) {
	rm := *target.(*json.RawMessage)
	if len(rm) == 0 || string(rm) == "null" {
		field.Set(reflect.Zero(field.Type()))
		return
	}
	dst := reflect.New(field.Type())
	if err := json.Unmarshal(rm, dst.Interface()); err != nil {
		panic(fmt.Sprintf("pg jsonb decode %s: %v", field.Type(), err))
	}
	field.Set(dst.Elem())
}

// gobCodec stores a struct as a gob-encoded BYTEA. Used for fields whose
// element type doesn't map cleanly to a built-in PG type (SCRAM auth state,
// for example). Round-trip is identical: decoder receives the same struct.
type gobCodec struct{}

func (gobCodec) Encode(field reflect.Value) any {
	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(field.Interface()); err != nil {
		// Encoding can fail if the struct contains unexported fields or
		// non-gob-encodable values; surface as panic since that's a coding
		// error in the type registration.
		panic(fmt.Sprintf("pg gob encode %s: %v", field.Type(), err))
	}
	return buf.Bytes()
}

func (gobCodec) NewScanTarget() any {
	var b []byte
	return &b
}

func (gobCodec) WriteScanned(target any, field reflect.Value) {
	raw := *target.(*[]byte)
	if len(raw) == 0 {
		field.Set(reflect.Zero(field.Type()))
		return
	}
	dst := reflect.New(field.Type())
	if err := gob.NewDecoder(bytes.NewReader(raw)).Decode(dst.Interface()); err != nil {
		panic(fmt.Sprintf("pg gob decode %s: %v", field.Type(), err))
	}
	field.Set(dst.Elem())
}

// binaryMarshalCodec uses encoding.BinaryMarshaler / Unmarshaler from the
// field type itself. The field type must implement both interfaces; verified
// at registration time would be ideal, but reflection makes that awkward,
// so we let the panic at first call surface mistakes loudly.
type binaryMarshalCodec struct{}

func (binaryMarshalCodec) Encode(field reflect.Value) any {
	bm, ok := field.Addr().Interface().(encoding.BinaryMarshaler)
	if !ok {
		bm, ok = field.Interface().(encoding.BinaryMarshaler)
	}
	if !ok {
		panic(fmt.Sprintf("pg binaryMarshalCodec: %s does not implement BinaryMarshaler", field.Type()))
	}
	raw, err := bm.MarshalBinary()
	if err != nil {
		panic(fmt.Sprintf("pg binary marshal %s: %v", field.Type(), err))
	}
	return raw
}

func (binaryMarshalCodec) NewScanTarget() any {
	var b []byte
	return &b
}

func (binaryMarshalCodec) WriteScanned(target any, field reflect.Value) {
	raw := *target.(*[]byte)
	if len(raw) == 0 {
		field.Set(reflect.Zero(field.Type()))
		return
	}
	bu, ok := field.Addr().Interface().(encoding.BinaryUnmarshaler)
	if !ok {
		panic(fmt.Sprintf("pg binaryMarshalCodec: %s does not implement BinaryUnmarshaler", field.Type()))
	}
	if err := bu.UnmarshalBinary(raw); err != nil {
		panic(fmt.Sprintf("pg binary unmarshal %s: %v", field.Type(), err))
	}
}

// bytes16Codec scans a BYTEA value into a [16]byte field and writes back via
// a []byte intermediate. pgx natively scans BYTEA into []byte; the helper
// copies to/from the fixed-size array.
type bytes16Codec struct{}

func (bytes16Codec) Encode(field reflect.Value) any {
	arr := field.Interface().([16]byte)
	out := make([]byte, 16)
	copy(out, arr[:])
	return out
}

func (bytes16Codec) NewScanTarget() any {
	var b []byte
	return &b
}

func (bytes16Codec) WriteScanned(target any, field reflect.Value) {
	raw := *target.(*[]byte)
	var arr [16]byte
	copy(arr[:], raw)
	field.Set(reflect.ValueOf(arr))
}
