package web

import (
	"bytes"
	"encoding/binary"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

// fileText is data as the text a person reads, and whether it is text at
// all. Windows tools' files are UTF-16 with a byte-order mark (PuTTY's .reg
// and Xshell's .xsh, for two), which shown as bytes is noise, so they are
// decoded; a UTF-8 byte-order mark is dropped; anything else that is not
// UTF-8 is not text.
func fileText(data []byte) (string, bool) {
	switch {
	case len(data) >= 2 && data[0] == 0xFF && data[1] == 0xFE:
		return utf16Text(data[2:], binary.LittleEndian), true
	case len(data) >= 2 && data[0] == 0xFE && data[1] == 0xFF:
		return utf16Text(data[2:], binary.BigEndian), true
	}
	data = bytes.TrimPrefix(data, []byte("\xEF\xBB\xBF"))
	if !utf8.Valid(data) {
		return "", false
	}
	return string(data), true
}

func utf16Text(data []byte, order binary.ByteOrder) string {
	units := make([]uint16, len(data)/2)
	for i := range units {
		units[i] = order.Uint16(data[2*i:])
	}
	return strings.ReplaceAll(string(utf16.Decode(units)), "\r\n", "\n")
}
