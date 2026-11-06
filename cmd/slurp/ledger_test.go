package main

// Run with a dummy key, e.g. API_KEY=x go test ./cmd/slurp (api's init exits without one).

import (
	"context"
	"maps"
	"testing"

	"github.com/gcapell/replicant/model"
)

func inv(loc string, items map[model.ResourceType]int) model.LocationInventory {
	li := model.LocationInventory{Location: loc}
	for t, q := range items {
		li.Items = append(li.Items, model.InventoryItem{ResourceType: string(t), Quantity: q})
	}
	return li
}

func testLedger(stock []model.LocationInventory) *ledger {
	return newLedger(func(context.Context) ([]model.LocationInventory, error) { return stock, nil }, []model.Star{
		{Designation: "HOME"},
		{Designation: "NEAR", Position: model.Position{X: 3}},
		{Designation: "FAR", Position: model.Position{X: 30}},
	})
}

func TestClaimNearestAndExclusive(t *testing.T) {
	ctx := context.Background()
	l := testLedger([]model.LocationInventory{
		inv("FAR-1", map[model.ResourceType]int{"carbon": 900}),
		inv("NEAR-2", map[model.ResourceType]int{"carbon": 100, "metals": 50}),
		inv("NEAR-BELT-1", map[model.ResourceType]int{"carbon": 5000}), // belts aren't sources
		inv("NEAR-2-L4", map[model.ResourceType]int{"carbon": 5000}),   // nor Lagrange points
		inv("NEAR-OBJ-7", map[model.ResourceType]int{"carbon": 5000}),  // nor asteroids: they can hit planets
		inv("UNKNOWN-1", map[model.ResourceType]int{"carbon": 5000}),   // nor stars we can't place
	})
	a, _ := l.Claim(ctx, "A", "HOME-1", 120, false)
	if a.loc != "NEAR-2" || !maps.Equal(a.amounts, map[model.ResourceType]int{"carbon": 100, "metals": 20}) {
		t.Fatalf("A claimed %+v", a)
	}
	// B sees only what A left at NEAR-2.
	b, _ := l.Claim(ctx, "B", "HOME-1", 100, false)
	if b.loc != "NEAR-2" || !maps.Equal(b.amounts, map[model.ResourceType]int{"metals": 30}) {
		t.Fatalf("B claimed %+v", b)
	}
	// NEAR-2 is fully claimed, so C goes further.
	if c, _ := l.Claim(ctx, "C", "HOME-1", 100, false); c.loc != "FAR-1" {
		t.Fatalf("C claimed %+v", c)
	}
	// Topping up within HOME finds nothing.
	l.Release("A")
	if d, _ := l.Claim(ctx, "D", "HOME-1", 100, true); d.loc != "" {
		t.Fatalf("D claimed %+v, want nothing in HOME", d)
	}
}

func TestNearestBelt(t *testing.T) {
	l := testLedger(nil)
	stock := []model.LocationInventory{
		inv("NEAR-BELT-1", map[model.ResourceType]int{"carbon": 1000}), // not more than 1000
		inv("FAR-BELT-2", map[model.ResourceType]int{"carbon": 1001}),
		inv("HOME-3", map[model.ResourceType]int{"carbon": 9999}), // not a belt
	}
	if got := l.nearestBelt(stock, "HOME-1"); got != "FAR-BELT-2" {
		t.Errorf("nearestBelt = %q, want FAR-BELT-2", got)
	}
	if got := l.nearestBelt(stock[:1], "HOME-1"); got != "" {
		t.Errorf("nearestBelt = %q, want none", got)
	}
}
