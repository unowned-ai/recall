package mcp

type ToolDefinition struct {
	Name        string                 `json:"name" yaml:"name"`
	Description string                 `json:"description" yaml:"description"`
	Parameters  map[string]interface{} `json:"parameters" yaml:"parameters"`
}
