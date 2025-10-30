# Critical Fixes Implemented

## Summary

Fixed two critical issues that were preventing the patch fixer from working correctly:
1. **Broken offset detection** - Regex wasn't matching git output format
2. **State pollution** - Re-applying original patch between attempts caused stale information

## Fix 1: Corrected Offset Detection

### Problem:
```go
// Old regex expected everything on one line with \n
offsetRegex := regexp.MustCompile(`Checking patch ([^.]+)\.\.\.\nHunk #\d+ succeeded at \d+ \(offset (\d+) lines?\)`)
```

Git output is actually:
```
Checking patch go.sum...
Hunk #1 succeeded at 935 (offset 2 lines).
```

Result: `offset_files: 0` (never detected)

### Solution:
Parse line by line, track current file, detect offset:

```go
var currentFile string
scanner := bufio.NewScanner(strings.NewReader(outputStr))
for scanner.Scan() {
    line := scanner.Text()
    
    // Track current file: "Checking patch go.sum..."
    if strings.HasPrefix(line, "Checking patch ") {
        parts := strings.Split(line, " ")
        if len(parts) >= 3 {
            currentFile = strings.TrimSuffix(parts[2], "...")
        }
    }
    
    // Detect offset: "Hunk #1 succeeded at 935 (offset 2 lines)."
    if currentFile != "" && strings.Contains(line, "succeeded at") && strings.Contains(line, "offset") {
        offsetRegex := regexp.MustCompile(`offset (\d+) lines?`)
        if match := offsetRegex.FindStringSubmatch(line); len(match) >= 2 {
            offset, _ := strconv.Atoi(match[1])
            result.OffsetFiles[currentFile] = offset
            logger.Info("Detected offset hunk", "file", currentFile, "offset", offset)
        }
    }
}
```

### Expected Result:
```
Detected offset hunk    {"file": "go.sum", "offset": 2}
Patch has conflicts     {"rej_files": 1, "offset_files": 1}  ← FIXED!
```

## Fix 2: Removed State Pollution

### Problem:
After each LLM attempt failed, we were:
1. Reverting changes
2. **Re-applying the ORIGINAL patch** (creating go.mod.rej again)
3. Extracting context from stale go.mod.rej
4. Showing "Failed Hunk in go.mod" even though go.sum actually failed

### Code Removed:
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

### Solution:
Extract context ONCE and reuse it:

```go
// Extract context ONCE from the original patch application
baseContext, err := ExtractPatchContext(rejFiles, patchFile, projectPath, 1, patchResult)
if err != nil {
    return fmt.Errorf("extracting patch context: %v", err)
}

logger.Info("Extracted base patch context", "token_count", baseContext.TokenCount, "hunks", len(baseContext.FailedHunks))

// Iterative refinement loop
var previousAttempts []string
var lastBuildError string

for attempt := 1; attempt <= opts.MaxAttempts; attempt++ {
    // REUSE base context, just update error info
    ctx := *baseContext // Create a copy
    ctx.BuildError = lastBuildError
    ctx.PreviousAttempts = previousAttempts
    
    logger.Info("Using base context for attempt", "token_count", ctx.TokenCount, "hunks", len(ctx.FailedHunks))
    
    // Call LLM
    fix, err := CallBedrockForPatchFix(&ctx, opts.Model, attempt)
    
    // Apply LLM fix
    if err := ApplyPatchFix(fix, projectPath); err != nil {
        // Store error for next attempt
        lastBuildError = err.Error()
        previousAttempts = append(previousAttempts, fix.Patch)
        
        // Revert to clean state
        RevertPatchFix(projectPath)
        
        // DON'T re-apply original patch ✅
        continue
    }
    
    // Validate...
}
```

### Expected Result:
```
Attempt 1:
  Extracted base patch context
  Using base context for attempt
  Failed to apply patch fix: "error: patch failed: go.sum:933"

Attempt 2:
  Using base context for attempt  ← REUSING, not re-extracting
  (No re-application of original patch)
  Previous attempt error shown in prompt
```

## Impact

### Before (Broken):

**Attempt 1:**
- Prompt: "Failed Hunk #1 in go.mod" ✅
- LLM fixes go.mod
- Apply fails on go.sum:933 ❌
- Re-apply original patch

**Attempt 2:**
- Prompt: "Failed Hunk #1 in go.mod" ❌ (STALE!)
- LLM fixes go.mod AGAIN ❌
- Apply fails on go.sum:933 AGAIN ❌
- Re-apply original patch

**Attempt 3:**
- Same as attempt 2 - no progress!

### After (Fixed):

**Attempt 1:**
- Prompt: "Failed Hunk #1 in go.mod" ✅
- Prompt: "go.sum: APPLIED WITH OFFSET (+2 lines)" ✅
- LLM fixes go.mod
- Apply fails on go.sum:933
- Store error, revert to clean state

**Attempt 2:**
- Prompt: "Failed Hunk #1 in go.mod" ✅ (original status)
- Prompt: "go.sum: APPLIED WITH OFFSET (+2 lines)" ✅
- Prompt: "Previous attempt failed: error: patch failed: go.sum:933" ✅
- LLM sees the actual error and can adjust
- LLM generates better fix

**Attempt 3:**
- Same accurate information
- LLM can continue to improve

## Files Modified

1. **pkg/commands/fixpatches/fixpatches.go**
   - Fixed offset detection regex (line-by-line parsing)
   - Extract context ONCE and reuse
   - Removed re-application of original patch (3 locations)

## Build Status

✅ Build succeeded
✅ No diagnostics

## Testing

To verify the fixes:

```bash
cd test/eks-anywhere-build-tooling
SKIP_VALIDATION=true ../../bin/version-tracker fix-patches \
    --project fluxcd/source-controller \
    --pr 4883 \
    --max-attempts 3 \
    --verbosity 6
```

### Verify in Logs:

1. ✅ "Detected offset hunk" with file and offset
2. ✅ "Patch has conflicts" with offset_files > 0
3. ✅ "Extracted base patch context" (once)
4. ✅ "Using base context for attempt" (each attempt)
5. ✅ NO "Re-applying original patch" messages

### Verify in Prompt (`/tmp/llm-prompt-attempt-2.txt`):

1. ✅ "go.sum: APPLIED WITH OFFSET (+2 lines)"
2. ✅ "IMPORTANT: You MUST include this file in your fixed patch"
3. ✅ "Previous Attempt #1 Failed" section with actual error
4. ✅ "error: patch failed: go.sum:933" shown

### Expected Outcome:

- ✅ LLM sees offset information
- ✅ LLM sees pristine content (v1.2.8, not v1.2.0)
- ✅ LLM sees actual errors from previous attempts
- ✅ LLM can generate correct patches
- ✅ Patch application succeeds

## Next Steps

After testing, if issues remain:
1. Parse git apply errors to extract which file failed
2. Update prompt to highlight the actual failure
3. Add more context about line number mismatches
