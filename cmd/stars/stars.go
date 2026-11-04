package main

import (
	"context"
	"fmt"
	"log"
	"math"

	"github.com/gcapell/replicant/api"
	"github.com/gcapell/replicant/model"
)

func main() {
	ctx := context.Background()
	stars, err := api.Stars(ctx, "4698E8E6")
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
			if d := u.Position.Distance(e.Position); d < best {
				best, bestU, bestE = d, u, e
			}
		}
	}

	fmt.Printf("\nClosest unexplored star: %s (entry point %s)\n", bestU.Designation, bestU.EntryPoint)
	fmt.Printf("Nearest explored star:   %s\n", bestE.Designation)
	fmt.Printf("Distance:                %.4f\n", best)
}
