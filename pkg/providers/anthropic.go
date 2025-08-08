package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"net/http"

	"github.com/unowned-ai/recall/pkg/mcp"
)

// AnthropicProvider implements the ModelProvider interface for Anthropic models.
type AnthropicProvider struct {
	APIKey    string
	ModelName string
	Logger    *log.Logger
}

// Name returns the name of the provider.
func (a *AnthropicProvider) Name() string {
	return "anthropic"
}

// CallWithTools makes a call to the Anthropic API.
func (a *AnthropicProvider) CallWithTools(ctx context.Context, message string, tools []mcp.ToolDefinition) (ModelResponse, error) {
	apiURL := "https://api.anthropic.com/v1/messages"

	var anthropicTools []map[string]interface{}
	if len(tools) > 0 {
		anthropicTools = make([]map[string]interface{}, len(tools))
		for i, tool := range tools {
			anthropicTools[i] = map[string]interface{}{
				"name":        tool.Name,
				"description": tool.Description,
				"input_schema": map[string]interface{}{
					"type":       "object",
					"properties": tool.Parameters,
				},
			}
		}
	}

	payload := map[string]interface{}{
		"model": a.ModelName,
		"messages": []map[string]interface{}{
			{"role": "user", "content": message},
		},
		"max_tokens": 4096,
	}

	// Only include tools if we have them
	if len(anthropicTools) > 0 {
		payload["tools"] = anthropicTools
	}

	bodyBytes, _ := json.Marshal(payload)
	req, _ := http.NewRequestWithContext(ctx, "POST", apiURL, bytes.NewBuffer(bodyBytes))
	req.Header.Set("x-api-key", a.APIKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("content-type", "application/json")

	body, err := CallAPI(req, a.Logger)
	if err != nil {
		return ModelResponse{}, err
	}

	var anthropicResp struct {
		Content []struct {
			Type  string          `json:"type"`
			Text  string          `json:"text"`
			Name  string          `json:"name"`
			Input json.RawMessage `json:"input"`
		} `json:"content"`
	}
	json.Unmarshal(body, &anthropicResp)

	modelResp := ModelResponse{
		ToolCalls:   []ModelToolCall{},
		RawResponse: string(body),
	}

	for _, content := range anthropicResp.Content {
		if content.Type == "tool_use" {
			var args map[string]interface{}
			json.Unmarshal(content.Input, &args)
			modelResp.ToolCalls = append(modelResp.ToolCalls, ModelToolCall{
				Name:      content.Name,
				Arguments: args,
			})
		} else if content.Type == "text" {
			modelResp.Message = content.Text
		}
	}

	return modelResp, nil
}
