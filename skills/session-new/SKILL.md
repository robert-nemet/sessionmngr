---
name: session-new
description: Create a new session and sync the current conversation into it. Use when the user says /new, "new session", "create session", or wants to start tracking the current conversation.
compatibility: Requires session-manager MCP server
metadata:
  author: session-manager
  version: "1.0"
---

## Workflow

The user invokes this as `/new <title> [tags]` or similar.

1. Call `start-session` with the provided title.
   - If no title given, ask the user for one before proceeding.
   - The server deduplicates by title — same title returns the existing session.

2. If tags were provided, call `tag-session` with the returned `session_id` and the tag list.

3. Call `sync-conversation` with the full current conversation messages and the `session_id`.

4. Confirm to the user: `Session '<title>' created and synced (N messages).`

## Notes

- Sessions are retroactive — it is fine to create a session for a conversation that is already in progress.
- Tags are optional. If the user provides them (e.g. `/new infra work pp-issue`), include all as the `add` array in `tag-session`.

## Message filtering

When passing messages to `sync-conversation`, include only:
- `user` messages where content is plain text (not tool_result blocks)
- `assistant` messages where content is plain text (not tool_use blocks)

Exclude: tool_result messages, assistant tool_use blocks, thinking blocks, system messages, skill invocation payloads. For assistant messages with mixed content (text + tool_use), include only the text portions.
