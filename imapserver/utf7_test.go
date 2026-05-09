package imapserver

import (
	"errors"
	"fmt"
	"testing"
)

func TestUTF7(t *testing.T) {
	check := func(input string, output string, expErr error) {
		t.Helper()

		r, err := utf7decode(input)
		if r != output {
			t.Fatalf("got %q, expected %q (err %v), for input %q", r, output, err, input)
		}
		if (expErr == nil) != (err == nil) || err != nil && !errors.Is(err, expErr) {
			t.Fatalf("got err %v, expected %v", err, expErr)
		}
		if expErr == nil {
			expInput := utf7encode(output)
			if expInput != input {
				t.Fatalf("encoding, got %s, expected %s", expInput, input)
			}
		}
	}

	check("plain", "plain", nil)
	check("&Jjo-", "☺", nil)
	check("test&Jjo-", "test☺", nil)
	check("&Jjo-test&Jjo-", "☺test☺", nil)
	check("&Jjo-test", "☺test", nil)
	check("&-", "&", nil)
	check("&Jjo", "", errUTF7UnfinishedShift)     // missing closing -
	check("&Jjo-&-", "", errUTF7SuperfluousShift) // shift just after unshift not allowed, should have been a single shift.
	check("&AGE-", "", errUTF7UnneededShift)      // Just 'a', does not need utf7.
	check("&☺-", "", errUTF7Base64)
	check("&YQ-", "", errUTF7OddSized) // Just a single byte 'a'
	check("&2AHcNw-", "𐐷", nil)
	check(fmt.Sprintf("&%s-", utf7encoding.EncodeToString([]byte{0xdc, 0x00, 0xd8, 0x00})), "", errUTF7BadSurrogate) // Low & high surrogate swapped.
	check(fmt.Sprintf("&%s-", utf7encoding.EncodeToString([]byte{0, 1, 0xdc, 0x00})), "", errUTF7BadSurrogate)       // ASCII + high surrogate.
	check(fmt.Sprintf("&%s-", utf7encoding.EncodeToString([]byte{0, 1, 0xd8, 0x00})), "", errUTF7BadSurrogate)       // ASCII + low surrogate.
	check(fmt.Sprintf("&%s-", utf7encoding.EncodeToString([]byte{0xd8, 0x00, 0, 1})), "", errUTF7BadSurrogate)       // low surrogate + ASCII.
	check(fmt.Sprintf("&%s-", utf7encoding.EncodeToString([]byte{0xdc, 0x00, 0, 1})), "", errUTF7BadSurrogate)       // high surrogate + ASCII.

	// ../rfc/9051:7967
	check("~peter/mail/&U,BTFw-/&ZeVnLIqe-", "~peter/mail/台北/日本語", nil)
	check("&U,BTFw-&ZeVnLIqe-", "", errUTF7SuperfluousShift)
}
