package simulator

import (
	"bytes"
	"io"
	"os"
	"testing"
)

// captureStdout runs f and returns everything it wrote to os.Stdout.
func captureStdout(t *testing.T, f func()) string {
	t.Helper()
	original := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = writer
	done := make(chan string)
	go func() {
		var buffer bytes.Buffer
		io.Copy(&buffer, reader)
		done <- buffer.String()
	}()
	f()
	writer.Close()
	os.Stdout = original
	return <-done
}

func TestStdoutOutputFunction(t *testing.T) {
	t.Run("a row is <time> <partition> [values]", func(t *testing.T) {
		got := captureStdout(t, func() {
			(&StdoutOutputFunction{}).Output("w", []float64{1.5, -2}, 3)
		})
		if got != "3 w [1.5 -2]\n" {
			t.Errorf("got %q", got)
		}
	})
	t.Run("a prefix leads each row, separated by a space", func(t *testing.T) {
		got := captureStdout(t, func() {
			(&StdoutOutputFunction{Prefix: "member=1 seed=7"}).Output("w", []float64{1.5, -2}, 3)
		})
		if got != "member=1 seed=7 3 w [1.5 -2]\n" {
			t.Errorf("got %q", got)
		}
	})
}
