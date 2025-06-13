package proofs

import (
	"fmt"
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

func Test_ProgramAction_EigenDADropCert(gt *testing.T) {
	type testCase struct {
		name  string
		certs []int // blocks is an ordered list of eigenda da cert each occupying a channel, int specifying the statusCode, 1 is success
		holoceneExpectations
	}

	// Depending on the blocks list,  we expect a different
	// progression of the safe head under Holocene
	// derivation rules, compared with pre Holocene.
	testCases := []testCase{
		// Standard channel composition
		{
			name: "recency-list", certs: []int{1, 1, 1},
			holoceneExpectations: holoceneExpectations{
				preHolocene: expectations{safeHead: 3},
				holocene:    expectations{safeHead: 3},
			},
		},
	}

	runEigenDADerivationTest := func(gt *testing.T, testCfg *helpers.TestCfg[testCase]) {
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
			c.ForceSubmitSingularBatch = true
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

		// finish setup, begin experiment

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

		// run derivation, and by this time, the system is alright
		env.Sequencer.ActL1HeadSignal(t)
		env.Sequencer.ActL2PipelineFull(t)

		l1Head := env.Miner.L1Chain().CurrentBlock()
		l2SafeHead := env.Engine.L2Chain().CurrentSafeBlock()

		fmt.Println("initial L1 block without error", "l1Head", l1Head.Number, "l2SafeHead", l2SafeHead.Number)

		//
		// Start another L2 block but this time instruct validity error 2
		//
		helpers.InstructProxyWithGETStatusCodeError(t, 2)
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

		env.Batcher.ActSubmitAll(t)

		env.Miner.ActL1StartBlock(12)(t)
		env.Miner.ActL1IncludeTxByHash(env.Batcher.LastSubmitted.Hash())(t)
		env.Miner.ActL1EndBlock(t)

		env.Miner.ActL1SafeNext(t)
		env.Miner.ActL1FinalizeNext(t)

		// run derivation, and by this time, the system is already skewed
		env.Sequencer.ActL1HeadSignal(t)
		env.Sequencer.ActL2PipelineFull(t)

		l1Head = env.Miner.L1Chain().CurrentBlock()
		l2SafeHead = env.Engine.L2Chain().CurrentSafeBlock()
		fmt.Println("second L1 block with discard cert error", "l1Head", l1Head.Number, "l2SafeHead", l2SafeHead.Number)

		// make sure channel timeout, so that it can continue to build block
		for i := 0; i < int(testParams.ChannelTimeout); i++ {
			env.Miner.ActL1StartBlock(12)(t)
			env.Miner.ActL1EndBlock(t)

			env.Miner.ActL1SafeNext(t)
			env.Miner.ActL1FinalizeNext(t)

			env.Sequencer.ActL1HeadSignal(t)
			env.Sequencer.ActL2PipelineFull(t)

			l1Head = env.Miner.L1Chain().CurrentBlock()
			l2SafeHead = env.Engine.L2Chain().CurrentSafeBlock()
			fmt.Println("in ChannelTimeout", "l1Head", l1Head.Number, "l2SafeHead", l2SafeHead.Number, "i", i)
		}

		l1Head = env.Miner.L1Chain().CurrentBlock()
		l2SafeHead = env.Engine.L2Chain().CurrentSafeBlock()
		fmt.Println("after ChannelTimeout", "l1Head", l1Head.Number, "l2SafeHead", l2SafeHead.Number)

		//
		// Finally create last block
		// this time, either an honest batcher has replaced the malicious batcher
		// or the malfunction batcher is fixed
		helpers.ResetProxyWithGETStatusCodeError(t)

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

		// flush the state
		env.Sequencer.ActL1HeadSignal(t)
		env.Sequencer.ActL2PipelineFull(t)

		l1Head = env.Miner.L1Chain().CurrentBlock()
		l2SafeHead = env.Engine.L2Chain().CurrentSafeBlock()
		fmt.Println("final L1 block with correct", "l1Head", l1Head.Number, "l2SafeHead", l2SafeHead.Number)

		// Ensure there is only 1 block on L1.
		require.Equal(t, uint64(testParams.ChannelTimeout+3), l1Head.Number.Uint64())
		// Ensure the block is marked as safe before we attempt to fault prove it.
		// the number of L2 block is 6 because once the channel is open, because the first 6 blocks is inluded without issue
		// the next 6 blocks have issue with its eigenda blob. The channel is still open
		// the last 6 blocks is inconsistent
		require.Equal(t, uint64(47), l2SafeHead.Number.Uint64())

		env.RunFaultProofProgram(t, l2SafeHead.Number.Uint64(), testCfg.CheckResult, testCfg.InputParams...)
	}

	matrix := helpers.NewMatrix[testCase]()
	defer matrix.Run(gt)

	for _, ordering := range testCases {
		matrix.AddTestCase(
			fmt.Sprintf("HonestClaim-%s", ordering.name),
			ordering,
			helpers.NewForkMatrix(helpers.LatestFork),
			runEigenDADerivationTest,
			helpers.ExpectNoError(),
		)
		/*
			matrix.AddTestCase(
				fmt.Sprintf("JunkClaim-%s", ordering.name),
				ordering,
				helpers.NewForkMatrix(helpers.LatestFork),
				runEigenDADerivationTest,
				helpers.ExpectError(claim.ErrClaimNotValid),
				helpers.WithL2Claim(common.HexToHash("0xdeadbeef")),
			)
		*/
	}
}
