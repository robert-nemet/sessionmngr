You are creating an incident report from a troubleshooting session. Your goal is to help engineers quickly fix this issue if it recurs.

Structure your report as:

# [Problem Title]

## Incident Summary
- **Symptom:** What was broken
- **Impact:** What was affected
- **Root Cause:** One-line cause
- **Resolution:** One-line fix

## Investigation

### Diagnostic Commands
```bash
# Every command run during investigation
# Include what each revealed
```

### Key Findings
- Error messages observed
- Logs examined and timestamps
- What pointed to the root cause

## Root Cause
- Exact cause of the failure
- Why it happened
- Contributing factors

## Key Data Points
List ALL numbers, metrics, and measurements discussed:
- Values that indicated the problem
- Thresholds or limits that were exceeded
- Before/after comparisons
- Any quantitative data that informed decisions

## Solution

### Commands to Fix
```bash
# Exact commands used to resolve
# Copy-pasteable for next time
```

### Config Changes
```yaml
# Actual config modifications
# Include file paths
```

### Files Modified
- `/path/to/file` - what changed

## Verification
```bash
# How to confirm it's fixed
```

**Expected result:**
```
what success looks like
```

## Prevention
- Changes to prevent recurrence
- Alerts to add
- Monitoring gaps

## Learnings
What generalizable lessons came from this incident:
- Patterns to recognize in future
- Assumptions that were wrong
- Knowledge gaps that were filled
- Rules of thumb discovered

## Quick Fix Reference
If this happens again:
```bash
# Minimal commands to resolve
```

Guidelines:
- Include ALL commands with exact syntax
- Show actual error messages
- Make it copy-pasteable
- Focus on "how to fix" not discussion
