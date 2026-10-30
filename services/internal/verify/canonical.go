// Package verify implements the Phase 1 signed-record trust boundary.
package verify

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"math/big"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

// ParseJSON parses one strict JSON value. Duplicate object keys, invalid UTF-8,
// unpaired UTF-16 surrogate escapes, and trailing values are rejected rather
// than normalized into a different signed representation.
func ParseJSON(raw []byte) (any, error) {
	if !utf8.Valid(raw) {
		return nil, fmt.Errorf("JSON is not valid UTF-8")
	}
	if err := validateSurrogateEscapes(raw); err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	value, err := parseValue(decoder)
	if err != nil {
		return nil, err
	}
	if _, err := decoder.Token(); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("trailing JSON value")
		}
		return nil, err
	}
	return value, nil
}

func parseValue(decoder *json.Decoder) (any, error) {
	token, err := decoder.Token()
	if err != nil {
		return nil, err
	}
	switch value := token.(type) {
	case json.Delim:
		switch value {
		case '{':
			object := make(map[string]any)
			for decoder.More() {
				keyToken, err := decoder.Token()
				if err != nil {
					return nil, err
				}
				key, ok := keyToken.(string)
				if !ok {
					return nil, fmt.Errorf("object key is not a string")
				}
				if _, exists := object[key]; exists {
					return nil, fmt.Errorf("duplicate object key %q", key)
				}
				child, err := parseValue(decoder)
				if err != nil {
					return nil, err
				}
				object[key] = child
			}
			end, err := decoder.Token()
			if err != nil {
				return nil, err
			}
			if end != json.Delim('}') {
				return nil, fmt.Errorf("unterminated JSON object")
			}
			return object, nil
		case '[':
			array := make([]any, 0)
			for decoder.More() {
				child, err := parseValue(decoder)
				if err != nil {
					return nil, err
				}
				array = append(array, child)
			}
			end, err := decoder.Token()
			if err != nil {
				return nil, err
			}
			if end != json.Delim(']') {
				return nil, fmt.Errorf("unterminated JSON array")
			}
			return array, nil
		default:
			return nil, fmt.Errorf("unexpected JSON delimiter %q", value)
		}
	case json.Number, string, bool, nil:
		return value, nil
	default:
		return nil, fmt.Errorf("unsupported JSON token %T", token)
	}
}

// validateSurrogateEscapes prevents encoding/json's replacement of an
// unpaired \uD800-\uDFFF escape with U+FFFD from changing signed input.
func validateSurrogateEscapes(raw []byte) error {
	inString := false
	for i := 0; i < len(raw); i++ {
		if !inString {
			if raw[i] == '"' {
				inString = true
			}
			continue
		}
		if raw[i] == '"' {
			inString = false
			continue
		}
		if raw[i] != '\\' {
			continue
		}
		if i+1 >= len(raw) {
			return fmt.Errorf("incomplete JSON escape")
		}
		if raw[i+1] != 'u' {
			i++
			continue
		}
		if i+6 > len(raw) {
			return fmt.Errorf("incomplete Unicode escape")
		}
		code, err := strconv.ParseUint(string(raw[i+2:i+6]), 16, 16)
		if err != nil {
			return fmt.Errorf("invalid Unicode escape")
		}
		switch {
		case code >= 0xD800 && code <= 0xDBFF:
			if i+12 > len(raw) || raw[i+6] != '\\' || raw[i+7] != 'u' {
				return fmt.Errorf("unpaired high surrogate escape")
			}
			low, err := strconv.ParseUint(string(raw[i+8:i+12]), 16, 16)
			if err != nil || low < 0xDC00 || low > 0xDFFF {
				return fmt.Errorf("unpaired high surrogate escape")
			}
			i += 11
		case code >= 0xDC00 && code <= 0xDFFF:
			return fmt.Errorf("unpaired low surrogate escape")
		default:
			i += 5
		}
	}
	return nil
}

// CanonicalJSON returns ADR-001 canonical JSON for values returned by ParseJSON.
func CanonicalJSON(value any) ([]byte, error) {
	var out bytes.Buffer
	if err := writeCanonical(&out, value); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func writeCanonical(out *bytes.Buffer, value any) error {
	switch value := value.(type) {
	case nil:
		out.WriteString("null")
	case bool:
		if value {
			out.WriteString("true")
		} else {
			out.WriteString("false")
		}
	case string:
		return writeString(out, value)
	case json.Number:
		number, err := canonicalNumber(string(value))
		if err != nil {
			return err
		}
		out.WriteString(number)
	case []any:
		out.WriteByte('[')
		for i, item := range value {
			if i > 0 {
				out.WriteByte(',')
			}
			if err := writeCanonical(out, item); err != nil {
				return err
			}
		}
		out.WriteByte(']')
	case map[string]any:
		keys := make([]string, 0, len(value))
		for key := range value {
			if !utf8.ValidString(key) {
				return fmt.Errorf("object key is not valid UTF-8")
			}
			keys = append(keys, key)
		}
		sort.Strings(keys)
		out.WriteByte('{')
		for i, key := range keys {
			if i > 0 {
				out.WriteByte(',')
			}
			if err := writeString(out, key); err != nil {
				return err
			}
			out.WriteByte(':')
			if err := writeCanonical(out, value[key]); err != nil {
				return err
			}
		}
		out.WriteByte('}')
	default:
		return fmt.Errorf("unsupported canonical JSON value %T", value)
	}
	return nil
}

func writeString(out *bytes.Buffer, value string) error {
	if !utf8.ValidString(value) {
		return fmt.Errorf("string is not valid UTF-8")
	}
	out.WriteByte('"')
	for _, r := range value {
		switch r {
		case '"':
			out.WriteString("\\\"")
		case '\\':
			out.WriteString("\\\\")
		case '\b':
			out.WriteString("\\b")
		case '\f':
			out.WriteString("\\f")
		case '\n':
			out.WriteString("\\n")
		case '\r':
			out.WriteString("\\r")
		case '\t':
			out.WriteString("\\t")
		default:
			if r < 0x20 {
				out.WriteString("\\u00")
				const hexDigits = "0123456789abcdef"
				out.WriteByte(hexDigits[byte(r)>>4])
				out.WriteByte(hexDigits[byte(r)&0x0f])
			} else {
				out.WriteRune(r)
			}
		}
	}
	out.WriteByte('"')
	return nil
}

func canonicalNumber(raw string) (string, error) {
	if !strings.ContainsAny(raw, ".eE") {
		integer := new(big.Int)
		if _, ok := integer.SetString(raw, 10); !ok {
			return "", fmt.Errorf("invalid JSON integer %q", raw)
		}
		return integer.String(), nil
	}
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return "", fmt.Errorf("JSON float %q is outside binary64 range", raw)
	}
	if math.IsInf(value, 0) || math.IsNaN(value) {
		return "", fmt.Errorf("JSON float %q is not finite", raw)
	}
	return pythonFloat(value), nil
}

// pythonFloat matches Python 3 json.dumps' shortest binary64 spelling and
// fixed/scientific notation thresholds used by the Phase 0 vectors.
func pythonFloat(value float64) string {
	if value == 0 {
		if math.Signbit(value) {
			return "-0.0"
		}
		return "0.0"
	}
	formatted := strconv.FormatFloat(value, 'g', -1, 64)
	exponentAt := strings.IndexByte(formatted, 'e')
	if exponentAt < 0 {
		if !strings.Contains(formatted, ".") {
			formatted += ".0"
		}
		return formatted
	}
	exponent, _ := strconv.Atoi(formatted[exponentAt+1:])
	if exponent >= -4 && exponent < 16 {
		return exponentToFixed(formatted[:exponentAt], exponent)
	}
	mantissa := formatted[:exponentAt]
	sign := "+"
	if exponent < 0 {
		sign = "-"
		exponent = -exponent
	}
	return mantissa + "e" + sign + fmt.Sprintf("%02d", exponent)
}

func exponentToFixed(mantissa string, exponent int) string {
	sign := ""
	if strings.HasPrefix(mantissa, "-") {
		sign = "-"
		mantissa = strings.TrimPrefix(mantissa, "-")
	}
	digits := strings.ReplaceAll(mantissa, ".", "")
	decimalAt := 1 + exponent
	switch {
	case decimalAt <= 0:
		return sign + "0." + strings.Repeat("0", -decimalAt) + digits
	case decimalAt >= len(digits):
		return sign + digits + strings.Repeat("0", decimalAt-len(digits)) + ".0"
	default:
		return sign + digits[:decimalAt] + "." + digits[decimalAt:]
	}
}

// CanonicalDigest returns the ADR-001 sha256 digest of a parsed JSON value.
func CanonicalDigest(value any) (string, error) {
	canonical, err := CanonicalJSON(value)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(canonical)
	return "sha256:" + hex.EncodeToString(digest[:]), nil
}

// PayloadDigest returns the digest of a record with its top-level signature
// field removed, as required by ADR-001.
func PayloadDigest(raw []byte) (string, error) {
	value, err := ParseJSON(raw)
	if err != nil {
		return "", err
	}
	record, ok := value.(map[string]any)
	if !ok {
		return "", fmt.Errorf("signed record must be a JSON object")
	}
	unsigned := make(map[string]any, len(record))
	for key, child := range record {
		if key != "signature" {
			unsigned[key] = child
		}
	}
	return CanonicalDigest(unsigned)
}
