package benchmark

import (
	"encoding/json"
	"fmt"
)

// HistoryOptions provides configuration for displaying version history
type HistoryOptions struct {
	ToolVersionManager *ToolVersionManager
	ToolName           string // Optional: show history for specific tool
	ShowPerformance    bool   // Show performance metrics
	JSONOutput         bool   // Output in JSON format
}

// ShowVersionHistory displays tool version history
func ShowVersionHistory(opts HistoryOptions) error {
	if opts.ToolName != "" {
		// Show history for specific tool
		return showToolHistoryForTool(opts)
	}

	// Show summary for all tools
	if opts.JSONOutput {
		jsonBytes, err := json.MarshalIndent(opts.ToolVersionManager.Tools, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal tool versions: %w", err)
		}
		fmt.Println(string(jsonBytes))
	} else {
		fmt.Println("=== Tool Version History ===")
		for name, tool := range opts.ToolVersionManager.Tools {
			fmt.Printf("\n🔧 %s\n", name)
			fmt.Printf("   Base Description: %s\n", tool.BaseDescription)

			if len(tool.ModelData) == 0 {
				fmt.Println("   No model-specific versions yet.")
				continue
			}

			for modelName, history := range tool.ModelData {
				fmt.Printf("   └── Model: %s (Current: v%d, Total: %d versions)\n",
					modelName, history.CurrentVersion, len(history.Versions))

				if opts.ShowPerformance && len(history.Versions) > 0 {
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
func showToolHistoryForTool(opts HistoryOptions) error {
	tool, exists := opts.ToolVersionManager.Tools[opts.ToolName]
	if !exists {
		return fmt.Errorf("tool %s not found", opts.ToolName)
	}

	if opts.JSONOutput {
		jsonBytes, err := json.MarshalIndent(tool, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal tool history: %w", err)
		}
		fmt.Println(string(jsonBytes))
		return nil
	}

	fmt.Printf("=== History for %s ===\n", opts.ToolName)
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

			if opts.ShowPerformance && version.Performance != nil {
				fmt.Printf("   Performance: %.1f%% success, %.2fs avg response\n",
					version.Performance.SuccessRate*100,
					version.Performance.ResponseTime)
			}
		}
	}

	return nil
}
