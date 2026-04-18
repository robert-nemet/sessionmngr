---
name: session-load-full
description: Load the complete message history for the current session. Expensive — uses significant context. Use only when the user explicitly says /load-full or "load full session".
compatibility: Requires session-manager MCP server
metadata:
  author: session-manager
  version: "1.0"
---

## Workflow

1. Find the current `session_id` from the conversation context (look for a session_id from a prior `start-session` or `switch-session` call).
   - If none found → stop with: `No active session. Use /new to create one.`

2. Warn the user: `This loads the complete session history and may use significant context. Continue? [y/n]`

3. If `y` → call `load-complete-session` with the `session_id`.

4. Confirm: `Loaded N messages.`
