package model

import (
	"fmt"
	"math"
)

func (p Position) Distance(o Position) float64 {
	d := p.Delta(o)
	return math.Sqrt(d.X*d.X + d.Y*d.Y + d.Z*d.Z)
}

func (p Position) Delta(o Position) Position {
	return Position{
		X: p.X - o.X,
		Y: p.Y - o.Y,
		Z: p.Z - o.Z,
	}
}

func (p Position) String() string {
	return fmt.Sprintf("(%.1f, %.1f, %.1f)", p.X, p.Y, p.Z)
}
