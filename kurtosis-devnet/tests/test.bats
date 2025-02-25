#!/usr/bin/env bash

setup() {
  load 'test_helper/bats-support/load'
  load 'test_helper/bats-assert/load'

  # get the containing directory of this file
  # use $BATS_TEST_FILENAME instead of ${BASH_SOURCE[0]} or $0,
  # as those will point to the bats executable's location or the preprocessed file respectively
  TESTS_DIR="$(cd "$(dirname "$BATS_TEST_FILENAME")" >/dev/null 2>&1 && pwd)"
  # add eigenda scripts to the PATH
  PATH="$TESTS_DIR/eigenda:$PATH"
}

# These commands take a few seconds to run each, so we run them once in setup_file
# and export the variables so they're available in all tests. We assume the kurtosis devnet
# is already running.
setup_file() {
  export OPNODE_ENDPOINT=$(kurtosis port print eigenda-memstore-devnet op-cl-1-op-node-op-geth-op-kurtosis http)
  export BATCHER_INBOX=$(cast rpc optimism_rollupConfig --rpc-url $OPNODE_ENDPOINT | jq -r .batch_inbox_address)
  export GETH_L1_ENDPOINT=$(kurtosis port print eigenda-memstore-devnet el-1-geth-teku rpc)
}

@test "failover" {
  L1_BLOCK_CUR=$(cast block-number --rpc-url $GETH_L1_ENDPOINT)
  run echo $L1_BLOCK_CUR
  assert_output "okok"
  run failover
  assert_output --partial 'transactions have the correct prefix'
}

failover() {
  curl -X POST $GETH_L1_ENDPOINT/graphql -H "Content-Type: application/json" \
    --data '{ "query": "query txInfo { blocks(from:30) { transactions { to { address } inputData } } }" }' |
    jq '
      # First extract all transactions to the batcher address
      (.data.blocks | map(.transactions) | flatten | map(select(
        .to != null and
        .to.address != null and
        .to.address == $inbox_addr and
        .inputData != null
      ))) as $batcherTxs |
      # Then check if all of them start with the required prefix
      if ($batcherTxs | all(.inputData | startswith("0x01010000")))
      then "SUCCESS: All \($batcherTxs | length) batcher transactions have the correct prefix"
      else "ERROR: \($batcherTxs | map(select(.inputData | startswith("0x01010000") | not)) | length) of \($batcherTxs | length) batcher transactions do not have the correct prefix"
      end
    ' --arg inbox_addr "$BATCHER_INBOX"
}
