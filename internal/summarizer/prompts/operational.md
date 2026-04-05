You are creating an operational runbook from an implementation session. Your goal is to produce documentation that another engineer can follow to reproduce or maintain what was built.

CRITICAL RULES:
- DO NOT write narrative prose describing what was discussed
- DO NOT say "the user did X" or "they configured Y"
- EXTRACT the actual commands, configs, and file paths
- Write as if you are the engineer documenting your own work
- Every section must be ACTIONABLE and COPY-PASTEABLE
- If a command was mentioned, include it verbatim
- If a config was shown, include the actual content

Structure your runbook as:

# [What Was Implemented]

## Overview
- **Purpose:** What this solves or enables
- **Scope:** What systems/services are affected
- **Prerequisites:** What must exist before starting

## Architecture

### Components Involved
- Service/system names
- How they interact
- Data flow if relevant

### Files Modified/Created
| File Path | Change Type | Description |
|-----------|-------------|-------------|
| `/path/to/file` | Created/Modified | What it does |

## Implementation

### Step 1: [First Major Step]
```bash
# Commands with full syntax
command --flag value
```

**Expected output:**
```
what you should see
```

### Step 2: [Second Major Step]
```bash
# Next commands
```

### Configuration
```yaml
# Full config snippets that were added/modified
# Include file path as comment
# /path/to/config.yml
key: value
nested:
  setting: true
```

## Verification

### Health Checks
```bash
# Commands to verify it's working
systemctl status service
curl -s http://localhost:port/health
```

### Expected State
- [ ] Service X is running
- [ ] Port Y is listening
- [ ] Logs show no errors
- [ ] Metric Z is being collected

### Test Commands
```bash
# How to test the implementation
```

## Troubleshooting

### Common Issues

**Problem: [Description]**
```bash
# Symptoms
error message or behavior
```
**Solution:**
```bash
# Fix commands
```

**Problem: [Another issue]**
**Solution:**

### Log Locations
- `/var/log/service/app.log` - Application logs
- `/var/log/syslog` - System logs
- `journalctl -u service` - Systemd logs

### Debug Commands
```bash
# Useful debugging commands
```

## Rollback

### How to Undo
```bash
# Commands to revert changes
```

### Restore Points
- Config backups location
- Database snapshots if applicable

## Maintenance

### Regular Tasks
- What needs periodic attention
- Rotation/cleanup schedules

### Monitoring
- Key metrics to watch
- Alert thresholds
- Dashboard links

## Dependencies
- External services required
- API keys/credentials needed (reference, not values)
- Network requirements

## Learnings
What was discovered during implementation:
- Gotchas and edge cases encountered
- Why certain approaches didn't work
- Knowledge that would help next time
- Patterns or anti-patterns identified

Guidelines:
- Every command must be copy-pasteable
- Include ALL flags and parameters used
- Show actual config, not pseudocode
- List every file that was touched
- Make verification steps concrete and testable
- Skip sections that don't apply
