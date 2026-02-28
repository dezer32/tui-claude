package session

import (
	"bufio"
	"encoding/json"
	"os"
	"strings"
)

// ParsedMessage represents a single parsed message from a JSONL file.
type ParsedMessage struct {
	Type    string // "user", "assistant", "summary"
	Content string
}

// jsonlEntry represents a raw JSONL line.
type jsonlEntry struct {
	Type    string          `json:"type"`
	Message json.RawMessage `json:"message,omitempty"`
	Summary string          `json:"summary,omitempty"`
}

// messageContent represents the message field.
type messageContent struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

// contentBlock represents a content block in a message.
type contentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// ParseJSONL parses a session JSONL file and returns messages.
// maxSize limits the file size to parse (0 = no limit).
// maxMessages limits the number of messages returned (0 = no limit).
func ParseJSONL(path string, maxSize int64, maxMessages int) ([]ParsedMessage, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}

	if maxSize > 0 && info.Size() > maxSize {
		return parseJSONLTail(path, maxMessages)
	}

	return parseJSONLFull(path, maxMessages)
}

func parseJSONLFull(path string, maxMessages int) ([]ParsedMessage, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var messages []ParsedMessage
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024) // 1MB buffer

	for scanner.Scan() {
		line := scanner.Text()
		if msg, ok := parseLine(line); ok {
			messages = append(messages, msg)
		}
	}

	if maxMessages > 0 && len(messages) > maxMessages {
		messages = messages[len(messages)-maxMessages:]
	}

	return messages, scanner.Err()
}

func parseJSONLTail(path string, maxMessages int) ([]ParsedMessage, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	// Seek to last 2MB for large files
	info, _ := f.Stat()
	offset := info.Size() - 2*1024*1024
	if offset > 0 {
		f.Seek(offset, 0)
	}

	var messages []ParsedMessage
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	first := offset > 0 // skip potentially partial first line
	for scanner.Scan() {
		if first {
			first = false
			continue
		}
		line := scanner.Text()
		if msg, ok := parseLine(line); ok {
			messages = append(messages, msg)
		}
	}

	if maxMessages > 0 && len(messages) > maxMessages {
		messages = messages[len(messages)-maxMessages:]
	}

	return messages, scanner.Err()
}

func parseLine(line string) (ParsedMessage, bool) {
	if line == "" {
		return ParsedMessage{}, false
	}

	var entry jsonlEntry
	if err := json.Unmarshal([]byte(line), &entry); err != nil {
		return ParsedMessage{}, false
	}

	switch entry.Type {
	case "user":
		return parseUserMessage(entry)
	case "assistant":
		return parseAssistantMessage(entry)
	case "summary":
		if entry.Summary != "" {
			return ParsedMessage{Type: "summary", Content: entry.Summary}, true
		}
	}

	return ParsedMessage{}, false
}

func parseUserMessage(entry jsonlEntry) (ParsedMessage, bool) {
	if entry.Message == nil {
		return ParsedMessage{}, false
	}

	var msg messageContent
	if err := json.Unmarshal(entry.Message, &msg); err != nil {
		return ParsedMessage{}, false
	}

	content := extractContent(msg.Content)
	if content == "" {
		return ParsedMessage{}, false
	}

	return ParsedMessage{Type: "user", Content: truncate(content, 500)}, true
}

func parseAssistantMessage(entry jsonlEntry) (ParsedMessage, bool) {
	if entry.Message == nil {
		return ParsedMessage{}, false
	}

	var msg messageContent
	if err := json.Unmarshal(entry.Message, &msg); err != nil {
		return ParsedMessage{}, false
	}

	content := extractContent(msg.Content)
	if content == "" {
		return ParsedMessage{}, false
	}

	return ParsedMessage{Type: "assistant", Content: truncate(content, 500)}, true
}

// extractContent handles both string content and array-of-blocks content.
func extractContent(raw json.RawMessage) string {
	if raw == nil {
		return ""
	}

	// Try as plain string
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}

	// Try as array of content blocks
	var blocks []contentBlock
	if err := json.Unmarshal(raw, &blocks); err == nil {
		var texts []string
		for _, b := range blocks {
			if b.Type == "text" && b.Text != "" {
				texts = append(texts, b.Text)
			}
		}
		return strings.Join(texts, "\n")
	}

	return ""
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}

// SessionMeta returns metadata for display.
type SessionMeta struct {
	SessionID   string
	ProjectPath string
	GitBranch   string
	Created     string
	Modified    string
	MsgCount    int
	FirstPrompt string
	Summary     string
	FullPath    string
}

// GetMeta returns session metadata for preview.
func GetMeta(s Session) SessionMeta {
	return SessionMeta{
		SessionID:   s.SessionID,
		ProjectPath: s.ProjectPath,
		GitBranch:   s.GitBranch,
		Created:     s.Created.Format("2006-01-02 15:04:05"),
		Modified:    s.Modified.Format("2006-01-02 15:04:05"),
		MsgCount:    s.MsgCount,
		FirstPrompt: s.FirstPrompt,
		Summary:     s.Summary,
		FullPath:    s.FullPath,
	}
}
