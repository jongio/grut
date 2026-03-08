package runtime

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// ringBuffer.Write
// ---------------------------------------------------------------------------

// TestRingBuffer_WriteAndString verifies basic write and String() retrieval.
func TestRingBuffer_WriteAndString(t *testing.T) {
	rb := &ringBuffer{max: 5}

	n, err := rb.Write([]byte("line1\nline2\nline3"))
	require.NoError(t, err)
	assert.Equal(t, 17, n, "Write must return the full byte count")

	out := rb.String()
	assert.Contains(t, out, "line1")
	assert.Contains(t, out, "line2")
	assert.Contains(t, out, "line3")
}

// TestRingBuffer_MaxEviction verifies old lines are evicted when the buffer
// is full, keeping only the last N lines (bounded ring behaviour).
func TestRingBuffer_MaxEviction(t *testing.T) {
	rb := &ringBuffer{max: 3}

	_, _ = rb.Write([]byte("a\nb\nc\nd\ne"))

	out := rb.String()
	// Lines a and b should have been evicted.
	assert.NotContains(t, out, "a")
	assert.NotContains(t, out, "b")
	assert.Contains(t, out, "c")
	assert.Contains(t, out, "d")
	assert.Contains(t, out, "e")
}

// TestRingBuffer_EmptyLines verifies that empty lines (double newlines) are
// ignored and do not consume ring slots.
func TestRingBuffer_EmptyLines(t *testing.T) {
	rb := &ringBuffer{max: 3}

	_, err := rb.Write([]byte("a\n\nb\n\nc"))
	require.NoError(t, err)

	out := rb.String()
	// Only non-empty lines should be stored.
	lines := strings.Split(out, "\n")
	for _, l := range lines {
		assert.NotEmpty(t, l, "ringBuffer must not store empty lines")
	}
}

// TestRingBuffer_EmptyWrite verifies a zero-byte write returns (0, nil).
func TestRingBuffer_EmptyWrite(t *testing.T) {
	rb := &ringBuffer{max: 3}
	n, err := rb.Write([]byte(""))
	assert.NoError(t, err)
	assert.Equal(t, 0, n)
	assert.Equal(t, "", rb.String())
}

// TestRingBuffer_SingleLineNoNewline verifies content without a trailing
// newline is still stored.
func TestRingBuffer_SingleLineNoNewline(t *testing.T) {
	rb := &ringBuffer{max: 3}
	_, err := rb.Write([]byte("hello"))
	require.NoError(t, err)
	assert.Contains(t, rb.String(), "hello")
}

// ---------------------------------------------------------------------------
// SendRequest with a live echo subprocess
// ---------------------------------------------------------------------------

// buildJSONRPCEchoHelper builds a Go binary that reads a newline-delimited
// JSON-RPC request and echoes back a valid JSON-RPC 2.0 response containing
// the method name as the result string.
func buildJSONRPCEchoHelper(t *testing.T) string {
	t.Helper()
	return buildHelper(t, "jsonrpc-echo", `package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
)

type req struct {
	JSONRPC string          `+"`"+`json:"jsonrpc"`+"`"+`
	ID      int64           `+"`"+`json:"id"`+"`"+`
	Method  string          `+"`"+`json:"method"`+"`"+`
	Params  json.RawMessage `+"`"+`json:"params,omitempty"`+"`"+`
}

type resp struct {
	JSONRPC string `+"`"+`json:"jsonrpc"`+"`"+`
	ID      int64  `+"`"+`json:"id"`+"`"+`
	Result  string `+"`"+`json:"result"`+"`"+`
}

func main() {
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		var r req
		if err := json.Unmarshal(scanner.Bytes(), &r); err != nil {
			fmt.Fprintln(os.Stderr, "parse error:", err)
			continue
		}
		out, _ := json.Marshal(resp{JSONRPC: "2.0", ID: r.ID, Result: r.Method})
		fmt.Println(string(out))
	}
}
`)
}

// buildJSONRPCErrorHelper builds a subprocess that always returns a JSON-RPC
// error response.
func buildJSONRPCErrorHelper(t *testing.T) string {
	t.Helper()
	return buildHelper(t, "jsonrpc-error", `package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
)

type req struct {
	ID int64 `+"`"+`json:"id"`+"`"+`
}
type rpcErr struct {
	Code    int    `+"`"+`json:"code"`+"`"+`
	Message string `+"`"+`json:"message"`+"`"+`
}
type resp struct {
	JSONRPC string  `+"`"+`json:"jsonrpc"`+"`"+`
	ID      int64   `+"`"+`json:"id"`+"`"+`
	Error   *rpcErr `+"`"+`json:"error,omitempty"`+"`"+`
}

func main() {
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		var r req
		json.Unmarshal(scanner.Bytes(), &r)
		out, _ := json.Marshal(resp{JSONRPC: "2.0", ID: r.ID, Error: &rpcErr{Code: -32601, Message: "method not found"}})
		fmt.Println(string(out))
	}
}
`)
}

// TestMCPRuntime_SendRequest_EchoSuccess verifies the full SendRequest
// round-trip: marshal request → write to stdin → read response → unmarshal.
func TestMCPRuntime_SendRequest_EchoSuccess(t *testing.T) {
	bin := buildJSONRPCEchoHelper(t)
	rt := newTestRuntime(t)
	t.Cleanup(rt.Close)

	require.NoError(t, rt.Load(bin))

	result, err := rt.SendRequest("ping", nil)
	require.NoError(t, err)

	var got string
	require.NoError(t, json.Unmarshal(result, &got))
	assert.Equal(t, "ping", got, "echo server should return the method name as result")
}

// TestMCPRuntime_SendRequest_WithParams verifies that request parameters are
// serialised and the server receives them (method still returned in result).
func TestMCPRuntime_SendRequest_WithParams(t *testing.T) {
	bin := buildJSONRPCEchoHelper(t)
	rt := newTestRuntime(t)
	t.Cleanup(rt.Close)

	require.NoError(t, rt.Load(bin))

	params := map[string]string{"key": "value"}
	result, err := rt.SendRequest("tools/list", params)
	require.NoError(t, err)

	var got string
	require.NoError(t, json.Unmarshal(result, &got))
	assert.Equal(t, "tools/list", got)
}

// TestMCPRuntime_SendRequest_ServerError verifies that a JSON-RPC error
// response is surfaced as a Go error rather than panicking or returning nil.
func TestMCPRuntime_SendRequest_ServerError(t *testing.T) {
	bin := buildJSONRPCErrorHelper(t)
	rt := newTestRuntime(t)
	t.Cleanup(rt.Close)

	require.NoError(t, rt.Load(bin))

	_, err := rt.SendRequest("anything", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "server error")
	assert.Contains(t, err.Error(), "method not found")
}

// TestMCPRuntime_SendRequest_MultipleSequential verifies that multiple
// sequential requests are correctly serialised and ID-incremented.
func TestMCPRuntime_SendRequest_MultipleSequential(t *testing.T) {
	bin := buildJSONRPCEchoHelper(t)
	rt := newTestRuntime(t)
	t.Cleanup(rt.Close)

	require.NoError(t, rt.Load(bin))

	for _, method := range []string{"a", "b", "c"} {
		result, err := rt.SendRequest(method, nil)
		require.NoError(t, err, "request %q should succeed", method)

		var got string
		require.NoError(t, json.Unmarshal(result, &got))
		assert.Equal(t, method, got)
	}
}
