package ui

type NodeType string

const (
	NodeTypeLLMChat  NodeType = "llm_chat"
	NodeTypeMarkdown NodeType = "markdown"
	NodeTypeImage    NodeType = "image"
	NodeTypeTable    NodeType = "table"
)

type Meta struct {
	Title       string
	Description string
	Tags        []string
}

type MessageRole string

const (
	RoleUser      MessageRole = "user"
	RoleAssistant MessageRole = "assistant"
)

type ChatMessage struct {
	ID      string
	Role    MessageRole
	Content string
}

type LLMChatState struct {
	Model        string
	IsResponding bool
	SendLocked   bool
	SendLockHint string
	Messages     []ChatMessage
}

type MarkdownState struct {
	Markdown string
}

type ImageState struct {
	Src string
	Alt string
}

type TableState struct {
	Columns []string
	Rows    [][]string
}

type Node struct {
	ID       string
	Type     NodeType
	Meta     Meta
	LLMChat  *LLMChatState
	Markdown *MarkdownState
	Image    *ImageState
	Table    *TableState
}
