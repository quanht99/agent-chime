package model

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/quanht99/agent-chime/go/internal/i18n"
)

func fp(f float64) *float64 { return &f }

func TestDurationShort(t *testing.T) {
	cases := map[float64]string{0: "0s", -5: "0s", 42: "42s", 59.4: "59s", 60: "1m00s", 61: "1m01s", 192: "3m12s", 3600: "1h00m", 3900: "1h05m"}
	for in, want := range cases {
		if got := DurationShort(in); got != want {
			t.Errorf("DurationShort(%v) = %q, want %q", in, got, want)
		}
	}
}

func TestSkipReasonText(t *testing.T) {
	en := i18n.EN
	if got := (SkipReason{Kind: "muted", Until: &Time{Forever}}).Text(en); got != "muted" {
		t.Errorf("got %q", got)
	}
	until := Time{time.Date(2027, 1, 5, 14, 5, 0, 0, time.Local)}
	if got := (SkipReason{Kind: "muted", Until: &until}).Text(en); got != "muted until 14:05" {
		t.Errorf("got %q", got)
	}
	if got := (SkipReason{Kind: "muted"}).Text(en); got != "muted" {
		t.Errorf("nil until: got %q", got)
	}
	if got := (SkipReason{Kind: "shortTurn", Duration: 12, Threshold: 60}).Text(en); got != "short turn (12s < 1m00s)" {
		t.Errorf("got %q", got)
	}
	want := map[string]string{"headless": "non-interactive run", "duplicate": "same as the previous alert", "nested": "agent running inside another agent"}
	for k, w := range want {
		if got := (SkipReason{Kind: k}).Text(en); got != w {
			t.Errorf("%s: got %q", k, got)
		}
	}
	if got := (SkipReason{Kind: "headless"}).Text(i18n.VI); got != "lượt chạy không tương tác" {
		t.Errorf("vi: got %q", got)
	}
}

func TestAlertSummary(t *testing.T) {
	en := i18n.EN
	if got := (AlertRecord{Agent: "claude", Kind: "done", Duration: fp(77)}).Summary(en); got != "Claude Code done (1m17s)" {
		t.Errorf("got %q", got)
	}
	if got := (AlertRecord{Agent: "codex", Kind: "done"}).Summary(en); got != "Codex done" {
		t.Errorf("got %q", got)
	}
	if got := (AlertRecord{Agent: "claude", Kind: "attention", Duration: fp(5)}).Summary(en); got != "Claude Code needs you" {
		t.Errorf("got %q", got)
	}
	if got := (SkipRecord{Agent: "codex", Kind: "done"}).Summary(en); got != "Codex done" {
		t.Errorf("got %q", got)
	}
	if got := (SkipRecord{Agent: "codex", Kind: "attention"}).Summary(i18n.VI); got != "Codex cần duyệt" {
		t.Errorf("got %q", got)
	}
}

func TestTimeJSON(t *testing.T) {
	in := Time{time.UnixMilli(1_800_000_000_123)}
	b, err := json.Marshal(in)
	if err != nil || string(b) != "1800000000.123" {
		t.Fatalf("marshal = %s, %v", b, err)
	}
	var out Time
	if err := json.Unmarshal(b, &out); err != nil || !out.Equal(in.Time) {
		t.Fatalf("unmarshal = %v, %v", out, err)
	}
	b, _ = json.Marshal(Time{Forever})
	if string(b) != "64092211200" {
		t.Errorf("forever = %s", b)
	}
	if err := json.Unmarshal([]byte(`"x"`), &out); err == nil {
		t.Error("expected error for string")
	}
	rec := AlertRecord{Agent: "claude", Kind: "done", Date: in}
	b, _ = json.Marshal(rec)
	if string(b) != `{"agent":"claude","kind":"done","date":1800000000.123}` {
		t.Errorf("record = %s", b)
	}
}

func TestAgentDisplayName(t *testing.T) {
	if Agent("claude").DisplayName() != "Claude Code" || Agent("codex").DisplayName() != "Codex" {
		t.Error("display names")
	}
}
