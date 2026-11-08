package model

import "testing"

// A real directive.resume_failure payload.
func TestDirectiveResumeFailureDecodes(t *testing.T) {
	dec, ok := EventRegistry["directive.resume_failure"]
	if !ok {
		t.Fatal("not registered")
	}
	v, err := dec([]byte(`{"command":{"command":"start_mining","resource_type":"rares"},"reason":"start_mining failed: No salvage available at this location"}`))
	if err != nil {
		t.Fatal(err)
	}
	f := v.(*DirectiveResumeFailure)
	if f.Command["command"] != "start_mining" || f.Command["resource_type"] != "rares" ||
		f.Reason != "start_mining failed: No salvage available at this location" {
		t.Errorf("decoded %+v", f)
	}
}
