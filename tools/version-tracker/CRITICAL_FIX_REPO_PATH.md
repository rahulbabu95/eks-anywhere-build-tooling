# Critical Fix: Repository Path for Context Extraction

## Problem Identified

During manual testing, the context extraction was failing with errors:

```
Warning: could not read file    {"file": "go.mod", "error": "open /Users/.../projects/fluxcd/source-controller/go.mod: no such file or directory"}
Warning: could not read file    {"file": "go.sum", "error": "open /Users/.../projects/fluxcd/source-controller/go.sum: no such file or directory"}
Extracted context for all files {"count": 0}
```

## Root Cause

The `extractContextForAllFiles()` function was looking for files in the **project path** (the patches directory):
```
/Users/.../projects/fluxcd/source-controller/
```

But the actual files are in the **checked-out repository** subdirectory:
```
/Users/.../projects/fluxcd/source-controller/source-controller/
```

This is because the `applySinglePatchWithReject()` function checks out the upstream repository into a subdirectory named after the repo (e.g., `source-controller/`), and that's where the files actually exist.

## The Fix

Updated `ExtractPatchContext()` to derive the correct repository path from the `.rej` file locations:

```go
// Determine the repository path from the .rej files
// .rej files are in the checked-out repo, e.g., /path/to/project/source-controller/go.mod.rej
// We need to extract the repo directory from the first .rej file path
var repoPath string
if len(rejFiles) > 0 {
    // Get the directory containing the .rej file
    // This is the repository root where the actual files are
    rejFileDir := filepath.Dir(rejFiles[0])
    repoPath = rejFileDir
    logger.Info("Determined repository path from .rej file", "repo_path", repoPath)
} else {
    // Fallback: assume repo is in projectPath/repoName
    repoPath = projectPath
    logger.Info("No .rej files to determine repo path, using project path", "repo_path", repoPath)
}

// Extract context for all files
allFileContexts := extractContextForAllFiles(patchFiles, repoPath)
```

## Why This Works

1. `.rej` files are created by `git apply --reject` in the repository directory
2. The `.rej` file path is: `/path/to/project/source-controller/go.mod.rej`
3. By taking `filepath.Dir()` of the `.rej` file, we get: `/path/to/project/source-controller/`
4. This is exactly where the actual `go.mod` and `go.sum` files are located
5. Now `extractContextForAllFiles()` can successfully read the files

## Expected Behavior After Fix

When the fix-patches command runs:

1. **Attempt 1**:
   ```
   Parsed patch files      {"count": 2}
   Determined repository path from .rej file    {"repo_path": "/path/to/source-controller"}
   Extracted context for all files {"count": 2}
   ```

2. **Prompt will include**:
   ```markdown
   ## Current File States
   
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
   Lines 923-947:
   github.com/sigstore/timestamp-authority v1.2.8 h1:...
   ...
   ```
   ```

3. **LLM will see**:
   - Complete context for both go.mod and go.sum
   - Actual content at line 933 in go.sum (where the offset occurred)
   - Can generate correct patches for both files

## Testing

Build succeeded:
```bash
cd tools/version-tracker
make build
# ✅ Build succeeded
```

No diagnostics:
```bash
# ✅ No errors in context.go
```

## Next Steps

Test with the source-controller patch:

```bash
cd test/eks-anywhere-build-tooling
SKIP_VALIDATION=true ../../bin/version-tracker fix-patches \
    --project fluxcd/source-controller \
    --pr 4883 \
    --max-attempts 3 \
    --verbosity 6
```

Verify in logs:
- ✅ "Determined repository path from .rej file"
- ✅ "Extracted context for all files" with count > 0
- ✅ Prompt file shows "Current File States" section with both files

## Files Modified

- `tools/version-tracker/pkg/commands/fixpatches/context.go` - Fixed repository path derivation

## Impact

This fix is **critical** for Approach 2 to work. Without it:
- ❌ No file contexts are extracted
- ❌ LLM has no visibility into offset files
- ❌ LLM cannot generate correct patches

With this fix:
- ✅ All file contexts are extracted correctly
- ✅ LLM sees complete information for all files
- ✅ LLM can generate correct patches for both failed and offset files
