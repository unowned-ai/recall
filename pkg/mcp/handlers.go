package mcp

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/unowned-ai/recall/pkg/memories"
)

const DefaultJournalName = "memory"

// ---- Helper to find tool definitions ----
func getToolDef(name string) *ToolDefinition {
	for _, tool := range DefaultToolRegistry {
		if tool.Name == name {
			return &tool
		}
	}
	return nil
}

// ---- Memory Overview Tool ----
const GetMemoryOverviewToolName = "get_memory_overview_run_in_start_of_interaction"

const memoryOverviewPreamble = `Your conversational memory is now connected, providing access to all previous interactions and stored context.

Available memory spaces (Journals) for this user:`

const memoryOverviewPostamble = `
These memory spaces help maintain continuity across all conversations. You can now naturally reference past discussions, build upon previous work, and provide contextually-aware responses throughout this session.`

// GetMemoryOverviewResult defines the structure of the result returned by GetMemoryOverview.
type GetMemoryOverviewResult struct {
	Preamble  string             `json:"preamble"`
	Journals  []memories.Journal `json:"journals"`
	Postamble string             `json:"postamble"`
}

// RegisterMemoryOverviewTool registers the GetMemoryOverview tool with the MCP server,
// following the existing pattern in this file.
func RegisterMemoryOverviewTool(s *server.MCPServer, db *sql.DB) {
	def := getToolDef(GetMemoryOverviewToolName)
	if def == nil {
		return // Or panic, depending on desired strictness
	}
	tool := mcp.NewTool(def.Name, mcp.WithDescription(def.Description))
	s.AddTool(tool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		journals, err := memories.ListJournals(ctx, db, false)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to list journals: %s", err.Error())), nil
		}

		result := GetMemoryOverviewResult{
			Preamble:  memoryOverviewPreamble,
			Journals:  journals,
			Postamble: memoryOverviewPostamble,
		}

		resultJSON, err := json.Marshal(result)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to marshal result to JSON: %s", err.Error())), nil
		}

		return mcp.NewToolResultText(string(resultJSON)), nil
	})
}

// RegisterPingTool registers a minimal health-check tool.
// This tool can be used to verify the Recall MCP server is alive and responsive,
// ensuring that the conversational memory and context management features are available.
func RegisterPingTool(s *server.MCPServer) {
	def := getToolDef("ping")
	if def == nil {
		return
	}
	pingTool := mcp.NewTool(def.Name, mcp.WithDescription(def.Description))
	s.AddTool(pingTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return mcp.NewToolResultText("pong_recall"), nil
	})
}

// RegisterCreateJournalTool registers the create_journal tool.
// Journals are fundamental for organizing your thoughts, project contexts, and conversation histories.
// Use this tool to create new journals to better structure and manage your recallable information.
func RegisterCreateJournalTool(s *server.MCPServer, db *sql.DB) {
	def := getToolDef("create_journal")
	if def == nil {
		return
	}
	tool := mcp.NewTool(
		def.Name,
		mcp.WithDescription(def.Description),
		mcp.WithString("name", mcp.Required(), mcp.Description("A meaningful name that captures the essence of what this memory space will contain.")),
		mcp.WithString("description", mcp.Description("Additional context about this memory space's purpose, helping future interactions understand its scope.")),
	)
	s.AddTool(tool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		name, _ := request.Params.Arguments["name"].(string)
		if strings.TrimSpace(name) == "" {
			return mcp.NewToolResultError("'name' parameter is required and must be non-empty"), nil
		}
		desc, _ := request.Params.Arguments["description"].(string)

		journal, err := memories.CreateJournal(ctx, db, name, desc)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to create journal: %v", err)), nil
		}
		b, _ := json.Marshal(journal)
		return mcp.NewToolResultText(string(b)), nil
	})
}

// RegisterListJournalsTool lists all journals (active & inactive).
// Useful for getting an overview of all your structured memory spaces.
// Call this to see where your information is organized and to decide where to store or retrieve context.
func RegisterListJournalsTool(s *server.MCPServer, db *sql.DB) {
	def := getToolDef("list_journals")
	if def == nil {
		return
	}
	tool := mcp.NewTool(def.Name, mcp.WithDescription(def.Description))
	s.AddTool(tool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		journals, err := memories.ListJournals(ctx, db, false)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to list journals: %v", err)), nil
		}
		if len(journals) == 0 {
			return mcp.NewToolResultText("[]"), nil
		}
		b, _ := json.Marshal(journals)
		return mcp.NewToolResultText(string(b)), nil
	})
}

// RegisterGetJournalTool retrieves a journal by name.
// Use this to get details about a specific journal, like its description, to understand its purpose for storing memories or context.
func RegisterGetJournalTool(s *server.MCPServer, db *sql.DB) {
	def := getToolDef("get_journal")
	if def == nil {
		return
	}
	tool := mcp.NewTool(
		def.Name,
		mcp.WithDescription(def.Description),
		mcp.WithString("name", mcp.Required(), mcp.Description("The memory space to explore in more detail.")),
	)
	s.AddTool(tool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		name, _ := request.Params.Arguments["name"].(string)
		if strings.TrimSpace(name) == "" {
			return mcp.NewToolResultError("'name' parameter is required"), nil
		}
		j, err := getJournalByName(ctx, db, name)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Error retrieving journal '%s': %v", name, err)), nil
		}
		if j == nil {
			return mcp.NewToolResultError(fmt.Sprintf("Journal '%s' not found", name)), nil
		}
		b, _ := json.Marshal(j)
		return mcp.NewToolResultText(string(b)), nil
	})
}

// RegisterUpdateJournalTool updates journal metadata.
// Keep your memory organization up-to-date by renaming journals, updating their descriptions, or changing their active status.
// This helps in maintaining a clear and relevant structure for your contextual information.
func RegisterUpdateJournalTool(s *server.MCPServer, db *sql.DB) {
	def := getToolDef("update_journal")
	if def == nil {
		return
	}
	tool := mcp.NewTool(
		def.Name,
		mcp.WithDescription(def.Description),
		mcp.WithString("name", mcp.Required(), mcp.Description("The memory space to evolve.")),
		mcp.WithString("new_name", mcp.Description("A new name if the space's purpose has shifted.")),
		mcp.WithString("description", mcp.Description("Updated context to reflect the space's current focus.")),
		mcp.WithBoolean("active", mcp.Description("Whether this space is actively used or archived for historical reference.")),
	)
	s.AddTool(tool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		name, _ := request.Params.Arguments["name"].(string)
		if strings.TrimSpace(name) == "" {
			return mcp.NewToolResultError("'name' parameter is required"), nil
		}
		currentJournal, err := getJournalByName(ctx, db, name)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Error retrieving journal: %v", err)), nil
		}
		if currentJournal == nil {
			return mcp.NewToolResultError(fmt.Sprintf("Journal '%s' not found", name)), nil
		}
		newNameVal, _ := request.Params.Arguments["new_name"].(string)
		if strings.TrimSpace(newNameVal) == "" {
			newNameVal = currentJournal.Name
		}
		newDescVal, _ := request.Params.Arguments["description"].(string)
		if newDescVal == "" {
			newDescVal = currentJournal.Description
		}
		activeVal := currentJournal.Active
		if av, ok := request.Params.Arguments["active"].(bool); ok {
			activeVal = av
		}
		updated, err := memories.UpdateJournal(ctx, db, currentJournal.ID, newNameVal, newDescVal, activeVal)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to update journal: %v", err)), nil
		}
		b, _ := json.Marshal(updated)
		return mcp.NewToolResultText(string(b)), nil
	})
}

// RegisterDeleteJournalTool deletes a journal by name (except the default).
// Use this to remove journals that are no longer relevant, helping to keep your memory space clean and focused.
// Note: Deleting a journal also deletes all its entries (memories/context snippets).
func RegisterDeleteJournalTool(s *server.MCPServer, db *sql.DB) {
	def := getToolDef("delete_journal")
	if def == nil {
		return
	}
	tool := mcp.NewTool(
		def.Name,
		mcp.WithDescription(def.Description),
		mcp.WithString("name", mcp.Required(), mcp.Description("The memory space to retire, along with all its contained memories.")),
	)
	s.AddTool(tool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		name, _ := request.Params.Arguments["name"].(string)
		if strings.TrimSpace(name) == "" {
			return mcp.NewToolResultError("'name' parameter is required"), nil
		}
		if name == DefaultJournalName {
			return mcp.NewToolResultError(fmt.Sprintf("Deleting the default journal '%s' is not allowed", DefaultJournalName)), nil
		}
		j, err := getJournalByName(ctx, db, name)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Error retrieving journal: %v", err)), nil
		}
		if j == nil {
			return mcp.NewToolResultText(fmt.Sprintf("Journal '%s' not found, nothing to delete.", name)), nil
		}
		if err := memories.DeleteJournal(ctx, db, j.ID); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to delete journal: %v", err)), nil
		}
		return mcp.NewToolResultText(fmt.Sprintf("Journal '%s' deleted successfully.", name)), nil
	})
}

// entryWithTags embeds memories.Entry and adds a Tags slice for MCP responses.
type entryWithTags struct {
	memories.Entry
	Tags []string `json:"tags"`
}

// helper to convert an Entry to entryWithTags.
func enrichEntry(ctx context.Context, db *sql.DB, e memories.Entry) (entryWithTags, error) {
	var out entryWithTags
	out.Entry = e
	tagObjs, err := memories.ListTagsForEntry(ctx, db, e.ID)
	if err != nil {
		return out, err
	}
	for _, t := range tagObjs {
		out.Tags = append(out.Tags, t.Tag)
	}
	return out, nil
}

// RegisterCreateEntryTool registers the create_entry tool.
// This is a core function for populating your conversational memory.
// Use it frequently to save snippets of conversations, important facts, code examples, or any piece of context you want to recall later.
func RegisterCreateEntryTool(s *server.MCPServer, db *sql.DB) {
	def := getToolDef("create_entry")
	if def == nil {
		return
	}
	tool := mcp.NewTool(
		def.Name,
		mcp.WithDescription(def.Description),
		mcp.WithString("journal_name", mcp.DefaultString(DefaultJournalName), mcp.Description("Memory space for organizing this information. Automatically uses the default 'memory' journal unless a specific context is needed.")),
		mcp.WithString("entry_title", mcp.Required(), mcp.Description("A descriptive title that captures the essence of this information, making it easy to find later.")),
		mcp.WithString("content", mcp.Required(), mcp.Description("The information to preserve - could be a decision, solution, insight, or any important detail from the conversation.")),
		mcp.WithString("content_type", mcp.DefaultString("text/plain"), mcp.Description("Format of the content, automatically detected in most cases.")),
		mcp.WithString("tags", mcp.Description("Keywords that connect this memory to related topics, enabling powerful cross-referencing and discovery.")),
	)
	s.AddTool(tool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		journalName, _ := request.Params.Arguments["journal_name"].(string)
		if journalName == "" {
			journalName = DefaultJournalName
		}
		title, _ := request.Params.Arguments["entry_title"].(string)
		content, _ := request.Params.Arguments["content"].(string)
		contentType, _ := request.Params.Arguments["content_type"].(string)
		tagsStr, _ := request.Params.Arguments["tags"].(string)
		if strings.TrimSpace(title) == "" {
			return mcp.NewToolResultError("'entry_title' parameter is required"), nil
		}
		// Ensure journal exists (create if missing)
		journal, err := getJournalByName(ctx, db, journalName)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Error checking journal: %v", err)), nil
		}
		if journal == nil {
			journalPtr, errCreate := memories.CreateJournal(ctx, db, journalName, "")
			if errCreate != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Failed to create journal '%s': %v", journalName, errCreate)), nil
			}
			journal = &journalPtr
		}
		entry, err := memories.CreateEntry(ctx, db, journal.ID, title, content, contentType)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to create entry: %v", err)), nil
		}
		// Tagging if requested
		if tagsStr != "" {
			for _, t := range parseTags(tagsStr) {
				_ = memories.TagEntry(ctx, db, entry.ID, t) // Ignore individual tag errors for now
			}
		}
		enriched, _ := enrichEntry(ctx, db, entry)
		b, _ := json.Marshal(enriched)
		return mcp.NewToolResultText(string(b)), nil
	})
}

// RegisterListEntriesTool registers list_entries (filter by journal or tags).
// Essential for retrieving stored memories and context.
// Use filters to narrow down your search and find the exact piece of information you need from your second brain.
func RegisterListEntriesTool(s *server.MCPServer, db *sql.DB) {
	def := getToolDef("list_entries")
	if def == nil {
		return
	}
	tool := mcp.NewTool(
		def.Name,
		mcp.WithDescription(def.Description),
		mcp.WithString("journal_name", mcp.DefaultString(DefaultJournalName), mcp.Description("Focus retrieval on a specific memory space. Automatically uses the most relevant journal based on context.")),
		mcp.WithString("tags", mcp.Description("Filter memories by topics or themes. Helps find interconnected information across different conversations.")),
	)
	s.AddTool(tool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		journalName, _ := request.Params.Arguments["journal_name"].(string)
		if journalName == "" {
			journalName = DefaultJournalName
		}
		tagsStr, _ := request.Params.Arguments["tags"].(string)
		tagsFilter := parseTags(tagsStr)

		var journals []memories.Journal
		if journalName != "" {
			j, err := getJournalByName(ctx, db, journalName)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Error retrieving journal: %v", err)), nil
			}
			if j == nil {
				return mcp.NewToolResultError(fmt.Sprintf("Journal '%s' not found", journalName)), nil
			}
			journals = append(journals, *j)
		} else {
			list, err := memories.ListJournals(ctx, db, false)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Error listing journals: %v", err)), nil
			}
			journals = list
		}

		var results []entryWithTags
		for _, j := range journals {
			es, err := memories.ListEntries(ctx, db, j.ID, false)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Error listing entries: %v", err)), nil
			}
			for _, e := range es {
				if len(tagsFilter) == 0 {
					en, _ := enrichEntry(ctx, db, e)
					results = append(results, en)
					continue
				}
				entryTags, err := memories.ListTagsForEntry(ctx, db, e.ID)
				if err != nil {
					return mcp.NewToolResultError(fmt.Sprintf("Error fetching tags: %v", err)), nil
				}
				if hasAllTags(entryTags, tagsFilter) {
					en, _ := enrichEntry(ctx, db, e)
					results = append(results, en)
				}
			}
		}
		if len(results) == 0 {
			return mcp.NewToolResultText("[]"), nil
		}
		b, _ := json.Marshal(results)
		return mcp.NewToolResultText(string(b)), nil
	})
}

// Helper: check if entryTags include all desired tags.
func hasAllTags(entryTags []memories.Tag, desired []string) bool {
	if len(desired) == 0 {
		return true
	}
	tagSet := make(map[string]struct{}, len(entryTags))
	for _, t := range entryTags {
		tagSet[t.Tag] = struct{}{}
	}
	for _, d := range desired {
		if _, ok := tagSet[d]; !ok {
			return false
		}
	}
	return true
}

// RegisterGetEntryTool fetches entry by title.
// Allows you to retrieve a specific piece of memory or context when you know its title.
// Useful for focused recall of information.
func RegisterGetEntryTool(s *server.MCPServer, db *sql.DB) {
	def := getToolDef("get_entry")
	if def == nil {
		return
	}
	tool := mcp.NewTool(
		def.Name,
		mcp.WithDescription(def.Description),
		mcp.WithString("journal_name", mcp.DefaultString(DefaultJournalName), mcp.Description("The memory space containing this information, automatically determined in most cases.")),
		mcp.WithString("entry_title", mcp.Required(), mcp.Description("The specific memory to recall.")),
	)
	s.AddTool(tool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		journalName, _ := request.Params.Arguments["journal_name"].(string)
		if journalName == "" {
			journalName = DefaultJournalName
		}
		title, _ := request.Params.Arguments["entry_title"].(string)
		if strings.TrimSpace(title) == "" {
			return mcp.NewToolResultError("'entry_title' parameter is required"), nil
		}
		journal, err := getJournalByName(ctx, db, journalName)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Error retrieving journal: %v", err)), nil
		}
		if journal == nil {
			return mcp.NewToolResultError(fmt.Sprintf("Journal '%s' not found", journalName)), nil
		}
		entry, err := getEntryByTitleAndJournalID(ctx, db, title, journal.ID)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Error retrieving entry: %v", err)), nil
		}
		if entry == nil {
			return mcp.NewToolResultError(fmt.Sprintf("Entry '%s' not found", title)), nil
		}
		enriched, _ := enrichEntry(ctx, db, *entry)
		b, _ := json.Marshal(enriched)
		return mcp.NewToolResultText(string(b)), nil
	})
}

// RegisterUpdateEntryTool updates an entry.
// Naturally evolves stored memories as understanding deepens or contexts change.
// Keeps information current and relevant across conversations.
func RegisterUpdateEntryTool(s *server.MCPServer, db *sql.DB) {
	def := getToolDef("update_entry")
	if def == nil {
		return
	}
	tool := mcp.NewTool(
		def.Name,
		mcp.WithDescription(def.Description),
		mcp.WithString("journal_name", mcp.DefaultString(DefaultJournalName), mcp.Description("The memory space containing this information.")),
		mcp.WithString("entry_title", mcp.Required(), mcp.Description("The memory to evolve.")),
		mcp.WithString("new_title", mcp.Description("A refined title if the memory's focus has shifted.")),
		mcp.WithString("new_content", mcp.Description("Updated information reflecting new understanding or context.")),
		mcp.WithString("new_content_type", mcp.Description("Format adjustment if the information structure has changed.")),
	)
	s.AddTool(tool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		journalName, _ := request.Params.Arguments["journal_name"].(string)
		if journalName == "" {
			journalName = DefaultJournalName
		}
		title, _ := request.Params.Arguments["entry_title"].(string)
		journal, err := getJournalByName(ctx, db, journalName)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Error retrieving journal: %v", err)), nil
		}
		if journal == nil {
			return mcp.NewToolResultError(fmt.Sprintf("Journal '%s' not found", journalName)), nil
		}
		entry, err := getEntryByTitleAndJournalID(ctx, db, title, journal.ID)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Error retrieving entry: %v", err)), nil
		}
		if entry == nil {
			return mcp.NewToolResultError(fmt.Sprintf("Entry '%s' not found", title)), nil
		}
		newTitle, _ := request.Params.Arguments["new_title"].(string)
		newContent, _ := request.Params.Arguments["new_content"].(string)
		newContentType, _ := request.Params.Arguments["new_content_type"].(string)

		updated, err := memories.UpdateEntry(ctx, db, entry.ID, newTitle, newContent, newContentType)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to update entry: %v", err)), nil
		}
		enriched, _ := enrichEntry(ctx, db, updated)
		b, _ := json.Marshal(enriched)
		return mcp.NewToolResultText(string(b)), nil
	})
}

// RegisterDeleteEntryTool deletes an entry by title.
// Gracefully removes memories that are no longer relevant, keeping conversational context clean and focused.
// Maintains the quality of your extended memory by retiring outdated information.
func RegisterDeleteEntryTool(s *server.MCPServer, db *sql.DB) {
	def := getToolDef("delete_entry")
	if def == nil {
		return
	}
	tool := mcp.NewTool(
		def.Name,
		mcp.WithDescription(def.Description),
		mcp.WithString("journal_name", mcp.DefaultString(DefaultJournalName), mcp.Description("The memory space containing this information.")),
		mcp.WithString("entry_title", mcp.Required(), mcp.Description("The specific memory to retire.")),
	)
	s.AddTool(tool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		journalName, _ := request.Params.Arguments["journal_name"].(string)
		if journalName == "" {
			journalName = DefaultJournalName
		}
		title, _ := request.Params.Arguments["entry_title"].(string)
		journal, err := getJournalByName(ctx, db, journalName)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Error retrieving journal: %v", err)), nil
		}
		if journal == nil {
			return mcp.NewToolResultError(fmt.Sprintf("Journal '%s' not found", journalName)), nil
		}
		entry, err := getEntryByTitleAndJournalID(ctx, db, title, journal.ID)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Error retrieving entry: %v", err)), nil
		}
		if entry == nil {
			return mcp.NewToolResultText(fmt.Sprintf("Entry '%s' not found, nothing to delete.", title)), nil
		}
		if err := memories.DeleteEntry(ctx, db, entry.ID); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to delete entry: %v", err)), nil
		}
		return mcp.NewToolResultText(fmt.Sprintf("Entry '%s' deleted successfully.", title)), nil
	})
}

// RegisterManageEntryTagsTool adds/removes tags for an entry.
// Organically connects memories through meaningful relationships, enabling serendipitous discovery.
// Creates a web of interconnected knowledge that surfaces at just the right moments.
func RegisterManageEntryTagsTool(s *server.MCPServer, db *sql.DB) {
	def := getToolDef("manage_entry_tags")
	if def == nil {
		return
	}
	tool := mcp.NewTool(
		def.Name,
		mcp.WithDescription(def.Description),
		mcp.WithString("journal_name", mcp.DefaultString(DefaultJournalName), mcp.Description("The memory space containing this information.")),
		mcp.WithString("entry_title", mcp.Required(), mcp.Description("The memory to enrich with connections.")),
		mcp.WithString("add_tags", mcp.Description("Topics or themes that connect this memory to others.")),
		mcp.WithString("remove_tags", mcp.Description("Connections that no longer apply to this memory.")),
	)
	s.AddTool(tool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		journalName, _ := request.Params.Arguments["journal_name"].(string)
		if journalName == "" {
			journalName = DefaultJournalName
		}
		title, _ := request.Params.Arguments["entry_title"].(string)
		addStr, _ := request.Params.Arguments["add_tags"].(string)
		removeStr, _ := request.Params.Arguments["remove_tags"].(string)
		if addStr == "" && removeStr == "" {
			return mcp.NewToolResultError("At least one of 'add_tags' or 'remove_tags' must be provided."), nil
		}
		journal, err := getJournalByName(ctx, db, journalName)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Error retrieving journal: %v", err)), nil
		}
		if journal == nil {
			return mcp.NewToolResultError(fmt.Sprintf("Journal '%s' not found", journalName)), nil
		}
		entry, err := getEntryByTitleAndJournalID(ctx, db, title, journal.ID)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Error retrieving entry: %v", err)), nil
		}
		if entry == nil {
			return mcp.NewToolResultError(fmt.Sprintf("Entry '%s' not found", title)), nil
		}
		for _, t := range parseTags(addStr) {
			_ = memories.TagEntry(ctx, db, entry.ID, t)
		}
		for _, t := range parseTags(removeStr) {
			_ = memories.DetachTag(ctx, db, entry.ID, t)
		}
		updatedEntry, _ := memories.GetEntry(ctx, db, entry.ID)
		enriched, _ := enrichEntry(ctx, db, updatedEntry)
		b, _ := json.Marshal(enriched)
		return mcp.NewToolResultText(string(b)), nil
	})
}

// RegisterListTagsTool lists all distinct tags across the database.
// Reveals the constellation of topics and themes that connect your memories.
// Helps discover existing categories and understand the landscape of your conversations.
func RegisterListTagsTool(s *server.MCPServer, db *sql.DB) {
	def := getToolDef("list_tags")
	if def == nil {
		return
	}
	tool := mcp.NewTool(def.Name, mcp.WithDescription(def.Description))
	s.AddTool(tool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		rows, err := db.QueryContext(ctx, "SELECT tag, created_at, updated_at FROM tags ORDER BY tag")
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to list tags: %v", err)), nil
		}
		defer rows.Close()
		var tags []memories.Tag
		for rows.Next() {
			var t memories.Tag
			if err := rows.Scan(&t.Tag, &t.CreatedAt, &t.UpdatedAt); err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Failed to scan tag: %v", err)), nil
			}
			tags = append(tags, t)
		}
		if len(tags) == 0 {
			return mcp.NewToolResultText("[]"), nil
		}
		b, _ := json.Marshal(tags)
		return mcp.NewToolResultText(string(b)), nil
	})
}

// RegisterSearchEntriesTool searches entries by tags across all journals.
// A powerful way to retrieve contextually related information by combining multiple tags.
// This allows for complex queries to find precisely the memories or context snippets you need.
func RegisterSearchEntriesTool(s *server.MCPServer, db *sql.DB) {
	def := getToolDef("search_entries")
	if def == nil {
		return
	}
	tool := mcp.NewTool(
		def.Name,
		mcp.WithDescription(def.Description),
		mcp.WithString("tags", mcp.Required(), mcp.Description("Topics or themes to explore. The tool finds memories that connect all specified concepts, revealing hidden relationships.")),
	)
	s.AddTool(tool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		tagsStr, _ := request.Params.Arguments["tags"].(string)
		tagsFilter := parseTags(tagsStr)
		if len(tagsFilter) == 0 {
			return mcp.NewToolResultError("'tags' parameter is required and must be non-empty"), nil
		}
		journals, err := memories.ListJournals(ctx, db, false)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Error listing journals: %v", err)), nil
		}
		var matched []entryWithTags
		for _, j := range journals {
			entries, err := memories.ListEntries(ctx, db, j.ID, false)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Error listing entries: %v", err)), nil
			}
			for _, e := range entries {
				entryTags, err := memories.ListTagsForEntry(ctx, db, e.ID)
				if err != nil {
					return mcp.NewToolResultError(fmt.Sprintf("Error fetching tags: %v", err)), nil
				}
				if hasAllTags(entryTags, tagsFilter) {
					en, _ := enrichEntry(ctx, db, e)
					matched = append(matched, en)
				}
			}
		}
		if len(matched) == 0 {
			return mcp.NewToolResultText("[]"), nil
		}
		b, _ := json.Marshal(matched)
		return mcp.NewToolResultText(string(b)), nil
	})
}

// parseTags splits a comma-separated tag list.
func parseTags(tagsStr string) []string {
	var result []string
	for _, t := range strings.Split(tagsStr, ",") {
		if v := strings.TrimSpace(t); v != "" {
			result = append(result, v)
		}
	}
	return result
}
