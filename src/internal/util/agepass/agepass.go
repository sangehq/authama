// Package agepass encrypts and decrypts with a passphrase in the age v1 file
// format (https://age-encryption.org/v1), using its scrypt recipient: the same
// bytes `age -p` writes and `age -d` reads, so a file authama makes can be
// opened with the age tool alone, and the other way round.
//
// Why this and not something new: age is a published, reviewed format, and the
// rest of authama's backups already use it (through the age binary, to an X25519
// recipient). A passphrase has no recipient file to point age at and age reads
// one only from a terminal, so the scrypt recipient is implemented here, from
// golang.org/x/crypto - scrypt, HKDF-SHA-256 and ChaCha20-Poly1305, the exact
// primitives the format specifies - rather than by adding a dependency. The
// construction is the format's, not ours:
//
//   - a random 16-byte file key, wrapped by ChaCha20-Poly1305 under
//     scrypt(passphrase, "age-encryption.org/v1/scrypt" || salt, N=2^logN, r=8, p=1);
//   - a header MAC, HMAC-SHA-256 under HKDF(file key, "header");
//   - the payload in 64 KiB chunks of ChaCha20-Poly1305 under HKDF(file key,
//     nonce, "payload"), each chunk's nonce a counter with a last-chunk flag,
//     so truncation, reordering and appending are all detected.
//
// The tests check it against the age binary when one is installed.
package agepass

import (
	"bufio"
	"bytes"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"golang.org/x/crypto/chacha20poly1305"
	"golang.org/x/crypto/hkdf"
	"golang.org/x/crypto/scrypt"
)

const (
	intro       = "age-encryption.org/v1"
	scryptLabel = "age-encryption.org/v1/scrypt"
	footer      = "---"
	columns     = 64
	chunkSize   = 64 * 1024
	fileKeySize = 16
	nonceSize   = 16
	saltSize    = 16
)

// DefaultWorkFactor is age's own: N = 2^18, about a second on a laptop.
const DefaultWorkFactor = 18

// MaxWorkFactor is the most a file may ask for when it is opened, as age
// allows: a file cannot make a decryption take minutes and gigabytes.
const MaxWorkFactor = 22

// MinPassphrase is the shortest passphrase Encrypt accepts. A passphrase is
// the only thing between the file and anyone who has a copy of it.
const MinPassphrase = 12

// ErrWrongPassphrase is a passphrase that does not open the file (or a file
// whose header was changed: the two cannot be told apart, by design).
var ErrWrongPassphrase = errors.New("the passphrase does not open this file")

// ErrNotPassphrase is an age file encrypted to a recipient's key, not a passphrase.
var ErrNotPassphrase = errors.New("this age file is encrypted to a key, not a passphrase: open it with its identity")

var b64 = base64.RawStdEncoding

// IsAge reports whether head (the first bytes of a file) starts an age file.
func IsAge(head []byte) bool { return bytes.HasPrefix(head, []byte(intro+"\n")) }

// IsPassphrase reports whether head starts an age file encrypted with a passphrase.
func IsPassphrase(head []byte) bool { return bytes.HasPrefix(head, []byte(intro+"\n-> scrypt ")) }

// Encrypt writes src to dst encrypted with passphrase, at work factor logN
// (DefaultWorkFactor when 0).
func Encrypt(dst io.Writer, src io.Reader, passphrase []byte, logN int) error {
	if len(passphrase) < MinPassphrase {
		return fmt.Errorf("the passphrase is %d characters; use at least %d", len(passphrase), MinPassphrase)
	}
	if logN == 0 {
		logN = DefaultWorkFactor
	}
	if logN < 1 || logN > MaxWorkFactor {
		return fmt.Errorf("work factor %d is outside 1..%d", logN, MaxWorkFactor)
	}
	fileKey := make([]byte, fileKeySize)
	salt := make([]byte, saltSize)
	nonce := make([]byte, nonceSize)
	for _, b := range [][]byte{fileKey, salt, nonce} {
		if _, err := rand.Read(b); err != nil {
			return err
		}
	}
	wrapKey, err := scrypt.Key(passphrase, append([]byte(scryptLabel), salt...), 1<<logN, 8, 1, chacha20poly1305.KeySize)
	if err != nil {
		return err
	}
	aead, err := chacha20poly1305.New(wrapKey)
	if err != nil {
		return err
	}
	body := aead.Seal(nil, make([]byte, chacha20poly1305.NonceSize), fileKey, nil)

	var hdr bytes.Buffer
	hdr.WriteString(intro + "\n")
	fmt.Fprintf(&hdr, "-> scrypt %s %d\n", b64.EncodeToString(salt), logN)
	writeWrapped(&hdr, b64.EncodeToString(body))
	hdr.WriteString(footer)
	mac, err := headerMAC(fileKey, hdr.Bytes())
	if err != nil {
		return err
	}
	hdr.WriteString(" " + b64.EncodeToString(mac) + "\n")
	if _, err := dst.Write(hdr.Bytes()); err != nil {
		return err
	}
	if _, err := dst.Write(nonce); err != nil {
		return err
	}
	payloadKey, err := hkdfKey(fileKey, nonce, "payload")
	if err != nil {
		return err
	}
	return sealStream(dst, src, payloadKey)
}

// writeWrapped writes s in lines of 64 columns; the last line is always
// shorter, so a body that fills its last line is followed by an empty one.
func writeWrapped(w *bytes.Buffer, s string) {
	for len(s) >= columns {
		w.WriteString(s[:columns] + "\n")
		s = s[columns:]
	}
	w.WriteString(s + "\n")
}

func headerMAC(fileKey, header []byte) ([]byte, error) {
	k, err := hkdfKey(fileKey, nil, "header")
	if err != nil {
		return nil, err
	}
	h := hmac.New(sha256.New, k)
	h.Write(header)
	return h.Sum(nil), nil
}

func hkdfKey(secret, salt []byte, info string) ([]byte, error) {
	k := make([]byte, chacha20poly1305.KeySize)
	if _, err := io.ReadFull(hkdf.New(sha256.New, secret, salt, []byte(info)), k); err != nil {
		return nil, err
	}
	return k, nil
}

// chunkNonce is the STREAM nonce: an 11-byte big-endian counter and a byte
// that is 1 on the last chunk only.
func chunkNonce(counter uint64, last bool) []byte {
	n := make([]byte, chacha20poly1305.NonceSize)
	for i := 10; i >= 3; i-- {
		n[i] = byte(counter)
		counter >>= 8
	}
	if last {
		n[11] = 1
	}
	return n
}

func sealStream(dst io.Writer, src io.Reader, key []byte) error {
	aead, err := chacha20poly1305.New(key)
	if err != nil {
		return err
	}
	r := bufio.NewReaderSize(src, chunkSize+1)
	buf := make([]byte, chunkSize)
	for counter := uint64(0); ; counter++ {
		n, err := io.ReadFull(r, buf)
		if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
			return err
		}
		last := n < chunkSize
		if !last {
			if _, perr := r.Peek(1); errors.Is(perr, io.EOF) {
				last = true
			} else if perr != nil {
				return perr
			}
		}
		if _, err := dst.Write(aead.Seal(nil, chunkNonce(counter, last), buf[:n], nil)); err != nil {
			return err
		}
		if last {
			return nil
		}
	}
}

// Decrypt writes the plaintext of the passphrase-encrypted age file src to
// dst. Nothing reaches dst before the header is authenticated, and every chunk
// is authenticated before it is written; a file cut short or extended fails.
func Decrypt(dst io.Writer, src io.Reader, passphrase []byte) error {
	r := bufio.NewReaderSize(src, chunkSize+chacha20poly1305.Overhead+1)
	fileKey, err := openHeader(r, passphrase)
	if err != nil {
		return err
	}
	nonce := make([]byte, nonceSize)
	if _, err := io.ReadFull(r, nonce); err != nil {
		return errors.New("the file ends before its payload")
	}
	key, err := hkdfKey(fileKey, nonce, "payload")
	if err != nil {
		return err
	}
	aead, err := chacha20poly1305.New(key)
	if err != nil {
		return err
	}
	buf := make([]byte, chunkSize+chacha20poly1305.Overhead)
	for counter := uint64(0); ; counter++ {
		n, err := io.ReadFull(r, buf)
		if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
			return err
		}
		last := n < len(buf)
		if !last {
			if _, perr := r.Peek(1); errors.Is(perr, io.EOF) {
				last = true
			}
		}
		if n < chacha20poly1305.Overhead {
			return errors.New("the file is cut short")
		}
		plain, err := aead.Open(nil, chunkNonce(counter, last), buf[:n], nil)
		if err != nil {
			return errors.New("the file is damaged or cut short: a chunk failed its check")
		}
		if last && len(plain) == 0 && counter > 0 {
			return errors.New("the file ends with an empty chunk, which age never writes")
		}
		if _, err := dst.Write(plain); err != nil {
			return err
		}
		if last {
			return nil
		}
	}
}

// openHeader reads and checks the header, returning the file key.
func openHeader(r *bufio.Reader, passphrase []byte) ([]byte, error) {
	var raw bytes.Buffer
	line := func() (string, error) {
		s, err := r.ReadString('\n')
		if err != nil {
			return "", errors.New("not an age file, or its header is cut short")
		}
		raw.WriteString(s)
		return strings.TrimSuffix(s, "\n"), nil
	}
	first, err := line()
	if err != nil || first != intro {
		return nil, errors.New("not an age file")
	}
	stanza, err := line()
	if err != nil {
		return nil, err
	}
	args := strings.Split(stanza, " ")
	if len(args) < 2 || args[0] != "->" {
		return nil, errors.New("the age header is malformed")
	}
	if args[1] != "scrypt" {
		return nil, ErrNotPassphrase
	}
	if len(args) != 4 {
		return nil, errors.New("the scrypt stanza is malformed")
	}
	salt, err := b64.Strict().DecodeString(args[2])
	if err != nil || len(salt) != saltSize {
		return nil, errors.New("the scrypt salt is malformed")
	}
	if args[3] == "" || args[3][0] == '0' {
		return nil, errors.New("the scrypt work factor is malformed")
	}
	logN, err := strconv.Atoi(args[3])
	if err != nil || logN < 1 {
		return nil, errors.New("the scrypt work factor is malformed")
	}
	if logN > MaxWorkFactor {
		return nil, fmt.Errorf("the file asks for scrypt work factor %d, over the %d this opens", logN, MaxWorkFactor)
	}
	var body strings.Builder
	for {
		l, err := line()
		if err != nil {
			return nil, err
		}
		if len(l) > columns {
			return nil, errors.New("the age header is malformed")
		}
		body.WriteString(l)
		if len(l) < columns {
			break
		}
	}
	wrapped, err := b64.Strict().DecodeString(body.String())
	if err != nil || len(wrapped) != fileKeySize+chacha20poly1305.Overhead {
		return nil, errors.New("the scrypt stanza's body is malformed")
	}
	// Exactly one stanza: age refuses a scrypt stanza beside any other, since a
	// second one would let whoever holds its key open a "passphrase" file.
	end, err := r.ReadString('\n')
	if err != nil {
		return nil, errors.New("the age header is cut short")
	}
	if strings.HasPrefix(end, "->") {
		return nil, errors.New("a passphrase file must have exactly one recipient stanza")
	}
	mac64, ok := strings.CutPrefix(strings.TrimSuffix(end, "\n"), footer+" ")
	if !ok {
		return nil, errors.New("the age header is malformed")
	}
	raw.WriteString(footer)
	gotMAC, err := b64.Strict().DecodeString(mac64)
	if err != nil {
		return nil, errors.New("the header MAC is malformed")
	}
	wrapKey, err := scrypt.Key(passphrase, append([]byte(scryptLabel), salt...), 1<<logN, 8, 1, chacha20poly1305.KeySize)
	if err != nil {
		return nil, err
	}
	aead, err := chacha20poly1305.New(wrapKey)
	if err != nil {
		return nil, err
	}
	fileKey, err := aead.Open(nil, make([]byte, chacha20poly1305.NonceSize), wrapped, nil)
	if err != nil {
		return nil, ErrWrongPassphrase
	}
	want, err := headerMAC(fileKey, raw.Bytes())
	if err != nil {
		return nil, err
	}
	if subtle.ConstantTimeCompare(want, gotMAC) != 1 {
		return nil, ErrWrongPassphrase
	}
	return fileKey, nil
}
