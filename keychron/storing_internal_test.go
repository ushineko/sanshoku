package keychron

import (
	"testing"

	"github.com/ushineko/sanshoku/lighting"
)

// The effect is held in memory, not in storage that wears (spec 034).
func TestTheEffectIsNotStored(t *testing.T) {
	var d any = &device{}
	if _, stores := d.(lighting.Storing); stores {
		t.Fatal("this device's effects are in memory, and it says they are stored")
	}
}
