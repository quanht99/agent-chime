package audio

import (
	"bytes"
	"errors"
	"reflect"
	"testing"

	"github.com/quanht99/agent-chime/go/internal/testenv"
)

func TestFindPlayers(t *testing.T) {
	look := func(n string) (string, error) {
		if n == "paplay" {
			return "/usr/bin/paplay", nil
		}
		return "", errors.New("nope")
	}
	if got := FindPlayers(look); got != (Players{Paplay: "/usr/bin/paplay"}) {
		t.Fatalf("got %+v", got)
	}
}

func TestLinuxCommandOrder(t *testing.T) {
	all := Players{PwPlay: "pw-play", Paplay: "paplay", Aplay: "aplay"}
	name, args, stdin, err := LinuxCommand(all, "/x.wav", 0.5)
	if err != nil || name != "pw-play" || !reflect.DeepEqual(args, []string{"--volume=0.50", "/x.wav"}) || stdin != nil {
		t.Fatalf("pw-play: %s %v %v", name, args, err)
	}
	name, args, _, err = LinuxCommand(Players{Paplay: "paplay", Aplay: "aplay"}, "/x.wav", 0.5)
	if err != nil || name != "paplay" || !reflect.DeepEqual(args, []string{"--volume=32768", "/x.wav"}) {
		t.Fatalf("paplay: %s %v %v", name, args, err)
	}

	src := testenv.WAV16([]int16{1000, -1000}, 8000)
	p := write(t, "x.wav", src)
	name, args, stdin, err = LinuxCommand(Players{Aplay: "aplay"}, p, 0.5)
	want, _ := ScaleWAV(src, 0.5)
	if err != nil || name != "aplay" || !reflect.DeepEqual(args, []string{"-q", "-"}) || !bytes.Equal(stdin, want) {
		t.Fatalf("aplay: %s %v %v", name, args, err)
	}

	// Unscalable WAV falls back to the path.
	bad := write(t, "adpcm.wav", buildWAV(fmtBody(2, 4), []byte{1, 2, 3, 4}))
	_, args, stdin, err = LinuxCommand(Players{Aplay: "aplay"}, bad, 0.5)
	if err != nil || stdin != nil || !reflect.DeepEqual(args, []string{"-q", bad}) {
		t.Fatalf("fallback: %v %v", args, err)
	}

	if _, _, _, err = LinuxCommand(Players{}, "/x.wav", 0.5); !errors.Is(err, ErrNoPlayer) {
		t.Fatalf("none: %v", err)
	}
}
