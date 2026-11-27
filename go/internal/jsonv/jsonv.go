// Package jsonv is an order-preserving JSON value. Numbers keep their raw
// text, and Serialize matches JSON.stringify(v, null, 2) byte for byte, so
// other tools' config content is never reordered or reformatted.
package jsonv

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

type Kind int

const (
	KindNull Kind = iota
	KindBool
	KindNumber
	KindString
	KindArray
	KindObject
)

type Member struct {
	Key string
	Val Value
}

type Value struct {
	Kind Kind
	B    bool
	Num  string // raw number text, exactly as parsed
	S    string
	A    []Value
	O    []Member
}

const maxDepth = 512

func Str(s string) Value     { return Value{Kind: KindString, S: s} }
func Bool(b bool) Value      { return Value{Kind: KindBool, B: b} }
func Arr(vs ...Value) Value  { return Value{Kind: KindArray, A: append([]Value{}, vs...)} }
func Obj(ms ...Member) Value { return Value{Kind: KindObject, O: append([]Member{}, ms...)} }

// Get returns the first member named key; false when v is not an object.
func (v Value) Get(key string) (Value, bool) {
	if v.Kind != KindObject {
		return Value{}, false
	}
	for _, m := range v.O {
		if m.Key == key {
			return m.Val, true
		}
	}
	return Value{}, false
}

// Set replaces the first member named key in place, else appends. No-op on a non-object.
func (v *Value) Set(key string, val Value) {
	if v.Kind != KindObject {
		return
	}
	for i := range v.O {
		if v.O[i].Key == key {
			v.O[i].Val = val
			return
		}
	}
	v.O = append(v.O, Member{Key: key, Val: val})
}

// Delete removes every member named key. No-op on a non-object.
func (v *Value) Delete(key string) {
	if v.Kind != KindObject {
		return
	}
	out := make([]Member, 0, len(v.O))
	for _, m := range v.O {
		if m.Key != key {
			out = append(out, m)
		}
	}
	v.O = out
}

// Serialize writes JSON.stringify(v, null, 2) with no trailing newline.
func (v Value) Serialize() string {
	var sb strings.Builder
	v.write(&sb, 0)
	return sb.String()
}

func (v Value) write(sb *strings.Builder, indent int) {
	switch v.Kind {
	case KindBool:
		if v.B {
			sb.WriteString("true")
		} else {
			sb.WriteString("false")
		}
	case KindNumber:
		sb.WriteString(v.Num)
	case KindString:
		writeString(sb, v.S)
	case KindArray:
		if len(v.A) == 0 {
			sb.WriteString("[]")
			return
		}
		pad := strings.Repeat(" ", indent+2)
		sb.WriteString("[\n")
		for i, it := range v.A {
			sb.WriteString(pad)
			it.write(sb, indent+2)
			if i < len(v.A)-1 {
				sb.WriteString(",\n")
			} else {
				sb.WriteString("\n")
			}
		}
		sb.WriteString(strings.Repeat(" ", indent) + "]")
	case KindObject:
		if len(v.O) == 0 {
			sb.WriteString("{}")
			return
		}
		pad := strings.Repeat(" ", indent+2)
		sb.WriteString("{\n")
		for i, m := range v.O {
			sb.WriteString(pad)
			writeString(sb, m.Key)
			sb.WriteString(": ")
			m.Val.write(sb, indent+2)
			if i < len(v.O)-1 {
				sb.WriteString(",\n")
			} else {
				sb.WriteString("\n")
			}
		}
		sb.WriteString(strings.Repeat(" ", indent) + "}")
	default:
		sb.WriteString("null")
	}
}

const hexDigits = "0123456789abcdef"

func writeString(sb *strings.Builder, s string) {
	sb.WriteByte('"')
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch c {
		case '"':
			sb.WriteString(`\"`)
		case '\\':
			sb.WriteString(`\\`)
		case '\b':
			sb.WriteString(`\b`)
		case '\f':
			sb.WriteString(`\f`)
		case '\n':
			sb.WriteString(`\n`)
		case '\r':
			sb.WriteString(`\r`)
		case '\t':
			sb.WriteString(`\t`)
		default:
			if c < 0x20 {
				sb.WriteString(`\u00`)
				sb.WriteByte(hexDigits[c>>4])
				sb.WriteByte(hexDigits[c&0xF])
			} else {
				sb.WriteByte(c)
			}
		}
	}
	sb.WriteByte('"')
}

// Parse is a strict RFC 8259 parser. One leading UTF-8 BOM is skipped.
// Duplicate keys are kept in order (as the Swift implementation does).
func Parse(data []byte) (Value, error) {
	if len(data) >= 3 && data[0] == 0xEF && data[1] == 0xBB && data[2] == 0xBF {
		data = data[3:]
	}
	p := &parser{b: data}
	p.ws()
	v, err := p.value(1)
	if err != nil {
		return Value{}, err
	}
	p.ws()
	if p.pos != len(p.b) {
		return Value{}, p.fail("unexpected trailing content")
	}
	return v, nil
}

type parser struct {
	b   []byte
	pos int
}

func (p *parser) fail(msg string) error {
	return fmt.Errorf("json: %s at offset %d", msg, p.pos)
}

func (p *parser) ws() {
	for p.pos < len(p.b) {
		switch p.b[p.pos] {
		case ' ', '\t', '\n', '\r':
			p.pos++
		default:
			return
		}
	}
}

// value parses a value; depth is the container nesting level it would occupy.
func (p *parser) value(depth int) (Value, error) {
	if p.pos >= len(p.b) {
		return Value{}, p.fail("unexpected end of input")
	}
	switch c := p.b[p.pos]; {
	case c == '{':
		return p.object(depth)
	case c == '[':
		return p.array(depth)
	case c == '"':
		s, err := p.str()
		return Value{Kind: KindString, S: s}, err
	case c == 't':
		return Value{Kind: KindBool, B: true}, p.expect("true")
	case c == 'f':
		return Value{Kind: KindBool}, p.expect("false")
	case c == 'n':
		return Value{}, p.expect("null")
	case c == '-' || (c >= '0' && c <= '9'):
		n, err := p.number()
		return Value{Kind: KindNumber, Num: n}, err
	}
	return Value{}, p.fail("unexpected character")
}

func (p *parser) expect(w string) error {
	if !strings.HasPrefix(string(p.b[p.pos:min(len(p.b), p.pos+len(w))]), w) {
		return p.fail("invalid literal")
	}
	p.pos += len(w)
	return nil
}

func (p *parser) object(depth int) (Value, error) {
	if depth > maxDepth {
		return Value{}, p.fail("nesting too deep")
	}
	p.pos++
	v := Value{Kind: KindObject, O: []Member{}}
	p.ws()
	if p.pos < len(p.b) && p.b[p.pos] == '}' {
		p.pos++
		return v, nil
	}
	for {
		p.ws()
		if p.pos >= len(p.b) || p.b[p.pos] != '"' {
			return Value{}, p.fail("expected string key")
		}
		key, err := p.str()
		if err != nil {
			return Value{}, err
		}
		p.ws()
		if p.pos >= len(p.b) || p.b[p.pos] != ':' {
			return Value{}, p.fail("expected ':'")
		}
		p.pos++
		p.ws()
		val, err := p.value(depth + 1)
		if err != nil {
			return Value{}, err
		}
		v.O = append(v.O, Member{Key: key, Val: val})
		p.ws()
		if p.pos >= len(p.b) {
			return Value{}, p.fail("unterminated object")
		}
		switch p.b[p.pos] {
		case ',':
			p.pos++
		case '}':
			p.pos++
			return v, nil
		default:
			return Value{}, p.fail("expected ',' or '}'")
		}
	}
}

func (p *parser) array(depth int) (Value, error) {
	if depth > maxDepth {
		return Value{}, p.fail("nesting too deep")
	}
	p.pos++
	v := Value{Kind: KindArray, A: []Value{}}
	p.ws()
	if p.pos < len(p.b) && p.b[p.pos] == ']' {
		p.pos++
		return v, nil
	}
	for {
		p.ws()
		it, err := p.value(depth + 1)
		if err != nil {
			return Value{}, err
		}
		v.A = append(v.A, it)
		p.ws()
		if p.pos >= len(p.b) {
			return Value{}, p.fail("unterminated array")
		}
		switch p.b[p.pos] {
		case ',':
			p.pos++
		case ']':
			p.pos++
			return v, nil
		default:
			return Value{}, p.fail("expected ',' or ']'")
		}
	}
}

func (p *parser) digit() bool { return p.pos < len(p.b) && p.b[p.pos] >= '0' && p.b[p.pos] <= '9' }

func (p *parser) number() (string, error) {
	start := p.pos
	if p.b[p.pos] == '-' {
		p.pos++
	}
	if !p.digit() {
		return "", p.fail("invalid number")
	}
	if p.b[p.pos] == '0' {
		p.pos++
		if p.digit() {
			return "", p.fail("leading zero")
		}
	} else {
		for p.digit() {
			p.pos++
		}
	}
	if p.pos < len(p.b) && p.b[p.pos] == '.' {
		p.pos++
		if !p.digit() {
			return "", p.fail("invalid fraction")
		}
		for p.digit() {
			p.pos++
		}
	}
	if p.pos < len(p.b) && (p.b[p.pos] == 'e' || p.b[p.pos] == 'E') {
		p.pos++
		if p.pos < len(p.b) && (p.b[p.pos] == '+' || p.b[p.pos] == '-') {
			p.pos++
		}
		if !p.digit() {
			return "", p.fail("invalid exponent")
		}
		for p.digit() {
			p.pos++
		}
	}
	return string(p.b[start:p.pos]), nil
}

var errHex = errors.New("invalid \\u escape")

func (p *parser) hex4() (rune, error) {
	if p.pos+4 > len(p.b) {
		return 0, p.fail("truncated \\u escape")
	}
	var v rune
	for i := 0; i < 4; i++ {
		c := p.b[p.pos]
		var d rune
		switch {
		case c >= '0' && c <= '9':
			d = rune(c - '0')
		case c >= 'A' && c <= 'F':
			d = rune(c-'A') + 10
		case c >= 'a' && c <= 'f':
			d = rune(c-'a') + 10
		default:
			return 0, p.fail(errHex.Error())
		}
		v = v*16 + d
		p.pos++
	}
	return v, nil
}

func (p *parser) str() (string, error) {
	p.pos++
	var raw []byte
	for {
		if p.pos >= len(p.b) {
			return "", p.fail("unterminated string")
		}
		c := p.b[p.pos]
		if c == '"' {
			p.pos++
			if !utf8.Valid(raw) {
				return "", p.fail("invalid UTF-8 in string")
			}
			return string(raw), nil
		}
		if c < 0x20 {
			return "", p.fail("unescaped control character")
		}
		p.pos++
		if c != '\\' {
			raw = append(raw, c)
			continue
		}
		if p.pos >= len(p.b) {
			return "", p.fail("unterminated escape")
		}
		e := p.b[p.pos]
		p.pos++
		switch e {
		case '"', '\\', '/':
			raw = append(raw, e)
		case 'b':
			raw = append(raw, 0x08)
		case 'f':
			raw = append(raw, 0x0C)
		case 'n':
			raw = append(raw, '\n')
		case 'r':
			raw = append(raw, '\r')
		case 't':
			raw = append(raw, '\t')
		case 'u':
			code, err := p.hex4()
			if err != nil {
				return "", err
			}
			if code >= 0xD800 && code <= 0xDBFF {
				if p.pos+1 >= len(p.b) || p.b[p.pos] != '\\' || p.b[p.pos+1] != 'u' {
					return "", p.fail("lone high surrogate")
				}
				p.pos += 2
				low, err := p.hex4()
				if err != nil {
					return "", err
				}
				if low < 0xDC00 || low > 0xDFFF {
					return "", p.fail("invalid low surrogate")
				}
				code = 0x10000 + ((code - 0xD800) << 10) + (low - 0xDC00)
			} else if code >= 0xDC00 && code <= 0xDFFF {
				return "", p.fail("lone low surrogate")
			}
			raw = utf8.AppendRune(raw, code)
		default:
			return "", p.fail("invalid escape")
		}
	}
}
