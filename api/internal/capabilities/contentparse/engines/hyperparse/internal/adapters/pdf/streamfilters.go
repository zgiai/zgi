package pdf

import (
	"bytes"
	golzw "compress/lzw"
	"compress/zlib"
	"encoding/ascii85"
	"encoding/hex"
	"fmt"
	"image"
	"image/png"
	"io"
	"strconv"
	"strings"

	"golang.org/x/image/ccitt"
)

// ParseFilterPipeline parses the /Filter chain from object dictionary bytes in PDF order.
func ParseFilterPipeline(dict []byte) []string {
	if len(dict) == 0 {
		return nil
	}
	idx := bytes.Index(dict, []byte("/Filter"))
	if idx < 0 {
		return nil
	}
	rest := bytes.TrimLeft(dict[idx+len("/Filter"):], " \t\r\n")
	if len(rest) == 0 {
		return nil
	}
	var names []string
	if rest[0] == '[' {
		names = parseFilterArray(rest)
	} else {
		n := readPDFNameToken(rest)
		if n != "" {
			names = []string{n}
		}
	}
	out := make([]string, 0, len(names))
	for _, n := range names {
		if m := mapPDFFilterName(n); m != "" {
			out = append(out, m)
		}
	}
	return out
}

func parseFilterArray(rest []byte) []string {
	if len(rest) == 0 || rest[0] != '[' {
		return nil
	}
	var out []string
	i := 1
	for i < len(rest) {
		for i < len(rest) && rest[i] <= 32 {
			i++
		}
		if i >= len(rest) || rest[i] == ']' {
			break
		}
		if rest[i] == '/' {
			tok := readPDFNameToken(rest[i:])
			if tok != "" {
				out = append(out, tok)
				i += len(tok)
				continue
			}
		}
		i++
	}
	return out
}

func readPDFNameToken(b []byte) string {
	if len(b) == 0 || b[0] != '/' {
		return ""
	}
	i := 1
	for i < len(b) {
		c := b[i]
		// PDFs often write `/Filter/FlateDecode`; the second slash starts a new name.
		if c == '/' {
			break
		}
		if c <= ' ' || strings.ContainsRune("[]<>()", rune(c)) {
			break
		}
		i++
	}
	return string(b[:i])
}

func mapPDFFilterName(name string) string {
	switch name {
	case "/FlateDecode":
		return "flate"
	case "/LZWDecode":
		return "lzw"
	case "/ASCII85Decode":
		return "ascii85"
	case "/ASCIIHexDecode":
		return "asciihex"
	case "/DCTDecode":
		return "dct"
	case "/CCITTFaxDecode":
		return "ccitt"
	case "/RunLengthDecode":
		return "runlength"
	default:
		return ""
	}
}

// DecodeStreamFilters decodes a stream according to the dictionary /Filter chain.
func DecodeStreamFilters(dict []byte, raw []byte) ([]byte, error) {
	filters := ParseFilterPipeline(dict)
	if len(filters) == 0 {
		return bytes.TrimSpace(raw), nil
	}
	decoded := bytes.TrimSpace(raw)
	var err error
	for _, f := range filters {
		switch f {
		case "flate":
			decoded, err = tryZlibDecode(decoded)
		case "lzw":
			decoded, err = tryLZWDecode(decoded)
		case "ascii85":
			decoded, err = tryASCII85Decode(decoded)
		case "asciihex":
			decoded, err = tryASCIIHexDecode(decoded)
		case "dct":
			// JPEG bitstream; no extra decoding.
		case "ccitt":
			decoded, err = decodeCCITTFaxToPNG(dict, decoded)
		case "runlength":
			decoded, err = decodeRunLengthPDF(decoded)
		default:
			return nil, fmt.Errorf("unsupported filter: %s", f)
		}
		if err != nil {
			return nil, err
		}
	}
	return decoded, nil
}

// DecodeStreamFiltersBestEffort falls back to raw stream bytes on unknown or
// failed filters so the whole extraction path does not abort.
func DecodeStreamFiltersBestEffort(dict []byte, streamBlock []byte) []byte {
	filters := ParseFilterPipeline(dict)
	if len(filters) == 0 {
		return streamBlock
	}
	decoded := bytes.TrimSpace(streamBlock)
	for _, f := range filters {
		var (
			next []byte
			err  error
		)
		switch f {
		case "flate":
			next, err = tryZlibDecode(decoded)
		case "lzw":
			next, err = tryLZWDecode(decoded)
		case "ascii85":
			next, err = tryASCII85Decode(decoded)
		case "asciihex":
			next, err = tryASCIIHexDecode(decoded)
		case "dct":
			next = decoded
		case "ccitt":
			next, err = decodeCCITTFaxToPNG(dict, decoded)
		case "runlength":
			next, err = decodeRunLengthPDF(decoded)
		default:
			return streamBlock
		}
		if err != nil {
			return streamBlock
		}
		decoded = next
	}
	return decoded
}

func decodeRunLengthPDF(b []byte) ([]byte, error) {
	src := bytes.TrimSpace(b)
	if len(src) == 0 {
		return nil, fmt.Errorf("empty runlength stream")
	}
	var out bytes.Buffer
	i := 0
	for i < len(src) {
		c := int(src[i])
		i++
		if c == 128 {
			break
		}
		if c < 128 {
			n := c + 1
			if i+n > len(src) {
				return nil, fmt.Errorf("runlength overrun")
			}
			out.Write(src[i : i+n])
			i += n
			continue
		}
		n := 257 - c
		if i >= len(src) {
			return nil, fmt.Errorf("runlength missing repeat byte")
		}
		ch := src[i]
		i++
		for j := 0; j < n; j++ {
			out.WriteByte(ch)
		}
	}
	return out.Bytes(), nil
}

func decodeCCITTFaxToPNG(dict []byte, raw []byte) ([]byte, error) {
	cols := parseIntKey(dict, "/Columns")
	rows := parseIntKey(dict, "/Rows")
	k := parseIntKey(dict, "/K")
	dp := mergeDecodeParms(dict)
	if dp != nil {
		if v := parseIntKey(dp, "/Columns"); v > 0 {
			cols = v
		}
		if v := parseIntKey(dp, "/Rows"); v > 0 {
			rows = v
		}
		if strings.Contains(string(dp), "/K") {
			k = parseIntKey(dp, "/K")
		}
	}
	sf := ccitt.Group4
	if k >= 0 {
		sf = ccitt.Group3
	}
	if cols <= 0 || rows <= 0 {
		return nil, fmt.Errorf("ccitt: missing /Columns or /Rows")
	}
	img := image.NewGray(image.Rect(0, 0, cols, rows))
	err := ccitt.DecodeIntoGray(img, bytes.NewReader(raw), ccitt.MSB, sf, nil)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func mergeDecodeParms(dict []byte) []byte {
	for _, key := range []string{"/DecodeParms", "/DP"} {
		pos := bytes.Index(dict, []byte(key))
		if pos < 0 {
			continue
		}
		rest := bytes.TrimLeft(dict[pos+len(key):], " \t\r\n")
		if len(rest) == 0 {
			return nil
		}
		if rest[0] == '<' && len(rest) > 1 && rest[1] == '<' {
			return extractInlineDict(rest)
		}
	}
	return nil
}

func extractInlineDict(rest []byte) []byte {
	if !bytes.HasPrefix(rest, []byte("<<")) {
		return nil
	}
	depth := 0
	for i := 0; i < len(rest)-1; i++ {
		if rest[i] == '<' && rest[i+1] == '<' {
			depth++
			i++
			continue
		}
		if rest[i] == '>' && rest[i+1] == '>' {
			depth--
			i++
			if depth == 0 {
				return rest[:i+1]
			}
		}
	}
	return nil
}

func parseIntKey(dict []byte, key string) int {
	pos := bytes.Index(dict, []byte(key))
	if pos < 0 {
		return 0
	}
	rest := bytes.TrimLeft(dict[pos+len(key):], " \t\r\n")
	var num []byte
	for i := 0; i < len(rest); i++ {
		c := rest[i]
		if c == '-' || (c >= '0' && c <= '9') {
			num = append(num, c)
			continue
		}
		break
	}
	if len(num) == 0 {
		return 0
	}
	n, err := strconv.Atoi(string(num))
	if err != nil {
		return 0
	}
	return n
}

func tryZlibDecode(b []byte) ([]byte, error) {
	r, err := zlib.NewReader(bytes.NewReader(bytes.TrimSpace(b)))
	if err != nil {
		return nil, err
	}
	defer r.Close()
	return io.ReadAll(r)
}

func tryLZWDecode(b []byte) ([]byte, error) {
	r := golzw.NewReader(bytes.NewReader(bytes.TrimSpace(b)), golzw.MSB, 8)
	defer r.Close()
	return io.ReadAll(r)
}

func tryASCII85Decode(b []byte) ([]byte, error) {
	src := bytes.TrimSpace(b)
	src = bytes.TrimSuffix(src, []byte("~>"))
	out := make([]byte, len(src))
	n, _, err := ascii85.Decode(out, src, true)
	if err != nil {
		return nil, err
	}
	return out[:n], nil
}

func tryASCIIHexDecode(b []byte) ([]byte, error) {
	src := strings.TrimSpace(string(b))
	src = strings.TrimSuffix(src, ">")
	src = strings.ReplaceAll(src, "\n", "")
	src = strings.ReplaceAll(src, "\r", "")
	src = strings.ReplaceAll(src, "\t", "")
	src = strings.ReplaceAll(src, " ", "")
	if len(src)%2 == 1 {
		src += "0"
	}
	return hex.DecodeString(src)
}
