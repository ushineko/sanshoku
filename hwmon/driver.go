package hwmon

import (
	"context"
	"time"

	"github.com/ushineko/sanshoku"
	"github.com/ushineko/sanshoku/support"
)

/*
Driver lists the hwmon chips present, so a scan shows the whole machine.

An opened chip satisfies no capability: hayami and hotaru read temperatures
through First, and a sensor capability is a later spec if a consumer wants
one. The zero value reads Root.
*/
type Driver struct{}

// Name is "hwmon".
func (Driver) Name() string { return "hwmon" }

// Find returns one candidate per chip under Root, ErrAbsent when there are
// none. The candidate's Path is the chip's directory, which carries the hwmon
// index; it identifies the chip for this boot and is never how a Sensor finds
// it.
func (Driver) Find(context.Context) ([]sanshoku.Candidate, error) {
	found, err := chips(Root)
	if err != nil {
		return nil, err
	}
	if len(found) == 0 {
		return nil, sanshoku.ErrAbsent
	}
	out := make([]sanshoku.Candidate, 0, len(found))
	for _, c := range found {
		id := sanshoku.Identity{Bus: sanshoku.BusHwmon, Name: c.name, Path: c.dir}
		out = append(out, sanshoku.Candidate{
			Identity: id,
			Driver:   "hwmon",
			Open: func(context.Context) (sanshoku.Device, error) {
				return device{id: id}, nil
			},
		})
	}
	return out, nil
}

// device is an opened chip. Opening holds nothing, so closing releases
// nothing.
type device struct{ id sanshoku.Identity }

func (d device) Identity() sanshoku.Identity { return d.id }
func (device) Close() error                  { return nil }

// Support is the hwmon sensor tables as support entries.
//
// Intel is Tested: spec 001's bench read coretemp, and hayami and hotaru both
// read it on the same hardware. AMD and the GPU sensors are Expected: the code
// path is the same and neither consumer recorded a measurement on such a
// machine (hotaru's runs NVIDIA's driver, which registers no hwmon).
func Support() []support.Entry {
	return []support.Entry{
		{
			Driver:       "hwmon",
			Device:       "Intel CPU package",
			Match:        "`coretemp` / `Package id 0`",
			Chips:        []string{"coretemp"},
			Capabilities: []string{"temperature"},
			Tier:         support.Tested,
			Hardware:     "Intel Core i9-14900K",
			Tested:       benched,
			Spec:         1,
		},
		{
			Driver:       "hwmon",
			Device:       "AMD CPU",
			Match:        "`k10temp` / `Tdie`, `Tctl`; `zenpower` / `Tdie`",
			Chips:        []string{"k10temp", "zenpower"},
			Capabilities: []string{"temperature"},
			Tier:         support.Expected,
			Spec:         1,
			Notes:        "Tctl only where Tdie is absent: it carries a fan-curve offset",
		},
		{
			Driver:       "hwmon",
			Device:       "AMD GPU, nouveau",
			Match:        "`amdgpu` / `edge`, `amdgpu`, `nouveau`",
			Chips:        []string{"amdgpu", "nouveau"},
			Capabilities: []string{"temperature"},
			Tier:         support.Expected,
			Spec:         1,
			Notes:        "NVIDIA's own driver registers no hwmon",
		},
	}
}

// benched is the date of spec 001's bench run on the Intel machine.
var benched = time.Date(2026, time.September, 29, 0, 0, 0, 0, time.UTC)

// Describe is what the driver reads: the kernel's sensor chips, on Linux, which
// alone has hwmon (spec 014).
func (Driver) Describe() sanshoku.Description {
	return sanshoku.Description{
		Name:         "hwmon",
		Finds:        "sensor chip",
		Capabilities: []string{"temperature"},
		Platforms:    []string{"linux"},
		Quiet:        false,
	}
}
