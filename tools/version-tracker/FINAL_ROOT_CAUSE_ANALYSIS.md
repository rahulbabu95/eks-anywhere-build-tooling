# Final Root Cause Analysis: Context Pollution

## Problem Confirmed

After testing, the prompts STILL show polluted context in both attempt 1 and attempt 2.

### Evidence from Prompts

**Attempt 1 - go.mod content (line 11):**
```
replace github.com/sigstore/timestamp-authority => github.com/sigstore/timestamp-authority v1.2.0
```
**This line is ALREADY THERE!** The file is in a dirty state.

**Attempt 2 - go.sum content:**
```
github.com/sigstore/timestamp-authority v1.2.0 h1:Ffk10QsHxu6aLwySQ7WuaoWkD63QkmcKtozlEFot/VI=
```
**This shows v1.2.0 instead of v1.2.8!** The file has been modified.

### Misleading Status Messages

**Attempt 2, Lines 295-300:**
```
### go.mod
**Status**: ❌ FAILED (see detailed context above)
```
**WRONG!** The error message clearly says "Applied patch go.mod cleanly" but we're showing it as FAILED!

**Attempt 2, Lines 400-410:**
```
**Patch Application Status:**
- ❌ FAILED (needs fixing): go.mod, go.sum
```
**WRONG!** go.mod succeeded in attempt 1, only go.sum failed!

## The Complete Flow (What's Actually Happening)

### Initial State
```
Repo: CLEAN (v1.2.8 in go.sum)
```

### Step 1: Apply Original Patch with --reject
```
git apply --reject original.patch
→ go.mod: FAILS (creates go.mod.rej)
→ go.sum: FAILS (creates go.sum.rej)
→ BUT: Files are MODIFIED (partial application)

Repo State: DIRTY
- go.mod: Has timestamp-authority line added (partial success)
- go.sum: Has v1.2.0 (partial success)
```

### Step 2: Extract Base Context
```
extractPristineContent() reads from DIRTY files!
→ go.mod shows: timestamp-authority line present
→ go.sum shows: v1.2.0

Context: POLLUTED
```

### Step 3: Attempt 1
```
Use polluted context
LLM generates fix based on WRONG state
Apply LLM patch → FAILS (because state is wrong)
Revert (added in our fix)
```

### Step 4: Extract NEW Context for Attempt 2
```
Problem: We extract from files that STILL have LLM's patch!
We need to revert FIRST, then extract!
```

## The Real Issues

### Issue 1: No Clean State Before Initial Context Extraction
```go
// applySinglePatchWithReject
checkoutCmd := exec.Command("make", "-C", projectPath, checkoutTarget)
// ↑ Doesn't reset if repo already exists!

// Extract pristine content
pristineContent, err := extractPristineContent(patchFile, repoPath)
// ↑ Reads from DIRTY files!

// Apply patch
cmd := exec.Command("git", "-C", repoPath, "apply", "--reject", ...)
// ↑ Modifies files further!
```

**Fix**: Reset to clean state BEFORE extracting pristine content

### Issue 2: No Clean State Before Extracting NEW Context
```go
// Apply LLM's patch
rejFiles, patchResult, applyErr := ApplyPatchFixWithReject(fix.Patch, projectPath)

// Extract NEW context
newContext, extractErr := ExtractPatchContext(rejFiles, ...)
// ↑ Extracts from files with LLM's patch still applied!

// Revert
RevertPatchFix(projectPath)
// ↑ Too late! Context already extracted from dirty state!
```

**Fix**: Revert BEFORE extracting new context

### Issue 3: Static Context Reuse
We're still showing the ORIGINAL patch status in "Current File States" section, not the actual current status.

## The Correct Flow Should Be

### Initial Setup
```
1. Checkout repo
2. RESET to clean state (git reset --hard HEAD)
3. Extract pristine content (NOW truly pristine)
4. Apply original patch with --reject
5. Extract base context from .rej files
6. RESET to clean state again
```

### Each Attempt
```
1. Use context from previous attempt
2. LLM generates fix
3. RESET to clean state
4. Apply LLM's patch with --reject
5. If failures:
   a. Extract NEW context from .rej files
   b. RESET to clean state
   c. Use NEW context for next attempt
6. If success:
   a. Validate and commit
```

## Required Fixes

### Fix 1: Reset Before Initial Context Extraction
```go
func applySinglePatchWithReject(...) {
    // Checkout
    checkoutCmd := exec.Command("make", "-C", projectPath, checkoutTarget)
    
    // CRITICAL: Reset to clean state FIRST
    resetCmd := exec.Command("git", "-C", repoPath, "reset", "--hard", "HEAD")
    cleanCmd := exec.Command("git", "-C", repoPath, "clean", "-fd")
    
    // NOW extract pristine content
    pristineContent, err := extractPristineContent(patchFile, repoPath)
    
    // Apply original patch
    cmd := exec.Command("git", "-C", repoPath, "apply", "--reject", ...)
    
    // Extract context from .rej files
    baseContext, err := ExtractPatchContext(rejFiles, ...)
    
    // Reset again for attempt 1
    resetCmd.Run()
    cleanCmd.Run()
    
    return rejFiles, patchResult, nil
}
```

### Fix 2: Reset Before Extracting NEW Context
```go
// In main loop
for attempt := 1; attempt <= opts.MaxAttempts; attempt++ {
    // LLM generates fix
    fix, err := CallBedrockForPatchFix(&ctx, opts.Model, attempt)
    
    // Reset to clean state
    RevertPatchFix(projectPath)
    
    // Apply LLM's patch
    rejFiles, patchResult, applyErr := ApplyPatchFixWithReject(fix.Patch, projectPath)
    
    if len(rejFiles) > 0 {
        // Extract NEW context from .rej files
        newContext, err := ExtractPatchContext(rejFiles, ...)
        currentContext = newContext
    }
    
    // Reset again for next attempt
    RevertPatchFix(projectPath)
}
```

### Fix 3: Update Status Messages
The "Current File States" section should show:
- For attempt 1: Original patch status
- For attempt 2+: Status from PREVIOUS attempt's LLM patch

## Summary

The pollution happens at THREE points:
1. **Initial extraction**: Reading from dirty repo (already modified by previous runs)
2. **After LLM patch**: Extracting new context before reverting
3. **Status messages**: Showing original status instead of current status

All three need to be fixed for clean, accurate context.
