# Approach 2 Implementation Complete

## Summary

Successfully implemented **Approach 2** from the session summary: "Parse original patch to find ALL files, extract context for each file around the lines being changed."

This enhancement addresses the critical issue where the LLM couldn't fix patches because it was missing context for files that applied with offset (like go.sum).

## Changes Made

### 1. Updated PatchContext Type (`pkg/types/fixpatches.go`)

Added new field to store context for all files in the patch:

```go
type PatchContext struct {
    // ... existing fields ...
    AllFileContexts   map[string]string  // filename -> current content around changed lines
}
```

### 2. Added Patch Parsing Functions (`pkg/commands/fixpatches/context.go`)

**New Types:**
- `PatchFile`: Represents a file being modified in a patch with its line ranges
- `LineRange`: Represents a range of lines (Start, End)

**New Functions:**

#### `parsePatchFiles(patchContent string) ([]PatchFile, error)`
- Parses unified diff format to extract all files and their changed line ranges
- Looks for `diff --git a/file b/file` markers to identify files
- Parses `@@ -oldStart,oldCount +newStart,newCount @@` headers to get line ranges
- Handles both multi-line and single-line hunk formats

#### `extractContextForAllFiles(patchFiles []PatchFile, projectPath string) map[string]string`
- Extracts current file content for all files in the patch
- For each file, reads ±10 lines around each changed line range
- Returns a map of filename -> context string
- Gracefully handles missing files with warnings

### 3. Enhanced ExtractPatchContext (`pkg/commands/fixpatches/context.go`)

Updated the main context extraction function to:
1. Initialize `AllFileContexts` map
2. Call `parsePatchFiles()` to identify all files in the patch
3. Call `extractContextForAllFiles()` to extract context for each file
4. Store results in `ctx.AllFileContexts`
5. Log the number of files parsed and contexts extracted

### 4. Updated Token Estimation (`pkg/commands/fixpatches/context.go`)

Modified `estimateTokenCount()` to include `AllFileContexts` in token calculation:

```go
// Count all file contexts
for _, context := range ctx.AllFileContexts {
    totalChars += len(context)
}
```

### 5. Enhanced Prompt Building (`pkg/commands/fixpatches/llm.go`)

Added new section "Current File States" to the prompt that shows:
- All files being modified in the patch
- Status for each file:
  - ❌ FAILED (has .rej files)
  - ⚠️ APPLIED WITH OFFSET (+N lines)
  - ✅ APPLIED CLEANLY
- Current content around changed lines for each file

This section appears after failed hunks but before previous attempts, giving the LLM complete visibility into all files.

## Expected Improvements

### Before (Previous Implementation)
- ❌ Only showed context for files with .rej files
- ❌ No context for files that applied with offset
- ❌ LLM couldn't fix go.sum because it had no visibility into line 933
- ❌ Stale status information (showed "go.mod FAILED" even after it succeeded)

### After (Approach 2 Implementation)
- ✅ Shows context for ALL files in the patch
- ✅ Includes files that applied with offset (like go.sum)
- ✅ LLM can see actual content at the offset location
- ✅ Clear status indicators for each file
- ✅ LLM has complete information to generate correct patches

## Example Prompt Output

For the source-controller patch with go.mod and go.sum:

```markdown
## Current File States

Here is the current content of all files being modified:

### go.mod

**Status**: ❌ FAILED (see detailed context above)

**Current content:**
```
Lines 1-20:
module github.com/fluxcd/source-controller
...
```

### go.sum

**Status**: ⚠️ APPLIED WITH OFFSET (+2 lines)

**Current content:**
```
Lines 923-943:
github.com/go-logr/logr v1.2.3 h1:...
github.com/go-logr/logr v1.2.3/go.mod h1:...
...
```
```

## Token Management

The implementation follows the session summary's token management strategy:
- Extracts only ±10 lines around changed lines (not entire file)
- Keeps context focused and relevant
- Estimated token increase: ~2x current usage (still well under 200K limit)

## Testing

Build completed successfully:
```bash
cd tools/version-tracker
make build
# ✅ Build succeeded
```

No diagnostics or errors in:
- `pkg/commands/fixpatches/context.go`
- `pkg/types/fixpatches.go`
- `pkg/commands/fixpatches/llm.go`

## Next Steps

1. **Test with source-controller patch**:
   ```bash
   cd test/eks-anywhere-build-tooling
   ../../tools/version-tracker/bin/version-tracker fix-patches \
       --project fluxcd/source-controller \
       --pr 4883 \
       --max-attempts 3 \
       2>&1 | tee auto-patch-$(date +%Y%m%d-%H%M%S).log
   ```

2. **Verify prompt contains all file contexts**:
   ```bash
   cat /tmp/llm-prompt-attempt-1.txt
   # Should show both go.mod and go.sum contexts
   ```

3. **Check LLM can now fix both files**:
   - LLM should see go.sum context at line 933
   - LLM should generate patch that fixes both go.mod and go.sum
   - Build should succeed after applying the fix

## Files Modified

1. `tools/version-tracker/pkg/types/fixpatches.go` - Added `AllFileContexts` field
2. `tools/version-tracker/pkg/commands/fixpatches/context.go` - Added parsing and extraction functions
3. `tools/version-tracker/pkg/commands/fixpatches/llm.go` - Enhanced prompt with all file contexts

## Implementation Matches Session Summary

This implementation follows the exact plan outlined in the session summary:
- ✅ Phase 1: Parse patch to find all files
- ✅ Phase 2: Extract context for each file
- ✅ Phase 3: Update ExtractPatchContext
- ✅ Phase 4: Update PatchContext type
- ✅ Phase 5: Update prompt to show all file contexts

All phases completed successfully!
