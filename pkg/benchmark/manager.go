package benchmark

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"time"

	"github.com/unowned-ai/recall/pkg/mcp"
)

// BenchmarkMetrics captures performance data for a tool version
type BenchmarkMetrics struct {
	SuccessRate    float64 `json:"success_rate"`
	ResponseTime   float64 `json:"avg_response_time"`
	ActivationRate float64 `json:"activation_rate"`
}

// ToolVersion represents a single version of a tool description
type ToolVersion struct {
	Version      int               `json:"version"`
	Description  string            `json:"description"`
	Timestamp    time.Time         `json:"timestamp"`
	Performance  *BenchmarkMetrics `json:"performance,omitempty"`
	ChangeReason string            `json:"change_reason"`
}

// ModelVersionHistory holds the version history for a specific model
type ModelVersionHistory struct {
	CurrentVersion int           `json:"current_version"`
	Versions       []ToolVersion `json:"versions"`
}

// VersionedTool represents a tool with version history
type VersionedTool struct {
	Name            string                          `json:"name"`
	BaseDescription string                          `json:"base_description"`
	Parameters      map[string]interface{}          `json:"parameters"`
	ModelData       map[string]*ModelVersionHistory `json:"model_data"`
}

// ToolVersionManager manages tool version history using JSON files
type ToolVersionManager struct {
	Tools       map[string]*VersionedTool `json:"tools"`
	versionFile string
}

// NewToolVersionManager creates a new version manager
func NewToolVersionManager(versionFile string) *ToolVersionManager {
	return &ToolVersionManager{
		Tools:       make(map[string]*VersionedTool),
		versionFile: versionFile,
	}
}

// LoadVersions loads tool versions from file
func (tvm *ToolVersionManager) LoadVersions(mcpConfig MCPServerConfig) error {
	if _, err := os.Stat(tvm.versionFile); os.IsNotExist(err) {
		return tvm.InitializeFromCurrentTools(mcpConfig)
	}

	data, err := ioutil.ReadFile(tvm.versionFile)
	if err != nil {
		return fmt.Errorf("failed to read version file: %w", err)
	}

	// Check if empty
	if len(data) == 0 {
		return tvm.InitializeFromCurrentTools(mcpConfig)
	}

	if err := json.Unmarshal(data, &tvm.Tools); err != nil {
		return err
	}
	// Normalize maps that may have been null in JSON
	for _, tool := range tvm.Tools {
		if tool.ModelData == nil {
			tool.ModelData = make(map[string]*ModelVersionHistory)
		}
	}
	return nil
}

// SaveVersions saves tool versions to file
func (tvm *ToolVersionManager) SaveVersions() error {
	data, err := json.MarshalIndent(tvm.Tools, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal versions: %w", err)
	}

	return ioutil.WriteFile(tvm.versionFile, data, 0644)
}

// InitializeFromCurrentTools creates initial versions from GetCoreToolDefinitions
func (tvm *ToolVersionManager) InitializeFromCurrentTools(mcpConfig MCPServerConfig) error {
	currentTools := ExtractToolDefinitionsFromMCPServer(mcpConfig)

	for _, tool := range currentTools {
		versionedTool := &VersionedTool{
			Name:            tool.Name,
			BaseDescription: tool.Description,
			Parameters:      tool.Parameters,
			ModelData:       make(map[string]*ModelVersionHistory),
		}
		tvm.Tools[tool.Name] = versionedTool
	}

	return tvm.SaveVersions()
}

// GetCurrentTools returns current tool definitions with versioned descriptions for a specific model
func (tvm *ToolVersionManager) GetCurrentTools(modelName string, mcpConfig MCPServerConfig) []mcp.ToolDefinition {
	var tools []mcp.ToolDefinition

	for _, versionedTool := range tvm.Tools {
		description := versionedTool.BaseDescription

		// Check for model-specific version
		if modelHistory, ok := versionedTool.ModelData[modelName]; ok && len(modelHistory.Versions) > 0 {
			currentVersionIndex := modelHistory.CurrentVersion - 1
			if currentVersionIndex >= 0 && currentVersionIndex < len(modelHistory.Versions) {
				description = modelHistory.Versions[currentVersionIndex].Description
			}
		}

		tool := mcp.ToolDefinition{
			Name:        versionedTool.Name,
			Description: description,
			Parameters:  versionedTool.Parameters,
		}
		tools = append(tools, tool)
	}

	// If no versioned tools available, extract fresh from MCP server
	if len(tools) == 0 {
		tools = ExtractToolDefinitionsFromMCPServer(mcpConfig)
	}

	return tools
}

// AddVersion adds a new version for a tool for a specific model
func (tvm *ToolVersionManager) AddVersion(toolName, modelName, newDescription, reason string) error {
	tool, exists := tvm.Tools[toolName]
	if !exists {
		return fmt.Errorf("tool %s not found", toolName)
	}

	if tool.ModelData == nil {
		tool.ModelData = make(map[string]*ModelVersionHistory)
	}

	modelHistory, ok := tool.ModelData[modelName]
	if !ok {
		// First version for this model, create a history
		modelHistory = &ModelVersionHistory{
			CurrentVersion: 0,
			Versions:       []ToolVersion{},
		}
		tool.ModelData[modelName] = modelHistory
	}

	newVersion := ToolVersion{
		Version:      len(modelHistory.Versions) + 1,
		Description:  newDescription,
		Timestamp:    time.Now(),
		ChangeReason: reason,
	}

	modelHistory.Versions = append(modelHistory.Versions, newVersion)
	modelHistory.CurrentVersion = newVersion.Version

	return tvm.SaveVersions()
}

// RevertToPreviousVersion reverts a tool to its previous version for a specific model
func (tvm *ToolVersionManager) RevertToPreviousVersion(toolName, modelName string) error {
	tool, exists := tvm.Tools[toolName]
	if !exists {
		return fmt.Errorf("tool %s not found", toolName)
	}

	modelHistory, ok := tool.ModelData[modelName]
	if !ok {
		return fmt.Errorf("no version history found for model %s on tool %s", modelName, toolName)
	}

	if modelHistory.CurrentVersion <= 1 {
		return fmt.Errorf("tool %s (model %s) is already at version 1, cannot revert", toolName, modelName)
	}

	modelHistory.CurrentVersion--
	return tvm.SaveVersions()
}

// UpdatePerformance updates performance metrics for the current version of a specific model
func (tvm *ToolVersionManager) UpdatePerformance(toolName, modelName string, metrics BenchmarkMetrics) error {
	tool, exists := tvm.Tools[toolName]
	if !exists {
		return fmt.Errorf("tool %s not found", toolName)
	}

	modelHistory, ok := tool.ModelData[modelName]
	if !ok {
		// Cannot update performance for a model that hasn't been used yet
		return fmt.Errorf("no version history found for model %s on tool %s", modelName, toolName)
	}

	if len(modelHistory.Versions) == 0 {
		return fmt.Errorf("no versions found for tool %s (model %s)", toolName, modelName)
	}

	currentVersionIndex := modelHistory.CurrentVersion - 1
	if currentVersionIndex < 0 || currentVersionIndex >= len(modelHistory.Versions) {
		return fmt.Errorf("current version index out of bounds for tool %s (model %s)", toolName, modelName)
	}

	modelHistory.Versions[currentVersionIndex].Performance = &metrics

	return tvm.SaveVersions()
}

func (tvm *ToolVersionManager) SyncWithCurrentTools(mcpConfig MCPServerConfig) error {
	currentTools := ExtractToolDefinitionsFromMCPServer(mcpConfig)
	updated := false

	for _, tool := range currentTools {
		if _, exists := tvm.Tools[tool.Name]; !exists {
			tvm.Tools[tool.Name] = &VersionedTool{
				Name:            tool.Name,
				BaseDescription: tool.Description,
				Parameters:      tool.Parameters,
				ModelData:       make(map[string]*ModelVersionHistory),
			}
			updated = true
		}
	}
	if updated {
		return tvm.SaveVersions()
	}
	return nil
}
