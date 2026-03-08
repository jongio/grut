package github

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// ---------------------------------------------------------------------------
// parseWorkflowInputs tests
// ---------------------------------------------------------------------------

func TestParseWorkflowInputs_FullWorkflow(t *testing.T) {
	content := `
name: Deploy
on:
  workflow_dispatch:
    inputs:
      environment:
        description: 'Target environment'
        required: true
        default: 'production'
        type: choice
        options:
          - production
          - staging
          - development
      debug:
        description: 'Enable debug mode'
        required: false
        default: 'false'
        type: boolean
      version:
        description: 'Version to deploy'
        type: string
jobs:
  deploy:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
`
	inputs, err := parseWorkflowInputs([]byte(content))
	require.NoError(t, err)
	require.Len(t, inputs, 3)

	assert.Equal(t, "environment", inputs[0].Name)
	assert.Equal(t, "Target environment", inputs[0].Description)
	assert.True(t, inputs[0].Required)
	assert.Equal(t, "production", inputs[0].Default)
	assert.Equal(t, "choice", inputs[0].Type)
	assert.Equal(t, []string{"production", "staging", "development"}, inputs[0].Options)

	assert.Equal(t, "debug", inputs[1].Name)
	assert.Equal(t, "Enable debug mode", inputs[1].Description)
	assert.False(t, inputs[1].Required)
	assert.Equal(t, "false", inputs[1].Default)
	assert.Equal(t, "boolean", inputs[1].Type)
	assert.Nil(t, inputs[1].Options)

	assert.Equal(t, "version", inputs[2].Name)
	assert.Equal(t, "Version to deploy", inputs[2].Description)
	assert.False(t, inputs[2].Required)
	assert.Equal(t, "", inputs[2].Default)
	assert.Equal(t, "string", inputs[2].Type)
}

func TestParseWorkflowInputs_NoWorkflowDispatch(t *testing.T) {
	content := `
name: CI
on:
  push:
    branches: [main]
  pull_request:
    branches: [main]
jobs:
  test:
    runs-on: ubuntu-latest
`
	inputs, err := parseWorkflowInputs([]byte(content))
	require.NoError(t, err)
	assert.Nil(t, inputs)
}

func TestParseWorkflowInputs_WorkflowDispatchNoInputs(t *testing.T) {
	content := `
name: Manual
on:
  workflow_dispatch:
jobs:
  run:
    runs-on: ubuntu-latest
`
	inputs, err := parseWorkflowInputs([]byte(content))
	require.NoError(t, err)
	assert.Nil(t, inputs)
}

func TestParseWorkflowInputs_OnAsString(t *testing.T) {
	content := `
name: Simple
on: push
jobs:
  build:
    runs-on: ubuntu-latest
`
	inputs, err := parseWorkflowInputs([]byte(content))
	require.NoError(t, err)
	assert.Nil(t, inputs)
}

func TestParseWorkflowInputs_OnAsArray(t *testing.T) {
	content := `
name: Multi
on: [push, pull_request]
jobs:
  build:
    runs-on: ubuntu-latest
`
	inputs, err := parseWorkflowInputs([]byte(content))
	require.NoError(t, err)
	assert.Nil(t, inputs)
}

func TestParseWorkflowInputs_EmptyInputsMapping(t *testing.T) {
	content := `
name: Deploy
on:
  workflow_dispatch:
    inputs:
jobs:
  deploy:
    runs-on: ubuntu-latest
`
	inputs, err := parseWorkflowInputs([]byte(content))
	require.NoError(t, err)
	assert.Nil(t, inputs)
}

func TestParseWorkflowInputs_InvalidYAML(t *testing.T) {
	_, err := parseWorkflowInputs([]byte(`not: valid: yaml: [`))
	assert.Error(t, err)
}

func TestParseWorkflowInputs_EmptyContent(t *testing.T) {
	inputs, err := parseWorkflowInputs([]byte(""))
	require.NoError(t, err)
	assert.Nil(t, inputs)
}

func TestParseWorkflowInputs_WorkflowDispatchEmptyMapping(t *testing.T) {
	content := `
name: Manual
on:
  workflow_dispatch: {}
jobs:
  run:
    runs-on: ubuntu-latest
`
	inputs, err := parseWorkflowInputs([]byte(content))
	require.NoError(t, err)
	assert.Nil(t, inputs)
}

func TestParseWorkflowInputs_InputWithOnlyDescription(t *testing.T) {
	content := `
name: Simple
on:
  workflow_dispatch:
    inputs:
      name:
        description: 'Your name'
`
	inputs, err := parseWorkflowInputs([]byte(content))
	require.NoError(t, err)
	require.Len(t, inputs, 1)
	assert.Equal(t, "name", inputs[0].Name)
	assert.Equal(t, "Your name", inputs[0].Description)
	assert.False(t, inputs[0].Required)
	assert.Equal(t, "", inputs[0].Default)
	assert.Equal(t, "", inputs[0].Type)
}

// ---------------------------------------------------------------------------
// findMappingValue tests
// ---------------------------------------------------------------------------

func TestFindMappingValue(t *testing.T) {
	content := `
alpha: 1
beta: 2
gamma: 3
`
	var doc yaml.Node
	require.NoError(t, yaml.Unmarshal([]byte(content), &doc))
	root := doc.Content[0]

	val := findMappingValue(root, "beta")
	require.NotNil(t, val)
	assert.Equal(t, "2", val.Value)

	val = findMappingValue(root, "missing", "gamma")
	require.NotNil(t, val)
	assert.Equal(t, "3", val.Value)

	val = findMappingValue(root, "delta")
	assert.Nil(t, val)
}

// ---------------------------------------------------------------------------
// GetWorkflowInputs tests (integration with mock HTTP server)
// ---------------------------------------------------------------------------

func TestGetWorkflowInputs_Success(t *testing.T) {
	workflowYAML := `name: CI
on:
  workflow_dispatch:
    inputs:
      env:
        description: Environment
        default: staging
        type: choice
        options:
          - staging
          - prod
`
	client, _ := setupMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/repos/owner/repo/contents/.github/workflows/ci.yml" {
			respondJSON(w, http.StatusOK, map[string]any{
				"type":     "file",
				"encoding": "base64",
				"content":  b64(workflowYAML),
			})
			return
		}
		http.NotFound(w, r)
	})

	inputs, err := client.GetWorkflowInputs(context.Background(), "owner", "repo", ".github/workflows/ci.yml", "main")
	require.NoError(t, err)
	require.Len(t, inputs, 1)

	assert.Equal(t, "env", inputs[0].Name)
	assert.Equal(t, "Environment", inputs[0].Description)
	assert.Equal(t, "staging", inputs[0].Default)
	assert.Equal(t, "choice", inputs[0].Type)
	assert.Equal(t, []string{"staging", "prod"}, inputs[0].Options)
}

func TestGetWorkflowInputs_NoDispatchTrigger(t *testing.T) {
	workflowYAML := `name: CI
on: push
jobs:
  test:
    runs-on: ubuntu-latest
`
	client, _ := setupMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/repos/owner/repo/contents/.github/workflows/ci.yml" {
			respondJSON(w, http.StatusOK, map[string]any{
				"type":     "file",
				"encoding": "base64",
				"content":  b64(workflowYAML),
			})
			return
		}
		http.NotFound(w, r)
	})

	inputs, err := client.GetWorkflowInputs(context.Background(), "owner", "repo", ".github/workflows/ci.yml", "main")
	require.NoError(t, err)
	assert.Nil(t, inputs)
}

func TestGetWorkflowInputs_APIError(t *testing.T) {
	client, _ := setupMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusNotFound, map[string]any{
			"message": "Not Found",
		})
	})

	_, err := client.GetWorkflowInputs(context.Background(), "owner", "repo", ".github/workflows/ci.yml", "main")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "get workflow file")
}

func TestGetWorkflowInputs_InvalidYAML(t *testing.T) {
	client, _ := setupMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/repos/owner/repo/contents/.github/workflows/ci.yml" {
			respondJSON(w, http.StatusOK, map[string]any{
				"type":     "file",
				"encoding": "base64",
				"content":  b64("not: valid: yaml: ["),
			})
			return
		}
		http.NotFound(w, r)
	})

	// Invalid YAML is handled gracefully: returns nil, nil.
	inputs, err := client.GetWorkflowInputs(context.Background(), "owner", "repo", ".github/workflows/ci.yml", "main")
	require.NoError(t, err)
	assert.Nil(t, inputs)
}

// ---------------------------------------------------------------------------
// DispatchWorkflow tests
// ---------------------------------------------------------------------------

func TestDispatchWorkflow_Success(t *testing.T) {
	client, _ := setupMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/repos/owner/repo/actions/workflows/123/dispatches" {
			var body map[string]any
			require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
			assert.Equal(t, "main", body["ref"])
			inputs, ok := body["inputs"].(map[string]any)
			require.True(t, ok)
			assert.Equal(t, "prod", inputs["env"])

			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	})

	err := client.DispatchWorkflow(context.Background(), "owner", "repo", 123, "main", map[string]any{
		"env": "prod",
	})
	require.NoError(t, err)
}

func TestDispatchWorkflow_Forbidden(t *testing.T) {
	client, _ := setupMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusForbidden, map[string]any{
			"message": "Resource not accessible by integration",
		})
	})

	err := client.DispatchWorkflow(context.Background(), "owner", "repo", 123, "main", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "forbidden")
	assert.Contains(t, err.Error(), "workflow")
}

func TestDispatchWorkflow_NilInputs(t *testing.T) {
	client, _ := setupMockClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/repos/owner/repo/actions/workflows/456/dispatches" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	})

	err := client.DispatchWorkflow(context.Background(), "owner", "repo", 456, "develop", nil)
	require.NoError(t, err)
}

// b64 encodes a string to standard base64 for mock GitHub API responses.
func b64(s string) string {
	return base64.StdEncoding.EncodeToString([]byte(s))
}
