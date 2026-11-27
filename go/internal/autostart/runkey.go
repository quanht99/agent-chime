package autostart

import "errors"

func runValue(program string) string { return `"` + program + `" --background` }

func enableRunKey(k RunKey, program string) error {
	if k == nil {
		return errors.ErrUnsupported
	}
	return k.Set(runValueName, runValue(program))
}

func runKeyEnabled(k RunKey, program string) bool {
	if k == nil {
		return false
	}
	v, ok := k.Get(runValueName)
	return ok && v == runValue(program)
}

func disableRunKey(k RunKey) error {
	if k == nil {
		return errors.ErrUnsupported
	}
	if _, ok := k.Get(runValueName); !ok {
		return nil
	}
	return k.Delete(runValueName)
}
