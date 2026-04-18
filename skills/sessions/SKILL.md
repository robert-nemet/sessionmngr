---
name: sessions
description: List sessions, show their title, message count, tags, and number of summaries.
compatibility: Requires session-manager MCP server
metadata:
  author: session-manager
  version: "1.0"
---

## Workflow

User invokes it with `/sessions [filter] [page:n]`
1. Call `list-sessions`.

2. If user provided `filter` then show only sessions filtered by tag or title or date that matches fully or partially `filter`. Fetch all pages by calling `list-sessions` repeatedly, incrementing `page` until `page == total_pages`, then filter across all results.

3. If user provided page number, `page:3` for example 3rd page, show sessions on that page.

4. If user provided `filter` and `page` combine point 2. and 3.

5. When listing session show session id, title, number of messages, date, number of summaries, tags.
