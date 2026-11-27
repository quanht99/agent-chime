package autostart

import (
	"os"
	"runtime"
	"strings"
	"testing"

	"github.com/quanht99/agent-chime/go/internal/testenv"
)

type fakeKey map[string]string

func (f fakeKey) Get(n string) (string, bool) { v, ok := f[n]; return v, ok }
func (f fakeKey) Set(n, v string) error       { f[n] = v; return nil }
func (f fakeKey) Delete(n string) error       { delete(f, n); return nil }

func TestSupported(t *testing.T) {
	want := runtime.GOOS == "linux" || runtime.GOOS == "windows"
	if Supported() != want {
		t.Fatalf("Supported = %v", Supported())
	}
}

func TestLinuxDesktopFileContent(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("linux only")
	}
	p := testenv.Home(t)
	p.Program = "/home/example user/agent chime/agentchime"
	if Enabled(p) {
		t.Fatal("enabled before Enable")
	}
	if err := Enable(p); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(p.AutostartFile)
	want := "[Desktop Entry]\nType=Application\nName=AgentChime\nExec=\"/home/example user/agent chime/agentchime\" --background\nX-GNOME-Autostart-enabled=true\n"
	if string(got) != want {
		t.Fatalf("got %q", got)
	}
	if !Enabled(p) {
		t.Fatal("not enabled")
	}
	os.WriteFile(p.AutostartFile, []byte("other"), 0o644)
	if Enabled(p) {
		t.Fatal("foreign content must not count as enabled")
	}
	if err := Disable(p); err != nil || Enabled(p) {
		t.Fatalf("disable: %v", err)
	}
	if err := Disable(p); err != nil {
		t.Fatalf("second disable: %v", err)
	}
}

func TestWindowsRunKeyFake(t *testing.T) {
	k := fakeKey{}
	prog := `C:\Users\example\AppData\Local\AgentChime\AgentChime.exe`
	if runKeyEnabled(k, prog) {
		t.Fatal("enabled before Enable")
	}
	if err := enableRunKey(k, prog); err != nil {
		t.Fatal(err)
	}
	if k["AgentChime"] != `"`+prog+`" --background` {
		t.Fatalf("value = %q", k["AgentChime"])
	}
	if !runKeyEnabled(k, prog) {
		t.Fatal("not enabled")
	}
	if runKeyEnabled(k, `C:\other.exe`) {
		t.Fatal("different program must not count as enabled")
	}
	if err := disableRunKey(k); err != nil || runKeyEnabled(k, prog) {
		t.Fatalf("disable: %v", err)
	}
	if err := disableRunKey(k); err != nil {
		t.Fatal(err)
	}
}

func TestDesktopExecEscaping(t *testing.T) {
	cases := map[string]string{
		"/home/example/.local/share/agentchime/agentchime": `"/home/example/.local/share/agentchime/agentchime" --background`,
		"/home/example user/agent chime/agentchime":        `"/home/example user/agent chime/agentchime" --background`,
		"/home/a%20b/agentchime":                           `"/home/a%%20b/agentchime" --background`,
		"/home/$USER/agentchime":                           `"/home/\\$USER/agentchime" --background`,
		"/home/a`b`/agentchime":                            "\"/home/a\\\\`b\\\\`/agentchime\" --background",
		`/home/say "hi"/agentchime`:                        `"/home/say \\"hi\\"/agentchime" --background`,
		`/home/back\slash/agentchime`:                      `"/home/back\\\\slash/agentchime" --background`,
		"/home/Nguyễn/agentchime":                          `"/home/Nguyễn/agentchime" --background`,
	}
	for in, want := range cases {
		if got := desktopExec(in); got != want {
			t.Errorf("desktopExec(%q)\n got %s\nwant %s", in, got, want)
		}
		// Undo the two layers the way a launcher does and get the path back.
		if got := unescapeExec(t, desktopExec(in)); got != in {
			t.Errorf("round trip %q gave %q", in, got)
		}
	}
}

// unescapeExec applies the string-escape rule, then %%, then the quoting rule, and returns the
// first argument.
func unescapeExec(t *testing.T, v string) string {
	t.Helper()
	var s []rune
	rs := []rune(v)
	for i := 0; i < len(rs); i++ {
		if rs[i] == '\\' && i+1 < len(rs) {
			i++
			switch rs[i] {
			case 'n':
				s = append(s, '\n')
			case 't':
				s = append(s, '\t')
			case 'r':
				s = append(s, '\r')
			default:
				s = append(s, rs[i])
			}
			continue
		}
		s = append(s, rs[i])
	}
	str := strings.ReplaceAll(string(s), "%%", "%")
	if !strings.HasPrefix(str, `"`) {
		t.Fatalf("not quoted: %s", str)
	}
	var arg []rune
	ar := []rune(str[1:])
	for i := 0; i < len(ar); i++ {
		switch {
		case ar[i] == '\\' && i+1 < len(ar):
			i++
			arg = append(arg, ar[i])
		case ar[i] == '"':
			if rest := string(ar[i+1:]); rest != " --background" {
				t.Fatalf("unexpected tail %q", rest)
			}
			return string(arg)
		default:
			arg = append(arg, ar[i])
		}
	}
	t.Fatalf("unterminated quote: %s", str)
	return ""
}
