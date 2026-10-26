package aula

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/sanshoku"
	"github.com/ushineko/sanshoku/hidraw"
	"github.com/ushineko/sanshoku/lighting"
)

/*
sent is every command any test's fake was written, on either device. The
frame and battery tests' receiver may be sent three commands and the cable
two, and none of the commands that read or write the keyboard's flash; only
the onboard tests' board may be sent those, and no frame. TestMain holds it
to that across the whole package (spec 019 R4.4, spec 020 R4.4).
*/
var sent = struct {
	sync.Mutex
	receiver, board, cable map[byte]bool
	reports                map[byte]bool
}{receiver: map[byte]bool{}, board: map[byte]bool{}, cable: map[byte]bool{}, reports: map[byte]bool{}}

func noteReceiver(command byte) {
	sent.Lock()
	defer sent.Unlock()
	sent.receiver[command] = true
}

func noteBoard(command byte) {
	sent.Lock()
	defer sent.Unlock()
	sent.board[command] = true
}

func noteCable(report, command byte) {
	sent.Lock()
	defer sent.Unlock()
	sent.reports[report] = true
	sent.cable[command] = true
}

func TestMain(m *testing.M) {
	code := m.Run()
	if !onlyAllowedCommandsWereSent() {
		code = 1
	}
	os.Exit(code)
}

// onlyAllowedCommandsWereSent reports, and says, whether any fake was written
// a command or a report the driver may not send.
func onlyAllowedCommandsWereSent() bool {
	sent.Lock()
	defer sent.Unlock()
	ok := true
	allowed := map[byte]bool{commandBat: true, commandModel: true, commandFrame: true}
	for c := range sent.receiver {
		if !allowed[c] {
			fmt.Printf("the receiver was sent command %#02x, which the driver may not send\n", c)
			ok = false
		}
	}
	onboard := map[byte]bool{
		commandModel: true, commandReadConfig: true, commandReadPalette: true,
		commandWriteConfig: true, commandWritePalette: true,
	}
	for c := range sent.board {
		if !onboard[c] {
			fmt.Printf("the onboard tests sent command %#02x, which Effect and SetColour may not send\n", c)
			ok = false
		}
	}
	for c := range sent.cable {
		if c != wiredModelCommand && c != wiredFrameCommand {
			fmt.Printf("the cable was sent command %#02x, which the driver may not send\n", c)
			ok = false
		}
	}
	for r := range sent.reports {
		if r != wiredReport {
			fmt.Printf("the cable was sent report %d; only report %d is ever sent\n", r, wiredReport)
			ok = false
		}
	}
	return ok
}

// modelReply is what a keyboard on the receiver answers when asked its model.
// With 0xCD it is the F75's answer as measured.
func modelReply(id byte) []byte {
	r := []byte{0x13, 0x05, 0x01, 0x00, 0x0a, 0x03, 0, 0, 0, 0, id, 0x01, 0, 0, 0, 0, 0, 0, 0, 0}
	r[reportLen-1] = checksum(r)
	return r
}

// commands is the command byte of every packet a fake was written.
func commands(f *fake) []byte {
	out := make([]byte, 0, len(f.requests))
	for _, r := range f.requests {
		out = append(out, r[1])
	}
	return out
}

func canvasOf(t *testing.T, dev sanshoku.Device) lighting.Canvas {
	t.Helper()
	cv, ok := dev.(lighting.Canvas)
	require.True(t, ok)
	return cv
}

// The measured answer is the F75's.
func TestTheMeasuredModelReplyIsAnF75(t *testing.T) {
	measured := []byte{0x13, 0x05, 0x01, 0x00, 0x0a, 0x03, 0, 0, 0, 0, 0xcd, 0x01, 0, 0, 0, 0, 0, 0, 0, 0xf4}

	assert.Equal(t, measured, modelReply(modelF75))
	assert.True(t, answersTo(commandModel)(measured))
	assert.Equal(t, []byte{0x13, 0x05, 0x01, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0x19},
		packet(commandModel, 1, 0, 0, nil), "the question the vendor's program asks")
}

// The receiver's device is a canvas that fades, and says so.
func TestTheReceiverIsACanvasThatFades(t *testing.T) {
	dev := open(t, &fake{})

	assert.Equal(t, []string{"battery", "lighting", "onboard"}, sanshoku.Capabilities(dev))
	cv := canvasOf(t, dev)
	assert.Len(t, cv.Keys(), keyCount)
	assert.Equal(t, frameFloor, cv.Floor())
	fading, ok := dev.(lighting.Fading)
	require.True(t, ok)
	assert.Equal(t, frameHold, fading.Hold())
}

// The model is asked before the first frame and not again.
func TestTheModelIsAskedOncePerHandle(t *testing.T) {
	f := &fake{answers: [][][]byte{{status, modelReply(modelF75)}}}
	cv := canvasOf(t, open(t, f))

	require.NoError(t, cv.Frame(context.Background(), one(red, 14)))
	require.NoError(t, cv.Frame(context.Background(), one(red, 9)))

	assert.Equal(t, []byte{commandModel, commandFrame, commandFrame}, commands(f))
}

// The same receiver id is other keyboards with other layouts. One that says
// it is not an F75 is never drawn on, however often it is asked.
func TestAnotherKeyboardOnTheReceiverIsNeverDrawnOn(t *testing.T) {
	const f99 = 0xA4
	f := &fake{answers: [][][]byte{{modelReply(f99)}}}
	cv := canvasOf(t, open(t, f))

	err := cv.Frame(context.Background(), one(red, 14))
	require.ErrorIs(t, err, lighting.ErrNoCanvas)
	require.ErrorIs(t, cv.Frame(context.Background(), one(red, 14)), lighting.ErrNoCanvas)

	assert.Equal(t, []byte{commandModel}, commands(f))
}

// A keyboard that is asleep answers nothing. No frame is sent, and it is
// asked again at the next frame rather than written off.
func TestASleepingKeyboardIsAskedAgain(t *testing.T) {
	f := &fake{answers: [][][]byte{nil, {modelReply(modelF75)}}}
	cv := canvasOf(t, open(t, f))

	err := cv.Frame(context.Background(), one(red, 14))
	require.ErrorIs(t, err, hidraw.ErrSilent)
	require.NotErrorIs(t, err, lighting.ErrNoCanvas)
	require.NoError(t, cv.Frame(context.Background(), one(red, 14)))

	assert.Equal(t, []byte{commandModel, commandModel, commandFrame}, commands(f))
}

/*
The keyboard darkens what a message does not name, and the interface says a
light a frame does not name is left as it was. So the second frame carries
the first one's keys too.
*/
func TestAFrameKeepsWhatWasAlreadyShown(t *testing.T) {
	f := &fake{answers: [][][]byte{{modelReply(modelF75)}}}
	cv := canvasOf(t, open(t, f))
	require.NoError(t, cv.Frame(context.Background(), one(red, 14)))

	require.NoError(t, cv.Frame(context.Background(), one(blue, 44)))

	px := one(red, 14)
	px = append(px, one(blue, 44)...)
	assert.Equal(t, receiverFrame(lit(t, px...))[0], f.requests[len(f.requests)-1])
}

// Release sends nothing, and the next frame starts from a dark board.
func TestReleaseForgetsTheFrameAndSendsNothing(t *testing.T) {
	f := &fake{answers: [][][]byte{{modelReply(modelF75)}}}
	cv := canvasOf(t, open(t, f))
	require.NoError(t, cv.Frame(context.Background(), one(red, 14)))
	before := len(f.requests)

	require.NoError(t, cv.Release(context.Background()))
	assert.Len(t, f.requests, before)

	require.NoError(t, cv.Frame(context.Background(), one(blue, 44)))
	assert.Equal(t, receiverFrame(lit(t, one(blue, 44)...))[0], f.requests[len(f.requests)-1])
}

// A slot with no light on it is refused before anything is asked or sent,
// and the frame already held is as it was.
func TestASlotWithNoLightIsRefusedAndNothingIsSent(t *testing.T) {
	f := &fake{}
	dev := open(t, f)

	for _, slot := range []byte{6, 84, 200} {
		err := canvasOf(t, dev).Frame(context.Background(), append(one(red, 14), one(red, slot)...))
		require.Error(t, err, "slot %d", slot)
	}

	assert.Empty(t, f.requests)
	assert.Equal(t, frame{}, dev.(*device).held)
}

// A closed canvas says so.
func TestAClosedCanvasSaysSo(t *testing.T) {
	dev := open(t, &fake{})
	require.NoError(t, dev.Close())

	require.ErrorIs(t, canvasOf(t, dev).Frame(context.Background(), one(red, 14)), os.ErrClosed)
	require.ErrorIs(t, canvasOf(t, dev).Release(context.Background()), os.ErrClosed)
}

/*
fakeCable is the wired keyboard's vendor collection: it records every feature
report it is sent and answers the model question with model, or with nothing
it was asked for when deaf.
*/
type fakeCable struct {
	model  byte
	deaf   bool
	sets   [][]byte
	closed bool
}

func (f *fakeCable) SetFeature(_ context.Context, report []byte) error {
	noteCable(report[0], report[1])
	f.sets = append(f.sets, append([]byte(nil), report...))
	return nil
}

func (f *fakeCable) GetFeature(_ context.Context, report []byte) error {
	if f.deaf {
		return nil
	}
	last := f.sets[len(f.sets)-1]
	copy(report, last[:wiredHeader])
	report[wiredHeader+wiredModelAt] = f.model
	return nil
}

func (f *fakeCable) Close() error { f.closed = true; return nil }

// cable is the F75 on its cable as Nodes reports it.
var cable = hidraw.Node{
	Path: "cable", Name: "BY Tech Gaming Keyboard", Vendor: wiredVendor, Product: wiredProduct,
	Reports: []hidraw.Report{{Kind: hidraw.ReportFeature, ID: wiredReport, Page: wiredPage, Usage: 1, Len: wiredLen}},
}

func openCable(t *testing.T, f *fakeCable) sanshoku.Device {
	t.Helper()
	dev, err := wiredCandidate(cable, func(string) (wiredNode, error) { return f, nil }).Open(context.Background())
	require.NoError(t, err)
	dev.(*wired).settle = time.Millisecond
	return dev
}

// The cable's node is told from the chip's other products and collections.
func TestTheCableIsFoundByProductAndReport(t *testing.T) {
	assert.True(t, isWired(cable))

	other := cable
	other.Product = 0x0049
	assert.False(t, isWired(other), "another Sino Wealth product")

	keys := cable
	keys.Reports = []hidraw.Report{{Kind: hidraw.ReportInput, ID: 1, Page: 1, Usage: 6, Len: 9}}
	assert.False(t, isWired(keys), "the keyboard's own interface")
}

// On its cable the F75 is a canvas that fades, named for the keyboard, with
// no battery to read.
func TestTheCableIsACanvasAndNotABattery(t *testing.T) {
	dev := openCable(t, &fakeCable{model: modelF75})

	assert.Equal(t, []string{"lighting"}, sanshoku.Capabilities(dev))
	assert.Equal(t, "AULA F75", dev.Identity().Name)
	_, fades := dev.(lighting.Fading)
	assert.True(t, fades)
	assert.Len(t, canvasOf(t, dev).Keys(), keyCount)
}

// The model is asked once, on report 6 with command 0x82 and index 1, and
// each frame after it is one feature report.
func TestACableFrameIsOneFeatureReportAfterTheModel(t *testing.T) {
	f := &fakeCable{model: modelF75}
	cv := canvasOf(t, openCable(t, f))

	require.NoError(t, cv.Frame(context.Background(), one(red, 14)))
	require.NoError(t, cv.Frame(context.Background(), one(blue, 44)))

	require.Len(t, f.sets, 3)
	assert.Equal(t, []byte{0x06, 0x82, 0x01, 0x00, 0x01, 0x00, 0x06, 0x00}, f.sets[0][:wiredHeader])
	px := one(red, 14)
	px = append(px, one(blue, 44)...)
	assert.Equal(t, wiredFrame(lit(t, px...)), f.sets[2], "the second frame keeps the first one's key")
}

// The same cable id is other keyboards. One that is not an F75 is never
// drawn on.
func TestAnotherKeyboardOnTheCableIsNeverDrawnOn(t *testing.T) {
	const l65 = 0xCA
	f := &fakeCable{model: l65}
	cv := canvasOf(t, openCable(t, f))

	require.ErrorIs(t, cv.Frame(context.Background(), one(red, 14)), lighting.ErrNoCanvas)
	require.ErrorIs(t, cv.Frame(context.Background(), one(red, 14)), lighting.ErrNoCanvas)

	assert.Len(t, f.sets, 1, "only the question was sent")
}

// An answer that does not echo the question is not a model, and no frame
// follows it.
func TestACableThatDoesNotAnswerIsNotDrawnOn(t *testing.T) {
	f := &fakeCable{deaf: true}
	cv := canvasOf(t, openCable(t, f))

	err := cv.Frame(context.Background(), one(red, 14))

	require.ErrorIs(t, err, errNoAnswer)
	assert.Len(t, f.sets, 1)
}

// A closed cable says so, and closes its node once.
func TestAClosedCableSaysSo(t *testing.T) {
	f := &fakeCable{model: modelF75}
	dev := openCable(t, f)
	require.NoError(t, dev.Close())
	require.NoError(t, dev.Close())
	assert.True(t, f.closed)

	require.ErrorIs(t, canvasOf(t, dev).Frame(context.Background(), one(red, 14)), os.ErrClosed)
	require.ErrorIs(t, canvasOf(t, dev).Release(context.Background()), os.ErrClosed)
}
