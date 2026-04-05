package summarizer

import (
	"context"
	"fmt"
	"strings"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/robert-nemet/sessionmngr/internal/session"
)

// OpenAIClient implements SummaryClient using the official OpenAI SDK.
type OpenAIClient struct {
	client       openai.Client
	model        string
	prompt       string
	promptSource string
}

// NewOpenAIClient creates a new OpenAI API client.
func NewOpenAIClient(apiKey, model, prompt, promptSource string) *OpenAIClient {
	client := openai.NewClient(
		option.WithAPIKey(apiKey),
	)

	return &OpenAIClient{
		client:       client,
		model:        model,
		prompt:       prompt,
		promptSource: promptSource,
	}
}

// GetModelName returns the name of the model used by the client.
func (c *OpenAIClient) GetModelName() string {
	return c.model
}

// GetPromptSource returns the source of the prompt used by the client.
func (c *OpenAIClient) GetPromptSource() string {
	return c.promptSource
}

// GenerateIncrementalResume updates an existing resume by merging in new messages.
func (c *OpenAIClient) GenerateIncrementalResume(ctx context.Context, priorResume string, delta []session.Message) (*SummaryResult, error) {
	userMessage := "PRIOR RESUME:\n" + priorResume + "\n\nCONVERSATION:\n" + buildConversationText(delta)

	if len(userMessage) > MaxConversationChars {
		return nil, fmt.Errorf("incremental input too large (%d chars, max %d)", len(userMessage), MaxConversationChars)
	}

	chatCompletion, err := c.client.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
		Model:     c.model,
		MaxTokens: openai.Int(6000),
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.SystemMessage(c.prompt),
			openai.UserMessage(userMessage),
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

	if len(chatCompletion.Choices) == 0 {
		return nil, fmt.Errorf("empty response from API")
	}

	markdown := strings.TrimSpace(chatCompletion.Choices[0].Message.Content)
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
			InputTokens:  chatCompletion.Usage.PromptTokens,
			OutputTokens: chatCompletion.Usage.CompletionTokens,
		},
	}, nil
}

// GenerateSummary generates a summary of the conversation using the OpenAI API.
func (c *OpenAIClient) GenerateSummary(ctx context.Context, messages []session.Message) (*SummaryResult, error) {
	conversationText := buildConversationText(messages)

	// TODO: Implement chunking strategy for large conversations.
	if len(conversationText) > MaxConversationChars {
		return nil, fmt.Errorf("conversation too large (%d chars, max %d) - chunking not yet implemented",
			len(conversationText), MaxConversationChars)
	}

	chatCompletion, err := c.client.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
		Model:     c.model,
		MaxTokens: openai.Int(4096),
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.SystemMessage(c.prompt),
			openai.UserMessage("Summarize this conversation:\n\n" + conversationText),
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

	if len(chatCompletion.Choices) == 0 {
		return nil, fmt.Errorf("empty response from API")
	}

	markdown := strings.TrimSpace(chatCompletion.Choices[0].Message.Content)
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
			InputTokens:  chatCompletion.Usage.PromptTokens,
			OutputTokens: chatCompletion.Usage.CompletionTokens,
		},
	}, nil
}
