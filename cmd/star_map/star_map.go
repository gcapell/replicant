// star_map writes an interactive 3D map of known stars to an HTML file and
// opens it in the browser. Drag to rotate, scroll to zoom, hover for names,
// click a legend entry to hide/show that group.
//
//	star_map [-replicant ID] [-o stars.html] [-open=false]
package main

import (
	"context"
	_ "embed"
	"flag"
	"html/template"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/gcapell/replicant/api"
	"github.com/gcapell/replicant/model"
)

//go:embed star_map.html.tmpl
var pageSrc string

var page = template.Must(template.New("page").Parse(pageSrc))

// trace is one Plotly scatter3d series; it's marshalled to JSON by html/template.
type trace struct {
	Name   string    `json:"name"`
	Type   string    `json:"type"`
	Mode   string    `json:"mode"`
	X      []float64 `json:"x"`
	Y      []float64 `json:"y"`
	Z      []float64 `json:"z"`
	Text   []string  `json:"text"`
	Hover  string    `json:"hovertemplate"`
	Marker marker    `json:"marker"`
}

type marker struct {
	Size  int    `json:"size"`
	Color string `json:"color"`
}

func main() {
	replicant := flag.String("replicant", "4698E8E6", "replicant whose star catalogue to map")
	out := flag.String("o", "stars.html", "output HTML file")
	open := flag.Bool("open", true, "open the file in the browser")
	flag.Parse()

	stars, err := api.Stars(context.Background(), *replicant)
	if err != nil {
		log.Fatal(err)
	}
	f, err := os.Create(*out)
	if err != nil {
		log.Fatal(err)
	}
	if err := render(f, stars.Stars); err != nil {
		f.Close()
		log.Fatal(err)
	}
	if err := f.Close(); err != nil {
		log.Fatal(err)
	}
	abs, _ := filepath.Abs(*out)
	log.Printf("wrote %d stars to %s", len(stars.Stars), abs)
	if *open {
		if err := exec.Command("open", abs).Run(); err != nil {
			log.Printf("couldn't open browser: %v", err)
		}
	}
}

// render writes the HTML page for stars to w, with positions shifted so the
// centre of the explored stars is at the origin.
func render(w io.Writer, stars []model.Star) error {
	centre := exploredCentre(stars)
	explored := newTrace("Explored", "#f5b942", 5)
	unexplored := newTrace("Unexplored", "#6b8cff", 3)
	for _, s := range stars {
		t := unexplored
		if s.Explored {
			t = explored
		}
		p := s.Position.Delta(centre)
		t.X = append(t.X, p.X)
		t.Y = append(t.Y, p.Y)
		t.Z = append(t.Z, p.Z)
		t.Text = append(t.Text, s.Designation)
	}
	return page.Execute(w, map[string]any{
		"Traces":     []*trace{explored, unexplored},
		"Count":      len(stars),
		"Explored":   len(explored.X),
		"Unexplored": len(unexplored.X),
		"Centre":     centre.String(),
	})
}

// exploredCentre is the mean position of the explored stars, or the origin
// (i.e. no shift) if none are explored.
func exploredCentre(stars []model.Star) model.Position {
	var sum model.Position
	n := 0
	for _, s := range stars {
		if s.Explored {
			sum.X += s.Position.X
			sum.Y += s.Position.Y
			sum.Z += s.Position.Z
			n++
		}
	}
	if n == 0 {
		return model.Position{}
	}
	return model.Position{X: sum.X / float64(n), Y: sum.Y / float64(n), Z: sum.Z / float64(n)}
}

func newTrace(name, color string, size int) *trace {
	return &trace{
		Name:   name,
		Type:   "scatter3d",
		Mode:   "markers",
		Hover:  "<b>%{text}</b><br>(%{x:.1f}, %{y:.1f}, %{z:.1f})<extra></extra>",
		Marker: marker{Size: size, Color: color},
		// Non-nil so an empty group marshals as [] rather than null.
		X: []float64{}, Y: []float64{}, Z: []float64{}, Text: []string{},
	}
}
