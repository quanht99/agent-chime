package sounds

import (
	"embed"
	"io/fs"
)

//go:embed builtin/*.wav
var builtinEmbed embed.FS

// BuiltinFS is rooted at the built-in sound files.
var BuiltinFS fs.FS = mustSub(builtinEmbed, "builtin")

func mustSub(f fs.FS, dir string) fs.FS {
	s, err := fs.Sub(f, dir)
	if err != nil {
		panic(err)
	}
	return s
}
