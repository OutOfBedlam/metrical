package svg

import (
	"math/rand/v2"
	"os"
	"testing"
	"time"
)

func TestSVG(t *testing.T) {
	times := make([]time.Time, 100)
	values := make([]float64, 100)
	for i := 0; i < 100; i++ {
		if i == 0 {
			times[i] = time.Date(2023, 10, 1, 12, 4, i, 0, time.UTC)
		} else {
			times[i] = times[i-1].Add(time.Second)
		}
		values[i] = rand.Float64() * 100
	}

	if err := os.MkdirAll("../../tmp", 0755); err != nil {
		t.Fatalf("failed to create tmp directory: %v", err)
	}
	out, err := os.OpenFile("../../tmp/svg_test.svg", os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		t.Fatalf("failed to open output file: %v", err)
	}
	defer out.Close()

	width, height := 200, 80
	s := NewCanvas(width, height)
	s.Times = times
	s.Values = values
	if err := s.Export(out); err != nil {
		t.Fatalf("failed to generate SVG: %v", err)
	}
}
