package eigenda

import (
	"context"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum-optimism/optimism/op-e2e/e2eutils/geth"
	"github.com/stretchr/testify/require"
)

func TestEigenDAV2Migration_Memstore(t *testing.T) {
	// 20 is enough in memstore mode since certs finalize instantly and thus land
	// in batcher-inbox very fast.
	testEigenDAV2Migration(t, 20)
}

func TestEigenDAV2Migration_Holesky(t *testing.T) {
	// The issue is that because of the holocene strict ordering rules,
	// after migrating from v1 to v2, the v2 certs, even though they finalize very quickly
	// and are returned to op-batcher, op-batcher cannot send them to the batch-inbox until
	// the v1 certs have been sent. So we need to wait at least 10-15 mins for the v1 certs to be returned
	// from proxy to op-batcher and submitted to batch-inbox.
	// starting kurtosis devnet takes ~10 mins and batcher logs tests take 15 mins,
	// so we are already at 25 mins. Trying to keep CI time under 30 mins,
	// so we skip this test for now. We could turn on if we had a nice inabox eigenda that would allow
	// for much faster v1 finalization.
	t.Skip("skipping test because would make ci take too long. We only run the memstore version in CI.")
	// testEigenDAV2Migration(t, 200)
}

// TestEigenDAV2Migration tests a rollup migration from eigenDA V1 to V2.
// We use the proxy's admin REST API to change the dispersal backend.
// See https://github.com/Layr-Labs/eigenda-proxy?tab=readme-ov-file#on-the-fly-migration for details.
// This test simply checks that the batcher txs have the correct version byte.
// The batches inbox transactions are queried via geth's GraphQL API.
//
// l1BlocksQueriedForBatcherTxs is the number of blocks to query for batcher txs, for v1 and v2 stages.
// Need to make sure each stage contains at least 2 commitments of the correct type. This can only happen if channels
// are being closed in time, which requires either: sending traffic with traffic-generator, or setting a low (e.g. 2 L1 blocks) channel-timeout.
//
// Note: because this test modifies the proxy's state config, it should be run in isolation (sequentially).
func testEigenDAV2Migration(t *testing.T, l1BlocksQueriedForBatcherTxs uint64) {
	l1BlockTime := 6 * time.Second
	eachStageTimeRequired := time.Duration(l1BlocksQueriedForBatcherTxs) * l1BlockTime
	numBatcherStages := 2                          // v1 and v2 stages
	opNodeFinalizationStageTime := 8 * time.Minute // we leave 8 mins for op-node finalization
	testTimeout := time.Duration(numBatcherStages)*eachStageTimeRequired + opNodeFinalizationStageTime
	ctxWithTestTimeout, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	harness := NewHarness(t)

	// explicitly set to v1 since kurtosis can also be started with proxy in v2 dispersal mode
	ctxWithTimeout, cancel := context.WithTimeout(ctxWithTestTimeout, 5*time.Second)
	defer cancel()
	certVersion, err := harness.Clients.ProxyClients.GetDispersalBackend(ctxWithTimeout)
	require.NoError(t, err)
	if certVersion != EigenDACertVersionV1 {
		// V1 certs take a long time to confirm because of batching+bridging,
		// so we only run this test if the proxy is already in V1 mode,
		// meaning it has a pipeline of certs that are waiting to be confirmed.
		t.Skip("skipping test because proxy is not in EigenDA V1 mode")
	}

	// 1. Check that the original commitments are EigenDA V1
	t.Logf("[Stage1: EigenDA V1] Checking that the initial commitments are EigenDA V1")

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

	ctxWithTimeout, cancel = context.WithTimeout(ctxWithTestTimeout, 5*time.Second)
	defer cancel()
	stage2FromBlockNum, err := harness.Clients.GethL1Client.BlockNumber(ctxWithTimeout)
	require.NoError(t, err)
	stage2ToBlockNum := stage2FromBlockNum + l1BlocksQueriedForBatcherTxs
	_, err = geth.WaitForBlock(big.NewInt(int64(stage2ToBlockNum)), harness.Clients.GethL1Client)
	require.NoError(t, err)

	requireBatcherTxsToBeFromLayer(t, stage2FromBlockNum, stage2ToBlockNum, DALayerEigenDAV2, harness.Endpoints.GethL1Endpoint, harness.BatchInboxAddr)

	// We also check that the op-node is still finalizing blocks after the migration to v2
	t.Logf("[Stage3] Check that op-node is still finalizing blocks after v2 migration")
	ctxWithTimeout, cancel = context.WithTimeout(ctxWithTestTimeout, 5*time.Second)
	defer cancel()
	syncStatus, err := harness.Clients.OpNodeClient.SyncStatus(ctxWithTimeout)
	require.NoError(t, err)
	afterFailoverFinalizedL2 := syncStatus.FinalizedL2
	t.Logf("[Finalization] Current finalized L2 block: %d. Waiting for next block to finalize to make sure finalization is still happening.", afterFailoverFinalizedL2.Number)
	// On average would expect this to take half an epoch, aka 16 L1 blocks, which at 6 sec/block means 1.5 minutes.
	// This generally takes longer (3-6 minutes), but I'm not quite sure why.
	_, err = geth.WaitForBlockToBeFinalized(new(big.Int).SetUint64(afterFailoverFinalizedL2.Number+1), harness.Clients.OpGethClient, 6*time.Minute)
	require.NoError(t, err, "op-node should still be finalizing blocks after failover")
}
