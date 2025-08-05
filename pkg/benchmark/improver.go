package benchmark

import (
	"context"
	"fmt"
	"log"
	"math"
	"strings"
	"time"

	"github.com/unowned-ai/recall/pkg/providers"
)

// EnhancedBenchmarkMetrics - NEW enhanced metrics (added alongside existing BenchmarkMetrics)
type EnhancedBenchmarkMetrics struct {
	// Basic metrics (for compatibility)
	SuccessRate    float64 `json:"success_rate"`
	ResponseTime   float64 `json:"avg_response_time"`
	ActivationRate float64 `json:"activation_rate"`

	// Enhanced metrics (NEW)
	Precision        float64   `json:"precision"`
	Recall           float64   `json:"recall"`
	F1Score          float64   `json:"f1_score"`
	RecentTrend      float64   `json:"recent_trend"`
	StabilityScore   float64   `json:"stability_score"`
	TotalTests       int       `json:"total_tests"`
	TruePositives    int       `json:"true_positives"`
	FalsePositives   int       `json:"false_positives"`
	FalseNegatives   int       `json:"false_negatives"`
	LastImprovement  time.Time `json:"last_improvement"`
	ImprovementCount int       `json:"improvement_count"`
}

// ImproveOptions provides configuration for the tool improvement process
type ImproveOptions struct {
	Logger              *log.Logger
	ToolVersionManager  *ToolVersionManager
	ImproveWith         string   // Model to use for generating improvements
	ImproveWithProvider string   // Provider for generating improvements
	ImproveAPIKey       string   // API key for generating improvements
	TargetTools         []string // Optional: specific tools to improve
	ModelName           string   // The model we're testing/improving for
	UseEnhancedLogic    bool     // NEW: Whether to use enhanced improvement logic (default: true)
}

// ImproveFailedTools attempts to improve tools that had failures
// ENHANCED: Now uses smart decision logic to prevent version explosion
func ImproveFailedTools(ctx context.Context, results []ModelTestResult, opts ImproveOptions) error {
	// Default to enhanced logic
	if !opts.UseEnhancedLogic {
		fmt.Println("Using legacy improvement logic")
		return improveFailedToolsLegacy(ctx, results, opts)
	}

	if opts.ImproveAPIKey == "" {
		fmt.Println("Skipping improvement step: API key not provided for improve command.")
		return nil
	}

	improverProvider, err := providers.NewModelProvider(opts.ImproveWithProvider, opts.ImproveAPIKey, opts.ImproveWith, opts.Logger)
	if err != nil {
		return err
	}

	// Calculate enhanced metrics for all tools
	toolMetrics := calculateAllEnhancedMetrics(results, opts.ToolVersionManager, opts.ModelName)

	// Improve each tool using smart decision logic
	improvementCount := 0
	for toolName, metrics := range toolMetrics {
		// Smart improvement decision
		shouldImprove, reason := shouldImproveToolEnhanced(toolName, metrics, opts.ToolVersionManager, opts.ModelName, opts.TargetTools)

		if !shouldImprove {
			if opts.Logger != nil {
				opts.Logger.Printf("Skipping tool '%s': %s", toolName, reason)
			}
			continue
		}

		// Find representative failed test
		failedTest := findFailedTestForTool(toolName, results)
		if failedTest == nil {
			continue
		}

		fmt.Printf("🔧 Improving tool '%s' for model '%s'...\n", toolName, opts.ModelName)
		fmt.Printf("   Reason: %s\n", reason)
		fmt.Printf("   Metrics: F1=%.3f, Precision=%.3f, Recall=%.3f, Trend=%.3f\n",
			metrics.F1Score, metrics.Precision, metrics.Recall, metrics.RecentTrend)

		// Get current tool description
		tool, exists := opts.ToolVersionManager.Tools[toolName]
		if !exists {
			continue
		}

		currentDescription := tool.BaseDescription
		if modelHistory, ok := tool.ModelData[opts.ModelName]; ok && len(modelHistory.Versions) > 0 {
			currentVersion := modelHistory.Versions[modelHistory.CurrentVersion-1]
			currentDescription = currentVersion.Description
		}

		// Create enhanced improvement prompt
		prompt := buildEnhancedPrompt(*failedTest, currentDescription, tool, opts.ModelName, metrics)

		fmt.Printf("improve provider: %v\n", improverProvider.Name())

		resp, err := improverProvider.CallWithTools(ctx, prompt, nil)
		if err != nil {
			fmt.Printf("   ❌ AI call failed: %v\n", err)
			log.Printf("Failed to get suggestion for tool %s: %v", toolName, err)
			continue
		}

		fmt.Printf("   📝 AI response: '%s'\n", resp.Message)
		newDescription := strings.Trim(resp.Message, "\" \n\t")
		fmt.Printf("   ✂️ Trimmed: '%s'\n", newDescription)

		if newDescription != "" && newDescription != currentDescription {
			fmt.Printf("   ✅ New description generated!\n")
			fmt.Printf("   Original: %s\n", currentDescription)
			fmt.Printf("   Improved: %s\n", newDescription)

			// Add new version with detailed reason
			improvementReason := fmt.Sprintf("Enhanced AI improvement - %s", reason)
			if err := opts.ToolVersionManager.AddVersion(toolName, opts.ModelName, newDescription, improvementReason); err != nil {
				fmt.Printf("   ❌ Failed to save version: %v\n", err)
				log.Printf("Failed to add new version for tool %s: %v", toolName, err)
			} else {
				fmt.Printf("   💾 Version saved successfully!\n")
				improvementCount++
			}
		} else {
			fmt.Printf("   ⚠️ No improvement: empty='%t', same='%t'\n",
				newDescription == "", newDescription == currentDescription)
		}
	}

	if improvementCount > 0 {
		fmt.Printf("✅ Enhanced improvement complete: %d tools improved\n", improvementCount)
	} else {
		fmt.Println("✅ No tools required improvement based on enhanced metrics")
	}

	return nil
}

// Legacy improvement function (preserves original behavior)
func improveFailedToolsLegacy(ctx context.Context, results []ModelTestResult, opts ImproveOptions) error {
	if opts.ImproveAPIKey == "" {
		fmt.Println("Skipping improvement step: API key not provided for improve command.")
		return nil
	}

	improverProvider, err := providers.NewModelProvider(opts.ImproveWithProvider, opts.ImproveAPIKey, opts.ImproveWith, opts.Logger)
	if err != nil {
		return err
	}

	// Find tools that need improvement (ORIGINAL LOGIC)
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

	// Improve each tool (ORIGINAL LOGIC)
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

		prompt := BuildImprovementPrompt(*failedTest, currentDescription, tool, opts.ModelName)
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

// NEW: Calculate enhanced metrics for all tools
func calculateAllEnhancedMetrics(results []ModelTestResult, tvm *ToolVersionManager, modelName string) map[string]*EnhancedBenchmarkMetrics {
	toolMetrics := make(map[string]*EnhancedBenchmarkMetrics)

	// Initialize metrics for all tools mentioned
	for _, result := range results {
		for _, expectedTool := range result.ExpectedTools {
			if _, exists := toolMetrics[expectedTool]; !exists {
				toolMetrics[expectedTool] = &EnhancedBenchmarkMetrics{}
			}
		}
		for _, actualTool := range result.ActualTools {
			if _, exists := toolMetrics[actualTool]; !exists {
				toolMetrics[actualTool] = &EnhancedBenchmarkMetrics{}
			}
		}
	}

	// Calculate enhanced metrics for each tool
	for toolName := range toolMetrics {
		metrics := calculateToolEnhancedMetrics(toolName, results)

		// Add improvement history data
		if tool, exists := tvm.Tools[toolName]; exists {
			if modelHistory, hasHistory := tool.ModelData[modelName]; hasHistory && len(modelHistory.Versions) > 0 {
				lastVersion := modelHistory.Versions[len(modelHistory.Versions)-1]
				metrics.LastImprovement = lastVersion.Timestamp
				metrics.ImprovementCount = len(modelHistory.Versions)
			}
		}

		toolMetrics[toolName] = metrics
	}

	return toolMetrics
}

// NEW: Calculate enhanced metrics for a specific tool
func calculateToolEnhancedMetrics(toolName string, results []ModelTestResult) *EnhancedBenchmarkMetrics {
	metrics := &EnhancedBenchmarkMetrics{}

	var totalResponseTime float64
	var recentResults []bool

	for _, result := range results {
		expectedHere := containsString(result.ExpectedTools, toolName)
		actuallyCalledHere := containsString(result.ActualTools, toolName)

		if expectedHere {
			metrics.TotalTests++
			totalResponseTime += result.ResponseTime

			if actuallyCalledHere {
				metrics.TruePositives++
				recentResults = append(recentResults, true)
			} else {
				metrics.FalseNegatives++
				recentResults = append(recentResults, false)
			}
		} else if actuallyCalledHere {
			metrics.FalsePositives++
		}
	}

	// Keep only last 10 results for trend analysis
	if len(recentResults) > 10 {
		recentResults = recentResults[len(recentResults)-10:]
	}

	// Calculate basic metrics
	if metrics.TotalTests > 0 {
		metrics.SuccessRate = float64(metrics.TruePositives) / float64(metrics.TotalTests)
		metrics.ResponseTime = totalResponseTime / float64(metrics.TotalTests)
		metrics.ActivationRate = metrics.SuccessRate
	}

	// Calculate precision and recall
	if metrics.TruePositives+metrics.FalseNegatives > 0 {
		metrics.Recall = float64(metrics.TruePositives) / float64(metrics.TruePositives+metrics.FalseNegatives)
	}

	if metrics.TruePositives+metrics.FalsePositives > 0 {
		metrics.Precision = float64(metrics.TruePositives) / float64(metrics.TruePositives+metrics.FalsePositives)
	}

	// F1 Score
	if metrics.Precision+metrics.Recall > 0 {
		metrics.F1Score = 2 * (metrics.Precision * metrics.Recall) / (metrics.Precision + metrics.Recall)
	}

	// Trend and stability
	metrics.RecentTrend = calculatePerformanceTrend(recentResults)
	metrics.StabilityScore = calculatePerformanceStability(recentResults)

	return metrics
}

// NEW: Smart improvement decision with 5-gate system
func shouldImproveToolEnhanced(toolName string, metrics *EnhancedBenchmarkMetrics, tvm *ToolVersionManager, modelName string, targetTools []string) (bool, string) {
	// FORCE improvement for get_memory_overview tool due to 0% activation (HIGHEST PRIORITY)
	if toolName == "get_memory_overview_run_in_start_of_interaction" && metrics.SuccessRate == 0.0 {
		return true, "FORCING improvement for 0% activation rate"
	}

	// Gate 1: Target tool filter
	if len(targetTools) > 0 {
		found := false
		for _, target := range targetTools {
			if target == toolName {
				found = true
				break
			}
		}
		if !found {
			return false, "not in target tool list"
		}
	}

	// Gate 2: Data sufficiency
	if metrics.TotalTests < 2 {
		return false, "insufficient test data (need at least 2 tests)"
	}

	// Gate 3: Time and version limits
	if metrics.ImprovementCount > 0 {
		if time.Since(metrics.LastImprovement) < 24*time.Hour {
			return false, "too recent improvement (less than 24 hours)"
		}

		if metrics.ImprovementCount > 10 {
			return false, "too many improvement attempts (more than 10 versions)"
		}
	}

	// Gate 4: Don't improve if trending upward
	if metrics.RecentTrend > 0.2 {
		return false, "performance is improving naturally"
	}

	// Gate 5: Multi-criteria failure detection
	reasons := []string{}

	if metrics.SuccessRate < 0.6 {
		reasons = append(reasons, fmt.Sprintf("low success rate (%.1f%%)", metrics.SuccessRate*100))
	}

	if metrics.F1Score < 0.5 {
		reasons = append(reasons, fmt.Sprintf("low F1 score (%.3f)", metrics.F1Score))
	}

	if metrics.Precision < 0.4 && metrics.FalsePositives > 2 {
		reasons = append(reasons, fmt.Sprintf("too many false positives (%d)", metrics.FalsePositives))
	}

	if metrics.RecentTrend < -0.3 {
		reasons = append(reasons, "declining performance trend")
	}

	if metrics.StabilityScore < 0.3 {
		reasons = append(reasons, "inconsistent performance")
	}

	if len(reasons) > 0 {
		return true, strings.Join(reasons, ", ")
	}

	return false, "performance metrics are acceptable"
}

// NEW: Enhanced improvement prompt with failure analysis
func buildEnhancedPrompt(failedTest ModelTestResult, originalDescription string, tool *VersionedTool, modelName string, metrics *EnhancedBenchmarkMetrics) string {

	fmt.Printf("Building enhanced prompt for tool %s\n", tool.Name)
	historyData := buildHistorySection(tool, modelName)
	fmt.Printf("History data: %s\n", historyData)
	failureAnalysis := buildFailureAnalysisSection(metrics, failedTest)
	fmt.Printf("Failure analysis: %s\n", failureAnalysis)

	return fmt.Sprintf(`You are an expert at writing tool descriptions that help AI models choose the right tools. A tool has performance issues and needs improvement.

**Context: Memory/Knowledge Management System**
The tools are part of a personal memory system called "Recall" that helps users store, search, and retrieve conversations and information.

**Failed Test Case:**
- User Message: "%s"
- Expected Tool: %v (should have been called)
- Actual Tools Called: %v (what was actually called instead)

**Current Tool Description:**
"%s"

**PERFORMANCE ANALYSIS:**
%s

**IMPROVEMENT HISTORY:**
%s

**Your Task:**
Write a DIRECT, ACTION-ORIENTED tool description that FORCES AI to call this tool. Focus on:

1. **Mandatory Activation**: Use "MUST call", "REQUIRED for", "ALWAYS activate when"
2. **Explicit Triggers**: List exact scenarios that demand this tool
3. **Direct Commands**: Tell AI exactly when to call it (not when it "naturally" activates)
4. **Success Patterns**: Copy language from high-performing versions

**CRITICAL REQUIREMENTS:**
- Start with "CALL THIS TOOL when..." or "REQUIRED for..."
- Use commanding language: "Must", "Always", "Required", "Essential"
- List specific triggers: "conversation start", "capability questions", "greeting messages"
- Avoid poetic descriptions - use direct instructions

**Rules:**
- ONLY return the new description text
- No preamble or explanation
- Use COMMANDING, DIRECTIVE language
- Make activation sound MANDATORY, not optional

New Description:`,
		failedTest.Message,
		failedTest.ExpectedTools,
		failedTest.ActualTools,
		originalDescription,
		failureAnalysis,
		historyData)
}

// NEW: Build failure analysis section
func buildFailureAnalysisSection(metrics *EnhancedBenchmarkMetrics, failedTest ModelTestResult) string {
	var analysis strings.Builder

	analysis.WriteString("=== PERFORMANCE METRICS ===\n")
	analysis.WriteString(fmt.Sprintf("Success Rate: %.1f%% (%d correct out of %d tests)\n",
		metrics.SuccessRate*100, metrics.TruePositives, metrics.TotalTests))
	analysis.WriteString(fmt.Sprintf("Precision: %.3f, Recall: %.3f, F1 Score: %.3f\n",
		metrics.Precision, metrics.Recall, metrics.F1Score))
	analysis.WriteString(fmt.Sprintf("Performance Trend: %.3f, Stability: %.3f\n",
		metrics.RecentTrend, metrics.StabilityScore))

	analysis.WriteString("\n=== PRIMARY ISSUES ===\n")
	if metrics.FalsePositives > 2 {
		analysis.WriteString("- EXCESSIVE FALSE POSITIVES: Tool called too often incorrectly\n")
	}
	if metrics.FalseNegatives > 2 {
		analysis.WriteString("- MISSED OPPORTUNITIES: Tool not called when needed\n")
	}
	if metrics.StabilityScore < 0.3 {
		analysis.WriteString("- INCONSISTENT PERFORMANCE: Unpredictable behavior\n")
	}

	return analysis.String()
}

// NEW: Calculate performance trend
func calculatePerformanceTrend(results []bool) float64 {
	if len(results) < 3 {
		return 0.0
	}

	mid := len(results) / 2
	early := results[:mid]
	recent := results[mid:]

	earlySuccess := float64(countTrueResults(early)) / float64(len(early))
	recentSuccess := float64(countTrueResults(recent)) / float64(len(recent))

	return math.Max(-1.0, math.Min(1.0, recentSuccess-earlySuccess))
}

// NEW: Calculate performance stability
func calculatePerformanceStability(results []bool) float64 {
	if len(results) < 2 {
		return 1.0
	}

	successRate := float64(countTrueResults(results)) / float64(len(results))
	variance := 0.0

	for _, result := range results {
		val := 0.0
		if result {
			val = 1.0
		}
		variance += math.Pow(val-successRate, 2)
	}
	variance /= float64(len(results))

	return math.Max(0.0, 1.0-variance*4)
}

// NEW: Find failed test for specific tool
func findFailedTestForTool(toolName string, results []ModelTestResult) *ModelTestResult {
	for i := range results {
		if !results[i].Success {
			for _, expected := range results[i].ExpectedTools {
				if expected == toolName {
					return &results[i]
				}
			}
		}
	}
	return nil
}

// NEW: Count true results
func countTrueResults(results []bool) int {
	count := 0
	for _, r := range results {
		if r {
			count++
		}
	}
	return count
}

// NEW: Enhanced performance metrics update (alongside existing function)
func UpdateToolPerformanceMetricsEnhanced(tvm *ToolVersionManager, results []ModelTestResult, modelName string) error {
	toolMetrics := calculateAllEnhancedMetrics(results, tvm, modelName)

	// Update performance using existing manager methods (maintains compatibility)
	for toolName, enhancedMetrics := range toolMetrics {
		// Convert enhanced metrics to simple metrics for manager compatibility
		simpleMetrics := BenchmarkMetrics{
			SuccessRate:    enhancedMetrics.SuccessRate,
			ResponseTime:   enhancedMetrics.ResponseTime,
			ActivationRate: enhancedMetrics.ActivationRate,
		}

		if err := tvm.UpdatePerformance(toolName, modelName, simpleMetrics); err != nil {
			return fmt.Errorf("failed to update performance for %s: %w", toolName, err)
		}
	}

	return nil
}

// Helper function (exists in your code, keeping for completeness)
func containsString(slice []string, str string) bool {
	for _, s := range slice {
		if s == str {
			return true
		}
	}
	return false
}

// BuildImprovementPrompt creates a prompt for AI to improve tool descriptions
func BuildImprovementPrompt(failedTest ModelTestResult, originalDescription string, tool *VersionedTool, currentModelName string) string {
	// Build improvement history from all models and versions
	historyData := buildHistorySection(tool, currentModelName)

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

**CRITICAL: Tool Activation Requirements**
AI models need EXPLICIT COMMANDS to call tools. Write descriptions that tell the AI WHEN and HOW to activate:

- Use DIRECT action words: "Call this tool when...", "Activate immediately if...", "Must be used for..."
- Include SPECIFIC trigger scenarios: "At conversation start", "When user asks about capabilities"
- Add EXPLICIT instructions: "This tool should be the FIRST action taken when..."
- Avoid poetic language - use clear, actionable directives

**Critical Guidelines:**
- DON'T use vague language like "naturally activates" or "seamlessly connects"
- DO use direct commands: "ALWAYS call this tool first when starting a conversation"
- Include explicit triggers: "Required for: greetings, capability questions, system overviews"
- Make it sound MANDATORY, not optional: "Must be called", "Required", "Essential first step"
- Focus on IMMEDIATE activation cues, not abstract benefits
- LEARN from the performance history - what worked well in previous versions?

**Rules:**
- ONLY return the new description text
- No preamble, explanation, or additional commentary
- Use DIRECT, ACTIONABLE language (not poetic descriptions)
- Include explicit "Call this tool when..." statements
- Make activation sound MANDATORY for specific scenarios

New Description:`, failedTest.Message, failedTest.ExpectedTools, failedTest.ActualTools, originalDescription, historyData, failedTest.Message)
}

// buildHistorySection creates a comprehensive history section showing all model versions and their performance
func buildHistorySection(tool *VersionedTool, currentModelName string) string {
	if len(tool.ModelData) == 0 {
		return "No previous versions or performance data available."
	}

	var historyBuilder strings.Builder
	historyBuilder.WriteString("=== VERSION HISTORY ACROSS ALL MODELS ===\n\n")

	// Show successful descriptions from other models first
	for modelName, modelHistory := range tool.ModelData {
		if modelName == currentModelName {
			continue // Skip current model, we'll show it last
		}

		if len(modelHistory.Versions) == 0 {
			continue
		}

		historyBuilder.WriteString(fmt.Sprintf("MODEL: %s\n", modelName))

		// Find the best performing version for this model
		var bestVersion *ToolVersion
		bestSuccessRate := 0.0
		for _, version := range modelHistory.Versions {
			if version.Performance != nil && version.Performance.SuccessRate > bestSuccessRate {
				bestSuccessRate = version.Performance.SuccessRate
				bestVersion = &version
			}
		}

		if bestVersion != nil && bestVersion.Performance != nil {
			historyBuilder.WriteString(fmt.Sprintf("  BEST VERSION (v%d): %.1f%% success rate\n",
				bestVersion.Version, bestVersion.Performance.SuccessRate*100))
			historyBuilder.WriteString(fmt.Sprintf("  Description: \"%s\"\n", bestVersion.Description))
			historyBuilder.WriteString(fmt.Sprintf("  Change Reason: %s\n", bestVersion.ChangeReason))
			historyBuilder.WriteString(fmt.Sprintf("  Avg Response Time: %.2fs\n\n", bestVersion.Performance.ResponseTime))
		}
	}

	// Now show current model's history
	if modelHistory, exists := tool.ModelData[currentModelName]; exists && len(modelHistory.Versions) > 0 {
		historyBuilder.WriteString(fmt.Sprintf("CURRENT MODEL: %s\n", currentModelName))
		historyBuilder.WriteString("Version History (most recent first):\n")

		// Show versions in reverse order (newest first)
		for i := len(modelHistory.Versions) - 1; i >= 0; i-- {
			version := modelHistory.Versions[i]
			marker := "  "
			if version.Version == modelHistory.CurrentVersion {
				marker = "→ " // Current version
			}

			historyBuilder.WriteString(fmt.Sprintf("%sv%d (%s):\n", marker, version.Version, version.Timestamp.Format("2006-01-02 15:04")))
			historyBuilder.WriteString(fmt.Sprintf("     Description: \"%s\"\n", version.Description))
			historyBuilder.WriteString(fmt.Sprintf("     Reason: %s\n", version.ChangeReason))

			if version.Performance != nil {
				historyBuilder.WriteString(fmt.Sprintf("     Performance: %.1f%% success, %.2fs avg response\n",
					version.Performance.SuccessRate*100, version.Performance.ResponseTime))
			} else {
				historyBuilder.WriteString("     Performance: Not yet measured\n")
			}
			historyBuilder.WriteString("\n")
		}
	}

	historyBuilder.WriteString("=== KEY INSIGHTS ===\n")
	historyBuilder.WriteString("- Look for patterns in successful descriptions across different models\n")
	historyBuilder.WriteString("- Consider what worked well in high-performing versions\n")
	historyBuilder.WriteString("- Avoid repeating patterns that led to poor performance\n")
	historyBuilder.WriteString("- Build upon successful elements while addressing current failure\n\n")

	return historyBuilder.String()
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

		// Update performance metrics (use enhanced version by default)
		if err := UpdateToolPerformanceMetricsEnhanced(opts.ToolVersionManager, iterationResults, opts.TestModel); err != nil {
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
					UseEnhancedLogic:    true, // NEW: Use enhanced logic by default
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
