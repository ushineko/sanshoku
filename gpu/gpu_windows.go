package gpu

import (
	"context"
	"fmt"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

/*
The card on Windows, without a subprocess (hayami spec 034).

**D3DKMT for the temperature and the name.** The graphics kernel's thunks in
gdi32 are what Task Manager reads: D3DKMTQueryAdapterInfo with
KMTQAITYPE_ADAPTERPERFDATA gives the temperature in tenths of a degree, from
any vendor's driver that reports one (WDDM 2.4 and later). They are plain
calls into a system library, so no cgo.

**PDH for the load.** "GPU Engine(*)\Utilization Percentage" is the counter
Task Manager's GPU column is made from: one instance per process per engine,
which busiestEngine reduces to the adapter's figure.
*/

var (
	gdi32                 = windows.NewLazySystemDLL("gdi32.dll")
	procEnumAdapters2     = gdi32.NewProc("D3DKMTEnumAdapters2")
	procQueryAdapterInfo  = gdi32.NewProc("D3DKMTQueryAdapterInfo")
	procCloseAdapter      = gdi32.NewProc("D3DKMTCloseAdapter")
	pdh                   = windows.NewLazySystemDLL("pdh.dll")
	procPdhOpenQuery      = pdh.NewProc("PdhOpenQueryW")
	procPdhAddCounter     = pdh.NewProc("PdhAddEnglishCounterW")
	procPdhCollect        = pdh.NewProc("PdhCollectQueryData")
	procPdhFormattedArray = pdh.NewProc("PdhGetFormattedCounterArrayW")
	procPdhCloseQuery     = pdh.NewProc("PdhCloseQuery")
)

// The D3DKMT query types this reads (d3dkmthk.h).
const (
	kmtqaiAdapterRegistryInfo = 8
	kmtqaiAdapterType         = 15
	kmtqaiAdapterPerfData     = 62
)

// softwareDevice is D3DKMT_ADAPTERTYPE's SoftwareDevice bit: the Microsoft
// Basic Render Driver, which is no card.
const softwareDevice = 1 << 2

// d3dkmtAdapterInfo is D3DKMT_ADAPTERINFO.
type d3dkmtAdapterInfo struct {
	Handle   uint32
	LUIDLow  uint32
	LUIDHigh int32
	Sources  uint32
	Precise  int32
}

// d3dkmtEnumAdapters2 is D3DKMT_ENUMADAPTERS2.
type d3dkmtEnumAdapters2 struct {
	Count    uint32
	Adapters *d3dkmtAdapterInfo
}

// d3dkmtQueryAdapterInfo is D3DKMT_QUERYADAPTERINFO.
type d3dkmtQueryAdapterInfo struct {
	Handle uint32
	Type   uint32
	Data   unsafe.Pointer
	Size   uint32
}

// d3dkmtAdapterPerfData is D3DKMT_ADAPTER_PERFDATA.
type d3dkmtAdapterPerfData struct {
	PhysicalAdapterIndex uint32
	MemoryFrequency      uint64
	MaxMemoryFrequency   uint64
	MaxMemoryFrequencyOC uint64
	MemoryBandwidth      uint64
	PCIEBandwidth        uint64
	FanRPM               uint32
	Power                uint32
	Temperature          uint32 // tenths of a degree Celsius
	PowerStateOverride   uint8
}

// d3dkmtAdapterRegistryInfo is D3DKMT_ADAPTERREGISTRYINFO.
type d3dkmtAdapterRegistryInfo struct {
	AdapterString [windows.MAX_PATH]uint16
	BiosString    [windows.MAX_PATH]uint16
	DacType       [windows.MAX_PATH]uint16
	ChipType      [windows.MAX_PATH]uint16
}

// adapter is one card as D3DKMT describes it.
type adapter struct {
	LUID        string
	Name        string
	Temperature float64
	HasTemp     bool
}

/*
Reader reads the card, keeping the load counter's query open between reads.
The zero value is ready to use, and one Reader is safe for concurrent use.

**The load is a rate, so a figure needs two collections.** The first Read
takes both, Warmup apart, so a program that reads once still gets a load; it
therefore takes Warmup to return. Later Reads difference against the previous
Read's collection, which makes the load a mean over the caller's own interval.
A Read that comes sooner than Warmup after the last waits out the rest, under
its context.

Close releases the query. A Reader that is never closed holds one PDH query
for the life of the process.
*/
type Reader struct {
	mu      sync.Mutex
	query   uintptr
	counter uintptr
	last    time.Time
	failed  bool
}

/*
Read is the first hardware adapter, preferring one that reports a
temperature. ErrNoCard when there is none.

A card whose driver reports no temperature, or a machine without the counter
set, is still a reading: the Card says which figures it has.
*/
func (r *Reader) Read(ctx context.Context) (Card, error) {
	if err := ctx.Err(); err != nil {
		return Card{}, fmt.Errorf("reading the graphics card: %w", err)
	}
	a, ok := firstAdapter()
	if !ok {
		return Card{}, ErrNoCard
	}
	card := Card{Name: a.Name, Temperature: a.Temperature, HasTemperature: a.HasTemp}
	card.Load, card.HasLoad = r.load(ctx, a.LUID)
	return card, nil
}

// Close releases the counter query. The Reader can be read again afterwards,
// and opens a new one.
func (r *Reader) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.closeQuery()
	r.failed = false
	return nil
}

// closeQuery drops the query, if one is open. The caller holds mu.
func (r *Reader) closeQuery() {
	if r.query != 0 {
		_, _, _ = procPdhCloseQuery.Call(r.query)
	}
	r.query, r.counter, r.last = 0, 0, time.Time{}
}

/*
firstAdapter is the first hardware adapter, preferring one that reports a
temperature. A machine lists the Basic Render Driver beside its card, and a
laptop lists two cards of which the integrated one may report nothing.
*/
func firstAdapter() (adapter, bool) {
	adapters := enumAdapters()
	for _, a := range adapters {
		if a.HasTemp {
			return a, true
		}
	}
	if len(adapters) > 0 {
		return adapters[0], true
	}
	return adapter{}, false
}

// enumAdapters is every hardware adapter, opened, read and closed again: the
// set changes with a driver update or a reset, so nothing is held.
func enumAdapters() []adapter {
	var e d3dkmtEnumAdapters2
	if r, _, _ := procEnumAdapters2.Call(uintptr(unsafe.Pointer(&e))); r != 0 || e.Count == 0 { //nolint:gosec // a Win32 call takes its arguments as pointers
		return nil
	}
	list := make([]d3dkmtAdapterInfo, e.Count)
	e.Adapters = &list[0]
	if r, _, _ := procEnumAdapters2.Call(uintptr(unsafe.Pointer(&e))); r != 0 { //nolint:gosec // a Win32 call takes its arguments as pointers
		return nil
	}

	var out []adapter
	for _, info := range list[:min(int(e.Count), len(list))] {
		a, hardware := describeAdapter(info.Handle)
		closeAdapter(info.Handle)
		if hardware {
			a.LUID = luidString(info.LUIDHigh, info.LUIDLow)
			out = append(out, a)
		}
	}
	return out
}

// describeAdapter reads one adapter's type, name and temperature. A software
// adapter is not hardware and is not described.
func describeAdapter(handle uint32) (adapter, bool) {
	var kind uint32
	if !queryAdapter(handle, kmtqaiAdapterType, unsafe.Pointer(&kind), unsafe.Sizeof(kind)) || //nolint:gosec // a Win32 call takes its arguments as pointers
		kind&softwareDevice != 0 {
		return adapter{}, false
	}
	var a adapter
	var reg d3dkmtAdapterRegistryInfo
	if queryAdapter(handle, kmtqaiAdapterRegistryInfo, unsafe.Pointer(&reg), unsafe.Sizeof(reg)) { //nolint:gosec // a Win32 call takes its arguments as pointers
		a.Name = windows.UTF16ToString(reg.AdapterString[:])
	}
	// A driver that does not report a temperature answers zero, which no
	// running card is.
	var perf d3dkmtAdapterPerfData
	if queryAdapter(handle, kmtqaiAdapterPerfData, unsafe.Pointer(&perf), unsafe.Sizeof(perf)) && //nolint:gosec // a Win32 call takes its arguments as pointers
		perf.Temperature > 0 {
		a.Temperature, a.HasTemp = float64(perf.Temperature)/10, true
	}
	return a, true
}

// queryAdapter is one D3DKMTQueryAdapterInfo, and whether it succeeded.
func queryAdapter(handle, kind uint32, data unsafe.Pointer, size uintptr) bool {
	q := d3dkmtQueryAdapterInfo{Handle: handle, Type: kind, Data: data, Size: uint32(size)} //nolint:gosec // a struct's size
	r, _, _ := procQueryAdapterInfo.Call(uintptr(unsafe.Pointer(&q)))                       //nolint:gosec // a Win32 call takes its arguments as pointers
	return r == 0
}

// closeAdapter closes a handle EnumAdapters2 opened.
func closeAdapter(handle uint32) {
	closing := struct{ Handle uint32 }{handle}
	_, _, _ = procCloseAdapter.Call(uintptr(unsafe.Pointer(&closing))) //nolint:gosec // a Win32 call takes its arguments as pointers
}

// The PDH constants this uses (pdh.h, pdhmsg.h).
const (
	pdhFmtDouble   = 0x00000200
	pdhFmtNoCap100 = 0x00008000
	pdhMoreData    = 0x800007D2
	engineCounter  = `\GPU Engine(*)\Utilization Percentage`
)

// pdhItem is PDH_FMT_COUNTERVALUE_ITEM_W with a double value.
type pdhItem struct {
	Name   *uint16
	Status uint32
	Value  float64
}

// load is the adapter's busiest engine since the previous call. See Reader
// for why the first call takes two collections and none is closer than Warmup
// to the last.
func (r *Reader) load(ctx context.Context, luid string) (float64, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.failed {
		return 0, false
	}
	if r.query == 0 && !r.open() {
		// A machine without the counter set (a server core install, a
		// disabled performance library) fails here, once, and has no load.
		r.closeQuery()
		r.failed = true
		return 0, false
	}
	if r.last.IsZero() {
		_, _, _ = procPdhCollect.Call(r.query)
		r.last = time.Now()
	}
	if soon := Warmup - time.Since(r.last); soon > 0 {
		wait := time.NewTimer(soon)
		select {
		case <-ctx.Done():
			wait.Stop()
			return 0, false
		case <-wait.C:
		}
	}
	status, _, _ := procPdhCollect.Call(r.query)
	r.last = time.Now()
	if status != 0 {
		return 0, false
	}
	return busiestEngine(r.values(), luid)
}

// open builds the query.
func (r *Reader) open() bool {
	if status, _, _ := procPdhOpenQuery.Call(0, 0, uintptr(unsafe.Pointer(&r.query))); status != 0 { //nolint:gosec // a Win32 call takes its arguments as pointers
		r.query = 0
		return false
	}
	path, err := windows.UTF16PtrFromString(engineCounter)
	if err != nil {
		return false
	}
	status, _, _ := procPdhAddCounter.Call(r.query, uintptr(unsafe.Pointer(path)), 0, uintptr(unsafe.Pointer(&r.counter))) //nolint:gosec // a Win32 call takes its arguments as pointers
	return status == 0
}

// values is the last collection, instance name to percentage. An instance
// whose value is not valid -- a process that has just appeared has one
// sample, not two -- is left out.
//
// The array is asked for its size and then filled, and is asked again if it
// grew in between: the instances are processes, which come and go.
func (r *Reader) values() map[string]float64 {
	for range 3 {
		var size, count uint32
		status, _, _ := procPdhFormattedArray.Call(r.counter, pdhFmtDouble|pdhFmtNoCap100,
			uintptr(unsafe.Pointer(&size)), uintptr(unsafe.Pointer(&count)), 0) //nolint:gosec // a Win32 call takes its arguments as pointers
		if uint32(status) != pdhMoreData || size == 0 { //nolint:gosec // PDH_STATUS is 32 bits
			return nil
		}
		// The buffer holds the items and, after them, the strings they point
		// at, so it is allocated as items to keep their alignment and sized
		// in bytes.
		itemSize := uint32(unsafe.Sizeof(pdhItem{}))
		buf := make([]pdhItem, (size+itemSize-1)/itemSize)
		status, _, _ = procPdhFormattedArray.Call(r.counter, pdhFmtDouble|pdhFmtNoCap100,
			uintptr(unsafe.Pointer(&size)), uintptr(unsafe.Pointer(&count)), uintptr(unsafe.Pointer(&buf[0]))) //nolint:gosec // a Win32 call takes its arguments as pointers
		switch {
		case uint32(status) == pdhMoreData: //nolint:gosec // PDH_STATUS is 32 bits
			continue
		case status != 0:
			return nil
		}
		out := make(map[string]float64, count)
		for _, item := range buf[:min(int(count), len(buf))] {
			if item.Status > 1 { // neither PDH_CSTATUS_VALID_DATA nor NEW_DATA
				continue
			}
			out[windows.UTF16PtrToString(item.Name)] = item.Value
		}
		return out
	}
	return nil
}
