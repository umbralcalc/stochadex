package simulator

import (
	"testing"
)

// didPanic checks if a function panics
func didPanic(f func()) (panicked bool) {
	defer func() {
		if r := recover(); r != nil {
			panicked = true
		}
	}()
	f()
	return panicked
}

func TestParams(t *testing.T) {
	t.Run(
		"test the params struct works as intended",
		func(t *testing.T) {
			params := NewParams(make(map[string][]float64))
			_, ok := params.GetOk("test")
			if ok {
				panic("test params appeared when they shouldn't exist")
			}
			panicked := didPanic(func() { params.Get("test") }) // should panic
			if !panicked {
				panic("get method didn't panic when getting unset params")
			}
			params.Set("test", []float64{0.0, 1.0, 2.0, 3.0})
			_ = params.Get("test")
		},
	)
}
