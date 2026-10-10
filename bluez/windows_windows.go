package bluez

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

/*
The property reads, through setupapi directly: no cgo, nothing beyond
golang.org/x/sys (spec 016).

x/sys wraps SetupDiGetDevicePropertyW only for strings, and the battery is a
byte, so the call is made here and the value decoded by its declared type.
*/
var (
	modSetupAPI           = windows.NewLazySystemDLL("setupapi.dll")
	procGetDeviceProperty = modSetupAPI.NewProc("SetupDiGetDevicePropertyW")
)

// enumerators are the bus enumerators a Bluetooth device's nodes hang off:
// BTHENUM for Classic devices and their profiles, BTHLE and BTHLEDEVICE for
// LE devices and their GATT services.
var enumerators = []string{"BTHENUM", "BTHLE", "BTHLEDEVICE"}

// radioEnumerator is the enumerator of the radio's own children (the
// Bluetooth enumerator, RFCOMM, PAN): present exactly when a radio is.
const radioEnumerator = "BTH"

/*
The property keys, each read on the desk spec 016 was written at.

The Bluetooth ones are DEVPKEY_Bluetooth_*, from devpkey.h's Bluetooth set
{2BD67D8B-8BEB-48D5-87E0-6CDA3428040A}. batteryKey is the one Windows' own
Bluetooth settings page shows a headset's level from; it is undocumented,
and it is where a QC35's level (80, then 90 an hour later) was, as a byte.
connectedKey is {83DA6326-...} 15, which read true on the device nodes of a
connected QC35 and DualSense and false on their profile nodes.
*/
var (
	friendlyNameKey = propertyKey("{A45C254E-DF1C-4EFD-8020-67D146A850E0}", 14)
	containerKey    = propertyKey("{8C7ED206-3F8A-4827-B3AB-AE9E1FAEFC6C}", 2)
	addressKey      = propertyKey("{2BD67D8B-8BEB-48D5-87E0-6CDA3428040A}", 1)
	vendorKey       = propertyKey("{2BD67D8B-8BEB-48D5-87E0-6CDA3428040A}", 7)
	productKey      = propertyKey("{2BD67D8B-8BEB-48D5-87E0-6CDA3428040A}", 8)
	classKey        = propertyKey("{2BD67D8B-8BEB-48D5-87E0-6CDA3428040A}", 10)
	connectedKey    = propertyKey("{83DA6326-97A6-4088-9453-A1923F573B29}", 15)
	batteryKey      = propertyKey("{104EA319-6EE2-4701-BD47-8DDBF425BBE5}", 2)
)

// propertyKey builds a DEVPROPKEY from its printed form. The strings are
// constants, so a malformed one is a bug and panics at start.
func propertyKey(fmtid string, pid uint32) windows.DEVPROPKEY {
	g, err := windows.GUIDFromString(fmtid)
	if err != nil {
		panic(err)
	}
	return windows.DEVPROPKEY{FmtID: windows.DEVPROPGUID(g), PID: windows.DEVPROPID(pid)}
}

/*
windowsDevices lists every connected Bluetooth device from the stack's device
properties. No radio is ErrNoBlueZ, which wraps sanshoku.ErrUnavailable, as a
machine with no Bluetooth is on Linux.

The calls are local and quick (the device tree, not the radio), so the
context is checked between nodes rather than bounding each call.
*/
func windowsDevices(ctx context.Context) ([]Device, error) {
	radio, err := listNodes(ctx, radioEnumerator)
	if err != nil {
		return nil, err
	}
	if len(radio) == 0 {
		return nil, fmt.Errorf("%w: no Bluetooth radio", ErrNoBlueZ)
	}
	var nodes []btNode
	for _, e := range enumerators {
		found, err := listNodes(ctx, e)
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, found...)
	}
	return joinNodes(nodes), nil
}

// listNodes reads every present node under one enumerator. An enumerator with
// no nodes is an empty list.
func listNodes(ctx context.Context, enumerator string) ([]btNode, error) {
	set, err := windows.SetupDiGetClassDevsEx(nil, enumerator, 0,
		windows.DIGCF_ALLCLASSES|windows.DIGCF_PRESENT, 0, "")
	if err != nil {
		return nil, fmt.Errorf("listing %s devices: %w", enumerator, err)
	}
	defer func() { _ = set.Close() }()

	var out []btNode
	for i := 0; ; i++ {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("listing bluetooth devices: %w", err)
		}
		data, err := set.EnumDeviceInfo(i)
		if errors.Is(err, windows.ERROR_NO_MORE_ITEMS) {
			return out, nil
		}
		if err != nil {
			return nil, fmt.Errorf("listing %s devices: %w", enumerator, err)
		}
		id, err := set.DeviceInstanceID(data)
		if err != nil {
			continue
		}
		out = append(out, readNode(set, data, id))
	}
}

// readNode reads the properties of one node this package uses. A property the
// node does not carry stays zero.
func readNode(set windows.DevInfo, data *windows.DevInfoData, id string) btNode {
	n := btNode{id: id}
	n.name, _ = stringProperty(set, data, &friendlyNameKey)
	n.address, _ = stringProperty(set, data, &addressKey)
	if v, typ, ok := property(set, data, &containerKey); ok && typ == windows.DEVPROP_TYPE_GUID && len(v) == 16 {
		n.container = (*windows.GUID)(unsafe.Pointer(&v[0])).String() //nolint:gosec // 16 bytes, a GUID's size
	}
	if v, ok := uintProperty(set, data, &classKey); ok {
		n.class, n.hasClass = uint32(v), true //nolint:gosec // a UINT32 property
	}
	if v, typ, ok := property(set, data, &connectedKey); ok && typ == windows.DEVPROP_TYPE_BOOLEAN && len(v) == 1 {
		n.connected = v[0] != 0
	}
	if v, ok := uintProperty(set, data, &batteryKey); ok {
		n.level, n.hasLevel = int(v), true //nolint:gosec // a BYTE property
	}
	if v, ok := uintProperty(set, data, &vendorKey); ok {
		n.vendor = uint16(v) //nolint:gosec // a UINT16 property
	}
	if v, ok := uintProperty(set, data, &productKey); ok {
		n.product = uint16(v) //nolint:gosec // a UINT16 property
	}
	return n
}

// propertyLen is the buffer a property is read into. The longest read here
// is a friendly name, which Windows caps well under this.
const propertyLen = 512

// property reads one property's raw bytes and declared type, and false where
// the node does not carry it.
func property(set windows.DevInfo, data *windows.DevInfoData, key *windows.DEVPROPKEY) ([]byte, windows.DEVPROPTYPE, bool) {
	buf := make([]byte, propertyLen)
	var typ windows.DEVPROPTYPE
	var size uint32
	ok, _, _ := procGetDeviceProperty.Call(uintptr(set), uintptr(unsafe.Pointer(data)), uintptr(unsafe.Pointer(key)), //nolint:gosec // a Win32 call takes its arguments as pointers
		uintptr(unsafe.Pointer(&typ)), uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)), uintptr(unsafe.Pointer(&size)), 0) //nolint:gosec // a Win32 call takes its arguments as pointers
	if ok == 0 || int(size) > len(buf) {
		return nil, 0, false
	}
	return buf[:size], typ, true
}

// uintProperty reads a BYTE, UINT16 or UINT32 property as a number.
func uintProperty(set windows.DevInfo, data *windows.DevInfoData, key *windows.DEVPROPKEY) (uint64, bool) {
	v, typ, ok := property(set, data, key)
	if !ok {
		return 0, false
	}
	switch {
	case typ == windows.DEVPROP_TYPE_BYTE && len(v) == 1:
		return uint64(v[0]), true
	case typ == windows.DEVPROP_TYPE_UINT16 && len(v) == 2:
		return uint64(binary.LittleEndian.Uint16(v)), true
	case typ == windows.DEVPROP_TYPE_UINT32 && len(v) == 4:
		return uint64(binary.LittleEndian.Uint32(v)), true
	}
	return 0, false
}

// stringProperty reads a STRING property.
func stringProperty(set windows.DevInfo, data *windows.DevInfoData, key *windows.DEVPROPKEY) (string, bool) {
	v, typ, ok := property(set, data, key)
	if !ok || typ != windows.DEVPROP_TYPE_STRING || len(v) < 2 {
		return "", false
	}
	u := unsafe.Slice((*uint16)(unsafe.Pointer(&v[0])), len(v)/2) //nolint:gosec // UTF-16 from the property buffer
	return windows.UTF16ToString(u), true
}
