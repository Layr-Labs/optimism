package helpers

import (
	"fmt"

	"github.com/Layr-Labs/eigenda-proxy/clients/memconfig_client"
	actionsHelpers "github.com/ethereum-optimism/optimism/op-e2e/actions/helpers"
	"github.com/stretchr/testify/require"
)

func InstructProxyWithGETStatusCodeError(
	t actionsHelpers.Testing,
	statusCode int,
) error {
	proxyAddr := "http://127.0.0.1:3100"

	client := memconfig_client.New(&memconfig_client.Config{URL: proxyAddr})

	memConfig, err := client.GetConfig(t.Ctx())
	if err != nil {
		return fmt.Errorf("GetConfig: %w", err)
	}
	memConfig.InstructedMode = memconfig_client.InstructedMode{
		GetReturnsStatusCode: statusCode,
		IsActivated:          true,
	}

	newMemConfig, err := client.UpdateConfig(t.Ctx(), memConfig)
	require.NoError(t, err)

	require.Equal(t, newMemConfig.InstructedMode.GetReturnsStatusCode, statusCode)
	return nil
}

func ResetProxyWithGETStatusCodeError(t actionsHelpers.Testing) error {
	return InstructProxyWithGETStatusCodeError(t, 1)
}
