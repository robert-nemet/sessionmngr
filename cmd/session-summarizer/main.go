// Summarizer cli tool
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/robert-nemet/sessionmngr/internal/config"
	"github.com/robert-nemet/sessionmngr/internal/storage"
	"github.com/robert-nemet/sessionmngr/internal/summarizer"
	"github.com/robert-nemet/sessionmngr/internal/version"
	"github.com/spf13/cobra"
)

var (
	sessionID    string
	summaryID    string
	startIdx     int
	endIdx       int
	format       string
	showAll      bool
	pageNumber   int
	promptFile   string
	promptType   string
	exportLatest bool
	exportIndex  int
)

func main() {
	setupCommands()
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

var rootCmd = &cobra.Command{
	Use:   "session-summarizer",
	Short: "AI-powered conversation summarizer for Claude sessions",
	Long: `session-summarizer generates AI-powered summaries of Claude conversations.

It reads sessions from the session-manager storage and uses AI (Anthropic or OpenAI)
to create structured summaries suitable for technical documentation.

Configuration via environment variables:
  ANTHROPIC_API_KEY       Anthropic API key (required for anthropic provider)
  OPENAI_API_KEY          OpenAI API key (required for openai provider)
  SUMMARIZER_PROVIDER     AI provider: anthropic or openai (default: auto-detect)
  SUMMARIZER_MODEL        Model to use (default: claude-sonnet-4-20250514)
  SUMMARIZER_MIN_MESSAGES Minimum messages for summarization (default: 10)
  SESSIONS_MCP_STORAGE    Storage location (default: ~/claude-sessions)`,
}

var summarizeCmd = &cobra.Command{
	Use:   "summarize",
	Short: "Generate a summary for a session",
	Long: `Generate an AI-powered summary for a session or message range.

Examples:
  # Summarize entire session with default prompt
  session-summarizer summarize -s abc12345

  # Use built-in prompt type (operational, business, troubleshooting, evaluation)
  session-summarizer summarize -s abc12345 --type operational
  session-summarizer summarize -s abc12345 -t business

  # Use custom prompt file (overrides --type)
  session-summarizer summarize -s abc12345 --prompt-file my-prompt.txt

  # Summarize specific message range
  session-summarizer summarize -s abc12345 --start 10 --end 50`,
	RunE: runSummarize,
}

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List summaries for a session",
	Long: `List all summaries for a given session.

Example:
  session-summarizer list --session abc12345`,
	RunE: runList,
}

var exportCmd = &cobra.Command{
	Use:   "export",
	Short: "Export a summary",
	Long: `Export a summary in JSON or Markdown format.

Examples:
  # Export most recent summary as Markdown
  session-summarizer export -s abc12345 --latest -f markdown

  # Export by index (1=most recent, matches 'list' output)
  session-summarizer export -s abc12345 -n 1 -f markdown
  session-summarizer export -s abc12345 -n 2 -f json

  # Export by summary ID
  session-summarizer export -s abc12345 --summary def45678

  # Auto-selects if session has only one summary
  session-summarizer export -s abc12345 -f markdown`,
	RunE: runExport,
}

var sessionsCmd = &cobra.Command{
	Use:   "sessions",
	Short: "List all sessions with summary status",
	Long: `List all sessions showing ID, title, message count, and number of summaries.

Examples:
  # List sessions (first page, 20 per page)
  session-summarizer sessions

  # List all sessions
  session-summarizer sessions --all

  # List specific page
  session-summarizer sessions --page 2`,
	RunE: runSessions,
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Run: func(_ *cobra.Command, _ []string) {
		fmt.Printf("session-summarizer %s (built %s)\n", version.Version, version.BuildTimestamp)
	},
}

func setupCommands() {
	summarizeCmd.Flags().StringVarP(&sessionID, "session", "s", "", "Session ID (required)")
	summarizeCmd.Flags().IntVar(&startIdx, "start", -1, "Start message index (optional)")
	summarizeCmd.Flags().IntVar(&endIdx, "end", -1, "End message index (optional)")
	summarizeCmd.Flags().StringVarP(&promptFile, "prompt-file", "p", "", "Custom prompt file (optional)")
	summarizeCmd.Flags().StringVarP(&promptType, "type", "t", "", "Prompt type: operational|business|troubleshooting|evaluation")
	_ = summarizeCmd.MarkFlagRequired("session")

	listCmd.Flags().StringVarP(&sessionID, "session", "s", "", "Session ID (required)")
	_ = listCmd.MarkFlagRequired("session")

	exportCmd.Flags().StringVarP(&sessionID, "session", "s", "", "Session ID (required)")
	exportCmd.Flags().StringVar(&summaryID, "summary", "", "Summary ID (optional, use with --latest or -n)")
	exportCmd.Flags().BoolVarP(&exportLatest, "latest", "l", false, "Export most recent summary")
	exportCmd.Flags().IntVarP(&exportIndex, "index", "n", 0, "Export by index (1=most recent)")
	exportCmd.Flags().StringVarP(&format, "format", "f", "json", "Output format: json or markdown")
	_ = exportCmd.MarkFlagRequired("session")

	sessionsCmd.Flags().BoolVarP(&showAll, "all", "a", false, "Show all sessions (no pagination)")
	sessionsCmd.Flags().IntVarP(&pageNumber, "page", "p", 1, "Page number (default: 1)")

	rootCmd.AddCommand(summarizeCmd)
	rootCmd.AddCommand(listCmd)
	rootCmd.AddCommand(exportCmd)
	rootCmd.AddCommand(sessionsCmd)
	rootCmd.AddCommand(versionCmd)
}

func createSummarizer(promptFilePath string) (*summarizer.Summarizer, error) {
	cfg := config.NewConfig()
	store := storage.NewBasicStorage(cfg)
	return summarizer.New(store, cfg, promptFilePath, "cli")
}

func runSummarize(_ *cobra.Command, _ []string) error {
	// Validate --type if provided
	if promptType != "" {
		if _, ok := summarizer.PromptTypes[promptType]; !ok {
			return fmt.Errorf("invalid prompt type: %s (valid: %v)", promptType, summarizer.ValidPromptTypes())
		}
	}

	// Resolve prompt: --prompt-file takes precedence over --type
	promptArg := promptFile
	if promptArg == "" && promptType != "" {
		promptArg = promptType // LoadPrompt will resolve this to embedded content
	}

	sum, err := createSummarizer(promptArg)
	if err != nil {
		return fmt.Errorf("failed to initialize: %w", err)
	}

	// Create context with timeout for API calls
	ctx, cancel := context.WithTimeout(context.Background(), summarizer.DefaultTimeout)
	defer cancel()

	var result *storage.Summary

	if startIdx >= 0 && endIdx > 0 {
		// Summarize range
		result, err = sum.SummarizeRange(ctx, sessionID, startIdx, endIdx)
	} else {
		// Summarize entire session
		result, err = sum.SummarizeSession(ctx, sessionID)
	}

	if err != nil {
		return err
	}

	output, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to format output: %w", err)
	}

	fmt.Println(string(output))

	// Suggest export command
	shortSessionID := sessionID
	if len(shortSessionID) > 8 {
		shortSessionID = shortSessionID[:8]
	}
	fmt.Fprintf(os.Stderr, "\nExport with:\n  session-summarizer export -s %s --latest -f markdown\n", shortSessionID)

	return nil
}

func runList(_ *cobra.Command, _ []string) error {
	sum, err := createSummarizer("")
	if err != nil {
		return fmt.Errorf("failed to initialize: %w", err)
	}
	ctx := context.Background()

	entries, err := sum.ListSummaries(ctx, sessionID)
	if err != nil {
		return err
	}

	if len(entries) == 0 {
		fmt.Println("No summaries found for this session.")
		return nil
	}

	// Sort by date (most recent first)
	storage.SortSummariesByDate(entries)

	// Print table with index
	fmt.Printf("%-3s  %-8s  %-40s  %s\n", "#", "ID", "TITLE", "CREATED")
	fmt.Printf("%-3s  %-8s  %-40s  %s\n", "---", "--------", "----------------------------------------", "----------------")

	for i, e := range entries {
		shortID := e.ID
		if len(shortID) > 8 {
			shortID = shortID[:8]
		}
		title := e.Title
		if len(title) > 40 {
			title = title[:37] + "..."
		}
		fmt.Printf("%-3d  %-8s  %-40s  %s\n",
			i+1, shortID, title, e.CreatedAt.Format("2006-01-02 15:04"))
	}

	return nil
}

func runExport(_ *cobra.Command, _ []string) error {
	sum, err := createSummarizer("")
	if err != nil {
		return fmt.Errorf("failed to initialize: %w", err)
	}

	ctx := context.Background()
	// Resolve which summary to export
	resolvedSummaryID, err := sum.SelectSummary(ctx, sessionID, summarizer.SelectSummaryOptions{
		SummaryID: summaryID,
		Latest:    exportLatest,
		Index:     exportIndex,
	})
	if err != nil {
		return err
	}

	s, err := sum.LoadSummary(ctx, sessionID, resolvedSummaryID)
	if err != nil {
		return err
	}

	switch strings.ToLower(format) {
	case "json":
		output, err := json.MarshalIndent(s, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to format output: %w", err)
		}
		fmt.Println(string(output))

	case "markdown", "md":
		fmt.Println(s.Content.Markdown)
		sessionPrefix := s.SessionID
		if len(sessionPrefix) > 8 {
			sessionPrefix = sessionPrefix[:8]
		}
		summaryPrefix := s.ID
		if len(summaryPrefix) > 8 {
			summaryPrefix = summaryPrefix[:8]
		}
		fmt.Printf("\n---\n*Summary ID: %s | Session: %s | Model: %s | Messages: %d-%d*\n",
			summaryPrefix, sessionPrefix, s.Model,
			s.MessageRange.Start, s.MessageRange.End)

	default:
		return fmt.Errorf("unknown format: %s (use json or markdown)", format)
	}

	return nil
}

func runSessions(_ *cobra.Command, _ []string) error {
	cfg := config.NewConfig()
	store := storage.NewBasicStorage(cfg)

	const perPage = 20
	ctx := context.Background()
	if showAll {
		// Fetch all sessions
		return printAllSessions(store, perPage)
	}

	// Fetch single page
	sessions, totalPages, totalSessions, err := store.ListSessions(ctx, pageNumber, perPage, nil)
	if err != nil {
		return fmt.Errorf("failed to list sessions: %w", err)
	}

	if totalSessions == 0 {
		fmt.Println("No sessions found.")
		return nil
	}

	printSessionsTable(store, sessions)
	fmt.Printf("\nPage %d/%d (total: %d sessions)\n", pageNumber, totalPages, totalSessions)

	return nil
}

func printAllSessions(store storage.Storage, perPage int) error {
	var allSessions []storage.SessionSummary
	ctx := context.Background()
	page := 1
	for {
		sessions, _, total, err := store.ListSessions(ctx, page, perPage, nil)
		if err != nil {
			return fmt.Errorf("failed to list sessions: %w", err)
		}

		if total == 0 {
			fmt.Println("No sessions found.")
			return nil
		}

		allSessions = append(allSessions, sessions...)

		if len(allSessions) >= total {
			break
		}
		page++
	}

	printSessionsTable(store, allSessions)
	fmt.Printf("\nTotal: %d sessions\n", len(allSessions))

	return nil
}

func printSessionsTable(store storage.Storage, sessions []storage.SessionSummary) {
	// Print header
	fmt.Printf("%-8s  %-40s  %5s  %9s\n", "ID", "TITLE", "MSGS", "SUMMARIES")
	fmt.Printf("%-8s  %-40s  %5s  %9s\n", "--------", "----------------------------------------", "-----", "---------")
	ctx := context.Background()
	for _, sess := range sessions {
		// Get summary count
		summaryCount := 0
		summaries, err := store.ListSummaries(ctx, sess.UUID)
		if err == nil {
			summaryCount = len(summaries)
		}

		// Truncate title if too long
		title := sess.Title
		if len(title) > 40 {
			title = title[:37] + "..."
		}

		// Show first 8 chars of UUID (consistent with MCP server)
		shortID := sess.UUID
		if len(shortID) > 8 {
			shortID = shortID[:8]
		}

		fmt.Printf("%-8s  %-40s  %5d  %9d\n", shortID, title, sess.MessageCount, summaryCount)
	}
}
