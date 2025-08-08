package providers

// ModelResponse represents the AI model's response
type ModelResponse struct {
	Message     string          `json:"message"`
	ToolCalls   []ModelToolCall `json:"tool_calls"`
	RawResponse string          `json:"raw_response"`
}

// ToolDefinition represents an MCP tool for AI models
type ToolDefinition struct {
	Name        string                 `json:"name" yaml:"name"`
	Description string                 `json:"description" yaml:"description"`
	Parameters  map[string]interface{} `json:"parameters" yaml:"parameters"`
}

// ModelToolCall represents a tool invocation by the model
type ModelToolCall struct {
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments"`
}

// ModelProvider defines the interface for calling a model
