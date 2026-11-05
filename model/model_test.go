package model

import (
	"encoding/json"
	"testing"
)

func TestInventoryListDecodes(t *testing.T) {
	raw := `{"locations": [{"location": "SOL-3", "items": [
		{"quantity": 20, "resource_type": "silicates"},
		{"quantity": 5, "resource_type": "rares"}]}], "next_cursor": null}`
	var l InventoryList
	if err := json.Unmarshal([]byte(raw), &l); err != nil {
		t.Fatal(err)
	}
	loc := l.Locations[0]
	if got := loc.Total(); got != 25 {
		t.Errorf("Total = %d, want 25", got)
	}
	if got := loc.Amounts()[Silicates]; got != 20 {
		t.Errorf("silicates = %d, want 20", got)
	}
	if l.NextCursor != nil {
		t.Errorf("NextCursor = %v, want nil", *l.NextCursor)
	}
}
