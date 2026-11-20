# Razer lighting: custom frames

What a Razer device takes for its lights, on the same 90-byte feature reports
the batteries are read with: the Mouse Dock Pro and the Basilisk V3 Pro
behind it first. This page is the source for the `razer-lights` driver (spec
043).

## Sources

- **O**: OpenRGB 1.1's Razer driver (`Controllers/RazerController`, tag
  `release_1.1`), which takes its reports from OpenRazer. GPL, so its code is
  not translated. See [credits.md](credits.md).
- **B**: the bench, recorded in spec 043's Verification.

## The report

The battery driver's report, unchanged: the status, the transaction ID, two
bytes of remaining packets, the protocol, the data size, the class and the
command, 80 bytes of arguments, an XOR checksum, and a reserved byte (O). A
device answers with the same report, its status filled in: 2 success, 1 busy,
4 timeout, 5 not supported (O).

The transaction ID is the product's (O): `FF` for the Mouse Dock Pro, `1F` for
the Basilisk V3 Pro, `3F` for older devices, and others.

## Custom frames

A frame is each row of the device's matrix, then the report that shows the
custom frame (O). Neither stores anything.

| Matrix | A row | Its data size | Then |
|---|---|---|---|
| Extended | `0F:03`: `00 00 <row> <first column> <last column>` and the row's R, G, B | the arguments | `0F:02`, size `0C`: `00 00 08` |
| Standard | `03:0B`: `FF <row> <first> <last>` and R, G, B | the arguments | `03:0A`, size 2: `05 00` |
| Linear | `03:0C`: `<first> <last>` and R, G, B | always `32` | `03:0A`, size 2: `05 00` |

A device's zones fill its matrix in order, row by row (O). An LED is named
after its zone, with `LED <n>` where the zone has more than one (O).

## Behind a receiver

A receiver lists its pairing slots in answer to `00:BF`: a slot count, then
per slot a connection byte and a big-endian product ID, `FFFF` for an empty
slot (O). A device that is not a receiver refuses it. A report sent through
the receiver's node on a paired device's transaction ID goes to that device.
The receiver answers busy while it relays, and drops a report sent meanwhile,
so OpenRGB polls for the answer, up to 50 times 2 ms apart, before the next
(O).

## Never sent

The driver builds only `00:BF`, `0F:02`, `0F:03`, `03:0A`, `03:0B` and
`03:0C`. Effects, brightness and the device mode are not among them; some of
them store what they set.

## On the bench (B)

The Mouse Dock Pro listed the Basilisk V3 Pro as paired. The dock wrote a
frame in under a millisecond; the mouse, relayed, in about 14 ms at the
median. Steady, breathe and wave at 50 ms showed on both, and both kept their
last frame with nothing sent.
