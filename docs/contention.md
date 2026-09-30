# Sharing the Kraken's nodes

What spec 005's bench runs showed about the `nzxt` driver sharing the Kraken
Elite's hidraw node with OpenRGB and with a second program that holds the
same node: hotaru's service today, and a second consumer of this module once
hayami adopts the driver. hayami's phase 2 spec cites this page.

## Summary

Across 900 status questions in three configurations, none went unanswered
and none needed a second attempt. The expected failure mode (a reply taken by
the other reader) did not occur, because hidraw gives every open descriptor
its own copy of every input report. What sharing does cause is the reverse:
**each reader sees the other readers' replies**. A foreign `75 01` is a valid
status reading and is harmless to take. A foreign panel reply could be taken
for the driver's own, and what prevents that is the usbfs claim, which only
one process can hold, and which the driver takes before it sends any panel
command.

## The runs

Kraken Elite (1e71:3012, bcdDevice 01.02) on hidraw6, 2026-09-29. Each run is
`sanshoku-bench read --driver nzxt`, `sanshoku-bench verify --driver nzxt`,
and a measurement loop: 300 status questions 50 ms apart on one handle, each
preceded by a drain that counts what it discards by report prefix, each
followed by up to twelve reads that count the reports skipped before the
`75 01` reply. The loop was a scratch test against `hidraw.Handle` and is not
in the repository; its numbers are below.

| Run | Holding hidraw6 besides the bench | Unanswered in 12 reports | Reports skipped before the reply | Drained before asking | Latency p50 / p95 / max |
|---|---|---|---|---|---|
| (a) hotaru and OpenRGB up | `openrgb`, `hotaru` (hotaru also holds the USB node) | 0 of 300 | 11 in total, at most 1 per question: `31 04` ×5, `75 02` ×3, `33 01` ×2, `33 02` ×1 | `31 04` ×75, `75 01` ×15, `75 02` ×14, `33 02` ×10, `37 02` ×6, `39 01` ×6, `37 01` ×5, `37 03` ×5, `33 01` ×3 | 1.73 / 2.24 / 52.7 ms |
| (b) hotaru stopped, OpenRGB up | `openrgb` | 0 of 300 | 2, at most 1: `75 02` ×2 | `75 02` ×15, `75 01` ×4 | 1.72 / 2.00 / 39.7 ms |
| (c) alone | nothing | 0 of 300 | 1: `75 02` ×1 | `75 02` ×15, `75 01` ×4 | 1.72 / 2.01 / 48.7 ms |

`verify` agreed with liquidctl in all three runs; the bench output is in
spec 005's Verification section.

## What the numbers say

- **Replies are not stolen.** hotaru's retry exists because a status reply
  "simply did not arrive" while OpenRGB held the node (hotaru spec 012). Here,
  none failed to arrive in 300 questions with OpenRGB up, or in 300 with
  hotaru and OpenRGB both up. The kernel queues each input report for every
  open descriptor, so another reader cannot consume this one's copy. The
  three-attempt retry stays: it costs nothing when unused, and the cause of
  what hotaru saw is not established.
- **Every reader sees every other reader's replies.** In run (a) the drain
  discarded hotaru's whole panel conversation (`31 04` slot queries, `33 0x`,
  `37 0x`, `39 01`) and fifteen `75 01` replies to questions this handle did
  not ask. The drain before each question removes what arrived earlier; a
  foreign reply that lands between this handle's write and its own reply is
  matched by prefix like any other.
  - For status that is harmless: any `75 01` is the device's current reading.
  - For the panel it would not be: a foreign `31 04` for another slot, or a
    `33 01` result for another program's reservation, would be read as this
    one's. Two programs driving the panel at once would corrupt each other's
    placement. The driver does not send a panel command until it has claimed
    the USB interface, and the kernel lets one process hold that claim, so
    the second program gets `screen.ErrNoPanel` and writes nothing.
- **A `75 01` arrives unasked even alone.** Run (c) drained four in 300
  questions with no other process on the node. The likely source is a
  duplicate or late reply to one of this handle's own earlier questions; the
  cause is not established. It is harmless for the same reason as above.
- **The broadcast rate is lower than one a second on this unit.** Fifteen
  `75 02` broadcasts in each 16.5 s run, about one every 1.1 s, where hotaru
  recorded "about once a second". A handle idle for a minute still holds
  about fifty, which is why every question drains first.
- **Latency does not change with the other readers.** The median exchange is
  1.7 ms in all three runs; the maxima (40 to 53 ms) occur in all three and
  are not attributable to sharing.

## Guidance for a consumer

- Two processes reading status from one Kraken is safe. hayami and hotaru
  can both hold the node.
- One process draws on the panel. The first to call a panel method claims it
  for as long as its `Device` is open; any other gets `screen.ErrNoPanel`
  wrapping `sanshoku.ErrAbsent` and should treat the panel as absent rather
  than retrying.
- `Close` hands the panel back to the firmware readout before releasing the
  claim, so the claim moves between programs only through a clean readout.

## Not measured

- A second consumer calling a panel method while hotaru holds the claim. The
  outcome above (`EBUSY` at the claim, reported as `screen.ErrNoPanel`) is
  what usbfs documents, not something the bench ran.
- Queue overflow. No handle in these runs was idle long enough to fill the
  kernel's 64-report queue; the drain bounds itself to that depth either way.
- Stopping OpenRGB for run (c): `systemctl --user stop openrgb-server` ran
  into the unit's stop timeout and systemd ended the process with SIGKILL.
  The service was started again afterwards and re-enumerated its devices.
