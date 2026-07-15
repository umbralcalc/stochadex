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
				t.Error("test params appeared when they shouldn't exist")
			}
			panicked := didPanic(func() { params.Get("test") }) // should panic
			if !panicked {
				t.Error("get method didn't panic when getting unset params")
			}
			params.Set("test", []float64{0.0, 1.0, 2.0, 3.0})
			_ = params.Get("test")
		},
	)
	t.Run(
		"Get returns the live slice, GetCopy returns an independent copy",
		func(t *testing.T) {
			params := NewParams(make(map[string][]float64))
			params.Set("test", []float64{0.0, 1.0, 2.0, 3.0})

			// Get returns the live backing slice: mutating it changes the store.
			live := params.Get("test")
			live[0] = 99.0
			if params.Get("test")[0] != 99.0 {
				t.Error("Get did not return the live slice")
			}

			// GetCopy returns an independent slice: mutating it leaves the
			// store untouched. This copy-on-retain guarantee is load-bearing.
			cp := params.GetCopy("test")
			cp[1] = -1.0
			if params.Get("test")[1] != 1.0 {
				t.Error("mutating a GetCopy result leaked back into the store")
			}
		},
	)
	t.Run(
		"GetCopyOk reports presence and copies present values",
		func(t *testing.T) {
			params := NewParams(make(map[string][]float64))

			if values, ok := params.GetCopyOk("absent"); ok || values != nil {
				t.Errorf(
					"GetCopyOk on absent name: got (%v, %v), want (nil, false)",
					values, ok,
				)
			}

			params.Set("test", []float64{4.0, 5.0})
			values, ok := params.GetCopyOk("test")
			if !ok {
				t.Fatal("GetCopyOk did not report a present name")
			}
			values[0] = 0.0
			if params.Get("test")[0] != 4.0 {
				t.Error("mutating a GetCopyOk result leaked back into the store")
			}
		},
	)
	t.Run(
		"SetIndex updates in place and panics on invalid access",
		func(t *testing.T) {
			params := NewParams(make(map[string][]float64))
			params.Set("test", []float64{0.0, 1.0, 2.0})

			params.SetIndex("test", 1, 42.0)
			if got := params.GetIndex("test", 1); got != 42.0 {
				t.Errorf("SetIndex did not update in place: got %f, want 42.0", got)
			}

			if !didPanic(func() { params.SetIndex("test", 3, 0.0) }) {
				t.Error("SetIndex did not panic on out-of-range index")
			}
			if !didPanic(func() { params.SetIndex("test", -1, 0.0) }) {
				t.Error("SetIndex did not panic on negative index")
			}
			if !didPanic(func() { params.SetIndex("absent", 0, 0.0) }) {
				t.Error("SetIndex did not panic on an unset name")
			}
		},
	)
}
