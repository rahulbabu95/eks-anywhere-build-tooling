# Fix: Regenerate .rej Files After Failed Attempts

## Problem

After the first failed attempt to apply an LLM-generated patch, subsequent attempts would fail with:

```
Warning: failed to parse rejection file
error: open /path/to/go.mod.rej: no such file or directory
Failed to extract patch context: no failed hunks extracted from rejection files
```

The workflow would fail after 3 attempts even though the LLM could potentially fix the issue with better context.

## Root Cause

The issue was in the retry logic:

### Attempt 1:
1. ✅ Apply original patch with `--reject` → generates `.rej` files
2. ✅ Extract context from `.rej` files
3. ✅ LLM generates fix
4. ❌ Apply LLM fix fails
5. ✅ Revert changes (this **deletes the `.rej` files**)

### Attempt 2:
1. ❌ Try to extract context from `.rej` files → **files don't exist!**
2. ❌ Fail with "no failed hunks extracted"

### Attempt 3:
1. ❌ Same problem - no `.rej` files
2. ❌ Give up

The `.rej` files were only generated once at the beginning, but after reverting a failed attempt, they were deleted. Subsequent attempts had no `.rej` files to work with.

## Solution

After each failed attempt (apply, build validation, or semantic validation), **regenerate the `.rej` files** by re-applying the original patch with `--reject`:

```go
if err := ApplyPatchFix(fix, projectPath); err != nil {
	logger.Info("Failed to apply patch fix", "error", err, "attempt", attempt)
	
	// Revert changes
	if revertErr := RevertPatchFix(projectPath); revertErr != nil {
		logger.Info("Failed to revert patch", "error", revertErr)
	}
	
	// Re-apply original patch with --reject to regenerate .rej files for next attempt
	if attempt < opts.MaxAttempts {
		logger.Info("Re-applying original patch with --reject to regenerate .rej files")
		_, reapplyErr := applySinglePatchWithReject(patchFile, projectPath, projectRepo)
		if reapplyErr != nil {
			logger.Info("Warning: failed to re-apply patch with --reject", "error", reapplyErr)
		}
	}
	
	// Store error for next attempt
	ctx.BuildError = err.Error()
	ctx.PreviousAttempts = append(ctx.PreviousAttempts, fix.Patch)
	continue
}
```

This is applied after:
1. Failed patch application
2. Failed build validation
3. Failed semantic validation

## Additional Debugging

Also added debug logging to save the LLM-generated patch to a persistent file:

```go
// Save to a debug file that persists (for debugging)
debugPatchFile := filepath.Join(projectPath, ".llm-patch-debug.txt")
if err := os.WriteFile(debugPatchFile, []byte(fix.Patch), 0644); err != nil {
	logger.Info("Warning: failed to write debug patch file", "error", err)
} else {
	logger.Info("Saved debug patch file", "file", debugPatchFile)
}
```

This allows you to inspect what the LLM generated even if the patch fails to apply.

## Expected Behavior After Fix

### Attempt 1:
1. ✅ Apply original patch with `--reject` → generates `.rej` files
2. ✅ Extract context from `.rej` files
3. ✅ LLM generates fix
4. ✅ Save debug patch to `.llm-patch-debug.txt`
5. ❌ Apply LLM fix fails
6. ✅ Revert changes
7. ✅ **Re-apply original patch with `--reject`** → regenerates `.rej` files

### Attempt 2:
1. ✅ Extract context from `.rej` files (they exist now!)
2. ✅ LLM generates improved fix (with previous attempt context)
3. ✅ Save debug patch
4. ✅ Apply LLM fix succeeds
5. ✅ Validate and write to file

## Debugging Failed Patches

When a patch fails to apply, you can now inspect:

```bash
# Check the LLM-generated patch
cat projects/fluxcd/source-controller/.llm-patch-debug.txt

# Check the .rej files
find projects/fluxcd/source-controller/source-controller -name "*.rej"
cat projects/fluxcd/source-controller/source-controller/go.mod.rej
```

This helps diagnose:
- Is the LLM generating valid patch format?
- Are the file paths correct?
- Are the line numbers matching?
- Is the context correct?

## Files Changed

- `tools/version-tracker/pkg/commands/fixpatches/fixpatches.go`
  - Added `.rej` file regeneration after failed attempts
  - Applied to all three failure points (apply, build, semantic)

- `tools/version-tracker/pkg/commands/fixpatches/applier.go`
  - Added debug patch file saving

## Testing

After this fix, when you run:

```bash
cd tools/version-tracker
./version-tracker fix-patches \
  --project fluxcd/source-controller \
  --pr 4883 \
  --max-attempts 3 \
  --verbosity 6
```

You should see in the logs:
```
Failed to apply patch fix attempt=1
Reverting patch changes
Patch changes reverted successfully
Re-applying original patch with --reject to regenerate .rej files
Applying single patch with reject
Found rejection files count=2
Starting fix attempt for patch attempt=2
Extracting patch context rej_files=2
```

And you can inspect the debug file:
```bash
cat projects/fluxcd/source-controller/.llm-patch-debug.txt
```

## Why This Matters

Without this fix:
- Only 1 real attempt (attempts 2 and 3 fail immediately)
- No learning from previous failures
- Hard to debug what went wrong

With this fix:
- All 3 attempts can use LLM
- Each attempt learns from previous failures
- Debug files help diagnose issues
- Better chance of success
