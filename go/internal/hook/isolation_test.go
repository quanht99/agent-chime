package hook

import (
	"os/exec"
	"strings"
	"testing"
)

// The hook must stay tiny and offline: it never links the updater, the app or
// the web server, and never pulls in net/http.
func TestHookDoesNotImportUpdate(t *testing.T) {
	out, err := exec.Command("go", "list", "-deps", ".").Output()
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Fields(string(out)) {
		if line == "net/http" ||
			strings.HasSuffix(line, "/internal/update") ||
			strings.HasSuffix(line, "/internal/app") ||
			strings.HasSuffix(line, "/internal/web") {
			t.Errorf("internal/hook must not depend on %s", line)
		}
	}
}
