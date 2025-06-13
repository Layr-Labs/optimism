package proofs_test

import (
	"testing"

	altda "github.com/ethereum-optimism/optimism/op-alt-da"
	"github.com/ethereum-optimism/optimism/op-batcher/flags"
	"github.com/ethereum-optimism/optimism/op-chain-ops/genesis"
	actionsHelpers "github.com/ethereum-optimism/optimism/op-e2e/actions/helpers"
	"github.com/ethereum-optimism/optimism/op-e2e/actions/proofs/helpers"
	"github.com/ethereum-optimism/optimism/op-e2e/config"
	"github.com/ethereum-optimism/optimism/op-e2e/e2eutils"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/stretchr/testify/require"
)

func runFailoverWithEigenDATest(gt *testing.T, testCfg *helpers.TestCfg[any]) {
	t := actionsHelpers.NewDefaultTesting(gt)
	testSetup := func(dc *genesis.DeployConfig) {
		dc.L1PragueTimeOffset = ptr(hexutil.Uint64(0))
		// Set non-trivial excess blob gas so that the L1 miner's blob logic is
		// properly tested.
		dc.L1GenesisBlockExcessBlobGas = ptr(hexutil.Uint64(1e8))
		// set to use altda
		dc.AltDADeployConfig.UseAltDA = true
		dc.AltDADeployConfig.DACommitmentType = altda.GenericCommitmentString
		dc.AltDADeployConfig.DAResolverRefundPercentage = 0
		dc.AltDADeployConfig.DAResolveWindow = 16
		dc.AltDADeployConfig.DAChallengeWindow = 16
		dc.AltDADeployConfig.DABondSize = 0
	}

	bcfg := helpers.NewBatcherCfg(func(c *actionsHelpers.BatcherCfg) {
		// eigenda only supports calldata
		c.DataAvailabilityType = flags.CalldataType
		c.UseAltDA = true
	})

	// Set altda test params
	testParams := &e2eutils.TestParams{
		MaxSequencerDrift:   40,
		SequencerWindowSize: 12,
		ChannelTimeout:      12,
		L1BlockTime:         12,
		UseAltDA:            true,
		AllocType:           config.AllocTypeAltDAGeneric,
	}

	env := helpers.NewL2FaultProofEnv(t, testCfg, testParams, bcfg, testSetup)

	// Build a block on L2
	env.Sequencer.ActL2StartBlock(t)
	// add tx from alice to bob
	env.Alice.L2.ActResetTxOpts(t)
	env.Alice.L2.ActSetTxToAddr(&env.Dp.Addresses.Bob)
	env.Alice.L2.ActMakeTx(t)
	env.Engine.ActL2IncludeTx(env.Alice.Address())(t)
	env.Sequencer.ActL2EndBlock(t)
	// Make 6 blocks in total
	for i := 0; i < 5; i++ {
		env.Sequencer.ActL2StartBlock(t)
		env.Sequencer.ActL2EndBlock(t)
	}

	// Instruct the batcher to submit the block to L1, and include the transaction.
	env.Batcher.ActSubmitAll(t)
	// even though there is only 1 L2 block, we still add 12 sec for L1 block
	env.Miner.ActL1StartBlock(12)(t)
	env.Miner.ActL1IncludeTxByHash(env.Batcher.LastSubmitted.Hash())(t)
	env.Miner.ActL1EndBlock(t)

	// Finalize the block with the batch on L1.
	env.Miner.ActL1SafeNext(t)
	env.Miner.ActL1FinalizeNext(t)

	//
	// Start another L2 block but this time failover to EthDA
	//
	env.Batcher.ActAltDAFailoverToEthDA(t)
	env.Sequencer.ActL2StartBlock(t)
	// add tx from bob to alice
	env.Alice.L2.ActResetTxOpts(t)
	env.Bob.L2.ActSetTxToAddr(&env.Dp.Addresses.Alice)
	env.Alice.L2.ActMakeTx(t)
	env.Engine.ActL2IncludeTx(env.Alice.Address())(t)
	env.Sequencer.ActL2EndBlock(t)
	// Make 6 blocks in total
	for i := 0; i < 5; i++ {
		env.Sequencer.ActL2StartBlock(t)
		env.Sequencer.ActL2EndBlock(t)
	}

	// Instruct the batcher to submit the block to L1, and include the transaction.
	// We swap out altda batcher for ethda batcher
	env.Batcher.ActSubmitAll(t)
	env.Miner.ActL1StartBlock(12)(t)
	env.Miner.ActL1IncludeTxByHash(env.Batcher.LastSubmitted.Hash())(t)
	env.Miner.ActL1EndBlock(t)

	// Finalize the block with the batch on L1.
	env.Miner.ActL1SafeNext(t)
	env.Miner.ActL1FinalizeNext(t)

	env.Sequencer.ActL1HeadSignal(t)
	env.Sequencer.ActL2PipelineFull(t)

	// flush the state
	l1Head := env.Miner.L1Chain().CurrentBlock()
	l2SafeHead := env.Engine.L2Chain().CurrentSafeBlock()

	// Ensure there is only 1 block on L1.
	require.Equal(t, uint64(2), l1Head.Number.Uint64())
	// Ensure the block is marked as safe before we attempt to fault prove it.
	require.Equal(t, uint64(12), l2SafeHead.Number.Uint64())

	env.RunFaultProofProgram(t, l2SafeHead.Number.Uint64(), testCfg.CheckResult, testCfg.InputParams...)
}

func Test_ProgramAction_FailoverWithEigenDA(gt *testing.T) {
	matrix := helpers.NewMatrix[any]()
	defer matrix.Run(gt)

	matrix.AddDefaultTestCases(
		nil,
		helpers.LatestForkOnly,
		runFailoverWithEigenDATest,
	)
}
