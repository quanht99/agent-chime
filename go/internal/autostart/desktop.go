package autostart

import "strings"

// desktopExec returns the Exec value that starts prog with --background, escaped per the
// freedesktop Desktop Entry spec. The program is quoted, and inside the quotes `"`, backtick,
// `$` and `\` get a backslash. The string-escape rule is applied on top of that, so every
// backslash is doubled again, and a literal `%` is written as `%%`.
func desktopExec(prog string) string {
	var q strings.Builder
	q.WriteByte('"')
	for _, r := range prog {
		switch r {
		case '"', '`', '$', '\\':
			q.WriteByte('\\')
		}
		q.WriteRune(r)
	}
	q.WriteByte('"')
	var out strings.Builder
	for _, r := range q.String() {
		switch r {
		case '\\':
			out.WriteString(`\\`)
		case '\n':
			out.WriteString(`\n`)
		case '\t':
			out.WriteString(`\t`)
		case '\r':
			out.WriteString(`\r`)
		case '%':
			out.WriteString("%%")
		default:
			out.WriteRune(r)
		}
	}
	return out.String() + " --background"
}
