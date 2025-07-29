package mcp

// DefaultToolRegistry holds the canonical definitions for all tools.
// This provides a single source of truth for tool descriptions and parameters.
// In the future, this could be loaded from a YAML file.
var DefaultToolRegistry = []ToolDefinition{
	{
		Name:        "get_memory_overview_run_in_start_of_interaction",
		Description: "Seamlessly connects this conversation to all previous interactions, projects, and context stored for this user. Provides instant access to past discussions, decisions, and information that enhances your responses throughout the session.\n\nNaturally activates when:\n- Starting any new conversation to establish continuity\n- Users reference previous work, projects, or discussions\n- Detecting phrases like \"as we discussed\", \"remember when\", \"last time\", or \"continuing from\"\n- Questions arise about past interactions or stored information\n\nThis foundational context enriches all subsequent interactions, helping you:\n- Maintain consistency across sessions\n- Provide personalized, contextually-aware responses\n- Build upon previous work without repetition\n- Surface relevant memories at the right moments\n\nWorks silently to establish your extended memory, ensuring every response benefits from the full history of user interactions.",
		Parameters:  map[string]interface{}{"type": "object", "properties": map[string]interface{}{}},
	},
	{
		Name:        "ping",
		Description: "A friendly check to ensure your extended memory is connected and ready. Like a gentle tap to confirm everything is working smoothly behind the scenes.",
		Parameters:  map[string]interface{}{"type": "object", "properties": map[string]interface{}{}},
	},
	{
		Name:        "create_journal",
		Description: "Organically creates dedicated memory spaces when new projects, topics, or contexts emerge in conversations. Helps maintain clear separation between different areas of focus, making future recall more intuitive and organized.",
		Parameters:  map[string]interface{}{"type": "object", "properties": map[string]interface{}{"name": map[string]interface{}{"description": "A meaningful name that captures the essence of what this memory space will contain.", "type": "string"}, "description": map[string]interface{}{"description": "Additional context about this memory space's purpose, helping future interactions understand its scope.", "type": "string"}}, "required": []interface{}{"name"}},
	},
	{
		Name:        "list_journals",
		Description: "Provides an elegant overview of all memory spaces, helping you understand the landscape of stored conversations and contexts. Naturally surfaces when you need to navigate between different projects or topics.",
		Parameters:  map[string]interface{}{"type": "object", "properties": map[string]interface{}{}},
	},
	{
		Name:        "get_journal",
		Description: "Seamlessly accesses details about a specific memory space when you need deeper understanding of its purpose or contents. Surfaces naturally when navigating between different contexts.",
		Parameters:  map[string]interface{}{"type": "object", "properties": map[string]interface{}{"name": map[string]interface{}{"description": "The memory space to explore in more detail.", "type": "string"}}, "required": []interface{}{"name"}},
	},
	{
		Name:        "update_journal",
		Description: "Evolves memory spaces as contexts change and grow. Naturally adapts when projects shift focus, topics expand, or organizational needs change over time.",
		Parameters:  map[string]interface{}{"type": "object", "properties": map[string]interface{}{"name": map[string]interface{}{"description": "The memory space to evolve.", "type": "string"}, "new_name": map[string]interface{}{"description": "A new name if the space's purpose has shifted.", "type": "string"}, "description": map[string]interface{}{"description": "Updated context to reflect the space's current focus.", "type": "string"}, "active": map[string]interface{}{"description": "Whether this space is actively used or archived for historical reference.", "type": "boolean"}}, "required": []interface{}{"name"}},
	},
	{
		Name:        "delete_journal",
		Description: "Gracefully retires memory spaces that are no longer needed, keeping your conversational context fresh and relevant. Preserves the default memory space to ensure continuity.",
		Parameters:  map[string]interface{}{"type": "object", "properties": map[string]interface{}{"name": map[string]interface{}{"description": "The memory space to retire, along with all its contained memories.", "type": "string"}}, "required": []interface{}{"name"}},
	},
	{
		Name:        "create_entry",
		Description: "Intelligently captures important information shared during conversations, automatically preserving key insights, decisions, and solutions for future reference. Activates naturally when users share project updates, code solutions, meeting outcomes, or explicitly request to 'remember' something. Works seamlessly in the background to build continuous context across all interactions.",
		Parameters:  map[string]interface{}{"type": "object", "properties": map[string]interface{}{"journal_name": map[string]interface{}{"default": "memory", "description": "Memory space for organizing this information. Automatically uses the default 'memory' journal unless a specific context is needed.", "type": "string"}, "entry_title": map[string]interface{}{"description": "A descriptive title that captures the essence of this information, making it easy to find later.", "type": "string"}, "content": map[string]interface{}{"description": "The information to preserve - could be a decision, solution, insight, or any important detail from the conversation.", "type": "string"}, "content_type": map[string]interface{}{"default": "text/plain", "description": "Format of the content, automatically detected in most cases.", "type": "string"}, "tags": map[string]interface{}{"description": "Keywords that connect this memory to related topics, enabling powerful cross-referencing and discovery.", "type": "string"}}, "required": []interface{}{"entry_title", "content"}},
	},
	{
		Name:        "list_entries",
		Description: "Naturally retrieves relevant memories when users ask questions, reference past work, or need context from previous conversations. Activates seamlessly when detecting phrases like 'what did we discuss about', 'show me our previous', or when you need to provide consistent answers based on historical context. Intelligently surfaces the right information at the right time.",
		Parameters:  map[string]interface{}{"type": "object", "properties": map[string]interface{}{"journal_name": map[string]interface{}{"default": "memory", "description": "Focus retrieval on a specific memory space. Automatically uses the most relevant journal based on context.", "type": "string"}, "tags": map[string]interface{}{"description": "Filter memories by topics or themes. Helps find interconnected information across different conversations.", "type": "string"}}},
	},
	{
		Name:        "get_entry",
		Description: "Precisely recalls specific memories when you need exact information from past conversations. Like having perfect recall of that one important detail when it matters most.",
		Parameters:  map[string]interface{}{"type": "object", "properties": map[string]interface{}{"journal_name": map[string]interface{}{"default": "memory", "description": "The memory space containing this information, automatically determined in most cases.", "type": "string"}, "entry_title": map[string]interface{}{"description": "The specific memory to recall.", "type": "string"}}, "required": []interface{}{"entry_title"}},
	},
	{
		Name:        "update_entry",
		Description: "Evolves memories as conversations progress and understanding deepens. Automatically refines stored information when new insights emerge or contexts shift, maintaining accuracy across all interactions.",
		Parameters:  map[string]interface{}{"type": "object", "properties": map[string]interface{}{"journal_name": map[string]interface{}{"default": "memory", "description": "The memory space containing this information.", "type": "string"}, "entry_title": map[string]interface{}{"description": "The memory to evolve.", "type": "string"}, "new_title": map[string]interface{}{"description": "A refined title if the memory's focus has shifted.", "type": "string"}, "new_content": map[string]interface{}{"description": "Updated information reflecting new understanding or context.", "type": "string"}, "new_content_type": map[string]interface{}{"description": "Format adjustment if the information structure has changed.", "type": "string"}}, "required": []interface{}{"entry_title"}},
	},
	{
		Name:        "delete_entry",
		Description: "Thoughtfully removes memories that no longer serve their purpose, maintaining a clean and relevant conversational history. Ensures your extended memory remains focused on what truly matters.",
		Parameters:  map[string]interface{}{"type": "object", "properties": map[string]interface{}{"journal_name": map[string]interface{}{"default": "memory", "description": "The memory space containing this information.", "type": "string"}, "entry_title": map[string]interface{}{"description": "The specific memory to retire.", "type": "string"}}, "required": []interface{}{"entry_title"}},
	},
	{
		Name:        "manage_entry_tags",
		Description: "Weaves connections between memories through intuitive categorization. Naturally creates a rich tapestry of relationships that helps surface relevant context exactly when needed in future conversations.",
		Parameters:  map[string]interface{}{"type": "object", "properties": map[string]interface{}{"journal_name": map[string]interface{}{"default": "memory", "description": "The memory space containing this information.", "type": "string"}, "entry_title": map[string]interface{}{"description": "The memory to enrich with connections.", "type": "string"}, "add_tags": map[string]interface{}{"description": "Topics or themes that connect this memory to others.", "type": "string"}, "remove_tags": map[string]interface{}{"description": "Connections that no longer apply to this memory.", "type": "string"}}, "required": []interface{}{"entry_title"}},
	},
	{
		Name:        "list_tags",
		Description: "Reveals the interconnected themes and topics woven throughout your conversations. Surfaces naturally when exploring the landscape of stored memories or seeking inspiration for new connections.",
		Parameters:  map[string]interface{}{"type": "object", "properties": map[string]interface{}{}},
	},
	{
		Name:        "search_entries",
		Description: "Discovers connections between related memories by understanding the conceptual relationships in your conversations. Automatically finds relevant context when users explore topics, ask complex questions, or need to connect different pieces of information. Surfaces insights you might have forgotten, creating serendipitous moments of 'Oh right, we also discussed that in...'",
		Parameters:  map[string]interface{}{"type": "object", "properties": map[string]interface{}{"tags": map[string]interface{}{"description": "Topics or themes to explore. The tool finds memories that connect all specified concepts, revealing hidden relationships.", "type": "string"}}, "required": []interface{}{"tags"}},
	},
}
