package eigenda

import (
	"context"
	"testing"
	"time"

	"github.com/kurtosis-tech/kurtosis/api/golang/engine/lib/kurtosis_context"
)

// These tests are log driven. The batcher doesn't expose an API to query its state outside of logs and metrics,
// so hard to do much better. We rely on some info logs appearing and some warning/error logs not appearing.
// These tests are not very sophisticated, but are at least a good sanity check...
// FIXME: one issue is that if op changes the log lines then our tests here might just silently pass and we won't know...
// A better approach might be to generate txs from inside the golang test instead of relying on the external tx-fuzzer.
// We could then increase traffic until the point where DA gets throttled, then change batcher parameters to increase blob size, etc.
// Updating the batcher params is currently hard to do however; see comments above the eigenda-devnet-restart-batcher command in the justfile.
func TestBatcherFromLogs(t *testing.T) {
	// We stream logs for 2 minute, and run all the below tests in parallel (they read the same log outputs)
	ctxWithTestTimeout, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	harness := NewHarness(t)

	// Make sure that no channel is ever timing out (fails to be sent to L1 in timely manner).
	// Make sure the testsTimer is longer than max-channel-duration in the batcher config (found in the eigenda-template-values/ files).
	// Currently max-channel-duration is set to 10 L1 blocks, meaning 10*6 seconds = 60 seconds.
	t.Run("No channel timeout", func(t *testing.T) {
		t.Parallel()
		// Log output is from https://github.com/Layr-Labs/optimism/blob/a5709b435f39cab0d7f5dc879b65e07e2f90a548/op-batcher/batcher/channel.go#L102
		filter := kurtosis_context.NewDoesContainMatchRegexLogLineFilter("channel timed out")
		c := harness.QueryBatcherLogs(ctxWithTestTimeout, true, filter)

		for {
			select {
			case <-ctxWithTestTimeout.Done():
				return
			case <-c:
				t.Logf("channel timed out on batcher... something went wrong.")
				t.Fail()
			}
		}
	})

	t.Run("No DA Throttling", func(t *testing.T) {
		t.Parallel()
		// Log output is from https://github.com/Layr-Labs/optimism/blob/a5709b435f39cab0d7f5dc879b65e07e2f90a548/op-batcher/batcher/driver.go#L540
		filter := kurtosis_context.NewDoesContainMatchRegexLogLineFilter("throttling DA")
		c := harness.QueryBatcherLogs(ctxWithTestTimeout, true, filter)

		for {
			select {
			case <-ctxWithTestTimeout.Done():
				return
			case <-c:
				t.Logf("da got throttled... something went wrong.")
				t.Fail()
			}
		}
	})

	t.Run("Transactions are confirming", func(t *testing.T) {
		t.Parallel()
		// Log line from https://github.com/Layr-Labs/optimism/blob/a5709b435f39cab0d7f5dc879b65e07e2f90a548/op-batcher/batcher/driver.go#L921
		filter := kurtosis_context.NewDoesContainMatchRegexLogLineFilter("Transaction confirmed")
		c := harness.QueryBatcherLogs(ctxWithTestTimeout, true, filter)

		confirmedTxsCount := 0
		for {
			select {
			case <-ctxWithTestTimeout.Done():
				if confirmedTxsCount == 0 {
					t.Logf("no transactions confirmed... something went wrong.")
					t.FailNow()
				}
				t.Logf("%d transactions confirmed", confirmedTxsCount)
				return
			case <-c:
				confirmedTxsCount++
				t.Logf("transaction confirmed at %v", time.Now())
			}
		}
	})
}
