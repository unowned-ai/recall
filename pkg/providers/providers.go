package providers

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"unicode/utf8"

	"github.com/unowned-ai/recall/pkg/mcp"
)

// ModelProvider defines the interface for calling a model

// ModelProvider defines the interface for calling an AI model's API.
// It abstracts away the specific details of each provider (OpenAI, Anthropic, etc.).
type ModelProvider interface {
	// Name returns the name of the provider (e.g., "openai", "anthropic").
	Name() string

	// CallWithTools sends a message to the model and returns its response,
	// including any requested tool calls.
	CallWithTools(ctx context.Context, message string, tools []mcp.ToolDefinition) (ModelResponse, error)
}

// NewModelProvider is a factory function that returns the specified model provider.
// It initializes the correct provider based on the given name.
func NewModelProvider(provider, apiKey, modelName string, logger *log.Logger) (ModelProvider, error) {
	switch provider {
	case "openai":
		return &OpenAIProvider{APIKey: apiKey, ModelName: modelName, Logger: logger}, nil
	case "anthropic":
		return &AnthropicProvider{APIKey: apiKey, ModelName: modelName, Logger: logger}, nil
	default:
		return nil, fmt.Errorf("unsupported provider: %s", provider)
	}
}

func CallAPI(req *http.Request, logger *log.Logger) ([]byte, error) {
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// Only log if logger is provided; avoid noisy stdout
	if logger != nil {
		const max = 200
		preview := body
		if len(preview) > max {
			preview = preview[:max]
			// ensure valid utf-8 in preview
			for !utf8.Valid(preview) && len(preview) > 0 {
				preview = preview[:len(preview)-1]
			}
		}
		logger.Printf("HTTP %s %s — %s\n", req.Method, req.URL.Host, resp.Status)
		if len(preview) > 0 {
			logger.Printf("Body (truncated): %s\n", string(preview))
		}
	}

	return body, nil
}
