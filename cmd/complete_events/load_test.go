package main

import (
	"maps"
	"testing"
)

func TestLoadStep(t *testing.T) {
	const V = "volatiles"
	for _, tc := range []struct {
		name                     string
		want, avail, hold, carry amounts
		capacity                 int
		deposit, take, short     amounts
	}{
		{
			name: "fits", want: amounts{C: 50}, avail: amounts{C: 100}, hold: amounts{V: 10},
			capacity: 100, take: amounts{C: 50},
		},
		{
			name: "unrelated cargo dumped when it doesn't fit", want: amounts{C: 80}, avail: amounts{C: 100},
			hold: amounts{V: 30}, capacity: 100, deposit: amounts{V: 30}, take: amounts{C: 80},
		},
		{
			name: "this delivery's cargo is kept", want: amounts{Si: 80}, avail: amounts{Si: 100},
			hold: amounts{C: 50, V: 10}, carry: amounts{C: 50}, capacity: 100,
			deposit: amounts{V: 10}, take: amounts{Si: 50},
		},
		{
			name: "more than capacity", want: amounts{C: 150, Si: 100}, avail: amounts{C: 200, Si: 200},
			capacity: 100, take: amounts{C: 100},
		},
		{
			name: "stockpile short", want: amounts{C: 50}, avail: amounts{C: 20},
			capacity: 100, take: amounts{C: 20}, short: amounts{C: 30},
		},
	} {
		dep, take, short := loadStep(tc.want, tc.avail, tc.hold, tc.carry, tc.capacity)
		for _, c := range []struct {
			what      string
			got, want amounts
		}{{"deposit", dep, tc.deposit}, {"take", take, tc.take}, {"short", short, tc.short}} {
			if c.want == nil {
				c.want = amounts{}
			}
			if !maps.Equal(c.got, c.want) {
				t.Errorf("%s: %s = %v, want %v", tc.name, c.what, c.got, c.want)
			}
		}
	}
}
