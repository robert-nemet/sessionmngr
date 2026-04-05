package summarizer

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExtractTags(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		wantMarkdown string
		wantTags     []string
	}{
		{
			name:         "valid tags at end",
			input:        "# Summary\n\nSome content.\n\n<!-- tags: golang, debugging, postgres -->",
			wantMarkdown: "# Summary\n\nSome content.",
			wantTags:     []string{"golang", "debugging", "postgres"},
		},
		{
			name:         "no tags comment",
			input:        "# Summary\n\nNo tags here.",
			wantMarkdown: "# Summary\n\nNo tags here.",
			wantTags:     nil,
		},
		{
			name:         "tags with digits",
			input:        "content\n<!-- tags: golang, kube-1-29, postgres -->",
			wantMarkdown: "content",
			wantTags:     []string{"golang", "kube-1-29", "postgres"},
		},
		{
			name:         "invalid tags filtered out",
			input:        "content\n<!-- tags: go, ok-tag, AB!, valid-one -->",
			wantMarkdown: "content",
			wantTags:     []string{"ok-tag", "valid-one"},
		},
		{
			name:         "tags mid-document",
			input:        "# Title\n<!-- tags: infra, deploy -->\n\nMore content.",
			wantMarkdown: "# Title\n\nMore content.",
			wantTags:     []string{"infra", "deploy"},
		},
		{
			name:         "all tags invalid returns nil",
			input:        "content\n<!-- tags: go, ab, x -->",
			wantMarkdown: "content",
			wantTags:     nil,
		},
		{
			name:         "extra whitespace in comment",
			input:        "content\n<!--   tags:   golang ,  debugging   -->",
			wantMarkdown: "content",
			wantTags:     []string{"golang", "debugging"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			markdown, tags := extractTags(tt.input)
			assert.Equal(t, tt.wantMarkdown, markdown)
			assert.Equal(t, tt.wantTags, tags)
		})
	}
}
