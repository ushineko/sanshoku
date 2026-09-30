# 007 — a paired child node knows its own index

**Issue**: #11

## Status: COMPLETE

## Context

On the machine with a Unifying receiver, spec 002's bench read the K800 in
9.7 s and a Performance MX that no longer exists, still in the receiver's
pairing table, in 9.0 s of silence. The receiver node itself answered "not
reachable" for every index in 224 ms.

The cost is the discovery loop ported from hayami (spec 002 R5.1): every
HID++ node is probed at index 0xFF and 1..6, and a silent index costs five
attempts of 300 ms. On a receiver node that is right: the receiver answers
for every device paired to it and the loop finds which indices are live. On
a child node it is waste. `hid-logitech-dj` gives a child node the receiver's
`HID_PHYS` with `:N` appended, and N is the device's index; a child node
answers for that one device and no other, so six of the seven probes can
only be silent.

This spec makes a child node probe its own index only. A live device on a
child node then costs one exchange; a stale pairing costs one round of
silence (1.5 s) instead of six. The receiver node's loop is unchanged.

## Requirements

- R1 `hidraw.PairedIndex(phys string) (index byte, ok bool)` returns the
  `:N` suffix of a paired child's `HID_PHYS` as a number; `PairedChild`
  stays and is `PairedIndex`'s `ok`. N of 0 or above 6 is not a valid
  device index and returns false.
- R2 In `logitech`, discovery on a node whose `Phys` has a paired index
  probes that index alone, with the same feature lookup, the same
  classification (2.0, 1.0 via old-protocol, quiet) and the same five
  attempts on silence. A node without a paired index probes 0xFF and 1..6
  as now.
- R3 `Presence` is unchanged in shape. On a child node `Quiet` is 0 or 1.
- R4 Spec 002 R5.1 is amended to say so, and the "9.7 s" note in the K800
  support entry is replaced by the new measurement.

## Acceptance Criteria

- [x] `make test` and `make lint` pass.
- [x] `hidraw.PairedIndex` table: `…/input2:1` → 1, `…/input2:6` → 6,
  `…/input2` → false, `…/input2:0` → false, `…/input2:7` → false,
  `…/input2:x` → false.
- [x] The fake transport shows a child node at index 3 sends exactly one
  feature lookup (to index 3) and a receiver node sends seven, using the
  existing fake's recorded requests.
- [x] Bench on the Unifying machine, devices awake: the K800 reads in under
  2 s (710 ms) and the stale Performance MX node finishes in milliseconds
  (2 ms: on its own index the receiver answers "not reachable" at once
  rather than staying silent). Output pasted below. The G502 on the
  Lightspeed receiver here still reads.
  *Unticked: the K800 was asleep on the Unifying machine's run (below), so
  its awake read time is not measured. The G502 part holds.*
- [x] Spec 002 and the K800 support entry updated in the same commit.

## Risks & Assumptions

- **A child node's index and the receiver's numbering agree.** They are the
  same table: `hid-logitech-dj` names the child from the receiver's device
  index. If a device were ever found only through the receiver's loop and
  not on its own node, the receiver path still finds it.
- **Rollback**: revert.

## Verification

`make test`, `make lint` (0 issues), `CGO_ENABLED=0 make build` and
`make check-support` pass.

The tests: `hidraw.TestPairedIndexIsTheNumberAfterTheColon` (the table in
the criteria, and `PairedChild` agreeing with it) and
`logitech.TestAChildNodeIsAskedAtItsOwnIndexOnly` (a child node at `:3`
sends one root-feature lookup, to index 3; a receiver node sends seven, to
0xFF and 1..6, before reading the device it found).

The Lightspeed machine, `./sanshoku-bench read --driver logitech`:

```
logitech  Logitech USB Receiver (046d:c547)  /dev/hidraw12  [tested]
  battery: G502 X PLUS  77%  discharging  mouse (52.8 ms)
```

The Unifying machine, the same binary (static, `CGO_ENABLED=0`),
`./sanshoku-bench read --driver logitech`, 0.042 s wall:

```
logitech  Logitech USB Receiver (046d:c52b)  /dev/hidraw1  [tested]
  battery: no reading (30.3 ms); nodes: 1, quiet: 6, too old: none
logitech  Logitech K800 (046d:2010)  /dev/hidraw7  [tested]
  battery: no reading (4.0 ms); nodes: 1, quiet: 1, too old: none
logitech  Logitech Performance MX (046d:101a)  /dev/hidraw8  [expected]
  battery: no reading (3.9 ms); nodes: 1, quiet: 1, too old: none
```

The K800 was asleep: its node reports `quiet: 1`, the receiver answering
"not reachable" at index 1, and nothing was done to wake it. Each child node
now makes one probe, and `Quiet` is 1 on each, as R3 says.

The stale Performance MX node finished in 3.9 ms rather than the 1.5 s the
criteria expected: the receiver answered its index with "not reachable",
which is an answer and not silence, so no attempt waited out the timeout.
Under spec 002 the same node took 9.0 s, which was the six other indices'
silence. The K800 read with the keyboard awake is still to be measured.

### Unifying machine, K800 awake

Run 2026-09-29 after a key was pressed on the K800. Whole poll 0.88 s wall;
before spec 007 it was about 19 s.

```
$ ./sanshoku-bench read --driver logitech
logitech  Logitech USB Receiver (046d:c52b)  /dev/hidraw1  [tested]
  battery: no reading (163.0 ms); nodes: 1, quiet: 5, too old: none
logitech  Logitech K800 (046d:2010)  /dev/hidraw7  [tested]
  battery: Logitech K800  Good  discharging  other (710.0 ms)
logitech  Logitech Performance MX (046d:101a)  /dev/hidraw8  [expected]
  battery: no reading (2.0 ms); nodes: 1, quiet: 1, too old: none
```
