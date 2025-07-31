package benchmark

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/unowned-ai/recall/pkg/providers"
)

// ImproveOptions provides configuration for the tool improvement process
type ImproveOptions struct {
	Logger              *log.Logger
	ToolVersionManager  *ToolVersionManager
	ImproveWith         string   // Model to use for generating improvements
	ImproveWithProvider string   // Provider for generating improvements
	ImproveAPIKey       string   // API key for generating improvements
	TargetTools         []string // Optional: specific tools to improve
	ModelName           string   // The model we're testing/improving for
}

// ImproveFailedTools attempts to improve tools that had failures
func ImproveFailedTools(ctx context.Context, results []ModelTestResult, opts ImproveOptions) error {
	if opts.ImproveAPIKey == "" {
		fmt.Println("Skipping improvement step: API key not provided for improve command.")
		return nil
	}

	improverProvider, err := providers.NewModelProvider(opts.ImproveWithProvider, opts.ImproveAPIKey, opts.ImproveWith, opts.Logger)
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
			shouldImprove := len(opts.TargetTools) == 0 // improve all by default
			if !shouldImprove {
				for _, targetTool := range opts.TargetTools {
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
		tool, exists := opts.ToolVersionManager.Tools[toolName]
		if !exists {
			continue
		}

		// Get the current description for the specific model
		currentDescription := tool.BaseDescription
		var currentPerformance *BenchmarkMetrics
		if modelHistory, ok := tool.ModelData[opts.ModelName]; ok && len(modelHistory.Versions) > 0 {
			currentVersion := modelHistory.Versions[modelHistory.CurrentVersion-1]
			currentDescription = currentVersion.Description
			currentPerformance = currentVersion.Performance
		}

		// Find a representative failed test for this tool
		var failedTest *ModelTestResult
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

		prompt := BuildImprovementPrompt(*failedTest, currentDescription)
		fmt.Printf("Attempting to improve tool '%s' for model '%s'...\n", toolName, opts.ModelName)

		resp, err := improverProvider.CallWithTools(ctx, prompt, nil)
		if err != nil {
			log.Printf("Failed to get suggestion for tool %s: %v", toolName, err)
			continue
		}

		newDescription := strings.Trim(resp.Message, "\" \n\t")
		if newDescription != "" && newDescription != currentDescription {
			fmt.Printf("  Original Description: %s\n", currentDescription)
			fmt.Printf("  Suggested Description: %s\n", newDescription)

			// Check if this tool consistently fails (success rate < 50%) before creating new version
			shouldImprove := true
			if currentPerformance != nil && currentPerformance.SuccessRate >= 0.5 {
				fmt.Printf("  Skipping improvement: Tool success rate %.1f%% is acceptable\n", currentPerformance.SuccessRate*100)
				shouldImprove = false
			}

			if shouldImprove {
				// Add new version
				reason := fmt.Sprintf("AI improvement for failed test: %s", failedTest.Message)
				if err := opts.ToolVersionManager.AddVersion(toolName, opts.ModelName, newDescription, reason); err != nil {
					log.Printf("Failed to add new version for tool %s: %v", toolName, err)
				}
			}
		}
	}

	return nil
}

// BuildImprovementPrompt creates a prompt for AI to improve tool descriptions
func BuildImprovementPrompt(failedTest ModelTestResult, originalDescription string) string {
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
- Keep it concise but broadly applicable (3-5 sentences ideal)
- Capture the essence, not specific phrases
- Build upon successful patterns from the history above

New Description:`, failedTest.Message, failedTest.ExpectedTools, failedTest.ActualTools, originalDescription, "%s", failedTest.Message)
}

// UpdateToolPerformanceMetrics calculates and updates performance metrics for tools
func UpdateToolPerformanceMetrics(tvm *ToolVersionManager, results []ModelTestResult, modelName string) error {
	toolMetrics := make(map[string]*BenchmarkMetrics)

	// Calculate metrics for each tool
	for _, result := range results {
		for _, expectedTool := range result.ExpectedTools {
			if _, exists := toolMetrics[expectedTool]; !exists {
				toolMetrics[expectedTool] = &BenchmarkMetrics{}
			}
		}

		for _, actualTool := range result.ActualTools {
			if _, exists := toolMetrics[actualTool]; !exists {
				toolMetrics[actualTool] = &BenchmarkMetrics{}
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

// ImprovementCycleOptions provides configuration for running improvement cycles
type ImprovementCycleOptions struct {
	Logger              *log.Logger
	ToolVersionManager  *ToolVersionManager
	MCPConfig           MCPServerConfig
	BenchmarkConfig     *BenchmarkConfig
	Iterations          int
	TestProvider        string   // Provider for testing
	TestAPIKey          string   // API key for testing
	TestModel           string   // Model being tested/improved
	ImproveWith         string   // Model used for generating improvements
	ImproveWithProvider string   // Provider for generating improvements
	ImproveAPIKey       string   // API key for generating improvements
	TargetTools         []string // Optional: specific tools to improve
	JSONOutput          bool     // Whether to suppress console output
}

// RunImprovementCycle executes multiple benchmark iterations with AI-powered improvements
func RunImprovementCycle(ctx context.Context, opts ImprovementCycleOptions) (*BenchmarkResult, error) {
	finalResults := &BenchmarkResult{
		Results: make([][]ModelTestResult, 0, opts.Iterations),
	}

	for i := 0; i < opts.Iterations; i++ {
		if !opts.JSONOutput && opts.Logger != nil {
			fmt.Printf("\n--- Starting Iteration %d/%d ---\n", i+1, opts.Iterations)
		}

		// Get current tools for this iteration
		tools := opts.ToolVersionManager.GetCurrentTools(opts.TestModel, opts.MCPConfig)

		// Run benchmark
		runOpts := RunOptions{
			Logger:       opts.Logger,
			Tools:        tools,
			ProviderName: opts.TestProvider,
			APIKeyValue:  opts.TestAPIKey,
			ModelName:    opts.TestModel,
			TargetTools:  opts.TargetTools,
		}

		iterationResults, err := RunSingleIteration(ctx, opts.BenchmarkConfig, runOpts)
		if err != nil {
			return nil, fmt.Errorf("error in iteration %d: %w", i+1, err)
		}
		finalResults.Results = append(finalResults.Results, iterationResults)

		// Update performance metrics
		if err := UpdateToolPerformanceMetrics(opts.ToolVersionManager, iterationResults, opts.TestModel); err != nil {
			if opts.Logger != nil {
				opts.Logger.Printf("Warning: could not update performance metrics: %v", err)
			}
		}

		// Check for failures and attempt to improve (if not last iteration)
		if i < opts.Iterations-1 {
			hasFailures := false
			for _, res := range iterationResults {
				if !res.Success {
					hasFailures = true
					break
				}
			}

			if hasFailures {
				if !opts.JSONOutput {
					fmt.Println("\n--- Analyzing failures and suggesting improvements ---")
				}

				improveOpts := ImproveOptions{
					Logger:              opts.Logger,
					ToolVersionManager:  opts.ToolVersionManager,
					ImproveWith:         opts.ImproveWith,
					ImproveWithProvider: opts.ImproveWithProvider,
					ImproveAPIKey:       opts.ImproveAPIKey,
					TargetTools:         opts.TargetTools,
					ModelName:           opts.TestModel,
				}

				if err := ImproveFailedTools(ctx, iterationResults, improveOpts); err != nil {
					if opts.Logger != nil {
						opts.Logger.Printf("Could not improve tools: %v", err)
					}
				}
			} else {
				if !opts.JSONOutput {
					fmt.Println("\n--- All tests passed! No improvements needed. ---")
				}
				break // Exit early if all tests pass
			}
		}
	}

	// Set final tools in result
	finalResults.Tools = opts.ToolVersionManager.GetCurrentTools(opts.TestModel, opts.MCPConfig)

	return finalResults, nil
}
