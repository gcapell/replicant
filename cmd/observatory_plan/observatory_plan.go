package main

import (
	"cmp"
	"fmt"
	"log"
	"slices"

	"github.com/gcapell/replicant/api"
	"github.com/gcapell/replicant/model"
)

func main() {
	stars, err := api.Stars("4698E8E6")
	if err != nil {
		log.Fatal(err)
	}

	type distantStar struct {
		s model.Star
		d float64
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
		fmt.Printf("  %s (%.1f)\n", s.s.Designation, s.d)
	}
	fmt.Printf("observatory order:\n")
	for _, s := range explored {
		delta := s.s.Position.Delta(centre)
		fmt.Printf("  %s (%.1f) %s\n", s.s.Designation, s.d, delta)
	}
}
