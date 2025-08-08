package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/unowned-ai/recall/pkg/mcp"
)

// OpenAIProvider implements OpenAI API testing
type OpenAIProvider struct {
	APIKey    string
	ModelName string
	Logger    *log.Logger
}

func (o *OpenAIProvider) Name() string {
	return fmt.Sprintf("OpenAI/%s", o.ModelName)
}

func (o *OpenAIProvider) CallWithTools(ctx context.Context, message string, tools []mcp.ToolDefinition) (ModelResponse, error) {
	openAITools := make([]map[string]interface{}, len(tools))
	for i, tool := range tools {
		openAITools[i] = map[string]interface{}{
			"type": "function",
			"function": map[string]interface{}{
				"name":        tool.Name,
				"description": tool.Description,
				"parameters":  tool.Parameters,
			},
		}
	}

	payload := map[string]interface{}{
		"model": o.ModelName,
		"messages": []map[string]string{
			{"role": "user", "content": message},
		},
		"tools":       openAITools,
		"tool_choice": "auto",
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return ModelResponse{}, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.openai.com/v1/chat/completions", bytes.NewBuffer(jsonData))
	if err != nil {
		return ModelResponse{}, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+o.APIKey)

	body, err := CallAPI(req, o.Logger)
	if err != nil {
		return ModelResponse{}, err
	}

	var openAIResp struct {
		Choices []struct {
			Message struct {
				Content   string `json:"content"`
				ToolCalls []struct {
					Function struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"message"`
		} `json:"choices"`
	}

	if err := json.Unmarshal(body, &openAIResp); err != nil {
		return ModelResponse{}, fmt.Errorf("failed to unmarshal openai response: %w, body: %s", err, string(body))
	}

	if len(openAIResp.Choices) == 0 {
		return ModelResponse{}, fmt.Errorf("no choices in response from openai")
	}

	modelResp := &ModelResponse{
		Message:     openAIResp.Choices[0].Message.Content,
		RawResponse: string(body),
		ToolCalls:   []ModelToolCall{},
	}

	for _, tc := range openAIResp.Choices[0].Message.ToolCalls {
		var args map[string]interface{}
		if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
			// In some cases, the model returns a string, not a JSON object, if no args are needed.
			// We can handle that by checking for unmarshal errors and keeping arguments nil.
		}
		modelResp.ToolCalls = append(modelResp.ToolCalls, ModelToolCall{
			Name:      tc.Function.Name,
			Arguments: args,
		})
	}

	return *modelResp, nil
}
