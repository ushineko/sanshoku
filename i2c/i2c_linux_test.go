//go:build linux

package i2c

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// An adapter's name, and its PCI device's IDs read through its parent, as
// sysfs lays them out.
func TestAdaptersReadSysfs(t *testing.T) {
	root := t.TempDir()
	pci := filepath.Join(root, "devices", "0000:01:00.0")
	adapter := filepath.Join(pci, "i2c-3")
	require.NoError(t, os.MkdirAll(adapter, 0o755))
	for name, v := range map[string]string{"vendor": "0x10de", "device": "0x2684", "subsystem_vendor": "0x1462", "subsystem_device": "0x5104"} {
		require.NoError(t, os.WriteFile(filepath.Join(pci, name), []byte(v+"\n"), 0o600))
	}
	class := filepath.Join(root, "class")
	require.NoError(t, os.MkdirAll(filepath.Join(class, "i2c-3"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(class, "i2c-3", "name"), []byte("NVIDIA i2c adapter 1 at 1:00.0\n"), 0o600))
	require.NoError(t, os.Symlink(adapter, filepath.Join(class, "i2c-3", "device")))
	was := sysClass
	sysClass = class
	t.Cleanup(func() { sysClass = was })

	got, err := Adapters()
	require.NoError(t, err)
	assert.Equal(t, []Adapter{{
		Path: "/dev/i2c-3", Name: "NVIDIA i2c adapter 1 at 1:00.0",
		Vendor: 0x10DE, Device: 0x2684, SubVendor: 0x1462, SubDevice: 0x5104,
	}}, got)
}

func TestABlockIsAtMost32Bytes(t *testing.T) {
	require.ErrorIs(t, checkBlock(make([]byte, 33)), ErrBlock)
	require.NoError(t, checkBlock(make([]byte, 32)))
}
