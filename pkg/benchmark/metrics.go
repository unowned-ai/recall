package benchmark

import "fmt"

// UpdateToolPerformanceMetrics calculates simple per-tool metrics and stores them
// on the current version for the specified model.
func UpdateToolPerformanceMetrics(tvm *ToolVersionManager, results []ModelTestResult, modelName string) error {
	// Aggregate per-tool counters
	type counters struct {
		totalTests       int
		successfulTests  int
		totalResponseSec float64
	}

	toolNameToCounters := make(map[string]*counters)

	// Collect tools that appear either as expected or actual to avoid missing keys
	for _, result := range results {
		for _, expectedTool := range result.ExpectedTools {
			if _, exists := toolNameToCounters[expectedTool]; !exists {
				toolNameToCounters[expectedTool] = &counters{}
			}
		}
		for _, actualTool := range result.ActualTools {
			if _, exists := toolNameToCounters[actualTool]; !exists {
				toolNameToCounters[actualTool] = &counters{}
			}
		}
	}

	// Populate counters for expected tools only (performance is defined vs. expectations)
	for toolName := range toolNameToCounters {
		c := toolNameToCounters[toolName]
		for _, result := range results {
			// Was this tool expected in this test?
			expectedHere := false
			for _, expected := range result.ExpectedTools {
				if expected == toolName {
					expectedHere = true
					break
				}
			}
			if !expectedHere {
				continue
			}

			c.totalTests++
			c.totalResponseSec += result.ResponseTime

			// Was it actually called?
			actuallyCalled := false
			for _, actual := range result.ActualTools {
				if actual == toolName {
					actuallyCalled = true
					break
				}
			}
			if actuallyCalled {
				c.successfulTests++
			}
		}
	}

	// Write metrics back to the version manager
	for toolName, c := range toolNameToCounters {
		if c.totalTests == 0 {
			// No expectation-based data for this tool; skip
			continue
		}
		metrics := BenchmarkMetrics{}
		metrics.SuccessRate = float64(c.successfulTests) / float64(c.totalTests)
		metrics.ResponseTime = c.totalResponseSec / float64(c.totalTests)
		// ActivationRate is not separately computed here; align with success for now
		metrics.ActivationRate = metrics.SuccessRate

		if err := tvm.UpdatePerformance(toolName, modelName, metrics); err != nil {
			// Do not fail entire update; report and continue
			// Note: callers may log this error
			_ = fmt.Errorf("update performance for %s failed: %v", toolName, err)
		}
	}
	return nil
}
