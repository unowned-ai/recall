package benchmark

import (
	"context"
	"fmt"
	"io/ioutil"
	"log"
	"time"

	"github.com/unowned-ai/recall/pkg/mcp"
	"github.com/unowned-ai/recall/pkg/providers"
	"gopkg.in/yaml.v3"
)

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

// LoadBenchmarkConfig loads a benchmark configuration file.
func LoadBenchmarkConfig(path string) (*BenchmarkConfig, error) {
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

// BenchmarkResult holds the result of a full benchmark run.
type BenchmarkResult struct {
	Tools   []mcp.ToolDefinition `json:"tools"`
	Results [][]ModelTestResult  `json:"results_per_iteration"`
}

// RunOptions provides configuration for running a benchmark iteration.
type RunOptions struct {
	Logger       *log.Logger
	Tools        []mcp.ToolDefinition
	ProviderName string
	APIKeyValue  string
	ModelName    string
	TargetTools  []string // Optional: filters scenarios to only these tools
}

// RunSingleIteration executes a single full benchmark run against a set of scenarios.
func RunSingleIteration(ctx context.Context, config *BenchmarkConfig, opts RunOptions) ([]ModelTestResult, error) {
	if opts.APIKeyValue == "" {
		return nil, fmt.Errorf("API key is required to run a benchmark iteration")
	}

	p, err := providers.NewModelProvider(opts.ProviderName, opts.APIKeyValue, opts.ModelName, opts.Logger)
	if err != nil {
		return nil, err
	}

	scenariosToRun := config.Scenarios

	// If specific tools are targeted for improvement, filter scenarios
	if len(opts.TargetTools) > 0 {
		targetTools := make(map[string]struct{})
		for _, t := range opts.TargetTools {
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

	fmt.Printf("Running benchmark for Provider: %s, Model: %s\n", opts.ProviderName, opts.ModelName)
	if len(opts.TargetTools) > 0 && len(scenariosToRun) > 0 {
		fmt.Printf("Found %d scenarios for specified tools: %v\n", len(scenariosToRun), opts.TargetTools)
	} else if len(opts.TargetTools) > 0 {
		fmt.Printf("Warning: No scenarios found for specified tools: %v\n", opts.TargetTools)
	}
	fmt.Println("==================================================")

	var results []ModelTestResult
	for _, s := range scenariosToRun {
		fmt.Printf("Running Scenario: %s\n", s.Name)
		fmt.Printf("User Message: '%s'\n", s.Message)

		start := time.Now()
		resp, err := p.CallWithTools(ctx, s.Message, opts.Tools)
		duration := time.Since(start)

		result := ModelTestResult{
			Provider:      p.Name(),
			Message:       s.Message,
			ExpectedTools: s.ExpectedTools,
			ResponseTime:  duration.Seconds(),
		}

		if err != nil {
			result.Error = err.Error()
			result.Success = false
			fmt.Printf("  Error: %v\n", err)
		} else {
			var actualTools []string
			for _, tc := range resp.ToolCalls {
				actualTools = append(actualTools, tc.Name)
			}

			result.ActualTools = actualTools
			result.Success = matchesExpected(actualTools, s.ExpectedTools)
			fmt.Printf("  Actual Tools: %v\n", actualTools)
			fmt.Printf("  Expected Tools: %v\n", s.ExpectedTools)
			fmt.Printf("  Success: %v\n", result.Success)
			if len(resp.ToolCalls) > 0 {
				for _, tc := range resp.ToolCalls {
					fmt.Printf("  Tool Call: %+v\n", tc)
				}
			}
		}
		fmt.Printf("  Response Time: %.2fs\n\n", duration.Seconds())
		results = append(results, result)
	}
	return results, nil
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

// PrintSummary prints a summary of the benchmark results to the console.
func PrintSummary(results []ModelTestResult) {
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
