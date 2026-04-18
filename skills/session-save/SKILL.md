---
name: session-save
description: Save and sync the current conversation to its session. Use when the user says /save, "save session", or "sync session".
compatibility: Requires session-manager MCP server
metadata:
  author: session-manager
  version: "1.0"
---

## Workflow

1. Find the current `session_id` from the conversation context (look for a session_id from a prior `start-session` or `switch-session` call).
   - If none found → stop with: `No active session. Use /new to create one.`

2. Look for a `<!-- session-baseline: <session_id> <N> -->` comment in the conversation (written by session-resume after switching). If found, `baseline = N`. Otherwise `baseline = 0`.

3. Count the messages added after the baseline to get `new_count`. These are the messages to append.

4. Call `append-messages` with `after_count = baseline` and the `new_count` messages (validated append).
   - On success → done.
   - On rejection (server count mismatch) → call `append-messages` without `after_count` (lossy append).

5. Confirm: `Saved (N messages).`

## Message filtering

When passing messages to `append-messages`, include only:
- `user` messages where content is plain text (not tool_result blocks)
- `assistant` messages where content is plain text (not tool_use blocks)

Exclude: tool_result messages, assistant tool_use blocks, thinking blocks, system messages, skill invocation payloads. For assistant messages with mixed content (text + tool_use), include only the text portions.
