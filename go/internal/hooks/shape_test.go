package hooks

import (
	"bytes"
	"errors"
	"testing"

	"github.com/quanht99/agent-chime/go/internal/model"
	"github.com/quanht99/agent-chime/go/internal/testenv"
)

var shapes = []struct{ name, json string }{
	{"root array", "[1, 2]"},
	{"root string", `"hello"`},
	{"hooks array", `{"hooks": []}`},
	{"hooks string", `{"hooks": "x"}`},
	{"event object", `{"hooks": {"Stop": {"a": 1}}}`},
	{"event string", `{"hooks": {"Notification": "x"}}`},
}

func TestInstallRefusesUnexpectedShapes(t *testing.T) {
	for _, s := range shapes {
		p := testenv.Home(t)
		orig := []byte(s.json)
		seed(t, p.ClaudeSettings, orig)
		_, err := Install(p.ClaudeSettings, model.Claude, linux, p.BackupsDir, t0)
		var ue *UnreadableError
		if !errors.As(err, &ue) || ue.Path != p.ClaudeSettings {
			t.Errorf("%s: err = %v", s.name, err)
		}
		if !bytes.Equal(readBytes(t, p.ClaudeSettings), orig) {
			t.Errorf("%s: file modified", s.name)
		}
	}
}

func TestUninstallRefusesUnexpectedShapes(t *testing.T) {
	for _, s := range shapes {
		p := testenv.Home(t)
		orig := []byte(s.json)
		seed(t, p.ClaudeSettings, orig)
		err := Uninstall(p.ClaudeSettings, model.Claude, p.BackupsDir, t0)
		var ue *UnreadableError
		if !errors.As(err, &ue) || ue.Path != p.ClaudeSettings {
			t.Errorf("%s: err = %v", s.name, err)
		}
		if !bytes.Equal(readBytes(t, p.ClaudeSettings), orig) {
			t.Errorf("%s: file modified", s.name)
		}
	}
}

func TestStatusReportsUnreadableForUnexpectedShapes(t *testing.T) {
	for _, s := range shapes {
		p := testenv.Home(t)
		seed(t, p.ClaudeSettings, []byte(s.json))
		if st := StatusOf(p.ClaudeSettings, model.Claude, linux); st.Kind != Unreadable || st.Path != p.ClaudeSettings {
			t.Errorf("%s: %+v", s.name, st)
		}
	}
}
