package summarizer

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/robert-nemet/sessionmngr/internal/session"
)

const (
	// DefaultTimeout for API requests.
	DefaultTimeout = 120 * time.Second

	// MaxConversationChars is the soft limit for conversation size.
	// Conversations larger than this should be chunked (not yet implemented).
	// Current value is conservative estimate for ~100k tokens.
	MaxConversationChars = 300000
)

// AnthropicClient implements SummaryClient using the official Anthropic SDK.
type AnthropicClient struct {
	client       anthropic.Client
	model        string
	prompt       string
	promptSource string
}

// NewAnthropicClient creates a new Anthropic API client.
func NewAnthropicClient(apiKey, model, prompt, promptSource string) *AnthropicClient {
	client := anthropic.NewClient(
		option.WithAPIKey(apiKey),
	)

	return &AnthropicClient{
		client:       client,
		model:        model,
		prompt:       prompt,
		promptSource: promptSource,
	}
}

// GetModelName returns the name of the model used by the client.ß
func (c *AnthropicClient) GetModelName() string {
	return c.model
}

// GetPromptSource returns the source of the prompt used by the client.
func (c *AnthropicClient) GetPromptSource() string {
	return c.promptSource
}

// GenerateIncrementalResume updates an existing resume by merging in new messages.
func (c *AnthropicClient) GenerateIncrementalResume(ctx context.Context, priorResume string, delta []session.Message) (*SummaryResult, error) {
	userMessage := "PRIOR RESUME:\n" + priorResume + "\n\nCONVERSATION:\n" + buildConversationText(delta)

	if len(userMessage) > MaxConversationChars {
		return nil, fmt.Errorf("incremental input too large (%d chars, max %d)", len(userMessage), MaxConversationChars)
	}

	message, err := c.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     anthropic.Model(c.model),
		MaxTokens: 6000,
		System: []anthropic.TextBlockParam{
			{Text: c.prompt},
		},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(
				anthropic.NewTextBlock(userMessage),
			),
		},
	})
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("request timed out: %w", err)
		}
		if ctx.Err() == context.Canceled {
			return nil, fmt.Errorf("request canceled: %w", err)
		}
		return nil, fmt.Errorf("API request failed: %w", err)
	}

	if len(message.Content) == 0 {
		return nil, fmt.Errorf("empty response from API")
	}

	var markdown string
	for _, block := range message.Content {
		if block.Type == "text" {
			markdown = strings.TrimSpace(block.Text)
			break
		}
	}

	if markdown == "" {
		return nil, fmt.Errorf("no text content in API response")
	}

	markdown, tags := extractTags(markdown)
	title := extractTitle(markdown)

	return &SummaryResult{
		Title:    title,
		Markdown: markdown,
		Tags:     tags,
		Usage: TokenUsage{
			InputTokens:  message.Usage.InputTokens,
			OutputTokens: message.Usage.OutputTokens,
		},
	}, nil
}

// GenerateSummary generates a summary of the given messages using the Anthropic API.ß
func (c *AnthropicClient) GenerateSummary(ctx context.Context, messages []session.Message) (*SummaryResult, error) {
	conversationText := buildConversationText(messages)

	// TODO: Implement chunking strategy for large conversations.
	if len(conversationText) > MaxConversationChars {
		return nil, fmt.Errorf("conversation too large (%d chars, max %d) - chunking not yet implemented",
			len(conversationText), MaxConversationChars)
	}

	message, err := c.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     anthropic.Model(c.model),
		MaxTokens: 4096,
		System: []anthropic.TextBlockParam{
			{Text: c.prompt},
		},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(
				anthropic.NewTextBlock("Summarize this conversation:\n\n" + conversationText),
			),
		},
	})
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("request timed out: %w", err)
		}
		if ctx.Err() == context.Canceled {
			return nil, fmt.Errorf("request canceled: %w", err)
		}
		return nil, fmt.Errorf("API request failed: %w", err)
	}

	if len(message.Content) == 0 {
		return nil, fmt.Errorf("empty response from API")
	}

	// Extract text from content blocks
	var markdown string
	for _, block := range message.Content {
		if block.Type == "text" {
			markdown = strings.TrimSpace(block.Text)
			break
		}
	}

	if markdown == "" {
		return nil, fmt.Errorf("no text content in API response")
	}

	markdown, tags := extractTags(markdown)
	title := extractTitle(markdown)

	return &SummaryResult{
		Title:    title,
		Markdown: markdown,
		Tags:     tags,
		Usage: TokenUsage{
			InputTokens:  message.Usage.InputTokens,
			OutputTokens: message.Usage.OutputTokens,
		},
	}, nil
}
