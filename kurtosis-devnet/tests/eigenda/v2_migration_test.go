package eigenda

import (
	"context"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum-optimism/optimism/op-e2e/e2eutils/geth"
	"github.com/stretchr/testify/require"
)

// TestFailover tests the failover behavior of the batcher, in response to the proxy returning 503 errors.
// See https://github.com/Layr-Labs/eigenda-proxy?tab=readme-ov-file#failover-signals for proxy behavior.
// The proxy's memstore's failover behavior is toggled on and off by this test via a REST api.
// We then check that the batcher correctly interprets the 503 signals and starts submitting batches to EthDACalldata instead.
// The test then toggles the failover back off and checks that the batcher starts submitting EigenDA batches again.
// The batches inbox transactions are queried via geth's GraphQL API.
//
// Note: because this test relies on modifying the proxy's memstore config, it should be run in isolation.
// That is, if we ever implement more kurtosis tests, they would currently need to be run sequentially.
func TestEigenDAV2Migration_Memstore(t *testing.T) {
	testTimeout := 4 * time.Minute // each stage is 20*6 seconds = 2 mins
	ctxWithTestTimeout, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	harness := NewHarness(t)

	// Number of blocks to query for batcher txs, for v1 and v2 stages.
	// Need to make sure each stage contains at least 2 commitments of the correct type. This can only happen if channels
	// are being closed in time, which requires either: sending traffic with traffic-generator, or setting a low (e.g. 2 L1 blocks) channel-timeout.
	l1BlocksQueriedForBatcherTxs := uint64(20)

	// 1. Check that the original commitments are EigenDA V1
	t.Logf("[Stage1: EigenDA V1] Checking that the initial commitments are EigenDA V1")
	// explicitly set to v1 since kurtosis can also be started with proxy in v2 dispersal mode
	ctxWithTimeout, cancel := context.WithTimeout(ctxWithTestTimeout, 5*time.Second)
	defer cancel()
	err := harness.Clients.ProxyClients.SetDispersalBackend(ctxWithTimeout, EigenDACertVersionV1)
	require.NoError(t, err)

	stage1FromBlockNum := harness.TestStartL1BlockNum
	stage1ToBlockNum := stage1FromBlockNum + l1BlocksQueriedForBatcherTxs
	_, err = geth.WaitForBlock(big.NewInt(int64(stage1ToBlockNum)), harness.Clients.GethL1Client)
	require.NoError(t, err)

	requireBatcherTxsToBeFromLayer(t, stage1FromBlockNum, stage1ToBlockNum, DALayerEigenDAV1, harness.Endpoints.GethL1Endpoint, harness.BatchInboxAddr)

	// 2. Change dispersal backend to EigenDA V2 and check that the new commitments are EigenDA V2
	t.Logf("[Stage2] Changing proxy's dispersal backend to submit to EigenDA V2")
	ctxWithTimeout, cancel = context.WithTimeout(ctxWithTestTimeout, 5*time.Second)
	defer cancel()
	err = harness.Clients.ProxyClients.SetDispersalBackend(ctxWithTimeout, EigenDACertVersionV2)
	require.NoError(t, err)

	stage2FromBlockNum, err := harness.Clients.GethL1Client.BlockNumber(ctxWithTimeout)
	require.NoError(t, err)
	stage2ToBlockNum := stage2FromBlockNum + l1BlocksQueriedForBatcherTxs
	_, err = geth.WaitForBlock(big.NewInt(int64(stage2ToBlockNum)), harness.Clients.GethL1Client)
	require.NoError(t, err)

	requireBatcherTxsToBeFromLayer(t, stage2FromBlockNum, stage2ToBlockNum, DALayerEigenDAV2, harness.Endpoints.GethL1Endpoint, harness.BatchInboxAddr)
}
