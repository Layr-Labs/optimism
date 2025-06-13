package helpers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	actionsHelpers "github.com/ethereum-optimism/optimism/op-e2e/actions/helpers"
	"github.com/stretchr/testify/require"
)

// The following type matches the eigenda memstore API in eigenda-proxy
type proxyStatusCodePayload struct {
	GetReturnsStatusCode int
}

func InstructProxyWithGETStatusCodeError(
	t actionsHelpers.Testing,
	statusCode int,
) error {
	proxyAddr := "http://127.0.0.1:3100"
	getTimeout := 5 * time.Second

	var setPayload = proxyStatusCodePayload{
		GetReturnsStatusCode: statusCode,
	}

	body, err := json.Marshal(setPayload)
	require.NoError(t, err)

	req, err := http.NewRequestWithContext(t.Ctx(), http.MethodPatch, fmt.Sprintf("%s/memstore/config", proxyAddr), bytes.NewReader(body))
	// a localhost request should not return error
	require.NoError(t, err)

	client := &http.Client{Timeout: getTimeout}
	resp, err := client.Do(req)
	require.NoError(t, err)

	require.NotEqual(t, resp.StatusCode, http.StatusNotFound)

	// Check it is configured correctly
	req, err = http.NewRequestWithContext(t.Ctx(), http.MethodGet, fmt.Sprintf("%s/memstore/config/status-code", proxyAddr), nil)
	require.NoError(t, err)
	client = &http.Client{Timeout: getTimeout}
	resp, err = client.Do(req)
	require.NoError(t, err)

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		panic(fmt.Errorf("read response: %w", err))
	}

	fmt.Println("memstore config body", string(respBody))

	var statusCodePayload proxyStatusCodePayload
	json.Unmarshal(respBody, &statusCodePayload)

	fmt.Println("memstore current status code ", statusCodePayload)

	require.Equal(t, statusCodePayload.GetReturnsStatusCode, statusCode)
	return nil
}

func ResetProxyWithGETStatusCodeError(t actionsHelpers.Testing) error {
	return InstructProxyWithGETStatusCodeError(t, 1)
}
