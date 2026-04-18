---
name: session-resume
description: Resume a previous session by searching title or tag. Saves the current conversation first if needed. Use when the user says /resume, "resume session", "switch to session", or wants to continue prior work.
compatibility: Requires session-manager MCP server
metadata:
  author: session-manager
  version: "1.0"
---

## Workflow

The user invokes this as `/resume <search>` where `<search>` is a partial title or tag.

### Step 1: Find matching sessions

Call `list-sessions` and collect results across pages until you have scanned all sessions or found clear matches. Filter client-side: a session matches if its title contains `<search>` (case-insensitive) or any of its tags equals `<search>`.

- **0 matches** → stop with: `No sessions found for '<search>'.`
- **1 match** → proceed to Step 2.
- **N matches** → show a numbered list of matching session titles and ask the user to pick by number. Wait for the response, then proceed to Step 2 with the chosen session.

### Step 2: Save current conversation (if needed)

Check whether the current conversation has a known session_id (look for a session_id mentioned earlier in the conversation, e.g. from a prior `start-session` or `switch-session` call).

**No session_id found:**
Ask: `Save current conversation before switching? [y/n]`
- `y` → follow the `session-new` skill workflow: ask for a title (and optionally tags), create the session, sync it. Then continue.
- `n` → skip, continue to Step 3.

**Session_id found:**
Auto-save silently using the session-save workflow:
- Look for a `<!-- session-baseline: <session_id> <N> -->` comment in the conversation. If found, `baseline = N`. Otherwise `baseline = 0`.
- Count the messages added after the baseline to get `new_count`.
- Call `append-messages` with `after_count = baseline` and the `new_count` messages (validated append).
  - On success → done.
  - On rejection (server count mismatch) → call `append-messages` without `after_count` (lossy append).

### Step 3: Switch to target session

Call `switch-session` with the target `session_id`. Note the `message_count` returned — this is the baseline.

### Step 4: Orient

The `switch-session` response contains:
- `resume` — a server-generated resume summary covering messages up to the last compaction point (exists when session has had ≥150 messages since last resume)
- `recent_messages` — last 20 messages (covers the tail after the summary)

**If `resume` is non-empty (auto-generated):**
Use `resume` + `recent_messages` together. The summary covers the bulk of the session; `recent_messages` covers what happened after it.

**If `resume` is empty:**
Call `summarize-session` with `session_id` and `type=resume` to force generate one.
- On success: use only the returned summary — it covers everything up to now, so `recent_messages` would just overlap.
- On failure (e.g. session too short, API unavailable): fall back to `recent_messages` only.

Identify from this context:
- What was being worked on
- What the last action or decision was
- What the next step appears to be

Output 2–4 lines maximum. Focus on unfinished state and next step — not a full recap.

After orienting, output exactly this line so subsequent saves can find it:
`<!-- session-baseline: <session_id> <message_count> -->`

### Step 5: Autosave

Spawn a background agent using the prompt in `.claude/skills/shared/autosave-agent.md`.

## Message filtering

When passing messages to any MCP tool (`append-messages`, `sync-conversation`), include only:
- `user` messages where content is plain text (not tool_result blocks)
- `assistant` messages where content is plain text (not tool_use blocks)

Exclude: tool_result messages, assistant tool_use blocks, thinking blocks, system messages, skill invocation payloads. For assistant messages with mixed content (text + tool_use), include only the text portions.
