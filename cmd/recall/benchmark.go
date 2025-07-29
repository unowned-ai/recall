package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/unowned-ai/recall/pkg/benchmark"
	"github.com/unowned-ai/recall/pkg/mcp"
	"github.com/unowned-ai/recall/pkg/providers"
	"gopkg.in/yaml.v3"
)

var (
	// Global flags that can be used across subcommands
	debug      bool
	logFile    string
	configFile string
	jsonOutput bool

	// Version file for tool history
	versionFile string = "benchmarks/tool_versions.json"
)

// Main benchmark command - now just a parent command
var benchmarkCmd = &cobra.Command{
	Use:   "benchmark",
	Short: "Run benchmarks and manage tool improvement cycles",
	Long: `Benchmark suite for testing and improving MCP tool activation.
Supports running tests, improving tool descriptions, and viewing version history.`,
}

// benchmark run subcommand
var benchmarkRunCmd = &cobra.Command{
	Use:   "run",
	Short: "Run benchmark tests",
	Long:  `Execute benchmark scenarios to test tool activation accuracy.`,
	RunE:  runBenchmarkCommand,
	PreRun: func(cmd *cobra.Command, args []string) {
		syncToolVersionsPreRun(cmd, args)
	},
}

// benchmark improve subcommand
var benchmarkImproveCmd = &cobra.Command{
	Use:   "improve",
	Short: "Run improvement cycles on tool descriptions",
	Long:  `Analyze benchmark failures and improve tool descriptions using AI suggestions.`,
	RunE:  runImproveCommand,
	PreRun: func(cmd *cobra.Command, args []string) {
		syncToolVersionsPreRun(cmd, args)
	},
}

// benchmark history subcommand
var benchmarkHistoryCmd = &cobra.Command{
	Use:   "history",
	Short: "View tool version history and performance",
	Long:  `Display version history and performance metrics for tools.`,
	RunE:  runHistoryCommand,
}

// benchmark revert subcommand
var benchmarkRevertCmd = &cobra.Command{
	Use:   "revert [tool-name]",
	Short: "Revert a tool to its previous version",
	Long:  `Reverts a tool to its previous version, making the second-to-last version the current one.`,
	Args:  cobra.ExactArgs(1),
	RunE:  runRevertCommand,
}

// Flags for run subcommand
var (
	provider string
	model    string
	apiKey   string
)

// Flags for improve subcommand
var (
	iterations          int
	improveTool         []string
	improveProvider     string // Provider for testing (can be same as improve-with-provider)
	improveModel        string // The model we're testing and improving descriptions FOR
	improveWith         string // The model we use to GENERATE improved descriptions
	improveWithProvider string // The provider we use to GENERATE improved descriptions
	improveAPIKey       string // API key for testing
	improveWithAPIKey   string // API key for generating improvements (if different provider)
)

// Flags for history subcommand
var (
	toolName        string
	showPerformance bool
	// Generalized MCP server config flags
	mcpCommand    string
	mcpArgs       []string
	mcpFlags      []string
	mcpWorkingDir string
)

// Helper to sync tool versions before any benchmark command runs
func syncToolVersionsPreRun(cmd *cobra.Command, args []string) {
	tvm := benchmark.NewToolVersionManager(versionFile)
	mcpConfig := benchmark.MCPServerConfig{
		Command:    mcpCommand,
		Args:       mcpArgs,
		Flags:      mcpFlags,
		WorkingDir: mcpWorkingDir,
	}
	if err := tvm.LoadVersions(mcpConfig); err != nil {
		fmt.Fprintf(os.Stderr, "failed to load tool versions: %v\n", err)
		os.Exit(1)
	}
	if err := tvm.SyncWithCurrentTools(mcpConfig); err != nil {
		fmt.Fprintf(os.Stderr, "failed to sync tool versions: %v\n", err)
		os.Exit(1)
	}
}

func initBenchmarkCmd() {
	// Global flags
	benchmarkCmd.PersistentFlags().BoolVar(&debug, "debug", false, "Enable debug logging")
	benchmarkCmd.PersistentFlags().StringVar(&logFile, "log-file", "", "Log file path")
	benchmarkCmd.PersistentFlags().StringVar(&configFile, "config", "benchmarks/default.yaml", "Benchmark config file")
	benchmarkCmd.PersistentFlags().BoolVar(&jsonOutput, "json", false, "Output results in JSON format")
	benchmarkCmd.PersistentFlags().StringVar(&versionFile, "version-file", "benchmarks/tool_versions.json", "Tool version history file")

	// Run subcommand flags
	benchmarkRunCmd.Flags().StringVar(&provider, "provider", "openai", "AI provider (openai, anthropic)")
	benchmarkRunCmd.Flags().StringVar(&model, "model", "gpt-4o", "Model name")
	benchmarkRunCmd.Flags().StringVar(&apiKey, "api-key", "", "API key")
	benchmarkRunCmd.MarkFlagRequired("api-key")

	// Improve subcommand flags
	benchmarkImproveCmd.Flags().IntVar(&iterations, "iterations", 3, "Number of improvement iterations")
	benchmarkImproveCmd.Flags().StringSliceVar(&improveTool, "tool", []string{}, "Specific tools to improve (default: all failed tools)")
	benchmarkImproveCmd.Flags().StringVar(&improveProvider, "provider", "openai", "AI provider for testing")
	benchmarkImproveCmd.Flags().StringVar(&improveModel, "model", "gpt-4o", "Model to test and improve descriptions for")
	benchmarkImproveCmd.Flags().StringVar(&improveWith, "improve-with", "gpt-4o", "Model to use for generating improved descriptions")
	benchmarkImproveCmd.Flags().StringVar(&improveWithProvider, "improve-with-provider", "openai", "Provider to use for generating improved descriptions")
	benchmarkImproveCmd.Flags().StringVar(&improveAPIKey, "api-key", "", "API key for testing")
	benchmarkImproveCmd.Flags().StringVar(&improveWithAPIKey, "improve-api-key", "", "API key for generating improvements (if different from --api-key)")
	benchmarkImproveCmd.MarkFlagRequired("api-key")

	// History subcommand flags
	benchmarkHistoryCmd.Flags().StringVar(&toolName, "tool", "", "Show history for specific tool")
	benchmarkHistoryCmd.Flags().BoolVar(&showPerformance, "performance", false, "Show performance metrics")

	// Generalized MCP server configuration flags (available on all subcommands)
	benchmarkCmd.PersistentFlags().StringVar(&mcpCommand, "mcp-command", "./recall", "MCP server command (e.g., './recall', 'npx', 'python')")
	benchmarkCmd.PersistentFlags().StringSliceVar(&mcpArgs, "mcp-args", []string{"mcp"}, "MCP server arguments (e.g., 'mcp', 'some-package', 'server.py')")
	benchmarkCmd.PersistentFlags().StringSliceVar(&mcpFlags, "mcp-flags", []string{}, "MCP server flags (e.g., '--verbose')")
	benchmarkCmd.PersistentFlags().StringVar(&mcpWorkingDir, "mcp-workdir", "", "Working directory for MCP server")

	// Add subcommands
	benchmarkCmd.AddCommand(benchmarkRunCmd)
	benchmarkCmd.AddCommand(benchmarkImproveCmd)
	benchmarkCmd.AddCommand(benchmarkHistoryCmd)
	benchmarkCmd.AddCommand(benchmarkRevertCmd)
}

// BenchmarkConfig defines the structure of the YAML file for benchmarks.
type BenchmarkConfig struct {
	Scenarios []BenchmarkScenario `yaml:"scenarios"`
}

// BenchmarkScenario is a test scenario from the YAML file.
type BenchmarkScenario struct {
	Name          string   `yaml:"name"`
	Message       string   `yaml:"message"`
	ExpectedTools []string `yaml:"expected_tools"`
}

func loadBenchmarkConfig(path string) (*BenchmarkConfig, error) {
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read benchmark config file: %w", err)
	}

	var config BenchmarkConfig
	err = yaml.Unmarshal(data, &config)
	if err != nil {
		return nil, fmt.Errorf("failed to parse benchmark config file: %w", err)
	}

	return &config, nil
}

type BenchmarkResult struct {
	Tools   []mcp.ToolDefinition          `json:"tools"`
	Results [][]benchmark.ModelTestResult `json:"results_per_iteration"`
}

// runBenchmarkCommand handles the "run" subcommand
func runBenchmarkCommand(cmd *cobra.Command, args []string) error {
	return runSingleBenchmark(context.Background())
}

// runImproveCommand handles the "improve" subcommand
func runImproveCommand(cmd *cobra.Command, args []string) error {
	return runImprovementCycle(context.Background())
}

// runHistoryCommand handles the "history" subcommand
func runHistoryCommand(cmd *cobra.Command, args []string) error {
	return showVersionHistory()
}

// runRevertCommand handles the "revert" subcommand
func runRevertCommand(cmd *cobra.Command, args []string) error {
	return revertToolVersion(args[0])
}

// runSingleBenchmark runs a single benchmark without improvement cycles
func runSingleBenchmark(ctx context.Context) error {
	var logger *log.Logger
	if debug {
		var logWriter io.Writer = os.Stdout
		if logFile != "" {
			f, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
			if err != nil {
				return fmt.Errorf("failed to open log file: %w", err)
			}
			defer f.Close()
			logWriter = f
		}
		logger = log.New(logWriter, "DEBUG: ", log.LstdFlags)
	}

	mcpConfig := benchmark.MCPServerConfig{
		Command:    mcpCommand,
		Args:       mcpArgs,
		Flags:      mcpFlags,
		WorkingDir: mcpWorkingDir,
	}

	// Load tool versions
	tvm := benchmark.NewToolVersionManager(versionFile)
	if err := tvm.LoadVersions(mcpConfig); err != nil {
		return fmt.Errorf("failed to load tool versions: %w", err)
	}

	// Get current tools from version manager
	tools := tvm.GetCurrentTools(model, mcpConfig)

	// Run benchmark
	results, err := runSingleBenchmarkIteration(ctx, logger, tools, provider, apiKey, model)
	if err != nil {
		return fmt.Errorf("error running benchmark: %w", err)
	}

	// Update performance metrics
	if err := updateToolPerformanceMetrics(tvm, results, model); err != nil {
		log.Printf("Warning: could not update performance metrics: %v", err)
	}

	// Output results
	if jsonOutput {
		benchmarkResult := BenchmarkResult{
			Tools:   tools,
			Results: [][]benchmark.ModelTestResult{results},
		}
		jsonBytes, err := json.MarshalIndent(benchmarkResult, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal results: %w", err)
		}
		fmt.Println(string(jsonBytes))
	} else {
		printSummary(results)
	}

	return nil
}

// revertToolVersion reverts a tool to its previous version
func revertToolVersion(toolToRevert string) error {
	tvm := benchmark.NewToolVersionManager(versionFile)
	mcpConfig := benchmark.MCPServerConfig{
		Command:    mcpCommand,
		Args:       mcpArgs,
		Flags:      mcpFlags,
		WorkingDir: mcpWorkingDir,
	}
	if err := tvm.LoadVersions(mcpConfig); err != nil {
		return fmt.Errorf("failed to load tool versions: %w", err)
	}

	// For now, we'll assume the model from the improve command flags.
	// This could be made more sophisticated later.
	modelToUse := improveModel
	if modelToUse == "" {
		modelToUse = model // fallback to run model
	}

	if err := tvm.RevertToPreviousVersion(toolToRevert, modelToUse); err != nil {
		return fmt.Errorf("failed to revert tool '%s' for model '%s': %w", toolToRevert, modelToUse, err)
	}

	if !jsonOutput {
		fmt.Printf("Successfully reverted tool '%s' for model '%s' to its previous version.\n\n", toolToRevert, modelToUse)
	}

	return showVersionHistory()
}

// runImprovementCycle runs benchmark with improvement iterations
func runImprovementCycle(ctx context.Context) error {
	var logger *log.Logger
	if debug {
		var logWriter io.Writer = os.Stdout
		if logFile != "" {
			f, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
			if err != nil {
				return fmt.Errorf("failed to open log file: %w", err)
			}
			defer f.Close()
			logWriter = f
		}
		logger = log.New(logWriter, "DEBUG: ", log.LstdFlags)
	}

	mcpConfig := benchmark.MCPServerConfig{
		Command:    mcpCommand,
		Args:       mcpArgs,
		Flags:      mcpFlags,
		WorkingDir: mcpWorkingDir,
	}

	// Load tool versions
	tvm := benchmark.NewToolVersionManager(versionFile)
	if err := tvm.LoadVersions(mcpConfig); err != nil {
		return fmt.Errorf("failed to load tool versions: %w", err)
	}

	finalResults := BenchmarkResult{
		Results: make([][]benchmark.ModelTestResult, 0, iterations),
	}

	for i := 0; i < iterations; i++ {
		if !jsonOutput {
			fmt.Printf("\n--- Starting Iteration %d/%d ---\n", i+1, iterations)
		}

		// Get current tools
		tools := tvm.GetCurrentTools(improveModel, mcpConfig)

		// Run benchmark
		iterationResults, err := runSingleBenchmarkIteration(ctx, logger, tools, improveProvider, improveAPIKey, improveModel)
		if err != nil {
			return fmt.Errorf("error in iteration %d: %w", i+1, err)
		}
		finalResults.Results = append(finalResults.Results, iterationResults)

		// Update performance metrics
		if err := updateToolPerformanceMetrics(tvm, iterationResults, improveModel); err != nil {
			log.Printf("Warning: could not update performance metrics: %v", err)
		}

		// Check for failures and attempt to improve
		hasFailures := false
		for _, res := range iterationResults {
			if !res.Success {
				hasFailures = true
				break
			}
		}

		if i < iterations-1 && hasFailures {
			if !jsonOutput {
				fmt.Println("\n--- Analyzing failures and suggesting improvements ---")
			}
			if err := improveFailedTools(ctx, logger, tvm, iterationResults, improveModel); err != nil {
				log.Printf("Could not improve tools: %v", err)
			}
		} else if i < iterations-1 {
			if !jsonOutput {
				fmt.Println("\n--- All tests passed! No improvements needed. ---")
			}
			break
		}
	}

	finalResults.Tools = tvm.GetCurrentTools(improveModel, mcpConfig)

	// Output results
	if jsonOutput {
		jsonBytes, err := json.MarshalIndent(finalResults, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal final results: %w", err)
		}
		fmt.Println(string(jsonBytes))
	} else {
		if len(finalResults.Results) > 0 {
			printSummary(finalResults.Results[len(finalResults.Results)-1])
		}
	}

	return nil
}

// showVersionHistory displays tool version history
func showVersionHistory() error {
	tvm := benchmark.NewToolVersionManager(versionFile)
	mcpConfig := benchmark.MCPServerConfig{
		Command:    mcpCommand,
		Args:       mcpArgs,
		Flags:      mcpFlags,
		WorkingDir: mcpWorkingDir,
	}
	if err := tvm.LoadVersions(mcpConfig); err != nil {
		return fmt.Errorf("failed to load tool versions: %w", err)
	}

	if toolName != "" {
		// Show history for specific tool
		return showToolHistoryForTool(tvm, toolName)
	}

	// Show summary for all tools
	if jsonOutput {
		jsonBytes, err := json.MarshalIndent(tvm.Tools, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal tool versions: %w", err)
		}
		fmt.Println(string(jsonBytes))
	} else {
		fmt.Println("=== Tool Version History ===")
		for name, tool := range tvm.Tools {
			fmt.Printf("\n🔧 %s\n", name)
			fmt.Printf("   Base Description: %s\n", tool.BaseDescription)

			if len(tool.ModelData) == 0 {
				fmt.Println("   No model-specific versions yet.")
				continue
			}

			for modelName, history := range tool.ModelData {
				fmt.Printf("   └── Model: %s (Current: v%d, Total: %d versions)\n",
					modelName, history.CurrentVersion, len(history.Versions))

				if showPerformance && len(history.Versions) > 0 {
					currentVersion := history.Versions[history.CurrentVersion-1]
					if currentVersion.Performance != nil {
						fmt.Printf("       Performance: %.1f%% success, %.2fs avg response\n",
							currentVersion.Performance.SuccessRate*100,
							currentVersion.Performance.ResponseTime)
					}
				}
			}
		}
	}

	return nil
}

// showToolHistoryForTool displays detailed history for a specific tool
func showToolHistoryForTool(tvm *benchmark.ToolVersionManager, toolName string) error {
	tool, exists := tvm.Tools[toolName]
	if !exists {
		return fmt.Errorf("tool %s not found", toolName)
	}

	if jsonOutput {
		jsonBytes, err := json.MarshalIndent(tool, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal tool history: %w", err)
		}
		fmt.Println(string(jsonBytes))
		return nil
	}

	fmt.Printf("=== History for %s ===\n", toolName)
	fmt.Printf("Base Description: %s\n", tool.BaseDescription)

	if len(tool.ModelData) == 0 {
		fmt.Println("\nNo model-specific versions yet.")
		return nil
	}

	for modelName, history := range tool.ModelData {
		fmt.Printf("\n--- Model: %s ---\n", modelName)
		fmt.Printf("Current Version: %d\n", history.CurrentVersion)
		fmt.Printf("Total Versions: %d\n", len(history.Versions))

		for _, version := range history.Versions {
			marker := "  "
			if version.Version == history.CurrentVersion {
				marker = "→ "
			}

			fmt.Printf("\n%sv%d (%s)\n", marker, version.Version, version.Timestamp.Format("2006-01-02 15:04"))
			fmt.Printf("   Reason: %s\n", version.ChangeReason)
			fmt.Printf("   Description: %s\n", version.Description)

			if showPerformance && version.Performance != nil {
				fmt.Printf("   Performance: %.1f%% success, %.2fs avg response\n",
					version.Performance.SuccessRate*100,
					version.Performance.ResponseTime)
			}
		}
	}

	return nil
}

// updateToolPerformanceMetrics calculates and updates performance metrics
func updateToolPerformanceMetrics(tvm *benchmark.ToolVersionManager, results []benchmark.ModelTestResult, modelName string) error {
	toolMetrics := make(map[string]*benchmark.BenchmarkMetrics)

	// Calculate metrics for each tool
	for _, result := range results {
		for _, expectedTool := range result.ExpectedTools {
			if _, exists := toolMetrics[expectedTool]; !exists {
				toolMetrics[expectedTool] = &benchmark.BenchmarkMetrics{}
			}
		}

		for _, actualTool := range result.ActualTools {
			if _, exists := toolMetrics[actualTool]; !exists {
				toolMetrics[actualTool] = &benchmark.BenchmarkMetrics{}
			}
		}
	}

	// Count successes and calculate averages
	for toolName := range toolMetrics {
		var totalTests, successes int
		var totalResponseTime float64

		for _, result := range results {
			// Check if this tool was expected in this test
			expectedHere := false
			for _, expected := range result.ExpectedTools {
				if expected == toolName {
					expectedHere = true
					break
				}
			}

			if expectedHere {
				totalTests++
				totalResponseTime += result.ResponseTime

				// Check if tool was actually called
				actuallyCalledHere := false
				for _, actual := range result.ActualTools {
					if actual == toolName {
						actuallyCalledHere = true
						break
					}
				}

				if actuallyCalledHere {
					successes++
				}
			}
		}

		if totalTests > 0 {
			toolMetrics[toolName].SuccessRate = float64(successes) / float64(totalTests)
			toolMetrics[toolName].ResponseTime = totalResponseTime / float64(totalTests)
			toolMetrics[toolName].ActivationRate = float64(successes) / float64(totalTests)
		}
	}

	// Update performance for each tool
	for toolName, metrics := range toolMetrics {
		if err := tvm.UpdatePerformance(toolName, modelName, *metrics); err != nil {
			return fmt.Errorf("failed to update performance for %s: %w", toolName, err)
		}
	}

	return nil
}

// improveFailedTools attempts to improve tools that had failures
// Each tool must use all existing models description for see window for improve description
func improveFailedTools(ctx context.Context, logger *log.Logger, tvm *benchmark.ToolVersionManager, results []benchmark.ModelTestResult, modelName string) error {
	if improveAPIKey == "" {
		fmt.Println("Skipping improvement step: --api-key not provided for improve command.")
		return nil
	}

	// Use improve-api-key if provided, otherwise fall back to main api-key
	apiKeyForImprovement := improveWithAPIKey
	if apiKeyForImprovement == "" {
		apiKeyForImprovement = improveAPIKey
	}

	improverProvider, err := providers.NewModelProvider(improveWithProvider, apiKeyForImprovement, improveWith, logger)
	if err != nil {
		return err
	}

	// Find tools that need improvement
	toolsToImprove := make(map[string]bool)

	for _, result := range results {
		if result.Success {
			continue
		}

		// Add expected but not called tools
		for _, expectedTool := range result.ExpectedTools {
			shouldImprove := len(improveTool) == 0 // improve all by default
			if !shouldImprove {
				for _, targetTool := range improveTool {
					if expectedTool == targetTool {
						shouldImprove = true
						break
					}
				}
			}

			if shouldImprove {
				toolsToImprove[expectedTool] = true
			}
		}
	}

	// Improve each tool
	for toolName := range toolsToImprove {
		tool, exists := tvm.Tools[toolName]
		if !exists {
			continue
		}

		// Get the current description for the specific model
		currentDescription := tool.BaseDescription
		var currentPerformance *benchmark.BenchmarkMetrics
		if modelHistory, ok := tool.ModelData[modelName]; ok && len(modelHistory.Versions) > 0 {
			currentVersion := modelHistory.Versions[modelHistory.CurrentVersion-1]
			currentDescription = currentVersion.Description
			currentPerformance = currentVersion.Performance
		}

		// Find a representative failed test for this tool
		var failedTest *benchmark.ModelTestResult
		for i := range results {
			if !results[i].Success {
				for _, expected := range results[i].ExpectedTools {
					if expected == toolName {
						failedTest = &results[i]
						break
					}
				}
			}
			if failedTest != nil {
				break
			}
		}

		if failedTest == nil {
			continue
		}

		prompt := buildImprovementPrompt(*failedTest, currentDescription)
		if !jsonOutput {
			fmt.Printf("Attempting to improve tool '%s' for model '%s'...\n", toolName, modelName)
		}

		resp, err := improverProvider.CallWithTools(ctx, prompt, nil)
		if err != nil {
			log.Printf("Failed to get suggestion for tool %s: %v", toolName, err)
			continue
		}

		newDescription := strings.Trim(resp.Message, "\" \n\t")
		if newDescription != "" && newDescription != currentDescription {
			if !jsonOutput {
				fmt.Printf("  Original Description: %s\n", currentDescription)
				fmt.Printf("  Suggested Description: %s\n", newDescription)
			}

			// Check if this tool consistently fails (success rate < 50%) before creating new version
			shouldImprove := true
			if currentPerformance != nil && currentPerformance.SuccessRate >= 0.5 {
				if !jsonOutput {
					fmt.Printf("  Skipping improvement: Tool success rate %.1f%% is acceptable\n", currentPerformance.SuccessRate*100)
				}
				shouldImprove = false
			}

			if shouldImprove {
				// Add new version
				reason := fmt.Sprintf("AI improvement for failed test: %s", failedTest.Message)
				if err := tvm.AddVersion(toolName, modelName, newDescription, reason); err != nil {
					log.Printf("Failed to add new version for tool %s: %v", toolName, err)
				}
			}
		}
	}

	return nil
}

func runSingleBenchmarkIteration(ctx context.Context, logger *log.Logger, tools []mcp.ToolDefinition, providerName, apiKeyValue, modelName string) ([]benchmark.ModelTestResult, error) {
	if apiKeyValue == "" {
		return nil, fmt.Errorf("the --api-key flag is required")
	}

	p, err := providers.NewModelProvider(providerName, apiKeyValue, modelName, logger)
	if err != nil {
		return nil, err
	}

	config, err := loadBenchmarkConfig(configFile)
	if err != nil {
		return nil, err
	}
	scenariosToRun := config.Scenarios

	// If specific tools are targeted for improvement, filter scenarios
	if len(improveTool) > 0 {
		targetTools := make(map[string]struct{})
		for _, t := range improveTool {
			targetTools[t] = struct{}{}
		}

		var filteredScenarios []BenchmarkScenario
		for _, s := range scenariosToRun {
			for _, expected := range s.ExpectedTools {
				if _, ok := targetTools[expected]; ok {
					filteredScenarios = append(filteredScenarios, s)
					break // Scenario added, move to the next one
				}
			}
		}
		scenariosToRun = filteredScenarios
	}

	if !jsonOutput {
		fmt.Printf("Running benchmark for Provider: %s, Model: %s\n", providerName, modelName)
		if len(improveTool) > 0 && len(scenariosToRun) > 0 {
			fmt.Printf("Found %d scenarios for specified tools: %v\n", len(scenariosToRun), improveTool)
		} else if len(improveTool) > 0 {
			fmt.Printf("Warning: No scenarios found for specified tools: %v\n", improveTool)
		}
		fmt.Println("==================================================")
	}

	var results []benchmark.ModelTestResult
	for _, s := range scenariosToRun {
		if !jsonOutput {
			fmt.Printf("Running Scenario: %s\n", s.Name)
			fmt.Printf("User Message: '%s'\n", s.Message)
		}

		start := time.Now()
		resp, err := p.CallWithTools(ctx, s.Message, tools)
		duration := time.Since(start)

		result := benchmark.ModelTestResult{
			Provider:      p.Name(),
			Message:       s.Message,
			ExpectedTools: s.ExpectedTools,
			ResponseTime:  duration.Seconds(),
		}

		if err != nil {
			result.Error = err.Error()
			result.Success = false
			if !jsonOutput {
				fmt.Printf("  Error: %v\n", err)
			}
		} else {
			var actualTools []string
			for _, tc := range resp.ToolCalls {
				actualTools = append(actualTools, tc.Name)
			}

			result.ActualTools = actualTools
			result.Success = matchesExpected(actualTools, s.ExpectedTools)
			if !jsonOutput {
				fmt.Printf("  Actual Tools: %v\n", actualTools)
				fmt.Printf("  Expected Tools: %v\n", s.ExpectedTools)
				fmt.Printf("  Success: %v\n", result.Success)
				if len(resp.ToolCalls) > 0 {
					for _, tc := range resp.ToolCalls {
						fmt.Printf("  Tool Call: %+v\n", tc)
					}
				}
			}
		}
		if !jsonOutput {
			fmt.Printf("  Response Time: %.2fs\n\n", duration.Seconds())
		}
		results = append(results, result)
	}
	return results, nil
}

func buildImprovementPrompt(failedTest benchmark.ModelTestResult, originalDescription string) string {
	return fmt.Sprintf(`You are an expert at writing tool descriptions that help AI models choose the right tools. A tool activation test failed and you need to improve the description based on complete performance history.

**Context: This is a Memory/Knowledge Management System**
The tools are part of a personal memory system called "Recall" that helps users store, search, and retrieve past conversations and information.

**Failed Test Case:**
- User Message: "%s"
- Expected Tool: %v (should have been called)
- Actual Tools Called: %v (what was actually called instead)

**Current Tool Description:**
"%s"

**IMPROVEMENT HISTORY & PERFORMANCE DATA:**
%s

**Analysis Questions to Consider:**
1. What is the core PURPOSE of this tool? (not just specific trigger phrases)
2. What types of USER INTENTS should activate this tool?
3. What are the BROADER SCENARIOS where this tool provides value?
4. How is this tool fundamentally different from other tools in the system?
5. What patterns do you see in the performance history above?
6. Which previous descriptions had better performance and why?

**Tool Context Guidelines:**
- get_memory_overview: For understanding conversation history and system capabilities at conversation start
- create_entry: For saving/remembering information  
- search_entries: For finding specific past information
- list_entries: For browsing stored memories

**Your Task:**
Write a new tool description that captures the GENERAL PURPOSE and BROAD ACTIVATION PATTERNS, learning from the performance history above. Focus on:

1. **Core Function**: What does this tool fundamentally do?
2. **User Intent Categories**: What kinds of needs/situations does it address?
3. **Activation Scenarios**: When should it naturally be used? (be broad, not specific)
4. **Value Proposition**: Why would users want this tool to be called?
5. **Performance Insights**: What can you learn from the success/failure patterns above?

**Critical Guidelines:**
- DON'T just copy the exact words from the failed test ("%s")
- DO describe the broader category of user intents and scenarios
- Focus on the tool's PURPOSE, not specific trigger phrases
- Make it robust across many similar scenarios, not just this one test
- Think about the tool's role in the overall system workflow
- Consider: Would this description work for related scenarios like "Can you help me understand how this works?" or "What are your capabilities?"
- LEARN from the performance history - what worked well in previous versions?

**Rules:**
- ONLY return the new description text
- No preamble, explanation, or additional commentary
- Keep it concise but broadly applicable (2-3 sentences ideal)
- Capture the essence, not specific phrases
- Build upon successful patterns from the history above

New Description:`, failedTest.Message, failedTest.ExpectedTools, failedTest.ActualTools, originalDescription, "%s", failedTest.Message)
}

func matchesExpected(actual, expected []string) bool {
	if len(actual) != len(expected) {
		return false
	}
	actualSet := make(map[string]struct{})
	for _, a := range actual {
		actualSet[a] = struct{}{}
	}
	for _, e := range expected {
		if _, ok := actualSet[e]; !ok {
			return false
		}
	}
	return true
}

func printSummary(results []benchmark.ModelTestResult) {
	total := len(results)
	if total == 0 {
		fmt.Println("No results to summarize.")
		return
	}

	successes := 0
	var totalResponseTime float64
	for _, r := range results {
		if r.Success {
			successes++
		}
		totalResponseTime += r.ResponseTime
	}

	successRate := float64(successes) / float64(total) * 100
	avgResponseTime := totalResponseTime / float64(total)

	fmt.Println("================ Benchmark Summary ================")
	fmt.Printf("Total Scenarios: %d\n", total)
	fmt.Printf("Successful:      %d\n", successes)
	fmt.Printf("Success Rate:    %.2f%%\n", successRate)
	fmt.Printf("Avg Response Time: %.2fs\n", avgResponseTime)
	fmt.Println("==================================================")
}
