package benchmark

import (
	"context"
	"fmt"
	"log"
	"regexp"
	"strings"

	"github.com/unowned-ai/recall/pkg/providers"
)

// DryRun controls whether improvements are only previewed (no save)
var DryRun bool

// ToolImprover - Clean, simple, pattern-aware
type ToolImprover struct {
	provider providers.ModelProvider
	manager  *ToolVersionManager
}

// NewToolImprover creates a pattern-learning improver
func NewToolImprover(provider providers.ModelProvider, manager *ToolVersionManager) *ToolImprover {
	return &ToolImprover{provider: provider, manager: manager}
}

// Pattern represents what we learn from test results
type Pattern struct {
	Tool         string
	Success      int            // Successful activations
	Failed       int            // Missed activations
	FailedInputs []string       // What users said when it failed
	WorkingTerms map[string]int // Terms that appear in successful cases
}

// Learn from results and improve
func (ti *ToolImprover) Improve(ctx context.Context, results []ModelTestResult, modelName string, targetTools []string) error {
	// Learn patterns from test results
	patterns := ti.learnPatterns(results)

	// Improve failing tools
	for tool, pattern := range patterns {
		// Skip if not targeted
		if len(targetTools) > 0 && !contains(targetTools, tool) {
			continue
		}

		// Improve if failing
		if pattern.needsImprovement() {
			ti.improve(ctx, pattern, modelName)
		}
	}

	return nil
}

// Learn what works and what doesn't
func (ti *ToolImprover) learnPatterns(results []ModelTestResult) map[string]*Pattern {
	patterns := make(map[string]*Pattern)

	for _, result := range results {
		// Track each expected tool
		for _, tool := range result.ExpectedTools {
			if patterns[tool] == nil {
				patterns[tool] = &Pattern{
					Tool:         tool,
					WorkingTerms: make(map[string]int),
				}
			}

			p := patterns[tool]

			// Did it work?
			if contains(result.ActualTools, tool) {
				p.Success++
				// Learn from success
				ti.extractTerms(p.WorkingTerms, result.Message)
			} else {
				p.Failed++
				p.FailedInputs = append(p.FailedInputs, result.Message)
			}
		}
	}

	return patterns
}

// Extract important terms from input (stronger token learning)
func (ti *ToolImprover) extractTerms(terms map[string]int, input string) {
	tokens := tokenizeInput(input)

	// Unigrams
	for _, t := range tokens {
		terms[t]++
	}

	// Bigrams
	for i := 0; i+1 < len(tokens); i++ {
		bigram := tokens[i] + " " + tokens[i+1]
		terms[bigram]++
	}

	// Trigrams
	for i := 0; i+2 < len(tokens); i++ {
		trigram := tokens[i] + " " + tokens[i+1] + " " + tokens[i+2]
		terms[trigram]++
	}
}

var nonAlphaNum = regexp.MustCompile(`[^a-z0-9]+`)

// Minimal English stopword list to reduce noise in triggers
var stopwords = map[string]struct{}{
	"the": {}, "a": {}, "an": {}, "and": {}, "or": {}, "but": {}, "if": {}, "then": {}, "than": {},
	"on": {}, "in": {}, "of": {}, "to": {}, "for": {}, "with": {}, "by": {}, "about": {}, "into": {},
	"at": {}, "from": {}, "as": {}, "is": {}, "are": {}, "was": {}, "were": {}, "be": {}, "been": {}, "being": {},
	"do": {}, "does": {}, "did": {}, "doing": {}, "can": {}, "could": {}, "should": {}, "would": {},
	"you": {}, "your": {}, "yours": {}, "me": {}, "my": {}, "we": {}, "our": {}, "us": {},
	"this": {}, "that": {}, "these": {}, "those": {}, "it": {}, "its": {}, "i": {},
	"hi": {}, "hey": {}, "please": {}, "help": {}, "show": {}, "tell": {},
}

func tokenizeInput(input string) []string {
	lower := strings.ToLower(input)
	// Replace non-alphanumeric with spaces, collapse repeats
	cleaned := nonAlphaNum.ReplaceAllString(lower, " ")
	fields := strings.Fields(cleaned)
	tokens := make([]string, 0, len(fields))
	for _, w := range fields {
		if len(w) <= 2 { // skip tiny tokens
			continue
		}
		if _, stop := stopwords[w]; stop {
			continue
		}
		tokens = append(tokens, w)
	}
	return tokens
}

// Simple decision: improve if < 80% success
func (p *Pattern) needsImprovement() bool {
	total := p.Success + p.Failed
	if total < 2 {
		return false // Need data
	}

	successRate := float64(p.Success) / float64(total)
	return successRate < 0.8
}

// Generate and apply improvement
func (ti *ToolImprover) improve(ctx context.Context, pattern *Pattern, modelName string) {
	successRate := float64(pattern.Success) / float64(pattern.Success+pattern.Failed)
	fmt.Printf("🔧 Improving %s (%.0f%% success)\n", pattern.Tool, successRate*100)

	// Get current description
	tool, exists := ti.manager.Tools[pattern.Tool]
	if !exists {
		fmt.Printf("   ❌ Tool not found\n")
		return
	}

	currentDesc := ti.getDescription(tool, modelName)

	// Build prompt with patterns
	prompt := ti.buildPrompt(pattern, currentDesc)

	// Get improvement
	resp, err := ti.provider.CallWithTools(ctx, prompt, nil)
	if err != nil {
		fmt.Printf("   ❌ Failed: %v\n", err)
		return
	}

	newDesc := strings.TrimSpace(resp.Message)
	newDesc = sanitizeDescription(newDesc)
	if newDesc == "" || newDesc == currentDesc {
		fmt.Printf("   ⚠️  No change\n")
		return
	}

	// In dry-run mode, just preview the change and return without saving
	if DryRun {
		// Compact, helpful preview
		firstLine := newDesc
		if idx := strings.IndexByte(firstLine, '\n'); idx >= 0 {
			firstLine = firstLine[:idx]
		}
		// Show top terms and examples count
		topTerms := ti.getTopTerms(pattern.WorkingTerms, 5)
		fmt.Printf("   🔎 DRY-RUN: %s\n", firstLine)
		if len(topTerms) > 0 {
			fmt.Printf("   🔎 Triggers: %s\n", strings.Join(topTerms, ", "))
		}
		if len(pattern.FailedInputs) > 0 {
			fmt.Printf("   🔎 Covers %d failing examples\n", len(pattern.FailedInputs))
		}
		fmt.Printf("   🔎 Not saved (dry-run)\n")
		return
	}

	// Save
	reason := fmt.Sprintf("%.0f%% → better", successRate*100)
	if err := ti.manager.AddVersion(pattern.Tool, modelName, newDesc, reason); err != nil {
		fmt.Printf("   ❌ Save failed: %v\n", err)
		return
	}

	fmt.Printf("   ✅ Improved\n")
}

// Build improvement prompt using learned patterns
func (ti *ToolImprover) buildPrompt(pattern *Pattern, currentDesc string) string {
	// Get top working terms
	topTerms := ti.getTopTerms(pattern.WorkingTerms, 5)

	// Show failed examples
	examples := ""
	for i := 0; i < 3 && i < len(pattern.FailedInputs); i++ {
		examples += fmt.Sprintf("• \"%s\"\n", pattern.FailedInputs[i])
	}

	return fmt.Sprintf(`Improve this tool description for better activation.

TOOL: %s
CURRENT: "%s"

PERFORMANCE: %d/%d successful

TERMS THAT WORK:
%s

FAILED TO ACTIVATE FOR:
%s

Write a NEW description that:
1. Uses the EXACT terms that work
2. Covers the failed cases
3. Is direct and specific

Output ONLY the new description in this exact template (no extra text):
- MUST call when: <one sentence of clear triggers>
- Use when: "term1", "term2", "term3" (include working terms)
- Don’t use when: <boundaries to avoid false positives>
- Examples that SHOULD trigger: "example 1", "example 2" (cover failed examples)
- Anti-examples: "example 1", "example 2"
`,
		pattern.Tool,
		currentDesc,
		pattern.Success,
		pattern.Success+pattern.Failed,
		strings.Join(topTerms, ", "),
		examples)
}

// sanitizeDescription removes model preambles and ensures we keep the templated content
func sanitizeDescription(s string) string {
	s = strings.TrimSpace(s)
	// If the model added a preamble like "Here's an improved description...", strip lines until we hit a likely template start
	lines := strings.Split(s, "\n")
	start := 0
	for i, ln := range lines {
		l := strings.ToLower(strings.TrimSpace(ln))
		if strings.HasPrefix(l, "- must call when:") || strings.HasPrefix(l, "must call when") || strings.HasPrefix(l, "- use when:") || strings.HasPrefix(l, "always use when") {
			start = i
			break
		}
	}
	cleaned := strings.Join(lines[start:], "\n")
	return strings.TrimSpace(cleaned)
}

// Get most common terms
func (ti *ToolImprover) getTopTerms(terms map[string]int, n int) []string {
	// Simple selection of frequent terms
	var top []string

	for term, count := range terms {
		if count > 1 || len(top) < n {
			top = append(top, fmt.Sprintf("\"%s\"", term))
			if len(top) >= n {
				break
			}
		}
	}

	return top
}

// Get description for model
func (ti *ToolImprover) getDescription(tool *VersionedTool, modelName string) string {
	if modelData, exists := tool.ModelData[modelName]; exists && len(modelData.Versions) > 0 {
		return modelData.Versions[modelData.CurrentVersion-1].Description
	}
	return tool.BaseDescription
}

// Simple helper
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
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
		if !opts.JSONOutput {
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

		// If not the last iteration, attempt improvements on failures
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
				// Build improver using the "improve-with" provider/model
				improveProvider, err := providers.NewModelProvider(
					opts.ImproveWithProvider,
					opts.ImproveAPIKey,
					opts.ImproveWith,
					opts.Logger,
				)
				if err != nil {
					if opts.Logger != nil {
						opts.Logger.Printf("Could not initialize improvement provider: %v", err)
					}
				} else {
					improver := NewToolImprover(improveProvider, opts.ToolVersionManager)
					// Use target model for saving versions
					_ = improver.Improve(ctx, iterationResults, opts.TestModel, opts.TargetTools)
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
