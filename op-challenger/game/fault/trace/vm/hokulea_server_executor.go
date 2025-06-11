package vm

import (
	"errors"
	"fmt"
	"strconv"

	"github.com/ethereum-optimism/optimism/op-challenger/game/fault/trace/utils"
	"github.com/ethereum-optimism/optimism/op-node/chaincfg"
)

type HokuleaExecutor struct {
	nativeMode bool
}

var _ OracleServerExecutor = (*HokuleaExecutor)(nil)

func NewHokuleaExecutor() *HokuleaExecutor {
	return &HokuleaExecutor{nativeMode: false}
}

func NewNativeHokuleaExecutor() *HokuleaExecutor {
	return &HokuleaExecutor{nativeMode: true}
}

func (s *HokuleaExecutor) OracleCommand(cfg Config, dataDir string, inputs utils.LocalGameInputs) ([]string, error) {
	if len(cfg.L2s) != 1 || len(cfg.RollupConfigPaths) > 1 || len(cfg.Networks) > 1 {
		return nil, errors.New("multiple L2s specified but only one supported")
	}

	addr := "http://127.0.0.1:3100"

	args := []string{
		cfg.Server,
		//"single", removed because unlike kona, hokulea only support a single chain
		"--l1-node-address", cfg.L1,
		"--l1-beacon-address", cfg.L1Beacon,
		"--l2-node-address", cfg.L2s[0],
		"--l1-head", inputs.L1Head.Hex(),
		"--l2-head", inputs.L2Head.Hex(),
		"--l2-output-root", inputs.L2OutputRoot.Hex(),
		"--l2-claim", inputs.L2Claim.Hex(),
		"--l2-block-number", inputs.L2SequenceNumber.Text(10),
		"--eigenda-proxy-address", addr,
	}

	if s.nativeMode {
		args = append(args, "--native")
	} else {
		args = append(args, "--server")
		args = append(args, "--data-dir", dataDir)
	}

	if len(cfg.RollupConfigPaths) > 0 {
		args = append(args, "--rollup-config-path", cfg.RollupConfigPaths[0])
	} else {
		if len(cfg.Networks) == 0 {
			return nil, errors.New("network is not defined")
		}

		chainCfg := chaincfg.ChainByName(cfg.Networks[0])
		args = append(args, "--l2-chain-id", strconv.FormatUint(chainCfg.ChainID, 10))
	}

	fmt.Println("args", args)

	return args, nil
}
