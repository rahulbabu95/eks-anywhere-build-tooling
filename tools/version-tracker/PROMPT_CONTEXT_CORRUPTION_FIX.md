# Prompt Context Corruption Issues

## Issues Found in Attempt 2 Prompt

### Issue 1: Stale File Status (Lines 126-135)

**What the prompt shows:**
```markdown
### go.mod

**Status**: ❌ FAILED (see detailed context above)

**Action Required**: Fix this file to resolve the conflict
```

**What actually happened:**
- Attempt 1: go.mod FAILED (original patch) ✅
- Attempt 1: LLM fixed go.mod successfully ✅
- Attempt 1: LLM patch failed on go.sum:935 ❌
- Attempt 2: Prompt STILL shows go.mod as FAILED ❌ (WRONG!)

**The problem:** We're showing the ORIGINAL patch status, not the CURRENT status after attempt 1.

### Issue 2: Misleading Status in "Patch Application Status" (Line 227)

**What the prompt shows:**
```markdown
**Patch Application Status:**
- ❌ FAILED (needs fixing): go.mod
- ⚠️  APPLIED WITH OFFSET (needs line number update): go.sum (offset: 2 lines)
```

**What it should show for attempt 2:**
```markdown
**Original Patch Status (for reference):**
- ❌ FAILED: go.mod (fixed in attempt 1)
- ⚠️  APPLIED WITH OFFSET: go.sum (offset: 2 lines)

**Attempt 1 Result:**
- ✅ go.mod: Successfully fixed
- ❌ go.sum: Failed at line 935 (line number mismatch)
```

## Root Cause

We're reusing the `baseContext` which contains the ORIGINAL patch status (from the first `git apply --reject`). This is correct for showing what the original patch did, but we need to ALSO show what happened in previous LLM attempts.

## The Solution

We need to track TWO types of status:

1. **Original Patch Status** (from base context) - What failed when applying the original patch
2. **Current Status** (from LLM attempts) - What's actually failing NOW

### Implementation

#### Step 1: Add tracking for LLM attempt results

```go
type AttemptResult struct {
    AttemptNumber int
    SuccessfulFiles []string  // Files that were fixed successfully
    FailedFiles     []string  // Files that failed in this attempt
    Error           string    // Error message
}

// In PatchContext
type PatchContext struct {
    // ... existing fields ...
    AttemptResults []AttemptResult // Track what happened in each attempt
}
```

#### Step 2: Update context after each attempt

```go
// After LLM patch fails
if err := ApplyPatchFix(fix, projectPath); err != nil {
    // Parse which file actually failed
    failedFile := parseFailedFile(err.Error()) // "go.sum" from "error: patch failed: go.sum:935"
    
    // Store attempt result
    attemptResult := AttemptResult{
        AttemptNumber: attempt,
        FailedFiles:   []string{failedFile},
        Error:         err.Error(),
    }
    
    // Update base context with attempt result
    baseContext.AttemptResults = append(baseContext.AttemptResults, attemptResult)
    
    // ... rest of error handling ...
}
```

#### Step 3: Update prompt to show both statuses

```go
// In BuildPrompt()

// Show ORIGINAL patch status
prompt.WriteString("## Original Patch Status\n\n")
prompt.WriteString("This shows what happened when the original patch was applied:\n\n")

for filename, context := range ctx.AllFileContexts {
    // Check if this file has a .rej (failed in ORIGINAL patch)
    hasFailed := false
    for _, hunk := range ctx.FailedHunks {
        if strings.Contains(hunk.FilePath, filename) {
            hasFailed = true
            break
        }
    }
    
    // Check if this file has offset (in ORIGINAL patch)
    hasOffset := false
    offsetAmount := 0
    if ctx.ApplicationResult != nil {
        if offset, ok := ctx.ApplicationResult.OffsetFiles[filename]; ok {
            hasOffset = true
            offsetAmount = offset
        }
    }
    
    prompt.WriteString(fmt.Sprintf("### %s\n", filename))
    if hasFailed {
        prompt.WriteString("**Original Status**: ❌ FAILED\n")
    } else if hasOffset {
        prompt.WriteString(fmt.Sprintf("**Original Status**: ⚠️ APPLIED WITH OFFSET (+%d lines)\n", offsetAmount))
    } else {
        prompt.WriteString("**Original Status**: ✅ APPLIED CLEANLY\n")
    }
    
    // Show CURRENT status (after LLM attempts)
    if len(ctx.AttemptResults) > 0 {
        lastAttempt := ctx.AttemptResults[len(ctx.AttemptResults)-1]
        
        // Check if this file failed in last attempt
        failedInLastAttempt := false
        for _, failedFile := range lastAttempt.FailedFiles {
            if strings.Contains(filename, failedFile) {
                failedInLastAttempt = true
                break
            }
        }
        
        if failedInLastAttempt {
            prompt.WriteString(fmt.Sprintf("**Current Status (Attempt %d)**: ❌ FAILED\n", lastAttempt.AttemptNumber))
            prompt.WriteString("**Action Required**: This file needs fixing\n\n")
        } else if hasFailed {
            prompt.WriteString(fmt.Sprintf("**Current Status (Attempt %d)**: ✅ FIXED\n", lastAttempt.AttemptNumber))
            prompt.WriteString("**Action Required**: Keep the fix, no changes needed\n\n")
        }
    }
    
    // Show pristine content
    prompt.WriteString("**Original content (BEFORE any patches):**\n")
    prompt.WriteString("```\n")
    prompt.WriteString(context)
    prompt.WriteString("\n```\n\n")
}
```

### Expected Result

**Attempt 2 prompt:**
```markdown
## Original Patch Status

This shows what happened when the original patch was applied:

### go.mod
**Original Status**: ❌ FAILED
**Current Status (Attempt 1)**: ✅ FIXED
**Action Required**: Keep the fix, no changes needed

**Original content (BEFORE any patches):**
```
[pristine content]
```

### go.sum
**Original Status**: ⚠️ APPLIED WITH OFFSET (+2 lines)
**Current Status (Attempt 1)**: ❌ FAILED
**Action Required**: This file needs fixing

**Original content (BEFORE any patches):**
```
[pristine content showing v1.2.8]
```

## Previous Attempt #1 Failed

**Error**: patch failed: go.sum:935

**Analysis**:
- go.mod was successfully fixed ✅
- go.sum failed because the line numbers don't match
- The patch tried to apply changes at line 933, but they should be at line 935 (offset +2)
```

## Additional Enhancement: Preserve Commit Message

The prompt should emphasize preserving the EXACT commit message:

```markdown
## Task
Generate a corrected patch that:
1. **Preserves the EXACT metadata from the original patch:**
   - From: Abhay Krishna Arunachalam <arnchlm@amazon.com>
   - Date: Wed, 7 Feb 2024 22:30:29 -0800
   - Subject: [PATCH] Replace timestamp-authority and go-fuzz-headers revisions
   
   **CRITICAL**: Do NOT modify the Subject line. Keep it exactly as shown above.

2. Includes ALL files from the original patch
3. For FIXED files (go.mod): Include the working fix from attempt 1
4. For FAILED files (go.sum): Fix the line number issue
5. Uses RELATIVE file paths NOT absolute paths
6. Will compile successfully
```

## Summary

**Two fixes needed:**

1. **Track LLM attempt results** - Know which files were fixed and which failed in each attempt
2. **Show both statuses** - Original patch status + Current status after LLM attempts
3. **Emphasize commit message preservation** - Make it clear the Subject line should not change

This will give the LLM accurate information about what's actually failing and what's already been fixed.
