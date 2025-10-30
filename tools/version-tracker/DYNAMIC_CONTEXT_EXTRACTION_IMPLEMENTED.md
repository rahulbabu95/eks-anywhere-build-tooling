# Dynamic Context Extraction: The Real Fix

## Problem Identified

After analyzing the logs from `auto-patch-source-controller.log`, we discovered the **real issue**:

**We were reusing the ORIGINAL patch's failure context for ALL attempts**, even though the LLM's fixes might have partially succeeded.

### Evidence from Logs

**Attempt 1 Prompt:**
```
## Failed Hunk #1 in go.mod
### Current file content:
replace github.com/sigstore/timestamp-authority => github.com/sigstore/timestamp-authority v1.2.0
```
**The line was ALREADY THERE!** The file was in a dirty state.

**Attempt 2 Prompt:**
```
## Failed Hunk #1 in go.mod
[same stale context showing go.mod as failed]
```
**Still showing go.mod as failed, even though it might have succeeded in attempt 1!**

## Root Causes

### 1. Dirty Repository State
- Repository was not reset to clean state before extracting context
- Files had modifications from previous runs
- "Pristine" content was actually modified content

### 2. Static Context Reuse
- Extracted context ONCE from original patch
- Reused SAME context for all attempts
- Never extracted NEW context from LLM's patch failures

## The Solution

### Two-Part Fix

#### Part 1: Ensure Clean State (Already Implemented)
```go
// Reset repository to clean state BEFORE extracting context
resetCmd := exec.Command("git", "-C", repoPath, "reset", "--hard", "HEAD")
cleanCmd := exec.Command("git", "-C", repoPath, "clean", "-fd")
```

#### Part 2: Dynamic Context Extraction (NEW)
```go
// Apply LLM's patch with --reject to see what fails
rejFiles, patchResult, err := ApplyPatchFixWithReject(fix.Patch, projectPath)

if len(rejFiles) > 0 {
    // Extract NEW context from THIS attempt's failures
    newContext, err := ExtractPatchContext(rejFiles, patchFile, projectPath, attempt+1, patchResult)
    
    // Use NEW context for next attempt
    currentContext = newContext
}

// Revert to clean state
RevertPatchFix(projectPath)
```

## Implementation Details

### New Function: ApplyPatchFixWithReject

```go
func ApplyPatchFixWithReject(patchContent string, projectPath string) ([]string, *types.PatchApplicationResult, error)
```

**Purpose**: Apply LLM's patch with `--reject` to allow partial success

**Returns**:
- `[]string`: List of .rej files (failures)
- `*types.PatchApplicationResult`: Offset information
- `error`: Error message if any

**Key Features**:
- Uses `git apply --reject` instead of `git apply`
- Allows partial success (some hunks succeed, some fail)
- Extracts offset information
- Returns .rej files for context extraction

### Updated Main Loop

```go
currentContext := baseContext  // Start with original patch context

for attempt := 1; attempt <= opts.MaxAttempts; attempt++ {
    // Use current context
    ctx := *currentContext
    
    // LLM generates fix
    fix, err := CallBedrockForPatchFix(&ctx, opts.Model, attempt)
    
    // Apply with --reject
    rejFiles, patchResult, err := ApplyPatchFixWithReject(fix.Patch, projectPath)
    
    if len(rejFiles) == 0 {
        // Success! Continue to validation
    } else {
        // Extract NEW context from failures
        newContext, err := ExtractPatchContext(rejFiles, patchFile, projectPath, attempt+1, patchResult)
        
        // Use NEW context for next attempt
        currentContext = newContext
        
        // Revert to clean state
        RevertPatchFix(projectPath)
    }
}
```

## Flow Comparison

### Before (BROKEN)

```
Extract context from ORIGINAL patch:
  - go.mod FAILED
  - go.sum FAILED

Attempt 1:
  - Show: go.mod FAILED, go.sum FAILED
  - LLM fixes both
  - Apply → go.mod succeeds, go.sum fails
  - Revert

Attempt 2:
  - Show: go.mod FAILED, go.sum FAILED  ← WRONG!
  - LLM tries to fix both again
  - Wasted effort

Attempt 3:
  - Show: go.mod FAILED, go.sum FAILED  ← STILL WRONG!
  - LLM confused
```

### After (CORRECT)

```
Extract context from ORIGINAL patch:
  - go.mod FAILED
  - go.sum FAILED

Attempt 1:
  - Show: go.mod FAILED, go.sum FAILED
  - LLM fixes both
  - Apply with --reject → go.mod succeeds, go.sum fails
  - Extract NEW context: go.sum FAILED
  - Revert

Attempt 2:
  - Show: go.sum FAILED ONLY  ← CORRECT!
  - LLM fixes go.sum only
  - Apply with --reject → go.sum succeeds
  - Success!
```

## Benefits

### 1. Accurate Context
- Shows only what ACTUALLY failed in the last attempt
- No stale information
- Clear signal to LLM

### 2. Focused Fixes
- LLM fixes only what's broken
- Doesn't waste effort on already-fixed files
- More targeted approach

### 3. Better Success Rate
- Each attempt builds on previous progress
- Incremental improvement
- Higher chance of success

### 4. Efficient Token Usage
- Smaller context (only failed files)
- Lower API costs
- Faster responses

## Example Scenario

### Original Patch
```diff
go.mod: Add 2 lines at position 8
go.sum: Change 2 lines at position 933
```

### Attempt 1
**Context**: go.mod FAILED, go.sum FAILED

**LLM Fix**: Fixes both files

**Apply Result**:
- go.mod: ✅ Applied successfully
- go.sum: ❌ Failed (line numbers off by 2)

**Extract NEW Context**: go.sum FAILED at line 935

### Attempt 2
**Context**: go.sum FAILED at line 935 (go.mod not mentioned!)

**LLM Fix**: Fixes only go.sum with correct line numbers

**Apply Result**:
- go.sum: ✅ Applied successfully

**Success!**

## Testing

### Verify Clean State
```bash
# Before each attempt, check repo is clean
git -C <repo> status
# Should show: "nothing to commit, working tree clean"
```

### Verify Dynamic Context
```bash
# Check prompts show different context
cat /tmp/llm-prompt-attempt-1.txt  # Should show: go.mod FAILED, go.sum FAILED
cat /tmp/llm-prompt-attempt-2.txt  # Should show: ONLY failures from attempt 1
cat /tmp/llm-prompt-attempt-3.txt  # Should show: ONLY failures from attempt 2
```

### Verify Incremental Progress
```bash
# Check that each attempt builds on previous
# Attempt 1: Fixes some files
# Attempt 2: Fixes remaining files
# Not: All attempts try to fix everything
```

## Files Modified

1. **applier.go**:
   - Added `ApplyPatchFixWithReject()` function
   - Applies patch with `--reject` flag
   - Returns .rej files and offset information

2. **fixpatches.go**:
   - Updated main loop to use dynamic context
   - Extracts NEW context after each attempt
   - Uses NEW context for next attempt
   - Added clean state reset before initial context extraction

## Status

✅ **Implemented**
✅ **Compiles successfully**
✅ **No diagnostics**
⏳ **Ready for testing**

## Next Steps

1. Test with real patch that has multiple files
2. Verify prompts show different context across attempts
3. Confirm incremental progress (not repeating same fixes)
4. Validate success rate improvement

## Key Insight

**The problem wasn't just dirty state - it was static context reuse.**

We needed BOTH fixes:
1. Clean state before extracting context (prevents dirty reads)
2. Dynamic context extraction (shows actual failures, not original failures)

Together, these ensure the LLM always sees accurate, current information about what needs to be fixed.
