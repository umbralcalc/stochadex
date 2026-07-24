package api

// didPanic reports whether calling f resulted in a panic. It is used by tests
// that assert a config-validation failure raises a panic.
func didPanic(f func()) (panicked bool) {
	defer func() {
		if r := recover(); r != nil {
			panicked = true
		}
	}()
	f()
	return panicked
}

// stringify renders a recovered panic value as a string for message assertions.
func stringify(v any) string {
	if err, ok := v.(error); ok {
		return err.Error()
	}
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}
