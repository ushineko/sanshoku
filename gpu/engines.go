package gpu

import (
	"fmt"
	"strings"
)

/*
engineInstance is one instance of Windows' "GPU Engine" counter set, read out
of its name:

	pid_4120_luid_0x00000000_0x0000E985_phys_0_eng_3_engtype_3D

One instance is one process's use of one engine of one adapter. The adapter is
its LUID, which D3DKMT reports too, and the engine is its physical index and
engine index; the type ("3D", "Copy", "VideoDecode") is what Task Manager
labels the engine with.
*/
type engineInstance struct {
	LUID   string
	Engine string
	Type   string
}

// parseEngineInstance reads an instance name, and whether it is one. The LUID
// is lower-cased, so it compares equal to luidString's.
func parseEngineInstance(name string) (engineInstance, bool) {
	_, rest, ok := strings.Cut(name, "_luid_")
	if !ok {
		return engineInstance{}, false
	}
	luid, rest, ok := strings.Cut(rest, "_phys_")
	if !ok {
		return engineInstance{}, false
	}
	engine, kind, ok := strings.Cut(rest, "_engtype_")
	if !ok {
		return engineInstance{}, false
	}
	return engineInstance{LUID: strings.ToLower(luid), Engine: "phys_" + engine, Type: kind}, true
}

// luidString is an adapter's LUID in the form the counter names carry it,
// lower-cased: "0x00000000_0x0000e985".
func luidString(high int32, low uint32) string {
	return fmt.Sprintf("0x%08x_0x%08x", uint32(high), low) //nolint:gosec // the high part's bits, as Windows prints them
}

/*
busiestEngine is an adapter's utilisation as Task Manager gives it: each
engine's use summed over every process, and the busiest engine.

**The busiest, not the sum or the mean.** A card decoding video with its 3D
engine idle is busy, and saying so is the point of the number; a mean over a
dozen engines, most of which are never used, reads a saturated encoder as 8 %.
An engine summed over processes can pass 100 on a counter's rounding and is
held to it.

values maps instance names to percentages; instances of other adapters, and
names that are not instances, are passed over. ok is false where the adapter
has no instance at all.
*/
func busiestEngine(values map[string]float64, luid string) (float64, bool) {
	luid = strings.ToLower(luid)
	engines := map[string]float64{}
	for name, v := range values {
		in, ok := parseEngineInstance(name)
		if !ok || in.LUID != luid {
			continue
		}
		engines[in.Engine] += v
	}
	if len(engines) == 0 {
		return 0, false
	}
	busiest := 0.0
	for _, v := range engines {
		busiest = max(busiest, min(v, 100))
	}
	return busiest, true
}
