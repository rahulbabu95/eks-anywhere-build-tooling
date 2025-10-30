# Critical Bug Fix - BuildError Not Persisting Across Attempts

## The Bug You Discovered

Great catch! The build error was being captured but **never made it to the prompt** because of a critical bug in the loop structure.

## Root Cause

### The Problem

```go
for attempt := 1; attempt <= opts.MaxAttempts; attempt++ {
    // Creates NEW context object - loses previous data!
    ctx, err := ExtractPatchContext(rejFiles, patchFile, projectPath, attempt)
    
    // ... attempt fails ...
    
    // Set BuildError on THIS context
    ctx.BuildError = err.Error()
    ctx.PreviousAttempts = append(ctx.PreviousAttempts, fix.Patch)
    continue  // Loop back to top
    
    // Next iteration: NEW context created, BuildError is LOST!
}
```

### Why It Happened

1. **Attempt 1**: Create context, try to fix, fail, set `ctx.BuildError = "error message"`
2. **Attempt 2**: Create **NEW** context (loses BuildError), try to fix, fail again
3. **Attempt 3**: Create **NEW** context (still no BuildError), try to fix, fail again

The context was being **recreated** at the start of each iteration, so any error information stored in the previous iteration was **lost**.

## The Fix

### Solution

Preserve `BuildError` and `PreviousAttempts` **outside the loop** and restore them after creating the new context:

```go
// Preserve error context across iterations
var previousAttempts []string
var lastBuildError string

for attempt := 1; attempt <= opts.MaxAttempts; attempt++ {
    // Create new context (gets fresh file data)
    ctx, err := ExtractPatchContext(rejFiles, patchFile, projectPath, attempt)
    
    // Restore error context from previous attempts
    ctx.BuildError = lastBuildError
    ctx.PreviousAttempts = previousAttempts
    
    // ... attempt fails ...
    
    // Store for next iteration
    lastBuildError = err.Error()
    previousAttempts = append(previousAttempts, fix.Patch)
    continue
}
```

### What Changed

**File: `tools/version-tracker/pkg/commands/fixpatches/fixpatches.go`**

1. **Added variables outside loop** (line ~121):
   ```go
   var previousAttempts []string
   var lastBuildError string
   ```

2. **Restore context after creation** (line ~135):
   ```go
   // Restore error context from previous attempts
   ctx.BuildError = lastBuildError
   ctx.PreviousAttempts = previousAttempts
   ```

3. **Store to variables instead of context** (3 locations):
   ```go
   // Before:
   ctx.BuildError = err.Error()
   ctx.PreviousAttempts = append(ctx.PreviousAttempts, fix.Patch)
   
   // After:
   lastBuildError = err.Error()
   previousAttempts = append(previousAttempts, fix.Patch)
   ```

## Impact

### Before (Broken)

**Attempt 1 Prompt**:
```markdown
## Failed Hunk #1 in go.mod
[context]

## Task
[instructions]
```

**Attempt 2 Prompt** (should have error, but doesn't):
```markdown
## Failed Hunk #1 in go.mod
[context]

## Task
[instructions]
```

**Attempt 3 Prompt** (should have 2 errors, but doesn't):
```markdown
## Failed Hunk #1 in go.mod
[context]

## Reflection Required
[generic request]

## Task
[instructions]
```

### After (Fixed)

**Attempt 1 Prompt**:
```markdown
## Failed Hunk #1 in go.mod
[context]

## Task
[instructions]
```

**Attempt 2 Prompt** (now has error!):
```markdown
## Failed Hunk #1 in go.mod
[context]

## Previous Attempt #1 Failed

**Error Message:**
```
error: patch failed: go.mod:8
error: go.mod: patch does not apply
```

**What you need to fix:**
Analyze the error message above...

## Task
[instructions]
```

**Attempt 3 Prompt** (now has accumulated errors!):
```markdown
## Failed Hunk #1 in go.mod
[context]

## Previous Attempt #2 Failed

**Error Message:**
```
error: patch failed: go.sum:933
error: go.sum: patch does not apply
```

**What you need to fix:**
Analyze the error message above...

## Reflection Required
[asks for analysis]

## Task
[instructions]
```

## Why This Matters

### Without This Fix
- LLM has **no feedback** about why previous attempts failed
- LLM generates **identical patches** across all attempts
- LLM is **flying blind** - can't learn from mistakes

### With This Fix
- LLM sees **actual error messages** from git apply
- LLM knows **exactly which file and line** failed
- LLM can **analyze the specific mismatch** and fix it
- LLM generates **different patches** on each retry

## Testing

### Verify the Fix

```bash
# Run test
cd test/eks-anywhere-build-tooling
./tools/version-tracker/bin/version-tracker fix-patches \
    --project fluxcd/source-controller \
    --pr 4883 \
    --max-attempts 3 \
    2>&1 | tee auto-patch-$(date +%Y%m%d-%H%M%S).log
```

### Check Prompts

```bash
# Attempt 1 should have NO error section
grep -c "Previous Attempt" /tmp/llm-prompt-attempt-1.txt
# Output: 0

# Attempt 2 should have error section
grep -c "Previous Attempt #1 Failed" /tmp/llm-prompt-attempt-2.txt
# Output: 1

# Attempt 2 should show actual error
grep -A 5 "Error Message" /tmp/llm-prompt-attempt-2.txt
# Should show: error: patch failed: go.mod:8

# Attempt 3 should have error section
grep -c "Previous Attempt #2 Failed" /tmp/llm-prompt-attempt-3.txt
# Output: 1
```

## Summary

This was a **critical bug** that completely prevented the LLM from learning between attempts. The error information was being captured but immediately discarded when the context was recreated.

**The fix**: Preserve error state outside the loop and restore it after creating fresh context.

**The impact**: LLM can now see actual error messages and learn from previous failures, leading to better fixes on retry.

Great debugging! This explains why all 3 attempts in the analysis were generating identical patches - the LLM literally had no new information to work with.
