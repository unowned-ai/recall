package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"

	"github.com/spf13/cobra"
	"github.com/unowned-ai/recall/pkg/benchmark"
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
	dryRun              bool   // Do not save changes; preview only
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
	benchmarkImproveCmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview proposed improvements without saving")
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

	historyOpts := benchmark.HistoryOptions{
		ToolVersionManager: tvm,
		ToolName:           toolName,
		ShowPerformance:    showPerformance,
		JSONOutput:         jsonOutput,
	}
	return benchmark.ShowVersionHistory(historyOpts)
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

	// Load benchmark config
	config, err := benchmark.LoadBenchmarkConfig(configFile)
	if err != nil {
		return err
	}

	// Run benchmark
	runOpts := benchmark.RunOptions{
		Logger:       logger,
		Tools:        tools,
		ProviderName: provider,
		APIKeyValue:  apiKey,
		ModelName:    model,
	}
	results, err := benchmark.RunSingleIteration(ctx, config, runOpts)
	if err != nil {
		return fmt.Errorf("error running benchmark: %w", err)
	}

	// Update performance metrics
	if err := benchmark.UpdateToolPerformanceMetrics(tvm, results, model); err != nil {
		log.Printf("Warning: could not update performance metrics: %v", err)
	}

	// Output results
	if jsonOutput {
		benchmarkResult := benchmark.BenchmarkResult{
			Tools:   tools,
			Results: [][]benchmark.ModelTestResult{results},
		}
		jsonBytes, err := json.MarshalIndent(benchmarkResult, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal results: %w", err)
		}
		fmt.Println(string(jsonBytes))
	} else {
		benchmark.PrintSummary(results)
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

	historyOpts := benchmark.HistoryOptions{
		ToolVersionManager: tvm,
		JSONOutput:         jsonOutput,
	}
	return benchmark.ShowVersionHistory(historyOpts)
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

	// Load benchmark config
	config, err := benchmark.LoadBenchmarkConfig(configFile)
	if err != nil {
		return err
	}

	// Use improve-api-key if provided, otherwise fall back to main api-key
	apiKeyForImprovement := improveWithAPIKey
	if apiKeyForImprovement == "" {
		apiKeyForImprovement = improveAPIKey
	}

	// Run improvement cycle using the reusable package function
	opts := benchmark.ImprovementCycleOptions{
		Logger:              logger,
		ToolVersionManager:  tvm,
		MCPConfig:           mcpConfig,
		BenchmarkConfig:     config,
		Iterations:          iterations,
		TestProvider:        improveProvider,
		TestAPIKey:          improveAPIKey,
		TestModel:           improveModel,
		ImproveWith:         improveWith,
		ImproveWithProvider: improveWithProvider,
		ImproveAPIKey:       apiKeyForImprovement,
		TargetTools:         improveTool,
		JSONOutput:          jsonOutput,
	}

	// Wire dry-run flag into the improver (package-level switch)
	benchmark.DryRun = dryRun

	finalResults, err := benchmark.RunImprovementCycle(ctx, opts)
	if err != nil {
		return err
	}

	// Output results
	if jsonOutput {
		jsonBytes, err := json.MarshalIndent(finalResults, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal final results: %w", err)
		}
		fmt.Println(string(jsonBytes))
	} else {
		if len(finalResults.Results) > 0 {
			benchmark.PrintSummary(finalResults.Results[len(finalResults.Results)-1])
		}
	}

	return nil
}
