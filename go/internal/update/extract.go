package update

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// Extract writes the wanted files from a .zip or .tar.gz archive into dir
// (mode 0755) and returns name -> path. Everything else is skipped. On any
// error nothing it created is left behind.
func Extract(archive, dir string, want []string) (_ map[string]string, err error) {
	for _, w := range want {
		if w == "" || w != filepath.Base(w) || strings.ContainsAny(w, `/\`) {
			return nil, fmt.Errorf("extract: bad wanted name %q", w)
		}
	}
	e := &extractor{dir: dir, want: want, out: map[string]string{}}
	defer func() {
		if err != nil {
			for _, p := range e.out {
				os.Remove(p)
			}
		}
	}()
	if strings.HasSuffix(archive, ".zip") {
		err = e.zip(archive)
	} else {
		err = e.tar(archive)
	}
	if err != nil {
		return nil, err
	}
	for _, w := range want {
		if _, ok := e.out[w]; !ok {
			return nil, fmt.Errorf("extract: %s missing from archive", w)
		}
	}
	return e.out, nil
}

type extractor struct {
	dir  string
	want []string
	out  map[string]string
}

// match validates an entry name and returns the wanted name it maps to, or "".
func (e *extractor) match(name string) (string, error) {
	if name == "" || strings.Contains(name, `\`) || strings.HasPrefix(name, "/") || filepath.IsAbs(name) {
		return "", fmt.Errorf("extract: unsafe entry name %q", name)
	}
	for _, el := range strings.Split(name, "/") {
		if el == ".." {
			return "", fmt.Errorf("extract: unsafe entry name %q", name)
		}
	}
	clean := path.Clean(name)
	for _, w := range e.want {
		if clean == w {
			return w, nil
		}
		if i := strings.Index(clean, "/"); i > 0 && clean[i+1:] == w && !strings.Contains(clean[:i], "/") {
			return w, nil
		}
	}
	return "", nil
}

func (e *extractor) write(w string, r io.Reader) error {
	if _, dup := e.out[w]; dup {
		return fmt.Errorf("extract: %s appears twice", w)
	}
	p := filepath.Join(e.dir, w)
	f, err := os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o755)
	if err != nil {
		return err
	}
	e.out[w] = p
	n, err := io.Copy(f, io.LimitReader(r, MaxArchive+1))
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return err
	}
	if n > MaxArchive {
		return fmt.Errorf("extract: %s too large", w)
	}
	return os.Chmod(p, 0o755)
}

func (e *extractor) zip(archive string) error {
	zr, err := zip.OpenReader(archive)
	if err != nil {
		return err
	}
	defer zr.Close()
	// Validate every name before writing anything.
	matches := make([]string, len(zr.File))
	for i, zf := range zr.File {
		if matches[i], err = e.match(zf.Name); err != nil {
			return err
		}
	}
	for i, zf := range zr.File {
		w := matches[i]
		if w == "" {
			continue
		}
		if !zf.Mode().IsRegular() {
			return fmt.Errorf("extract: %s is not a regular file", zf.Name)
		}
		if zf.UncompressedSize64 > MaxArchive {
			return fmt.Errorf("extract: %s too large", zf.Name)
		}
		rc, err := zf.Open()
		if err != nil {
			return err
		}
		err = e.write(w, rc)
		rc.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func (e *extractor) tar(archive string) error {
	f, err := os.Open(archive)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		if h.Typeflag == tar.TypeXGlobalHeader {
			continue
		}
		w, err := e.match(h.Name)
		if err != nil {
			return err
		}
		if w == "" {
			continue
		}
		if h.Typeflag != tar.TypeReg {
			return fmt.Errorf("extract: %s is not a regular file", h.Name)
		}
		if h.Size > MaxArchive {
			return fmt.Errorf("extract: %s too large", h.Name)
		}
		if err := e.write(w, tr); err != nil {
			return err
		}
	}
}
