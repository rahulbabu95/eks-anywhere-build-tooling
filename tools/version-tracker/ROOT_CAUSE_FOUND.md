# Root Cause Found: Re-applying Original Patch Between Attempts

## The Smoking Gun

**Line 165-172 in fixpatches.go:**
```go
// Re-apply original patch with --reject to regenerate .rej files for next attempt
if attempt < opts.MaxAttempts {
    logger.Info("Re-applying original patch with --reject to regenerate .rej files")
    _, _, reapplyErr := applySinglePatchWithReject(patchFile, projectPath, projectRepo)
    if reapplyErr != nil {
        logger.Info("Warning: failed to re-apply patch with --reject", "error", reapplyErr)
    }
}
```

## What's Happening

### Attempt 1:
1. Apply ORIGINAL patch with --reject
   - go.mod FAILS → creates go.mod.rej
   - go.sum SUCCEEDS with offset
2. Extract context from go.mod.rej
3. Prompt shows: "Failed Hunk #1 in go.mod" ✅ (correct for attempt 1)
4. LLM generates fix for go.mod
5. Apply LLM patch → **FAILS on go.sum:933** (because go.sum was already modified)
6. Error: "patch failed: go.sum:933"
7. Revert changes
8. **Re-apply ORIGINAL patch** → creates go.mod.rej AGAIN ❌

### Attempt 2:
1. Extract context from go.mod.rej (STALE! This is from the original patch, not the LLM failure)
2. Prompt shows: "Failed Hunk #1 in go.mod" ❌ (WRONG! go.sum failed, not go.mod)
3. LLM generates fix for go.mod AGAIN
4. Apply LLM patch → **FAILS on go.sum:933 AGAIN**
5. Error: "patch failed: go.sum:933"
6. Revert changes
7. **Re-apply ORIGINAL patch** → creates go.mod.rej AGAIN ❌

### Attempt 3:
Same as attempt 2 - infinite loop of wrong information!

## Why This is Wrong

1. **Stale Information**: We're showing the LLM what failed in the ORIGINAL patch, not what failed in the LLM's attempt
2. **State Pollution**: Re-applying the original patch pollutes the state
3. **Wrong Focus**: LLM keeps trying to fix go.mod when go.sum is the actual problem
4. **No Learning**: Each attempt gets the same stale information, so LLM can't improve

## What Should Happen

### Attempt 1:
1. Apply ORIGINAL patch ONCE
   - go.mod FAILS → creates go.mod.rej
   - go.sum SUCCEEDS with offset
2. Extract context ONCE and STORE it
3. Prompt shows: "Failed Hunk #1 in go.mod" ✅
4. LLM generates fix
5. Apply LLM patch → FAILS on go.sum:933
6. **Parse the ACTUAL error**: "go.sum failed at line 933"
7. Revert to clean state (git reset --hard)
8. **DON'T re-apply original patch** ✅

### Attempt 2:
1. **REUSE stored context** (don't re-extract)
2. **UPDATE context with ACTUAL failure**: "Previous attempt failed on go.sum:933"
3. Prompt shows:
   - "Original patch: go.mod FAILED, go.sum OFFSET"
   - "Attempt 1 result: LLM patch failed on go.sum:933"
   - "Focus: Fix go.sum line numbers"
4. LLM generates better fix
5. Apply LLM patch
6. If fails, parse ACTUAL error again

## The Fix

### Step 1: Extract Context ONCE

```go
func fixSinglePatch(...) error {
    // Apply original patch ONCE
    rejFiles, patchResult, err := applySinglePatchWithReject(patchFile, projectPath, projectRepo)
    
    // Extract context ONCE
    baseContext, err := ExtractPatchContext(rejFiles, patchFile, projectPath, 1, patchResult)
    if err != nil {
        return fmt.Errorf("extracting patch context: %v", err)
    }
    
    // Store pristine context for reuse
    var previousAttempts []string
    var lastBuildError string
    
    for attempt := 1; attempt <= opts.MaxAttempts; attempt++ {
        // REUSE base context, just update error info
        ctx := *baseContext // Copy
        ctx.BuildError = lastBuildError
        ctx.PreviousAttempts = previousAttempts
        
        // Call LLM
        fix, err := CallBedrockForPatchFix(&ctx, opts.Model, attempt)
        
        // Apply LLM fix
        if err := ApplyPatchFix(fix, projectPath); err != nil {
            // Parse ACTUAL error
            lastBuildError = err.Error()
            previousAttempts = append(previousAttempts, fix.Patch)
            
            // Revert to clean state
            RevertPatchFix(projectPath)
            
            // DON'T re-apply original patch! ✅
            continue
        }
        
        // Validate...
    }
}
```

### Step 2: Remove Re-application Code

**DELETE these lines:**
```go
// Re-apply original patch with --reject to regenerate .rej files for next attempt
if attempt < opts.MaxAttempts {
    logger.Info("Re-applying original patch with --reject to regenerate .rej files")
    _, _, reapplyErr := applySinglePatchWithReject(patchFile, projectPath, projectRepo)
    if reapplyErr != nil {
        logger.Info("Warning: failed to re-apply patch with --reject", "error", reapplyErr)
    }
}
```

### Step 3: Parse Actual Errors (Optional Enhancement)

```go
func parseGitApplyError(errorOutput string) string {
    // Parse: "error: patch failed: go.sum:933"
    if strings.Contains(errorOutput, "patch failed:") {
        parts := strings.Split(errorOutput, "patch failed:")
        if len(parts) >= 2 {
            fileInfo := strings.TrimSpace(parts[1])
            return fmt.Sprintf("Patch application failed: %s", fileInfo)
        }
    }
    return errorOutput
}
```

## Expected Behavior After Fix

### Logs:
```
Attempt 1:
  Applying single patch with reject
  Patch has conflicts    {"rej_files": 1, "offset_files": 1}
  Extracting patch context
  Calling Bedrock API
  Applying LLM-generated patch
  git apply failed: "error: patch failed: go.sum:933"
  Failed to apply patch fix    {"error": "...", "attempt": 1}
  Reverting patch changes
  (NO re-application of original patch) ✅

Attempt 2:
  Starting fix attempt    {"attempt": 2}
  (REUSING context, not re-extracting) ✅
  Calling Bedrock API
  Applying LLM-generated patch
```

### Prompt Attempt 2:
```markdown
## Failed Hunk #1 in go.mod
(Original patch failure - for reference)

## Current File States

### go.mod
**Status**: ❌ FAILED (in original patch)

### go.sum
**Status**: ⚠️ APPLIED WITH OFFSET (+2 lines)

## Previous Attempt #1 Failed

**Error Message:**
```
error: patch failed: go.sum:933
error: go.sum: patch does not apply
```

**What you need to fix:**
The previous attempt failed on go.sum at line 933.
This suggests the line numbers in your patch don't match the current file.
```

## Impact

**Before (Broken):**
- ❌ Shows stale go.mod failure on every attempt
- ❌ LLM never learns what actually failed
- ❌ Same mistake repeated 3 times
- ❌ No progress toward solution

**After (Fixed):**
- ✅ Shows original patch status (go.mod failed, go.sum offset)
- ✅ Shows actual LLM patch failure (go.sum:933)
- ✅ LLM can learn and adjust
- ✅ Progress toward solution
