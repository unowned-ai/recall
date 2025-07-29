package benchmark

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/unowned-ai/recall/pkg/mcp"
)

// MCPServerConfig holds configuration for running any MCP server for benchmarking.
type MCPServerConfig struct {
	Command    string            // e.g., "./recall", "npx", "python"
	Args       []string          // e.g., ["mcp"], ["some-mcp-package"], ["server.py"]
	Flags      []string          // e.g., ["--memory-aware", "--verbose"]
	Env        map[string]string // Environment variables
	WorkingDir string            // Working directory for the command
	TempDB     string            // Optional temp database path
}

// ModelTestResult captures the result of a benchmark test against a model.
type ModelTestResult struct {
	Provider      string   `json:"provider"`
	Message       string   `json:"message"`
	ExpectedTools []string `json:"expected_tools"`
	ActualTools   []string `json:"actual_tools"`
	Success       bool     `json:"success"`
	ResponseTime  float64  `json:"response_time_ms"`
	Error         string   `json:"error,omitempty"`
}

// ExtractToolDefinitionsFromMCPServer runs an MCP server as a subprocess
// and extracts its tool definitions by communicating with it over stdio.
func ExtractToolDefinitionsFromMCPServer(config MCPServerConfig) []mcp.ToolDefinition {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	args := make([]string, len(config.Args))
	copy(args, config.Args)

	if config.Command == "./recall" || contains(config.Args, "mcp") {
		if config.TempDB == "" {
			tmpfile, err := os.CreateTemp("", "mcp-benchmark-*.db")
			if err != nil {
				log.Printf("Failed to create temp db: %v", err)
				return nil
			}
			tmpfile.Close()
			config.TempDB = tmpfile.Name()
			defer os.Remove(config.TempDB)
		}
		args = append(args, "--db", config.TempDB)
	}

	args = append(args, config.Flags...)

	cmd := exec.CommandContext(ctx, config.Command, args...)
	cmd.Stderr = os.Stderr

	if config.WorkingDir != "" {
		cmd.Dir = config.WorkingDir
	}
	if len(config.Env) > 0 {
		cmd.Env = os.Environ()
		for key, value := range config.Env {
			cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", key, value))
		}
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		log.Printf("Failed to create stdin pipe for MCP server: %v", err)
		return nil
	}
	defer stdin.Close()

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		log.Printf("Failed to create stdout pipe for MCP server: %v", err)
		return nil
	}
	defer stdout.Close()

	if err := cmd.Start(); err != nil {
		log.Printf("Failed to start MCP server process: %v", err)
		return nil
	}
	defer cmd.Process.Kill()

	client := NewClient(stdout, stdin)
	defer client.Close()

	initParams := map[string]interface{}{
		"protocolVersion": "2024-05-01",
		"clientInfo":      map[string]string{"name": "recall-benchmark-extractor"},
	}
	if err := client.Call(ctx, "initialize", initParams, nil); err != nil {
		log.Printf("failed to initialize MCP server: %v", err)
		return nil
	}

	var toolListResp struct {
		Tools []mcp.ToolDefinition `json:"tools"`
	}
	if err := client.Call(ctx, "tools/list", nil, &toolListResp); err != nil {
		log.Printf("failed to list tools from MCP server: %v", err)
		return nil
	}

	return toolListResp.Tools
}

// Client is a simple JSON-RPC client for communicating over stdio.
type Client struct {
	r      *json.Decoder
	w      io.WriteCloser
	wMutex *sync.Mutex
	id     uint64
}

// NewClient creates a new JSON-RPC client.
func NewClient(r io.Reader, w io.WriteCloser) *Client {
	return &Client{
		r:      json.NewDecoder(r),
		w:      w,
		wMutex: &sync.Mutex{},
		id:     1,
	}
}

// Call sends a JSON-RPC request.
func (c *Client) Call(ctx context.Context, method string, params, result interface{}) error {
	req := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      c.id,
		"method":  method,
		"params":  params,
	}
	c.id++

	if err := c.send(req); err != nil {
		return err
	}

	var resp struct {
		ID     uint64          `json:"id"`
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}

	if err := c.r.Decode(&resp); err != nil {
		return fmt.Errorf("failed to decode rpc response: %w", err)
	}
	if resp.Error != nil {
		return fmt.Errorf("rpc error: %s (%d)", resp.Error.Message, resp.Error.Code)
	}
	if result != nil && len(resp.Result) > 0 {
		return json.Unmarshal(resp.Result, result)
	}
	return nil
}

func (c *Client) send(req interface{}) error {
	c.wMutex.Lock()
	defer c.wMutex.Unlock()
	return json.NewEncoder(c.w).Encode(req)
}

// Close closes the underlying writer.
func (c *Client) Close() error {
	return c.w.Close()
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
