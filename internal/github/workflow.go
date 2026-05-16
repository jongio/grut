package github

import (
	"context"
	"fmt"
	"log/slog"

	gh "github.com/google/go-github/v68/github"
	"gopkg.in/yaml.v3"
)

// WorkflowInput describes a single input parameter defined in a
// workflow_dispatch trigger.
type WorkflowInput struct {
	Name        string
	Description string
	Default     string
	Type        string   // "string", "choice", "boolean", "environment"
	Options     []string // for "choice" type
	Required    bool
}

// GetWorkflowInputs fetches the workflow YAML file from the repository and
// parses the workflow_dispatch inputs. Returns nil (no error) when the
// workflow has no workflow_dispatch trigger or no inputs.
func (c *clientImpl) GetWorkflowInputs(ctx context.Context, owner, repo, path, ref string) ([]WorkflowInput, error) {
	opts := &gh.RepositoryContentGetOptions{Ref: ref}
	fileContent, _, _, err := c.gh.Repositories.GetContents(ctx, owner, repo, path, opts)
	if err != nil {
		return nil, fmt.Errorf("get workflow file %s: %w", path, err)
	}
	if fileContent == nil {
		return nil, fmt.Errorf("workflow file %s: expected file, got directory", path)
	}
	content, err := fileContent.GetContent()
	if err != nil {
		return nil, fmt.Errorf("decode workflow file %s: %w", path, err)
	}
	inputs, err := parseWorkflowInputs([]byte(content))
	if err != nil {
		slog.Warn("parse workflow inputs", "path", path, "err", err)
		return nil, nil // graceful fallback — show generic dialog
	}
	return inputs, nil
}

// fieldType is the YAML key for an input's type property.
const fieldType = "type"

// parseWorkflowInputs extracts workflow_dispatch input definitions from
// a GitHub Actions workflow YAML file. It uses yaml.Node traversal to
// handle all variants of the "on" key (string, array, mapping) and the
// YAML 1.1 interpretation of "on" as boolean true.
func parseWorkflowInputs(yamlContent []byte) ([]WorkflowInput, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(yamlContent, &doc); err != nil {
		return nil, fmt.Errorf("parse YAML: %w", err)
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 {
		return nil, nil
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return nil, nil
	}
	// Find the "on" key. YAML 1.1 parsers may interpret bare "on" as
	// boolean true, so we check for both string values.
	onNode := findMappingValue(root, "on", "true")
	if onNode == nil || onNode.Kind != yaml.MappingNode {
		return nil, nil // "on" absent or not a mapping — no dispatch inputs
	}
	// Find "workflow_dispatch" within "on".
	dispatchNode := findMappingValue(onNode, "workflow_dispatch")
	if dispatchNode == nil {
		return nil, nil
	}
	// workflow_dispatch can be null/empty (just the trigger with no config).
	if dispatchNode.Kind != yaml.MappingNode {
		return nil, nil
	}
	// Find "inputs" within "workflow_dispatch".
	inputsNode := findMappingValue(dispatchNode, "inputs")
	if inputsNode == nil || inputsNode.Kind != yaml.MappingNode {
		return nil, nil
	}
	// Parse each input definition.
	var result []WorkflowInput
	for i := 0; i < len(inputsNode.Content)-1; i += 2 {
		inputName := inputsNode.Content[i].Value
		inputDef := inputsNode.Content[i+1]
		input := WorkflowInput{Name: inputName}
		if inputDef.Kind == yaml.MappingNode {
			for j := 0; j < len(inputDef.Content)-1; j += 2 {
				key := inputDef.Content[j].Value
				val := inputDef.Content[j+1]
				switch key {
				case "description":
					input.Description = val.Value
				case "required":
					input.Required = val.Value == "true"
				case "default":
					input.Default = val.Value
				case fieldType:
					input.Type = val.Value
				case "options":
					if val.Kind == yaml.SequenceNode {
						for _, opt := range val.Content {
							input.Options = append(input.Options, opt.Value)
						}
					}
				}
			}
		}
		result = append(result, input)
	}
	return result, nil
}

// findMappingValue locates a value in a YAML mapping node by key name.
// Accepts multiple candidate key names to handle YAML 1.1 quirks
// (e.g. "on" vs "true").
func findMappingValue(mapping *yaml.Node, keys ...string) *yaml.Node {
	if mapping.Kind != yaml.MappingNode {
		return nil
	}
	keySet := make(map[string]struct{}, len(keys))
	for _, k := range keys {
		keySet[k] = struct{}{}
	}
	for i := 0; i < len(mapping.Content)-1; i += 2 {
		if _, ok := keySet[mapping.Content[i].Value]; ok {
			return mapping.Content[i+1]
		}
	}
	return nil
}
