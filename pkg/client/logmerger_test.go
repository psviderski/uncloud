package client

import (
	"testing"
	"time"

	"github.com/psviderski/uncloud/pkg/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testEntry creates a ServiceLogEntry for testing.
func testEntry(stream api.LogStreamType, ts time.Time, msg string) api.ServiceLogEntry {
	return api.ServiceLogEntry{
		ContainerLogEntry: api.ContainerLogEntry{
			Stream:    stream,
			Timestamp: ts,
			Message:   []byte(msg),
		},
	}
}

// collectEntries collects up to maxCount entries from the channel or until it is closed.
func collectEntries(t *testing.T, ch <-chan api.ServiceLogEntry, maxCount int) []api.ServiceLogEntry {
	t.Helper()
	var entries []api.ServiceLogEntry

	for i := 0; i < maxCount || maxCount <= 0; i++ {
		select {
		case e, ok := <-ch:
			if !ok {
				return entries
			}
			entries = append(entries, e)
		case <-time.After(1 * time.Second):
			require.FailNow(t, "timed out waiting for log entry")
		}
	}

	return entries
}

func TestLogMerger_EmptyStreams(t *testing.T) {
	t.Parallel()

	merger := NewLogMerger(nil, LogMergerOptions{})
	output := merger.Stream()

	_, ok := <-output
	assert.False(t, ok, "output channel should be closed for empty streams")
}

func TestLogMerger_SingleStream(t *testing.T) {
	t.Parallel()

	ch := make(chan api.ServiceLogEntry, 10)
	merger := NewLogMerger([]<-chan api.ServiceLogEntry{ch}, LogMergerOptions{})
	output := merger.Stream()

	t1 := time.Now()
	t2 := t1.Add(time.Second)

	ch <- testEntry(api.LogStreamStdout, t1, "first")
	ch <- testEntry(api.LogStreamStdout, t2, "second")

	results := collectEntries(t, output, 2)

	require.Len(t, results, 2)
	assert.Equal(t, "first", string(results[0].Message))
	assert.Equal(t, "second", string(results[1].Message))

	close(ch)

	results = collectEntries(t, output, 0)
	assert.Len(t, results, 0, "no entries expected after close")
}

func TestLogMerger_PreservesData(t *testing.T) {
	t.Parallel()

	ch := make(chan api.ServiceLogEntry, 10)
	merger := NewLogMerger([]<-chan api.ServiceLogEntry{ch}, LogMergerOptions{})
	output := merger.Stream()

	metadata := api.ServiceLogEntryMetadata{
		ServiceID:   "svc-123",
		ServiceName: "my-service",
		MachineID:   "machine-456",
		MachineName: "machine-1",
	}

	e := api.ServiceLogEntry{
		Metadata: metadata,
		ContainerLogEntry: api.ContainerLogEntry{
			Stream:    api.LogStreamStdout,
			Timestamp: time.Now(),
			Message:   []byte("test"),
		},
	}
	ch <- e
	close(ch)

	results := collectEntries(t, output, 0)
	require.Len(t, results, 1)
	assert.Equal(t, e, results[0])
}

func TestLogMerger_BasicMerge(t *testing.T) {
	t.Parallel()

	ch1 := make(chan api.ServiceLogEntry, 10)
	ch2 := make(chan api.ServiceLogEntry, 10)

	merger := NewLogMerger([]<-chan api.ServiceLogEntry{ch1, ch2}, LogMergerOptions{})
	output := merger.Stream()

	t1 := time.Now()
	t2 := t1.Add(time.Second)
	t3 := t1.Add(2 * time.Second)
	t4 := t1.Add(3 * time.Second)

	// Send interleaved entries from both streams.
	ch1 <- testEntry(api.LogStreamStdout, t1, "ch1-first")
	ch2 <- testEntry(api.LogStreamStdout, t2, "ch2-first")
	ch1 <- testEntry(api.LogStreamStdout, t3, "ch1-second")
	ch2 <- testEntry(api.LogStreamStdout, t4, "ch2-second")

	// Send heartbeats to advance watermark past all entries.
	t5 := t1.Add(4 * time.Second)
	ch1 <- testEntry(api.LogStreamHeartbeat, t5, "")
	ch2 <- testEntry(api.LogStreamHeartbeat, t5, "")

	results := collectEntries(t, output, 4)
	require.Len(t, results, 4)
	assert.Equal(t, "ch1-first", string(results[0].Message))
	assert.Equal(t, "ch2-first", string(results[1].Message))
	assert.Equal(t, "ch1-second", string(results[2].Message))
	assert.Equal(t, "ch2-second", string(results[3].Message))

	close(ch1)
	close(ch2)

	results = collectEntries(t, output, 0)
	assert.Len(t, results, 1, "one debounced heartbeat expected after close")
	assert.Equal(t, api.LogStreamHeartbeat, results[0].Stream)
	assert.Equal(t, t5, results[0].Timestamp)
}

func TestLogMerger_HeartbeatAdvancesWatermark(t *testing.T) {
	t.Parallel()

	ch1 := make(chan api.ServiceLogEntry, 10)
	ch2 := make(chan api.ServiceLogEntry, 10)

	merger := NewLogMerger([]<-chan api.ServiceLogEntry{ch1, ch2}, LogMergerOptions{})
	output := merger.Stream()

	t1 := time.Now()
	t2 := t1.Add(time.Second)
	t3 := t1.Add(2 * time.Second)

	// Stream 1 sends a log entry.
	ch1 <- testEntry(api.LogStreamStdout, t1, "ch1-first")
	ch1 <- testEntry(api.LogStreamStdout, t3, "ch1-second")
	// Stream 2 is quiet but sends a heartbeat.
	ch2 <- testEntry(api.LogStreamHeartbeat, t2, "")

	// Now stream 1's entry should be emitted because watermark is t1 (min of t1, t2).
	results := collectEntries(t, output, 1)
	require.Len(t, results, 1)
	assert.Equal(t, "ch1-first", string(results[0].Message))

	close(ch1)
	close(ch2)

	results = collectEntries(t, output, 0)
	require.NotEmpty(t, results)

	// Depending on timing, we may get a heartbeat before the second log entry.
	require.LessOrEqual(t, len(results), 2, "one or two entries expected after close")
	if len(results) == 2 {
		assert.Equal(t, api.LogStreamHeartbeat, results[0].Stream)
		assert.Equal(t, t2, results[0].Timestamp)
		results = results[1:]
	}
	assert.Equal(t, "ch1-second", string(results[0].Message))
}

func TestLogMerger_ErrorForwarding(t *testing.T) {
	t.Parallel()

	ch1 := make(chan api.ServiceLogEntry, 10)
	ch2 := make(chan api.ServiceLogEntry, 10)
	merger := NewLogMerger([]<-chan api.ServiceLogEntry{ch1, ch2}, LogMergerOptions{})
	output := merger.Stream()

	t1 := time.Now()

	ch1 <- testEntry(api.LogStreamStdout, t1, "ch1-first")
	// Send an error entry.
	ch1 <- api.ServiceLogEntry{
		ContainerLogEntry: api.ContainerLogEntry{
			Err: assert.AnError,
		},
	}

	results := collectEntries(t, output, 1)
	require.Len(t, results, 1, "expected error to emitted out of order")
	assert.Equal(t, assert.AnError, results[0].Err)

	close(ch1)
	close(ch2)

	results = collectEntries(t, output, 0)
	assert.Len(t, results, 1)
	assert.Equal(t, "ch1-first", string(results[0].Message))
}

func TestLogMerger_OutOfOrderSingleStream(t *testing.T) {
	t.Parallel()

	ch := make(chan api.ServiceLogEntry, 10)
	merger := NewLogMerger([]<-chan api.ServiceLogEntry{ch}, LogMergerOptions{})
	output := merger.Stream()

	t1 := time.Now()
	t2 := t1.Add(time.Second)
	t3 := t1.Add(2 * time.Second)
	t4 := t1.Add(3 * time.Second)

	// Send entries out of order within the stream.
	ch <- testEntry(api.LogStreamStdout, t3, "third")
	ch <- testEntry(api.LogStreamStdout, t2, "second")
	ch <- testEntry(api.LogStreamStdout, t4, "forth")
	ch <- testEntry(api.LogStreamStdout, t1, "first")

	results := collectEntries(t, output, 4)
	require.Len(t, results, 4)
	// Emitted in the same order as sent since no buffering/reordering is done within a single stream.
	assert.Equal(t, "third", string(results[0].Message))
	assert.Equal(t, "second", string(results[1].Message))
	assert.Equal(t, "forth", string(results[2].Message))
	assert.Equal(t, "first", string(results[3].Message))

	close(ch)

	results = collectEntries(t, output, 0)
	assert.Len(t, results, 0, "no entries expected after close")
}

// Test that entries from streams with different rates are merged correctly in chronological order.
func TestLogMerger_UnevenStreams(t *testing.T) {
	t.Parallel()

	numFastEntries := logMergerMaxInFlightPerStream

	// Use buffered channels to allow sends without blocking on receiver.
	ch1 := make(chan api.ServiceLogEntry, numFastEntries+5)
	ch2 := make(chan api.ServiceLogEntry, 10)

	merger := NewLogMerger([]<-chan api.ServiceLogEntry{ch1, ch2}, LogMergerOptions{})
	output := merger.Stream()

	baseTime := time.Now()

	// Stream 1 sends many entries quickly (0ms - 104ms).
	for i := 0; i < numFastEntries; i++ {
		ch1 <- testEntry(api.LogStreamStdout, baseTime.Add(time.Duration(i)*time.Millisecond), "fast")
	}

	// Stream 2 sends entries that interleave with fast entries.
	ch2 <- testEntry(api.LogStreamStdout, baseTime.Add(5*time.Millisecond), "slow")
	ch2 <- testEntry(api.LogStreamStdout, baseTime.Add(50*time.Millisecond), "slow")

	// Send heartbeats to advance watermark past all entries so they get emitted.
	finalTime := baseTime.Add(time.Second)
	ch1 <- testEntry(api.LogStreamHeartbeat, finalTime, "")
	ch2 <- testEntry(api.LogStreamHeartbeat, finalTime.Add(10*time.Millisecond), "")

	// Read exactly numFastEntries + 2 log entries.
	expectedCount := numFastEntries + 2
	results := collectEntries(t, output, expectedCount)
	require.Len(t, results, expectedCount)

	// Count entries from each stream.
	fastCount := 0
	slowCount := 0
	for _, entry := range results {
		switch string(entry.Message) {
		case "fast":
			fastCount++
		case "slow":
			slowCount++
		}
	}
	assert.Equal(t, numFastEntries, fastCount)
	assert.Equal(t, 2, slowCount)

	// Verify entries are in chronological order.
	for i := 1; i < len(results); i++ {
		assert.False(t, results[i].Timestamp.Before(results[i-1].Timestamp),
			"entry %d (ts=%v) should not be before entry %d (ts=%v)",
			i, results[i].Timestamp, i-1, results[i-1].Timestamp)
	}

	close(ch1)
	close(ch2)

	results = collectEntries(t, output, 0)
	require.Len(t, results, 1, "one debounced heartbeat expected after close")
	assert.Equal(t, api.LogStreamHeartbeat, results[0].Stream)
}

func TestLogMerger_StalledStreamExcludedFromWatermark(t *testing.T) {
	t.Parallel()

	ch1 := make(chan api.ServiceLogEntry, 10)
	ch2 := make(chan api.ServiceLogEntry, 10)

	stallTimeout := 100 * time.Millisecond
	merger := NewLogMerger([]<-chan api.ServiceLogEntry{ch1, ch2}, LogMergerOptions{
		StallTimeout:       stallTimeout,
		StallCheckInterval: 20 * time.Millisecond,
	})
	output := merger.Stream()

	//// Collect all output entries in a separate goroutine.
	//var results []api.ServiceLogEntry
	//doneCollecting := make(chan struct{})
	//go func() {
	//	for entry := range output {
	//		results = append(results, entry)
	//	}
	//	close(doneCollecting)
	//}()

	t1 := time.Now()
	t2 := t1.Add(time.Second)
	t3 := t1.Add(2 * time.Second)
	t4 := t1.Add(3 * time.Second)

	e1 := testEntry(api.LogStreamStdout, t1, "ch1-entry")
	e1.Metadata = api.ServiceLogEntryMetadata{
		ServiceID:   "ch1-svc1",
		ServiceName: "ch1-svcName",
		ContainerID: "ch1-ctr1",
		MachineID:   "ch1-machine1",
		MachineName: "ch1-machineName",
	}

	ch1 <- e1
	ch2 <- testEntry(api.LogStreamStdout, t2, "ch2-first")

	// Watermark is t1 now so ch1's entry could be collected.
	results := collectEntries(t, output, 1)
	require.Len(t, results, 1)
	assert.Equal(t, "ch1-entry", string(results[0].Message))

	// Keep pushing to stream 2 so that it does not stall. But it can't emit anything because watermark is stuck at t1.
	time.Sleep(stallTimeout / 2)
	ch2 <- testEntry(api.LogStreamStdout, t3, "ch2-second")

	// 1/2 + 2/3 = 7/6 > 1, so stream 1 should be considered stalled now.
	time.Sleep(stallTimeout / 3 * 2)
	ch2 <- testEntry(api.LogStreamStdout, t4, "ch2-third")

	results = collectEntries(t, output, 1)
	require.ErrorIs(t, results[0].Err, api.ErrLogStreamStalled)
	assert.Equal(t, e1.Metadata, results[0].Metadata, "stalled entry should have stream metadata")

	// Now that stream 1 is marked as stalled and ignored, all in-flight entries from stream 2 are emitted.
	results = collectEntries(t, output, 3)
	require.Len(t, results, 3)
	assert.Equal(t, "ch2-first", string(results[0].Message))
	assert.Equal(t, "ch2-second", string(results[1].Message))
	assert.Equal(t, "ch2-third", string(results[2].Message))

	close(ch1)
	close(ch2)

	results = collectEntries(t, output, 0)
	assert.Len(t, results, 0)
}
