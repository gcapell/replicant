package main

import (
	"fmt"
	"log"
	"math"

	"github.com/gcapell/replicant/api"
	"github.com/gcapell/replicant/model"
)

func dist(a, b model.Position) float64 {
	dx, dy, dz := a.X-b.X, a.Y-b.Y, a.Z-b.Z
	return math.Sqrt(dx*dx + dy*dy + dz*dz)
}

func main() {
	stars, err := api.Stars("4698E8E6")
	if err != nil {
		log.Fatal(err)
	}

	var explored, unexplored []model.Star
	for _, s := range stars.Stars {
		if s.Explored {
			explored = append(explored, s)
		} else {
			unexplored = append(unexplored, s)
		}
	}

	fmt.Printf("Loaded %d stars: %d explored, %d unexplored\n",
		len(stars.Stars), len(explored), len(unexplored))

	if len(explored) == 0 || len(unexplored) == 0 {
		fmt.Println("Need at least one explored and one unexplored star.")
		return
	}

	best := math.Inf(1)
	var bestU, bestE model.Star
	for _, u := range unexplored {
		for _, e := range explored {
			if d := dist(u.Position, e.Position); d < best {
				best, bestU, bestE = d, u, e
			}
		}
	}

	fmt.Printf("\nClosest unexplored star: %s (entry point %s)\n", bestU.Designation, bestU.EntryPoint)
	fmt.Printf("Nearest explored star:   %s\n", bestE.Designation)
	fmt.Printf("Distance:                %.4f\n", best)
}
