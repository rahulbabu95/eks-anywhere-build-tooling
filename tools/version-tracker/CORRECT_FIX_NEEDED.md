# The Correct Fix: Extract Context from LLM's Patch Failures

## The Real Problem

We're reusing the ORIGINAL patch's failure context for all attempts, but we should extract NEW context from each LLM attempt's failures.

## Current Flow (BROKEN)

```
1. Apply ORIGINAL patch with --reject
   → go.mod FAILS, go.sum FAILS
   → Extract context: "go.mod FAILED, go.sum FAILED"
   → Store as baseContext

2. Attempt 1:
   → Use baseContext (go.mod FAILED, go.sum FAILED)
   → LLM generates fix for both
   → Apply LLM's fix
   → go.mod succeeds, go.sum fails
   → Revert to clean

3. Attempt 2:
   → Use SAME baseContext (go.mod FAILED, go.sum FAILED)  ← WRONG!
   → LLM tries to fix both again
   → Wasted effort, confusion
```

## Correct Flow (WHAT WE NEED)

```
1. Apply ORIGINAL patch with --reject
   → go.mod FAILS, go.sum FAILS
   → Extract context: "go.mod FAILED, go.sum FAILED"

2. Attempt 1:
   → Show: go.mod FAILED, go.sum FAILED
   → LLM generates fix for both
   → Apply LLM's fix with --reject
   → go.mod succeeds, go.sum fails
   → Extract NEW context: "go.sum FAILED" (go.mod succeeded!)
   → Revert to clean

3. Attempt 2:
   → Show: go.sum FAILED ONLY  ← CORRECT!
   → LLM generates fix for go.sum only
   → More focused, better chance of success
```

## The Fix

We need to:
1. After LLM generates a fix
2. Apply it with `git apply --reject` (not just `git apply`)
3. Extract NEW failure context from the .rej files
4. Use THAT context for the next attempt
5. Revert to clean state

### Implementation

```go
for attempt := 1; attempt <= opts.MaxAttempts; attempt++ {
    // Use context from previous attempt (or base context for attempt 1)
    ctx := currentContext
    
    // LLM generates fix
    fix, err := CallBedrockForPatchFix(&ctx, opts.Model, attempt)
    
    // Apply fix with --reject to see what fails
    rejFiles, patchResult, err := applyPatchWithReject(fix.Patch, projectPath, repoName)
    
    if len(rejFiles) == 0 {
        // Success! No failures
        return nil
    }
    
    // Extract NEW context from THIS attempt's failures
    newContext, err := ExtractPatchContext(rejFiles, fix.Patch, projectPath, attempt, patchResult)
    
    // Store error message
    newContext.BuildError = err.Error()
    
    // Revert to clean state
    RevertPatchFix(projectPath)
    
    // Use NEW context for next attempt
    currentContext = newContext
}
```

## Key Changes Needed

### 1. Apply LLM Patch with --reject

Instead of:
```go
ApplyPatchFix(fix, projectPath)  // Uses git apply (fails completely)
```

We need:
```go
rejFiles, patchResult, err := applyPatchWithReject(fix.Patch, projectPath, repoName)
// This uses git apply --reject (partial success possible)
```

### 2. Extract Context from LLM's Failures

```go
if len(rejFiles) > 0 {
    // Extract NEW context from LLM's patch failures
    newContext, err := ExtractPatchContext(rejFiles, fix.Patch, projectPath, attempt+1, patchResult)
    newContext.BuildError = lastBuildError
    currentContext = newContext
}
```

### 3. Clean State Between Attempts

Already done with `RevertPatchFix()` which does:
```go
git reset --hard HEAD
git clean -fd
```

## Benefits

1. **Accurate Context**: Shows only what actually failed in the last attempt
2. **Focused Fixes**: LLM fixes only what's broken, not already-fixed files
3. **Better Success Rate**: More targeted fixes = higher success rate
4. **No Confusion**: Clear signal about what needs fixing

## Example

### Original Patch
- go.mod: Add line at position 8
- go.sum: Change 2 lines at position 933

### Attempt 1
- LLM fixes both
- Apply with --reject:
  - go.mod: ✅ SUCCESS
  - go.sum: ❌ FAILED (wrong line numbers)
- Extract context: "go.sum FAILED at line 935"

### Attempt 2
- Show ONLY: "go.sum FAILED at line 935"
- LLM focuses on go.sum only
- Apply with --reject:
  - go.sum: ✅ SUCCESS
- Done!

## Implementation Plan

1. Create `applyPatchWithReject()` function that:
   - Takes a patch string (not file)
   - Applies with `git apply --reject`
   - Returns .rej files and result

2. Modify main loop to:
   - Apply LLM's patch with --reject
   - Extract NEW context from failures
   - Use NEW context for next attempt

3. Keep clean state management:
   - Reset before extracting original context
   - Reset between attempts
   - Always start from clean state
