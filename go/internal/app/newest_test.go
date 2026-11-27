package app

import (
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/quanht99/agent-chime/go/internal/instance"
	"github.com/quanht99/agent-chime/go/internal/testenv"
)

func TestChooseStartTable(t *testing.T) {
	cases := []struct {
		self, data            string
		selfIsData, dataThere bool
		want                  startAction
	}{
		{"0.3.0", "0.2.0", true, true, startAsIs},
		{"0.3.0", "", false, false, startDeploy},
		{"0.3.0", "bogus", false, true, startDeploy},
		{"0.3.0", "0.2.0", false, true, startDeployNewer},
		{"0.10.0", "0.9.0", false, true, startDeployNewer},
		{"0.2.0", "0.3.0", false, true, startRunData},
		{"0.3.0", "0.3.0", false, true, startDeploy},
		{"dev", "0.3.0", false, true, startDeploy},
		{"0.3.0", "dev", false, true, startDeploy},
		{"e2e-new", "0.3.0", false, true, startDeploy},
	}
	for _, c := range cases {
		if got := chooseStart(c.self, c.data, c.selfIsData, c.dataThere); got != c.want {
			t.Errorf("chooseStart(%q,%q,%v,%v) = %d, want %d", c.self, c.data, c.selfIsData, c.dataThere, got, c.want)
		}
	}
}

func buildVersion(t *testing.T, root, ver string) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "agentchime")
	args := []string{"build"}
	if ver != "" {
		args = append(args, "-ldflags", "-X github.com/quanht99/agent-chime/go/internal/version.Version="+ver)
	}
	args = append(args, "-o", bin, "./cmd/agentchime")
	cmd := exec.Command("go", args...)
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	return bin
}

func TestDebNewer(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("no .deb on Windows")
	}
	if testing.Short() {
		t.Skip("builds binaries")
	}
	out, err := exec.Command("go", "env", "GOMOD").Output()
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Dir(strings.TrimSpace(string(out)))
	p := testenv.Home(t)
	if p.DebDir == "" { // paths leaves DebDir empty off Linux; debNewerOS gets the goos explicitly
		p.DebDir = filepath.Join(p.Home, "usr-lib-agentchime")
	}
	if want := filepath.Join(p.Home, "usr-lib-agentchime"); p.DebDir != want {
		t.Fatalf("DebDir = %q, want %q", p.DebDir, want)
	}
	bins := map[string]string{}
	for _, v := range []string{"0.9.0", "0.9.1", "0.9.2", ""} {
		bins[v] = buildVersion(t, root, v)
	}
	put := func(dst, src string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			t.Fatal(err)
		}
		b, err := os.ReadFile(src)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(dst, b, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	put(p.Program, bins["0.9.1"])

	if _, ok := debNewerOS(p, "linux"); ok {
		t.Error("no deb folder: want false")
	}
	if _, ok := debNewerOS(p, "darwin"); ok {
		t.Error("non-linux: want false")
	}
	deb := filepath.Join(p.DebDir, filepath.Base(p.Program))
	for _, c := range []struct {
		ver  string
		want bool
	}{{"0.9.0", false}, {"0.9.2", true}, {"0.9.1", false}, {"", false}} {
		put(deb, bins[c.ver])
		f, ok := debNewerOS(p, "linux")
		if ok != c.want {
			t.Errorf("deb %q: got %v, want %v", c.ver, ok, c.want)
		}
		if ok {
			if f.Program != deb || f.Helper != "" {
				t.Errorf("files = %+v", f)
			}
		}
	}
	put(deb, bins["0.9.2"])
	helper := filepath.Join(p.DebDir, filepath.Base(p.SettingsHelper))
	put(helper, bins["0.9.2"])
	if f, ok := debNewerOS(p, "linux"); !ok || f.Helper != helper {
		t.Errorf("with helper: %+v %v", f, ok)
	}
}

func infoFor(t *testing.T, s *httptest.Server, token string) instance.Info {
	t.Helper()
	i := strings.LastIndex(s.URL, ":")
	port, err := strconv.Atoi(s.URL[i+1:])
	if err != nil {
		t.Fatal(err)
	}
	return instance.Info{Port: port, Token: token}
}

func TestPostRestart404IsError(t *testing.T) {
	s := httptest.NewServer(http.NotFoundHandler())
	defer s.Close()
	if err := postRestart(infoFor(t, s, "tok")); err == nil {
		t.Error("404 should be an error")
	}

	var gotTok, gotHost, gotPath string
	ok := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotTok, gotHost, gotPath = r.Header.Get("X-AgentChime-Token"), r.Host, r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))
	defer ok.Close()
	info := infoFor(t, ok, "tok")
	if err := postRestart(info); err != nil {
		t.Fatal(err)
	}
	if gotTok != "tok" || gotPath != "/api/update/restart" || gotHost != "127.0.0.1:"+strconv.Itoa(info.Port) {
		t.Errorf("token %q path %q host %q", gotTok, gotPath, gotHost)
	}
}
