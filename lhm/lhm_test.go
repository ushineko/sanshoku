package lhm_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/sanshoku/lhm"
)

// The fixture has the shape of a LibreHardwareMonitor 0.9.6 answer, with
// invented hardware and figures.
func fixture(t *testing.T) []byte {
	t.Helper()
	body, err := os.ReadFile("testdata/lhm.json")
	require.NoError(t, err)
	return body
}

func tree(t *testing.T, body []byte) lhm.Node {
	t.Helper()
	var root lhm.Node
	require.NoError(t, json.Unmarshal(body, &root))
	return root
}

// serve answers every request with body and status.
func serve(t *testing.T, status int, body []byte) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/data.json" {
			t.Errorf("asked %s %s; only GET /data.json is ever asked", r.Method, r.URL)
		}
		w.WriteHeader(status)
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	return srv.URL + "/data.json"
}

// quick is a client with the package's own timeout; patient waits as long as
// a loaded test machine needs, for the tests that are about size and not time.
func quick() *http.Client   { return &http.Client{Timeout: lhm.Timeout} }
func patient() *http.Client { return &http.Client{Timeout: time.Minute} }

// The processor's temperature is the CPU hardware's, not the card's, though
// both are temperatures.
func TestTheProcessorsTemperatureIsReadFromTheTree(t *testing.T) {
	v, label, ok := lhm.CPUTemperature(tree(t, fixture(t)))
	require.True(t, ok)
	assert.InDelta(t, 47.3, v, 0.001)
	assert.Equal(t, "Core (Tctl/Tdie)", label)
}

// By label in hwmon's order, never by the sensor's number: Tdie wins over
// the one named for both, which wins over Tctl, and Intel's package last.
func TestTheProcessorsLabelsArePreferredInOrder(t *testing.T) {
	sensor := func(id, text, value string) lhm.Node {
		return lhm.Node{SensorID: id, Text: text, Type: "Temperature", RawValue: lhm.Text(value)}
	}
	cases := []struct {
		name    string
		sensors []lhm.Node
		want    float64
		label   string
	}{
		{"Tdie over Tctl", []lhm.Node{
			sensor("/amdcpu/0/temperature/0", "Core (Tctl)", "70.0 °C"),
			sensor("/amdcpu/0/temperature/1", "Core (Tdie)", "60.0 °C"),
		}, 60, "Core (Tdie)"},
		{"both names over Tctl", []lhm.Node{
			sensor("/amdcpu/0/temperature/0", "Core (Tctl)", "70.0 °C"),
			sensor("/amdcpu/0/temperature/9", "Core (Tctl/Tdie)", "65.0 °C"),
		}, 65, "Core (Tctl/Tdie)"},
		{"Intel package", []lhm.Node{
			sensor("/intelcpu/0/temperature/0", "CPU Core #1", "55.0 °C"),
			sensor("/intelcpu/0/temperature/8", "CPU Package", "58.0 °C"),
		}, 58, "CPU Package"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			v, label, ok := lhm.CPUTemperature(lhm.Node{Children: c.sensors})
			require.True(t, ok)
			assert.InDelta(t, c.want, v, 0.001)
			assert.Equal(t, c.label, label)
		})
	}
}

// A card's "GPU Core" or a drive's temperature is never the processor's, and
// a tree with no processor temperature has none.
func TestNoProcessorTemperatureIsNone(t *testing.T) {
	root := lhm.Node{Children: []lhm.Node{
		{SensorID: "/gpu-nvidia/0/temperature/0", Text: "Core (Tctl/Tdie)", Type: "Temperature", RawValue: "39.0 °C"},
		{SensorID: "/amdcpu/0/load/0", Text: "Core (Tctl/Tdie)", Type: "Load", RawValue: "12.0 %"},
	}}
	_, _, ok := lhm.CPUTemperature(root)
	assert.False(t, ok)
}

// The values are text, formatted in the server's locale.
func TestAValueIsReadWithEitherDecimalMark(t *testing.T) {
	for in, want := range map[string]float64{
		"51.5 °C": 51.5, "51,5 °C": 51.5, " 47 °C": 47, "-3.5 °C": -3.5, "100": 100,
	} {
		v, ok := lhm.ParseValue(in)
		require.True(t, ok, in)
		assert.InDelta(t, want, v, 0.001, in)
	}
	for _, in := range []string{"", "°C", "n/a"} {
		_, ok := lhm.ParseValue(in)
		assert.False(t, ok, in)
	}
}

// A number where text was expected is read too, and a SensorId that appears
// twice -- the fixture's "/gpu-nvidia/0/load/3" -- costs nothing.
func TestTheTreeIsReadWhateverItsValuesAre(t *testing.T) {
	root := tree(t, []byte(`{"Children":[{"Text":"Core (Tdie)","SensorId":"/amdcpu/0/temperature/1","Type":"Temperature","RawValue":48.5,"Value":null}]}`))
	v, _, ok := lhm.CPUTemperature(root)
	require.True(t, ok)
	assert.InDelta(t, 48.5, v, 0.001)

	assert.Equal(t, 2, strings.Count(string(fixture(t)), `"/gpu-nvidia/0/load/3"`), "the fixture keeps the duplicate")
	_, _, ok = lhm.CPUTemperature(tree(t, fixture(t)))
	assert.True(t, ok)
}

// Over HTTP, a server that answers gives its temperature and the label.
func TestTheServersTemperatureIsTheProcessors(t *testing.T) {
	v, label, err := lhm.Read(t.Context(), quick(), serve(t, http.StatusOK, fixture(t)))
	require.NoError(t, err)
	assert.InDelta(t, 47.3, v, 0.001)
	assert.Equal(t, "Core (Tctl/Tdie)", label)
}

// A server that answers with no processor temperature says so, which is what
// one without its PawnIO driver serves.
func TestAServerWithNoProcessorTemperatureSaysSo(t *testing.T) {
	_, _, err := lhm.Read(t.Context(), quick(), serve(t, http.StatusOK, []byte(`{"Children":[]}`)))
	require.ErrorIs(t, err, lhm.ErrNoSensor)
}

// A server that wants a password is told apart from one that is not there.
func TestAServerThatWantsAPasswordSaysSo(t *testing.T) {
	_, err := lhm.Fetch(t.Context(), quick(), serve(t, http.StatusUnauthorized, nil))
	require.ErrorIs(t, err, lhm.ErrAuth)
	assert.NotErrorIs(t, err, lhm.ErrUnreachable)
}

// Nothing listening is unreachable. The address is a closed server's, so no
// port is guessed at.
func TestNothingAnsweringIsUnreachable(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL + "/data.json"
	srv.Close()

	_, err := lhm.Fetch(t.Context(), quick(), url)
	require.ErrorIs(t, err, lhm.ErrUnreachable)
}

// A server that does not answer in time is not waited on, with the client's
// timeout or, for a client that has none, the package's.
func TestASlowServerIsNotWaitedOn(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { <-release }))
	t.Cleanup(func() { close(release); srv.Close() })

	for name, client := range map[string]*http.Client{"its own timeout": quick(), "none of its own": {}} {
		t.Run(name, func(t *testing.T) {
			start := time.Now()
			_, err := lhm.Fetch(t.Context(), client, srv.URL+"/data.json")
			require.ErrorIs(t, err, lhm.ErrUnreachable)
			assert.Less(t, time.Since(start), lhm.Timeout+time.Second)
		})
	}
}

// A body far past any sensor tree is refused rather than held.
func TestAnOversizedAnswerIsRefused(t *testing.T) {
	// An answer that declares more than the cap is refused on its header.
	// This one declares nine mebibytes and sends two bytes: read instead, it
	// would end early and say so, so only the header can give this reason.
	declared := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", strconv.Itoa(9<<20))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("{}"))
	}))
	t.Cleanup(declared.Close)
	_, err := lhm.Fetch(t.Context(), patient(), declared.URL+"/data.json")
	require.ErrorIs(t, err, lhm.ErrTooLarge, "an answer declared over the cap was read")

	// One that declares nothing (chunked) is read up to the cap and refused.
	chunked := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		_, _ = w.Write([]byte(`{"Text":"` + strings.Repeat("x", 9<<20) + `"}`))
	}))
	t.Cleanup(chunked.Close)
	_, err = lhm.Fetch(t.Context(), patient(), chunked.URL+"/data.json")
	require.ErrorIs(t, err, lhm.ErrTooLarge, "an undeclared answer over the cap was read in full")
}

// A cancelled poll is the caller's, not a server to explain.
func TestACancelledPollIsTheCallers(t *testing.T) {
	url := serve(t, http.StatusOK, fixture(t))
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, err := lhm.Fetch(ctx, quick(), url)
	require.ErrorIs(t, err, context.Canceled)
	assert.NotErrorIs(t, err, lhm.ErrUnreachable)
}

// The probe asks without administrator rights and does not fail: whatever
// this machine has, the answer is three booleans, and off Windows all three
// are false. What a Windows machine has is logged rather than asserted, since
// the suite does not require LibreHardwareMonitor to be installed.
func TestTheHostProbeAnswersWithoutRights(t *testing.T) {
	host := lhm.ProbeHost()
	t.Logf("PawnIO installed: %v, LibreHardwareMonitor running: %v, startup task: %v",
		host.PawnIO, host.Running, host.Task)
	assert.Equal(t, host.Task, lhm.ProbeHost().Task, "the task index does not change between two reads")
}
