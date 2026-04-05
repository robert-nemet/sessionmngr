// Package summarizer provides AI-powered conversation summarization.
//
// # Known Limitations
//
//   - LoadPrompt and prompt file tests are not implemented yet
//   - Prompt is read once at client creation; file changes require restart
//   - PromptSource stores full file path in JSON output (privacy consideration)
//   - No validation that prompt content is reasonable for summarization
package summarizer

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/robert-nemet/sessionmngr/internal/config"
	"github.com/robert-nemet/sessionmngr/internal/session"
)

// TokenUsage holds input/output token counts from an AI API call.
type TokenUsage struct {
	InputTokens  int64
	OutputTokens int64
}

// SummaryResult contains the AI-generated summary.
type SummaryResult struct {
	Title    string // Short title extracted from first heading
	Markdown string // Full summary as markdown
	Tags     []string
	Usage    TokenUsage
}

// SummaryClient abstracts AI provider for generating summaries.
//
// Future improvements (not yet implemented):
//   - Retry logic with exponential backoff for transient failures
//   - Rate limit handling
type SummaryClient interface {
	// GenerateSummary creates a markdown summary from conversation messages.
	// The context controls cancellation and timeout.
	GenerateSummary(ctx context.Context, messages []session.Message) (*SummaryResult, error)

	// GenerateIncrementalResume updates an existing resume with new messages.
	// priorResume is the full markdown of the current resume.
	// delta contains only the messages added since that resume was generated.
	GenerateIncrementalResume(ctx context.Context, priorResume string, delta []session.Message) (*SummaryResult, error)

	// GetModelName returns the model identifier used for generation.
	GetModelName() string

	// GetPromptSource returns "default" or the file path of the prompt used.
	GetPromptSource() string
}

// Provider identifies which AI provider to use.
type Provider string

const (
	// ProviderAnthropic name
	ProviderAnthropic Provider = "anthropic"
	// ProviderOpenAI name
	ProviderOpenAI Provider = "openai"
)

const summarySystemPrompt = `You are a technical documentation writer. Analyze the conversation and create a concise summary in markdown format.

Structure your summary as:

# Title
A brief, descriptive title for this conversation.

## Introduction
What problem or task was being addressed? Provide context and background.

## Analysis
Key findings, investigation steps, or technical details discussed.

## Solution
What solution was implemented or recommended? Include specific commands, code, or configurations if applicable.

## Conclusion
The outcome, current status, or next steps.

Guidelines:
- Be concise but complete
- Use code blocks for commands and code snippets
- Focus on actionable technical information
- Skip sections that don't apply to the conversation`

// LoadPrompt loads a prompt from file, embedded type, or returns the default.
// fileOrType can be:
//   - Empty string: returns default prompt
//   - Embedded type name (operational, business, troubleshooting, evaluation): returns embedded prompt
//   - File path: reads and returns file content
//
// Returns the prompt content and source ("default", "type:<name>", or file path).
func LoadPrompt(fileOrType string) (prompt, source string, err error) {
	if fileOrType == "" {
		return summarySystemPrompt, "default", nil
	}

	// Check if it's an embedded type
	if content, err := GetPrompt(fileOrType); err == nil {
		return content, "type:" + fileOrType, nil
	}

	// Try as file path
	data, err := os.ReadFile(fileOrType)
	if err != nil {
		return "", "", fmt.Errorf("failed to read prompt file: %w", err)
	}
	content := strings.TrimSpace(string(data))
	if content == "" {
		return "", "", fmt.Errorf("prompt file is empty: %s", fileOrType)
	}
	return content, fileOrType, nil
}

// buildConversationText formats messages for the API.
func buildConversationText(messages []session.Message) string {
	var b strings.Builder
	for _, msg := range messages {
		b.WriteString(msg.Role)
		b.WriteString(": ")
		b.WriteString(msg.Content)
		b.WriteString("\n\n")
	}
	return b.String()
}

// extractTitle gets the title from the first markdown heading.
func extractTitle(markdown string) string {
	if markdown == "" {
		return "Untitled"
	}

	first := strings.SplitN(markdown, "\n", 2)[0]
	first = strings.TrimSpace(first)

	if strings.HasPrefix(first, "# ") {
		return strings.TrimPrefix(first, "# ")
	}

	if first == "" {
		return "Untitled"
	}

	// No heading found, truncate safely
	return truncateString(first, 60)
}

var tagCommentRe = regexp.MustCompile(`(?m)^\s*<!--\s*tags:\s*(.+?)\s*-->\s*$`)
var validTagRe = regexp.MustCompile(`^[a-z0-9_-]+$`)

// extractTags finds and removes a <!-- tags: ... --> comment from markdown,
// returning the cleaned markdown and validated tags.
func extractTags(markdown string) (string, []string) {
	match := tagCommentRe.FindStringSubmatchIndex(markdown)
	if match == nil {
		return markdown, nil
	}

	raw := markdown[match[2]:match[3]]
	cleaned := strings.TrimSpace(markdown[:match[0]] + markdown[match[1]:])

	var tags []string
	for _, t := range strings.Split(raw, ",") {
		t = strings.ToLower(strings.TrimSpace(t))
		if len(t) >= 4 && validTagRe.MatchString(t) {
			tags = append(tags, t)
		}
	}
	return cleaned, tags
}

// truncateString safely truncates a string to maxLen runes.
func truncateString(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "..."
}

// NewClient creates a SummaryClient based on provider and configuration.
// Returns error if API key is not configured for the selected provider.
func NewClient(cfg config.Config, promptFile string) (SummaryClient, error) {
	provider := Provider(cfg.GetSummarizerProvider())
	model := cfg.GetSummarizerModel()

	prompt, source, err := LoadPrompt(promptFile)
	if err != nil {
		return nil, err
	}

	switch provider {
	case ProviderAnthropic:
		apiKey := cfg.GetAnthropicAPIKey()
		if apiKey == "" {
			return nil, fmt.Errorf("ANTHROPIC_API_KEY is required for anthropic provider")
		}
		return NewAnthropicClient(apiKey, model, prompt, source), nil

	case ProviderOpenAI:
		apiKey := cfg.GetOpenAIAPIKey()
		if apiKey == "" {
			return nil, fmt.Errorf("OPENAI_API_KEY is required for openai provider")
		}
		return NewOpenAIClient(apiKey, model, prompt, source), nil

	default:
		return nil, fmt.Errorf("unknown provider: %s", provider)
	}
}
