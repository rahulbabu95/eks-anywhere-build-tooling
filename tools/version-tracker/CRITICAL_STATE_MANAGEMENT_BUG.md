# CRITICAL BUG: State Management Issue

## Problem Discovered

After analyzing `auto-patch-source-controller.log` and the LLM prompts, we found a **critical state management bug**:

### The Issue

**Prompts show stale failure context across attempts:**
- Attempt 1: Shows go.mod and go.sum as FAILED
- Attempt 2: STILL shows go.mod and go.sum as FAILED (even though go.mod might be fixed)
- Attempt 3: STILL shows BOTH as FAILED

**But looking at the file content in the prompt:**
```
Current file content (around line 8):
...
replace github.com/sigstore/timestamp-authority => github.com/sigstore/timestamp-authority v1.2.0
...
```

**The fix is ALREADY APPLIED in the file!** Yet we're telling the LLM it failed!

## Root Cause

We're extracting context from a **DIRTY repository state**, not a clean one.

### What's Happening

1. **First run** (or previous test):
   - Repo is checked out
   - Patch is applied with `--reject`
   - Some hunks succeed, some fail
   - Files are MODIFIED

2. **Second run** (current):
   - Repo is ALREADY checked out (marker file exists)
   - `make checkout-target` does NOTHING (already done)
   - Files are STILL MODIFIED from previous run
   - We extract "pristine" content from MODIFIED files
   - Context shows wrong state

### The Bug

```go
// applySinglePatchWithReject
checkoutCmd := exec.Command("make", "-C", projectPath, checkoutTarget)
// ↑ This does NOTHING if repo already checked out!

// Extract pristine content
pristineContent, err := extractPristineContent(patchFile, repoPath)
// ↑ This reads from MODIFIED files, not pristine!

// Apply patch
cmd := exec.Command("git", "-C", repoPath, "apply", "--reject", ...)
// ↑ This applies on top of ALREADY MODIFIED files!
```

## Evidence from Logs

### Attempt 1 Prompt
```
## Failed Hunk #1 in go.mod
### Current file content:
replace github.com/sigstore/timestamp-authority => github.com/sigstore/timestamp-authority v1.2.0
```
**The line is ALREADY THERE!**

### Attempt 1 Error
```
error: patch failed: go.mod:8
error: go.mod: patch does not apply
```
**Of course it doesn't apply - it's already applied!**

### Attempt 2 Prompt
```
## Failed Hunk #1 in go.mod
[same stale context]
```
**We're reusing the SAME stale context!**

## The Fix Needed

We need to ensure a **CLEAN repository state** before extracting context:

### Option 1: Clean Before Checkout
```go
// Remove the repo directory entirely
os.RemoveAll(repoPath)

// Checkout fresh
checkoutCmd := exec.Command("make", "-C", projectPath, checkoutTarget)

// Now extract pristine content from CLEAN state
pristineContent, err := extractPristineContent(patchFile, repoPath)
```

### Option 2: Git Reset
```go
// Reset repo to clean state
exec.Command("git", "-C", repoPath, "reset", "--hard", "HEAD").Run()
exec.Command("git", "-C", repoPath, "clean", "-fd").Run()

// Now extract pristine content
pristineContent, err := extractPristineContent(patchFile, repoPath)
```

### Option 3: Remove Marker File
```go
// Remove the checkout marker to force re-checkout
markerFile := filepath.Join(projectPath, repoName, fmt.Sprintf("eks-anywhere-checkout-%s", gitTag))
os.Remove(markerFile)

// Now checkout will run fresh
checkoutCmd := exec.Command("make", "-C", projectPath, checkoutTarget)
```

## Why This Matters

This bug causes:
1. **Misleading prompts**: LLM sees wrong file state
2. **Wasted attempts**: LLM tries to fix already-fixed files
3. **Confusion**: Mixed signals about what's actually broken
4. **Failed fixes**: Patches don't apply because files already modified

## Recommended Fix

**Option 2 (Git Reset)** is best because:
- ✅ Fast (no re-download)
- ✅ Reliable (git guarantees clean state)
- ✅ Safe (doesn't affect other files)

### Implementation

```go
func applySinglePatchWithReject(patchFile string, projectPath string, repoName string) ([]string, *types.PatchApplicationResult, error) {
    // ... existing checkout code ...
    
    // CRITICAL: Reset repo to clean state before extracting context
    logger.Info("Resetting repository to clean state")
    resetCmd := exec.Command("git", "-C", repoPath, "reset", "--hard", "HEAD")
    if err := resetCmd.Run(); err != nil {
        logger.Info("Warning: git reset failed", "error", err)
    }
    
    cleanCmd := exec.Command("git", "-C", repoPath, "clean", "-fd")
    if err := cleanCmd.Run(); err != nil {
        logger.Info("Warning: git clean failed", "error", err)
    }
    
    logger.Info("Repository reset to clean state")
    
    // NOW extract pristine content from CLEAN state
    pristineContent, err := extractPristineContent(patchFile, repoPath)
    // ...
}
```

## Impact

**Without this fix:**
- ❌ Context is wrong
- ❌ LLM is confused
- ❌ Fixes don't work
- ❌ Wasted API calls

**With this fix:**
- ✅ Context is correct
- ✅ LLM sees true state
- ✅ Fixes apply correctly
- ✅ Efficient attempts

## Next Steps

1. Implement git reset before extracting context
2. Test with clean repo
3. Verify prompts show correct state
4. Validate fixes apply successfully
