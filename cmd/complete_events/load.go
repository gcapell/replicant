package main

import (
	"maps"
	"slices"
)

// loadStep decides what a freighter does at one collection stop.
//
//	want     still to be collected here for this delivery
//	avail    what the stockpile here holds now
//	hold     the freighter's cargo now
//	carrying the part of hold collected for this delivery
//
// If what's wanted (and available) doesn't fit, everything in the hold that
// isn't for this delivery is deposited here first. take is then as much of
// the wanted resources as fits, in resource order; short is what was wanted
// but isn't in the stockpile.
func loadStep(want, avail, hold, carrying amounts, capacity int) (deposit, take, short amounts) {
	deposit, take, short = amounts{}, amounts{}, amounts{}
	here := 0
	for r, q := range want {
		got := min(q, avail[r])
		if got < q {
			short[r] = q - got
		}
		here += got
	}
	free := capacity - total(hold)
	if here > free {
		for r, q := range hold {
			if extra := q - carrying[r]; extra > 0 {
				deposit[r] = extra
				free += extra
			}
		}
	}
	for _, r := range slices.Sorted(maps.Keys(want)) {
		if free <= 0 {
			break
		}
		if q := min(want[r]-short[r], free); q > 0 {
			take[r] = q
			free -= q
		}
	}
	return deposit, take, short
}

func total(am amounts) int {
	n := 0
	for _, q := range am {
		n += q
	}
	return n
}
