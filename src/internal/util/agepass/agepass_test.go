package agepass

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strings"
	"testing"
)

// pass makes a fresh passphrase for each test: no secret is written into the source.
func pass(t *testing.T) []byte {
	t.Helper()
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	return []byte(hex.EncodeToString(b))
}

func seal(t *testing.T, plain, pw []byte) []byte {
	t.Helper()
	var out bytes.Buffer
	if err := Encrypt(&out, bytes.NewReader(plain), pw, 4); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func open(ct, pw []byte) ([]byte, error) {
	var out bytes.Buffer
	err := Decrypt(&out, bytes.NewReader(ct), pw)
	return out.Bytes(), err
}

func TestRoundTripAcrossChunkBoundaries(t *testing.T) {
	pw := pass(t)
	for _, n := range []int{0, 1, chunkSize - 1, chunkSize, chunkSize + 1, 2 * chunkSize, 3*chunkSize + 17} {
		plain := make([]byte, n)
		rand.Read(plain)
		ct := seal(t, plain, pw)
		if !IsAge(ct) || !IsPassphrase(ct) {
			t.Fatalf("%d bytes: header not recognised:\n%s", n, ct[:80])
		}
		got, err := open(ct, pw)
		if err != nil {
			t.Fatalf("%d bytes: %v", n, err)
		}
		if !bytes.Equal(got, plain) {
			t.Fatalf("%d bytes: round trip differs", n)
		}
	}
}

func TestWrongPassphraseAndTampering(t *testing.T) {
	pw := pass(t)
	plain := bytes.Repeat([]byte("x"), chunkSize+100)
	ct := seal(t, plain, pw)

	if _, err := open(ct, pass(t)); !errors.Is(err, ErrWrongPassphrase) {
		t.Fatalf("wrong passphrase: %v", err)
	}
	hdrEnd := bytes.Index(ct, []byte("\n---")) + 1

	cases := map[string][]byte{
		"payload byte flipped": func() []byte { c := bytes.Clone(ct); c[len(c)-5] ^= 1; return c }(),
		"last chunk dropped":   ct[:len(ct)-(100+16)],
		"bytes appended":       append(bytes.Clone(ct), 0),
		"header MAC changed": func() []byte {
			c := bytes.Clone(ct)
			c[hdrEnd+5] ^= 1
			return c
		}(),
		"work factor raised": bytes.Replace(ct, []byte(" 4\n"), []byte(" 5\n"), 1),
	}
	for name, c := range cases {
		out, err := open(c, pw)
		if err == nil {
			t.Errorf("%s: opened", name)
		}
		if name == "payload byte flipped" || name == "last chunk dropped" || name == "bytes appended" {
			// Whole chunks before the damage may be written; never the damaged one.
			if len(out) > chunkSize {
				t.Errorf("%s: wrote %d bytes, past the first chunk", name, len(out))
			}
		} else if len(out) != 0 {
			t.Errorf("%s: wrote plaintext before the header was checked", name)
		}
	}
}

func TestRefusals(t *testing.T) {
	var out bytes.Buffer
	if err := Encrypt(&out, strings.NewReader("x"), []byte("short"), 0); err == nil {
		t.Fatal("a short passphrase was accepted")
	}
	if err := Encrypt(&out, strings.NewReader("x"), pass(t), MaxWorkFactor+1); err == nil {
		t.Fatal("an excessive work factor was accepted")
	}
	keyFile := "age-encryption.org/v1\n-> X25519 abc\nAAAA\n--- AAAA\n"
	if _, err := open([]byte(keyFile), pass(t)); !errors.Is(err, ErrNotPassphrase) {
		t.Fatalf("a key-recipient file: %v", err)
	}
	if IsPassphrase([]byte(keyFile)) {
		t.Fatal("a key-recipient file was taken for a passphrase file")
	}
	greedy := "age-encryption.org/v1\n-> scrypt AAAAAAAAAAAAAAAAAAAAAA 30\nAAAA\n--- AAAA\n"
	if _, err := open([]byte(greedy), pass(t)); err == nil || !strings.Contains(err.Error(), "work factor 30") {
		t.Fatalf("a greedy work factor: %v", err)
	}
	if _, err := open([]byte("not age at all\n"), pass(t)); err == nil {
		t.Fatal("garbage opened")
	}
	// A second stanza beside the scrypt one is refused, as age does.
	ct := seal(t, []byte("hello"), pass(t))
	i := bytes.Index(ct, []byte("\n---")) + 1
	two := append(append(bytes.Clone(ct[:i]), []byte("-> X25519 abc\nAAAA\n")...), ct[i:]...)
	if _, err := open(two, pass(t)); err == nil || !strings.Contains(err.Error(), "exactly one") {
		t.Fatalf("two stanzas: %v", err)
	}
}
