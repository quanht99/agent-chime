// Package i18n holds the EN/VI text tables shared with the macOS app.
package i18n

import "strings"

type Lang string

const (
	EN Lang = "en"
	VI Lang = "vi"
)

func (l Lang) NativeName() string {
	if l == VI {
		return "Tiếng Việt"
	}
	return "English"
}

type Key string

type entry struct {
	key    Key
	en, vi string
}

var table = func() map[Key]entry {
	m := make(map[Key]entry, len(entries))
	for _, e := range entries {
		m[e.key] = e
	}
	return m
}()

// Keys lists every key in table order.
func Keys() []Key {
	ks := make([]Key, len(entries))
	for i, e := range entries {
		ks[i] = e.key
	}
	return ks
}

// T returns the text for key, replacing each "%@" in order with args.
// An unknown language falls back to English; an unknown key yields "".
func T(key Key, lang Lang, args ...string) string {
	e, ok := table[key]
	if !ok {
		return ""
	}
	s := e.en
	if lang == VI {
		s = e.vi
	}
	for _, a := range args {
		i := strings.Index(s, "%@")
		if i < 0 {
			break
		}
		s = s[:i] + a + s[i+2:]
	}
	return s
}
