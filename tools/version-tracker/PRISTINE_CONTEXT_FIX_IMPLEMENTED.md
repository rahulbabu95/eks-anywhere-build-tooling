# Pristine Context Fix Implemented

## Summary

Successfully implemented the critical fix to extract file context BEFORE `git apply --reject` modifies them. This ensures the LLM sees the original state of files, not the state after partial patch application.

## Problem Solved

### Before (Broken):
```
1. Checkout v1.7.2
2. Run: git apply --reject patch.patch
   → go.mod: FAILS, creates go.mod.rej
   → go.sum: SUCCEEDS with offset, MODIFIES the file (v1.2.8 → v1.2.0) ❌
3. Extract context from files
   → go.sum: reads MODIFIED content (shows v1.2.0) ❌
4. LLM sees v1.2.0 and thinks "it's already done"
5. LLM generates patch that doesn't change go.sum
6. Patch fails because go.sum needs to be included
```

### After (Fixed):
```
1. Checkout v1.7.2
2. Extract PRISTINE content BEFORE applying patch ✅
   → go.mod: captures original content
   → go.sum: captures original content (v1.2.8) ✅
3. Run: git apply --reject patch.patch
   → go.mod: FAILS, creates go.mod.rej
   → go.sum: SUCCEEDS with offset, modifies the file
4. Extract context from PRISTINE content ✅
   → go.sum: shows ORIGINAL v1.2.8 (not modified v1.2.0) ✅
5. LLM sees v1.2.8 and knows to change it to v1.2.0
6. LLM generates correct patch
```

## Changes Made

### 1. Updated Types (`pkg/types/fixpatches.go`)

Added `PristineContent` field to store original file content:

```go
type PatchApplicationResult struct {
    OffsetFiles     map[string]int    // filename -> line offset
    GitOutput       string            // Full git apply output
    PristineContent map[string]string // filename -> content BEFORE git apply ✅
}
```

### 2. Added Pristine Extraction (`pkg/commands/fixpatches/fixpatches.go`)

**New function `extractPristineContent()`:**
- Parses the patch file to find all files being modified
- Reads the original content of each file BEFORE `git apply`
- Stores in a map: filename → pristine content

**Modified `applySinglePatchWithReject()`:**
```go
// CRITICAL: Extract pristine content BEFORE applying patch
logger.Info("Extracting pristine file content before applying patch")
pristineContent, err := extractPristineContent(patchFile, repoPath)

// ... then apply patch ...

// Store pristine content in result
result := &types.PatchApplicationResult{
    OffsetFiles:     make(map[string]int),
    GitOutput:       outputStr,
    PristineContent: pristineContent, // ✅
}
```

### 3. Updated Context Extraction (`pkg/commands/fixpatches/context.go`)

**New function `extractContextFromPristine()`:**
- Extracts context from pristine content (not modified files)
- Shows the LLM the ORIGINAL state before any modifications

**Modified `ExtractPatchContext()`:**
```go
// Use PRISTINE content from patchResult if available
if patchResult != nil && len(patchResult.PristineContent) > 0 {
    logger.Info("Using pristine content from before patch application")
    allFileContexts := extractContextFromPristine(patchFiles, patchResult.PristineContent)
    ctx.AllFileContexts = allFileContexts
} else {
    // Fallback: read from current files (may be modified)
    logger.Info("Warning: no pristine content available")
    // ... fallback logic ...
}
```

### 4. Improved Prompt (`pkg/commands/fixpatches/llm.go`)

**Better status messages for offset files:**
```markdown
**Status**: ⚠️ APPLIED WITH OFFSET (+2 lines)

**IMPORTANT**: This file applied successfully but at different line numbers than expected.
You MUST include this file in your fixed patch with updated line numbers.
The patch expected changes at certain lines, but they were found 2 lines later.

**Original content (BEFORE patch application):**
```
[pristine content showing v1.2.8]
```
```

**Better status messages for all files:**
- ❌ FAILED: "Action Required: Fix this file to resolve the conflict"
- ⚠️ OFFSET: "IMPORTANT: You MUST include this file with updated line numbers"
- ✅ CLEAN: "Action Required: Include this file in your fixed patch"

## Expected Behavior

### Logs Will Show:
```
Extracting pristine file content before applying patch
Captured pristine content    {"file": "go.mod", "size": 1234}
Captured pristine content    {"file": "go.sum", "size": 56789}
Extracted pristine content   {"files": 2}

Applying patch with git apply --reject
Patch application failed with conflicts (expected)

Using pristine content from before patch application    {"files": 2}
Extracted context from pristine files    {"count": 2}
```

### Prompt Will Show:
```markdown
## Current File States

### go.mod

**Status**: ❌ FAILED (see detailed context above)
**Action Required**: Fix this file to resolve the conflict

**Original content (BEFORE patch application):**
```
[pristine go.mod content]
```

### go.sum

**Status**: ⚠️ APPLIED WITH OFFSET (+2 lines)

**IMPORTANT**: This file applied successfully but at different line numbers than expected.
You MUST include this file in your fixed patch with updated line numbers.
The patch expected changes at certain lines, but they were found 2 lines later.

**Original content (BEFORE patch application):**
```
Lines 925-945:
...
github.com/sigstore/timestamp-authority v1.2.8 h1:BEV3fkphwU4zBp3allFAhCqQb99HkiyCXB853RIwuEE=
github.com/sigstore/timestamp-authority v1.2.8/go.mod h1:G2/0hAZmLPnevEwT1S9IvtNHUm9Ktzvso6xuRhl94ZY=
...
```
```

### LLM Will See:
- ✅ go.sum shows **v1.2.8** (original), not v1.2.0 (modified)
- ✅ Clear instruction to include go.sum in the fixed patch
- ✅ Explanation that line numbers need updating due to offset
- ✅ Complete context for both files

### LLM Will Generate:
```diff
diff --git a/go.mod b/go.mod
[correct go.mod changes]

diff --git a/go.sum b/go.sum
@@ -935,8 +935,8 @@  ← Updated line numbers (935, not 933)
-github.com/sigstore/timestamp-authority v1.2.8 h1:BEV3fkphwU4zBp3allFAhCqQb99HkiyCXB853RIwuEE=
-github.com/sigstore/timestamp-authority v1.2.8/go.mod h1:G2/0hAZmLPnevEwT1S9IvtNHUm9Ktzvso6xuRhl94ZY=
+github.com/sigstore/timestamp-authority v1.2.0 h1:Ffk10QsHxu6aLwySQ7WuaoWkD63QkmcKtozlEFot/VI=
+github.com/sigstore/timestamp-authority v1.2.0/go.mod h1:ojKaftH78Ovfow9DzuNl5WgTCEYSa4m5622UkKDHRXc=
```

## Files Modified

1. **pkg/types/fixpatches.go**
   - Added `PristineContent` field to `PatchApplicationResult`

2. **pkg/commands/fixpatches/fixpatches.go**
   - Added `extractPristineContent()` function
   - Modified `applySinglePatchWithReject()` to extract pristine content first

3. **pkg/commands/fixpatches/context.go**
   - Added `extractContextFromPristine()` function
   - Modified `ExtractPatchContext()` to use pristine content

4. **pkg/commands/fixpatches/llm.go**
   - Improved status messages for offset files
   - Added clear instructions for what LLM should do with each file
   - Changed "Current content" to "Original content (BEFORE patch application)"

## Build Status

✅ Build succeeded with no errors
✅ No diagnostics in any modified files

## Testing

To test the fix:

```bash
cd test/eks-anywhere-build-tooling
SKIP_VALIDATION=true ../../bin/version-tracker fix-patches \
    --project fluxcd/source-controller \
    --pr 4883 \
    --max-attempts 3 \
    --verbosity 6
```

### Verify in Logs:
1. ✅ "Extracting pristine file content before applying patch"
2. ✅ "Captured pristine content" for each file
3. ✅ "Using pristine content from before patch application"
4. ✅ "Extracted context from pristine files"

### Verify in Prompt (`/tmp/llm-prompt-attempt-1.txt`):
1. ✅ go.sum shows v1.2.8 (not v1.2.0)
2. ✅ "APPLIED WITH OFFSET" status with clear instructions
3. ✅ "Original content (BEFORE patch application)" label
4. ✅ "You MUST include this file in your fixed patch"

### Expected Result:
- ✅ LLM sees correct original content
- ✅ LLM understands offset files need to be included
- ✅ LLM generates patch with both go.mod and go.sum
- ✅ LLM uses correct line numbers (935, not 933)
- ✅ Patch applies successfully

## Impact

This fix is **CRITICAL** for the patch fixer to work correctly. Without it:
- ❌ LLM sees modified content and thinks changes are already applied
- ❌ LLM doesn't include offset files in the fixed patch
- ❌ Patch application fails repeatedly with the same error

With this fix:
- ✅ LLM sees original content and knows what needs to change
- ✅ LLM includes all files (failed and offset) in the fixed patch
- ✅ LLM uses correct line numbers for offset files
- ✅ Patch application succeeds
