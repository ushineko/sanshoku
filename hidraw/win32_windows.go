package hidraw

import (
	"errors"
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

/*
The Windows HID calls, through hid.dll and cfgmgr32.dll directly: no cgo, no
hidapi, and nothing beyond golang.org/x/sys (spec 012).

hid.dll is the HID class driver's user-mode half. It answers what a collection
is (attributes, capabilities, strings) on a handle opened with no access at
all, which is the only kind Windows grants on a keyboard or mouse it owns.
*/
var (
	modHID      = windows.NewLazySystemDLL("hid.dll")
	modCfgMgr32 = windows.NewLazySystemDLL("cfgmgr32.dll")

	procGetAttributes      = modHID.NewProc("HidD_GetAttributes")
	procGetPreparsedData   = modHID.NewProc("HidD_GetPreparsedData")
	procFreePreparsedData  = modHID.NewProc("HidD_FreePreparsedData")
	procGetCaps            = modHID.NewProc("HidP_GetCaps")
	procGetValueCaps       = modHID.NewProc("HidP_GetValueCaps")
	procGetButtonCaps      = modHID.NewProc("HidP_GetButtonCaps")
	procGetProductString   = modHID.NewProc("HidD_GetProductString")
	procGetManufacturerStr = modHID.NewProc("HidD_GetManufacturerString")

	procInterfaceProperty = modCfgMgr32.NewProc("CM_Get_Device_Interface_PropertyW")
	procLocateDevNode     = modCfgMgr32.NewProc("CM_Locate_DevNodeW")
	procGetParent         = modCfgMgr32.NewProc("CM_Get_Parent")
	procGetDeviceID       = modCfgMgr32.NewProc("CM_Get_Device_IDW")
)

// hidClass is GUID_DEVINTERFACE_HID, the interface class every HID
// collection registers, as HidD_GetHidGuid returns it.
var hidClass = windows.GUID{
	Data1: 0x4D1E55B2, Data2: 0xF16F, Data3: 0x11CF,
	Data4: [8]byte{0x88, 0xCB, 0x00, 0x11, 0x11, 0x00, 0x00, 0x30},
}

// instanceIDKey is DEVPKEY_Device_InstanceId, the device instance an
// interface belongs to.
var instanceIDKey = windows.DEVPROPKEY{
	FmtID: windows.DEVPROPGUID{
		Data1: 0x78C34FC8, Data2: 0x104A, Data3: 0x4ACA,
		Data4: [8]byte{0x9E, 0xA4, 0x52, 0x4D, 0x52, 0x99, 0x6E, 0x57},
	},
	PID: 256,
}

// hidpSuccess is HIDP_STATUS_SUCCESS: the HidP_ calls return an NTSTATUS of
// their own and not a Win32 error.
const hidpSuccess = 0x00110000

/*
The feature-report IOCTLs, IOCTL_HID_SET_FEATURE and IOCTL_HID_GET_FEATURE.

They are what HidD_SetFeature and HidD_GetFeature send, issued here directly so
that they can take an OVERLAPPED and with it a deadline: the HidD_ calls block
until the device answers, and a device that does not answer would hang the
caller. CTL_CODE(FILE_DEVICE_KEYBOARD, 100, METHOD_IN_DIRECT or
METHOD_OUT_DIRECT, FILE_ANY_ACCESS), which is why they work on a handle opened
with no access.
*/
const (
	ioctlSetFeature = 0x000B0191
	ioctlGetFeature = 0x000B0192
)

// hidAttributes is HIDD_ATTRIBUTES.
type hidAttributes struct {
	Size    uint32
	Vendor  uint16
	Product uint16
	Version uint16
}

// hidCaps is HIDP_CAPS.
type hidCaps struct {
	Usage, UsagePage                    uint16
	InputLen, OutputLen, FeatureLen     uint16
	_                                   [17]uint16
	LinkCollectionNodes                 uint16
	InputButtonCaps, InputValueCaps     uint16
	InputDataIndices                    uint16
	OutputButtonCaps, OutputValueCaps   uint16
	OutputDataIndices                   uint16
	FeatureButtonCaps, FeatureValueCaps uint16
	FeatureDataIndices                  uint16
}

/*
hidCap is the part of HIDP_VALUE_CAPS and HIDP_BUTTON_CAPS this package reads.

The two structures are both 72 bytes and agree on everything read here: the
usage page and report ID at the front, and at offset 56 the union whose first
field is the usage (or, for a range, the first usage of it).
*/
type hidCap struct {
	UsagePage uint16
	ReportID  uint8
	_         [53]byte
	Usage     uint16
	_         [14]byte
}

// HIDP_REPORT_TYPE, the argument the capability calls take.
const (
	hidpInput   = 0
	hidpOutput  = 1
	hidpFeature = 2
)

// maxCaps bounds the capabilities read per kind. The busiest collection on
// the desk this was written on declared nine.
const maxCaps = 256

// collectionInfo is what one collection says about itself, read on a handle
// with no access.
type collectionInfo struct {
	path         string
	parent       string
	vendor       uint16
	product      uint16
	manufacturer string
	name         string
	page, usage  uint16
	inLen        int
	outLen       int
	featLen      int
	reports      []Report
}

// interfaces lists every present HID collection's device path.
func interfaces() ([]string, error) {
	paths, err := windows.CM_Get_Device_Interface_List("", &hidClass, windows.CM_GET_DEVICE_INTERFACE_LIST_PRESENT)
	if err != nil {
		return nil, fmt.Errorf("listing HID collections: %w", err)
	}
	return paths, nil
}

// openCollection opens one collection for overlapped I/O with the given
// access, sharing it with every other program that has it open.
func openCollection(path string, access uint32) (windows.Handle, error) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return windows.InvalidHandle, fmt.Errorf("naming %s: %w", path, err)
	}
	h, err := windows.CreateFile(p, access, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil,
		windows.OPEN_EXISTING, windows.FILE_FLAG_OVERLAPPED, 0)
	if err != nil {
		return windows.InvalidHandle, fmt.Errorf("opening %s: %w", path, err)
	}
	return h, nil
}

// describe reads what a collection says about itself.
func describe(path string) (collectionInfo, error) {
	info := collectionInfo{path: path}
	h, err := openCollection(path, 0)
	if err != nil {
		return info, err
	}
	defer func() { _ = windows.CloseHandle(h) }()

	attrs := hidAttributes{Size: uint32(unsafe.Sizeof(hidAttributes{}))}
	if ok, _, err := procGetAttributes.Call(uintptr(h), uintptr(unsafe.Pointer(&attrs))); ok == 0 { //nolint:gosec // a Win32 call takes its arguments as pointers
		return info, fmt.Errorf("reading the attributes of %s: %w", path, err)
	}
	info.vendor, info.product = attrs.Vendor, attrs.Product

	var preparsed uintptr
	if ok, _, err := procGetPreparsedData.Call(uintptr(h), uintptr(unsafe.Pointer(&preparsed))); ok == 0 { //nolint:gosec // a Win32 call takes its arguments as pointers
		return info, fmt.Errorf("reading the capabilities of %s: %w", path, err)
	}
	defer func() { _, _, _ = procFreePreparsedData.Call(preparsed) }()
	var caps hidCaps
	if s, _, _ := procGetCaps.Call(preparsed, uintptr(unsafe.Pointer(&caps))); s != hidpSuccess { //nolint:gosec // a Win32 call takes its arguments as pointers
		return info, fmt.Errorf("reading the capabilities of %s: status %#x", path, s)
	}
	info.page, info.usage = caps.UsagePage, caps.Usage
	info.inLen, info.outLen, info.featLen = int(caps.InputLen), int(caps.OutputLen), int(caps.FeatureLen)
	info.reports = reportsFromCaps(info, func(kind uintptr, button bool) []hidCap {
		return readCaps(preparsed, kind, button)
	})

	info.manufacturer = hidString(procGetManufacturerStr, h)
	info.name = hidString(procGetProductString, h)
	info.parent, err = parentOf(path)
	if err != nil {
		return info, err
	}
	return info, nil
}

// readCaps reads one kind of a collection's value or button capabilities.
func readCaps(preparsed, kind uintptr, button bool) []hidCap {
	proc := procGetValueCaps
	if button {
		proc = procGetButtonCaps
	}
	n := uint16(maxCaps)
	buf := make([]hidCap, n)
	if s, _, _ := proc.Call(kind, uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&n)), preparsed); s != hidpSuccess { //nolint:gosec // a Win32 call takes its arguments as pointers
		return nil
	}
	return buf[:n]
}

// hidStringLen is the buffer a device string is read into, in bytes. USB
// caps a string descriptor at 126 characters.
const hidStringLen = 256

// hidString reads one of the device's strings, or nothing where it has none.
func hidString(proc *windows.LazyProc, h windows.Handle) string {
	buf := make([]uint16, hidStringLen/2)
	if ok, _, _ := proc.Call(uintptr(h), uintptr(unsafe.Pointer(&buf[0])), hidStringLen); ok == 0 { //nolint:gosec // a Win32 call takes its arguments as pointers
		return ""
	}
	return windows.UTF16ToString(buf)
}

// errConfig is a configuration manager call that did not succeed.
var errConfig = errors.New("configuration manager")

/*
parentOf is the device instance a collection hangs off: for a USB device, the
USB interface (`USB\VID_046D&PID_C52B&MI_02\...`), which is what one Node is.

Through the configuration manager rather than by cutting up the device path:
the path's form is the HID class driver's business, and a keyboard's carries a
`\KBD` reference string after the class GUID that a string cut has to know
about.
*/
func parentOf(path string) (string, error) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return "", fmt.Errorf("naming %s: %w", path, err)
	}
	buf := make([]uint16, windows.MAX_DEVICE_ID_LEN+1)
	size := uint32(len(buf) * 2) //nolint:gosec // MAX_DEVICE_ID_LEN characters
	var typ uint32
	if r, _, _ := procInterfaceProperty.Call(uintptr(unsafe.Pointer(p)), uintptr(unsafe.Pointer(&instanceIDKey)), //nolint:gosec // a Win32 call takes its arguments as pointers
		uintptr(unsafe.Pointer(&typ)), uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&size)), 0); r != 0 { //nolint:gosec // a Win32 call takes its arguments as pointers
		return "", fmt.Errorf("%w: the instance of %s: %#x", errConfig, path, r)
	}
	var inst, parent uint32
	if r, _, _ := procLocateDevNode.Call(uintptr(unsafe.Pointer(&inst)), uintptr(unsafe.Pointer(&buf[0])), 0); r != 0 { //nolint:gosec // a Win32 call takes its arguments as pointers
		return "", fmt.Errorf("%w: locating %s: %#x", errConfig, path, r)
	}
	if r, _, _ := procGetParent.Call(uintptr(unsafe.Pointer(&parent)), uintptr(inst), 0); r != 0 { //nolint:gosec // a Win32 call takes its arguments as pointers
		return "", fmt.Errorf("%w: the parent of %s: %#x", errConfig, path, r)
	}
	id := make([]uint16, windows.MAX_DEVICE_ID_LEN+1)
	if r, _, _ := procGetDeviceID.Call(uintptr(parent), uintptr(unsafe.Pointer(&id[0])), uintptr(len(id)), 0); r != 0 { //nolint:gosec // a Win32 call takes its arguments as pointers
		return "", fmt.Errorf("%w: naming the parent of %s: %#x", errConfig, path, r)
	}
	return windows.UTF16ToString(id), nil
}
