package auto

import "testing"

func TestIsPlanetOrMoon(t *testing.T) {
	for loc, want := range map[string]bool{
		"SOL-3":             true,
		"SOL-3-1":           true,
		"SOL-12-10":         true,
		"SOL":               false,
		"SOL-BELT-1":        false,
		"SOL-BELT-1-SITE-3": false,
		"SOL-5-L4":          false,
		"SOL-OBJ-7":         false, // asteroids can collide with planets
		"SOL-OBJ-7-1":       false,
		"SOL-KUIPER":        false,
		"SOLAR-3":           false, // a different star sharing a prefix
		"SOL-3-1-2":         false,
	} {
		if got := IsPlanetOrMoon("SOL", loc); got != want {
			t.Errorf("IsPlanetOrMoon(SOL, %q) = %v, want %v", loc, got, want)
		}
	}
}
