package hwmon_test

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/sanshoku/hwmon"
)

// chip writes one hwmon directory: a name, and labelled temperatures.
func chip(t *testing.T, root, dir, name string, temps map[string]string) {
	t.Helper()
	path := filepath.Join(root, dir)
	require.NoError(t, os.MkdirAll(path, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(path, "name"), []byte(name+"\n"), 0o600))
	i := 1
	for label, milli := range temps {
		base := filepath.Join(path, "temp"+strconv.Itoa(i))
		require.NoError(t, os.WriteFile(base+"_label", []byte(label+"\n"), 0o600))
		require.NoError(t, os.WriteFile(base+"_input", []byte(milli+"\n"), 0o600))
		i++
	}
}

// The numbers move between boots. A program that remembered hwmon10 would
// report another chip's temperature after a reboot rather than failing, which
// is the worst way to be wrong, so the same tree renumbered must give the same
// answer.
func TestFirstFindsTheLabelWhateverTheChipIsNumbered(t *testing.T) {
	for _, dir := range []string{"hwmon0", "hwmon10", "hwmon7"} {
		root := t.TempDir()
		chip(t, root, "hwmon3", "nct6798", map[string]string{"SYSTIN": "31000"})
		chip(t, root, dir, "k10temp", map[string]string{"Tctl": "52000", "Tdie": "42000"})

		s, v, err := hwmon.First(root, hwmon.CPU)

		require.NoError(t, err, "k10temp at %s", dir)
		assert.Equal(t, hwmon.Sensor{Chip: "k10temp", Label: "Tdie"}, s, "Tctl was taken over Tdie")
		assert.InDelta(t, 42.0, v, 0.001, "the kernel writes thousandths")
	}
}

// On an Intel machine the package temperature is the one read, ahead of an
// AMD sensor that happens to be present.
func TestFirstOnAnIntelMachineReadsThePackage(t *testing.T) {
	root := t.TempDir()
	chip(t, root, "hwmon1", "k10temp", map[string]string{"Tctl": "52000"})
	chip(t, root, "hwmon2", "coretemp", map[string]string{"Core 0": "45000", "Package id 0": "38000"})

	s, v, err := hwmon.First(root, hwmon.CPU)

	require.NoError(t, err)
	assert.Equal(t, hwmon.Sensor{Chip: "coretemp", Label: "Package id 0"}, s)
	assert.InDelta(t, 38.0, v, 0.001)
}
