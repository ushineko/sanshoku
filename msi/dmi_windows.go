//go:build windows

package msi

import "golang.org/x/sys/windows/registry"

// dmiBoard is the mainboard's maker and name, from where Windows keeps the
// firmware's DMI table.
func dmiBoard() (maker, name string) {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, `HARDWARE\DESCRIPTION\System\BIOS`, registry.QUERY_VALUE)
	if err != nil {
		return "", ""
	}
	defer func() { _ = k.Close() }()
	maker, _, _ = k.GetStringValue("BaseBoardManufacturer")
	name, _, _ = k.GetStringValue("BaseBoardProduct")
	return maker, name
}
