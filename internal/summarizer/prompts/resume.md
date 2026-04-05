You are creating or updating a session resume for an AI assistant that will continue this conversation. Your goal is to provide enough context to resume work without reading the full history.

If a PRIOR RESUME is provided, update it integrating the new messages. Preserve all prior key decisions and context unless directly contradicted by new messages. Update Current State and Open Thread to reflect latest activity.

If no prior resume is provided, create a fresh resume from the conversation.

CRITICAL RULES:
- Target 1000-1500 words — long sessions require fidelity, not brevity
- Write for an LLM that needs to continue this conversation, not for a human reader
- Include specific names: files, services, commands, error messages, versions, paths
- Skip pleasantries, introductions, and meta-commentary
- Do not describe the act of summarizing — just output the resume

Structure your resume as:

# [Session Title]

## Goal
What is being accomplished? One or two sentences. Update only if the goal has shifted.

## Current State
Where do things stand NOW? What's working, what's not? Be specific about the latest state.

## Key Decisions
Bullet list of all decisions that affect future work. Include decisions from prior sessions if a prior resume is provided. Only drop a decision if it was explicitly reversed.

## Open Thread
What was being actively worked on at the end of the conversation? What is the immediate next step?

## Context
Important details that would be lost without the full history — accumulate across the full session:
- Error messages or behaviors encountered
- Constraints or requirements discovered
- Names, paths, identifiers, versions referenced
- Approaches tried and abandoned, and why
- Dependencies or external systems involved

Guidelines:
- If the session has no clear goal or is exploratory, say so
- If multiple topics were covered, focus on the most recent active thread
- Omit sections that genuinely don't apply
- When updating from a prior resume, do not append — merge and replace

After your summary, on the last line output exactly:
<!-- tags: tag1, tag2, tag3 -->
Provide 3-5 tags. Lowercase, 4+ chars, only a-z 0-9 hyphens underscores. Categorize by topic, technology, and activity type.
