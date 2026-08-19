// Streaming, encoding-aware line reader used by content search. Instead of
// reading an entire (<= maxSize) file into memory and decoding it as a whole,
// it decodes and yields one line at a time, so a file never occupies more
// than a line's worth of memory while scanning. (See docs/PERFORMANCE.md S4.)
package content

import (
	"bufio"
	"io"
	"unicode/utf8"

	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
)

// lineScanner is the common interface for the line readers we build.
type lineScanner interface {
	Scan() bool
	Text() string
}

// newLineScanner wraps r and picks a decoder based on the leading bytes:
//   - UTF-16 by BOM (LE/BE)
//   - BOM-less UTF-16 by the alternating-NUL heuristic
//   - UTF-8 / ASCII as-is
//   - otherwise GBK fallback (Chinese files without BOM)
func newLineScanner(r *bufio.Reader, maxSize int64) lineScanner {
	header, _ := r.Peek(512)
	if len(header) >= 2 {
		if header[0] == 0xFF && header[1] == 0xFE {
			return newUTF16Scanner(r, false)
		}
		if header[0] == 0xFE && header[1] == 0xFF {
			return newUTF16Scanner(r, true)
		}
	}
	if utf8.Valid(header) && !looksLikeUTF16(header) {
		return &bufLine{sc: bufio.NewScanner(r)}
	}
	if looksLikeUTF16(header) {
		return newUTF16Scanner(r, false) // BOM-less UTF-16LE
	}
	return &bufLine{sc: bufio.NewScanner(transform.NewReader(r, simplifiedchinese.GBK.NewDecoder()))}
}

// bufLine scans plain decoded text (UTF-8 or GBK-transcoded) via a Scanner.
type bufLine struct {
	sc *bufio.Scanner
}

func (b *bufLine) Scan() bool { return b.sc.Scan() }
func (b *bufLine) Text() string {
	return trimCR(b.sc.Text())
}

// utf16Scanner decodes UTF-16 code units from the raw reader into lines.
type utf16Scanner struct {
	r         *bufio.Reader
	bigEndian bool
	buf       []byte // assembled line (UTF-8 encoded)
	done      bool
	err       error
}

func newUTF16Scanner(r *bufio.Reader, bigEndian bool) *utf16Scanner {
	return &utf16Scanner{r: r, bigEndian: bigEndian}
}

func (u *utf16Scanner) readRune() (rune, bool) {
	b := make([]byte, 2)
	n, err := io.ReadFull(u.r, b)
	if n < 2 {
		return 0, false // EOF or odd trailing byte
	}
	if err != nil && err != io.ErrUnexpectedEOF {
		u.err = err
		return 0, false
	}
	var c uint16
	if u.bigEndian {
		c = uint16(b[0])<<8 | uint16(b[1])
	} else {
		c = uint16(b[0]) | uint16(b[1])<<8
	}
	r := rune(c)
	// surrogate pair (rare in the files we index)
	if r >= 0xD800 && r <= 0xDBFF {
		lo := make([]byte, 2)
		n2, _ := io.ReadFull(u.r, lo)
		if n2 == 2 {
			var c2 uint16
			if u.bigEndian {
				c2 = uint16(lo[0])<<8 | uint16(lo[1])
			} else {
				c2 = uint16(lo[0]) | uint16(lo[1])<<8
			}
			if c2 >= 0xDC00 && c2 <= 0xDFFF {
				r = ((r - 0xD800) << 10) + (rune(c2) - 0xDC00) + 0x10000
			}
		}
	}
	return r, true
}

func (u *utf16Scanner) Scan() bool {
	if u.done || u.err != nil {
		return false
	}
	u.buf = u.buf[:0]
	for {
		r, ok := u.readRune()
		if !ok {
			u.done = true
			return len(u.buf) > 0 // emit a trailing line without newline
		}
		if r == '\n' {
			return true
		}
		// drop \r before emitting
		if r == '\r' {
			continue
		}
		u.buf = appendUTF8(u.buf, r)
	}
}

func (u *utf16Scanner) Text() string { return string(u.buf) }

// trimCR removes a trailing CR (CRLF inputs).
func trimCR(s string) string {
	if len(s) > 0 && s[len(s)-1] == '\r' {
		return s[:len(s)-1]
	}
	return s
}

// appendUTF8 appends the UTF-8 encoding of r to dst.
func appendUTF8(dst []byte, r rune) []byte {
	if r < utf8.RuneSelf {
		return append(dst, byte(r))
	}
	var tmp [utf8.UTFMax]byte
	n := utf8.EncodeRune(tmp[:], r)
	return append(dst, tmp[:n]...)
}
