package main

import (
	"cmp"
	"context"
	"fmt"
	"log"
	"slices"

	"github.com/gcapell/replicant/api"
	"github.com/gcapell/replicant/model"
)

type distantStar struct {
	s model.Star
	d float64
}

func main() {
	ctx := context.Background()
	stars, err := api.Stars(ctx, "4698E8E6")
	if err != nil {
		log.Fatal(err)
	}

	var explored, unexplored []*distantStar
	for _, s := range stars.Stars {
		d := &distantStar{s: s}
		if s.Explored {
			explored = append(explored, d)
		} else {
			unexplored = append(unexplored, d)
		}
	}
	var centre model.Position
	{
		var x, y, z float64
		for _, s := range explored {
			x += s.s.Position.X
			y += s.s.Position.Y
			z += s.s.Position.Z
		}
		n := float64(len(explored))
		centre = model.Position{X: x / n, Y: y / n, Z: z / n}
	}
	for _, s := range unexplored {
		s.d = s.s.Position.Distance(centre)
	}
	for _, s := range explored {
		s.d = s.s.Position.Distance(centre)
	}

	slices.SortFunc(unexplored, func(a, b *distantStar) int {
		return cmp.Compare(a.d, b.d)
	})
	slices.SortFunc(explored, func(a, b *distantStar) int {
		return cmp.Compare(a.d, b.d)
	})

	fmt.Printf("explore order:\n")
	for _, s := range unexplored {
		e := closest(s.s.Position, explored)
		fmt.Printf("  %s (%.1f) (%.1f from %s)\n", s.s.Designation, s.d, e.d, e.s.Designation)
	}
	fmt.Printf("observatory order:\n")
	for _, s := range explored {
		delta := s.s.Position.Delta(centre)
		fmt.Printf("  %s (%.1f) %s\n", s.s.Designation, s.d, delta)
	}
}

func closest(p model.Position, stars []*distantStar) *distantStar {
	best := stars[0]
	bestDist := p.Distance(best.s.Position)
	for _, s := range stars[1:] {
		if d := p.Distance(s.s.Position); d < bestDist {
			best, bestDist = s, d
		}
	}
	return best
}
