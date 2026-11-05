package main

// Note: run with a dummy key, e.g. API_KEY=x go test ./cmd/star_map (api's init exits without one).
// Set STAR_MAP_SAMPLE=path.html to also write the sample page somewhere you can open it.

import (
	"bytes"
	"math"
	"os"
	"strings"
	"testing"

	"github.com/gcapell/replicant/model"
)

func sampleStars() []model.Star {
	var stars []model.Star
	for i := range 60 {
		a := float64(i) * 0.7
		stars = append(stars, model.Star{
			Designation: "STAR" + string(rune('A'+i%26)) + string(rune('A'+i/26)),
			Explored:    i%5 == 0,
			Position:    model.Position{X: 40 * math.Cos(a), Y: 40 * math.Sin(a), Z: float64(i%13) - 6},
		})
	}
	stars = append(stars, model.Star{Designation: `<script>alert("x")</script>`})
	return stars
}

func TestRender(t *testing.T) {
	var buf bytes.Buffer
	if err := render(&buf, sampleStars()); err != nil {
		t.Fatal(err)
	}
	html := buf.String()
	for _, want := range []string{`"type":"scatter3d"`, `"name":"Explored"`, `61 stars · 12 explored · 49 unexplored`} {
		if !strings.Contains(html, want) {
			t.Errorf("output missing %q", want)
		}
	}
	// A designation must not be able to break out of the script.
	if strings.Contains(html, `<script>alert`) {
		t.Error("designation was not escaped")
	}
	if p := os.Getenv("STAR_MAP_SAMPLE"); p != "" {
		if err := os.WriteFile(p, buf.Bytes(), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestRenderEmptyGroup(t *testing.T) {
	var buf bytes.Buffer
	if err := render(&buf, []model.Star{{Designation: "SOL", Explored: true}}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), `"x":null`) {
		t.Error("empty group rendered as null; Plotly expects []")
	}
}

func TestRenderCentresOnExplored(t *testing.T) {
	stars := []model.Star{
		{Designation: "A", Explored: true, Position: model.Position{X: 10, Y: 20, Z: 30}},
		{Designation: "B", Explored: true, Position: model.Position{X: 30, Y: 40, Z: 50}},
		{Designation: "C", Position: model.Position{X: 20, Y: 30, Z: 40}},
	}
	if got, want := exploredCentre(stars), (model.Position{X: 20, Y: 30, Z: 40}); got != want {
		t.Fatalf("centre = %v, want %v", got, want)
	}
	var buf bytes.Buffer
	if err := render(&buf, stars); err != nil {
		t.Fatal(err)
	}
	html := buf.String()
	// A and B straddle the origin; C sits exactly on it.
	for _, want := range []string{`"x":[-10,10]`, `"z":[-10,10]`, `"x":[0]`, `originally at (20.0, 30.0, 40.0)`} {
		if !strings.Contains(html, want) {
			t.Errorf("output missing %q", want)
		}
	}
}

func TestExploredCentreNoneExplored(t *testing.T) {
	stars := []model.Star{{Position: model.Position{X: 5}}}
	if got := exploredCentre(stars); got != (model.Position{}) {
		t.Errorf("centre = %v, want origin", got)
	}
}
