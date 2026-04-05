package summarizer

import (
	"embed"
	"fmt"
)

//go:embed prompts/*.md
var embeddedPrompts embed.FS

// PromptTypes maps prompt type names to their embedded file paths.
var PromptTypes = map[string]string{
	"operational":     "prompts/operational.md",
	"business":        "prompts/business.md",
	"troubleshooting": "prompts/troubleshooting.md",
	"evaluation":      "prompts/evaluation.md",
	"resume":          "prompts/resume.md",
}

// ValidPromptTypes returns a list of valid prompt type names.
func ValidPromptTypes() []string {
	return []string{"operational", "business", "troubleshooting", "evaluation", "resume"}
}

// GetPrompt returns the content of an embedded prompt by type name.
// Returns error if the prompt type is not recognized.
func GetPrompt(promptType string) (string, error) {
	path, ok := PromptTypes[promptType]
	if !ok {
		return "", fmt.Errorf("unknown prompt type: %s (valid types: operational, business, troubleshooting, evaluation, resume)", promptType)
	}

	content, err := embeddedPrompts.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("failed to read embedded prompt: %w", err)
	}

	return string(content), nil
}
