# State Management Issues Identified

## Issues Found from Test Logs and Prompts

### Issue 1: Offset Detection Regex is Broken ❌

**Log shows:**
```
"offset_files": 0
```

**Git output contains:**
```
Checking patch go.sum...
Hunk #1 succeeded at 935 (offset 2 lines).
```

**Current regex:**
```go
offsetRegex := regexp.MustCompile(`Checking patch ([^.]+)\.\.\.\nHunk #\d+ succeeded at \d+ \(offset (\d+) lines?\)`)
```

**Problem**: The regex expects "Checking patch" and "Hunk succeeded" on the SAME line with `\n` between them, but they're on separate lines in the actual output.

**Fix needed**: Parse line by line, track current file, detect offset hunks

### Issue 2: Status Shows "APPLIED CLEANLY" Instead of "APPLIED WITH OFFSET" ❌

**Prompt shows:**
```markdown
### go.sum

**Status**: ✅ APPLIED CLEANLY

**Action Required**: Include this file in your fixed patch (no changes needed to line numbers)
```

**Should show:**
```markdown
### go.sum

**Status**: ⚠️ APPLIED WITH OFFSET (+2 lines)

**IMPORTANT**: This file applied successfully but at different line numbers than expected.
You MUST include this file in your fixed patch with updated line numbers.
```

**Root cause**: Offset detection isn't working, so `hasOffset` is always false

### Issue 3: Pristine Content is Working ✅

**Good news**: The pristine content extraction IS working!

**Prompt shows:**
```
github.com/sigstore/timestamp-authority v1.2.8 h1:BEV3fkphwU4zBp3allFAhCqQb99HkiyCXB853RIwuEE=
```

This is v1.2.8 (original), not v1.2.0 (modified) ✅

### Issue 4: State Management Across Attempts

**From logs:**
```
Attempt 1: "Patch application failed with conflicts" - go.mod fails, go.sum succeeds with offset
Attempt 2: "Patch application failed with conflicts" - SAME output, go.mod fails again
Attempt 3: "Patch application failed with conflicts" - SAME output, go.mod fails again
```

**Problem**: We're re-applying the ORIGINAL patch every time, not the LLM-generated fix!

**Current flow:**
```
Attempt 1:
  1. Apply original patch → go.mod fails, go.sum succeeds
  2. Extract context
  3. LLM generates fix
  4. Apply LLM fix → fails
  5. Revert
  6. Re-apply original patch with --reject (for next attempt)

Attempt 2:
  1. Apply original patch AGAIN → go.mod fails, go.sum succeeds AGAIN
  2. Extract SAME context
  3. LLM generates SAME fix
  4. Apply LLM fix → fails AGAIN
```

**The problem**: We're always showing go.mod as failed because we keep re-applying the original patch!

### Issue 5: Not Tracking What Actually Failed in LLM Attempt

**What should happen:**
```
Attempt 1:
  - Original patch: go.mod FAILS, go.sum succeeds with offset
  - LLM generates fix for go.mod
  - Apply LLM patch → go.sum FAILS (because it's trying to change v1.2.8 to v1.2.0, but v1.2.0 is already there)
  - Error: "patch failed: go.sum:933"

Attempt 2:
  - Show: "Previous attempt failed on go.sum at line 933"
  - Show: go.sum status as FAILED (not go.mod)
  - LLM should focus on fixing go.sum
```

**What actually happens:**
```
Attempt 2:
  - Re-apply original patch → go.mod FAILS again
  - Show: go.mod as FAILED (stale info)
  - LLM tries to fix go.mod again (wrong focus)
```

## Root Causes

### Root Cause 1: Broken Offset Detection

The regex pattern doesn't match the actual git output format.

**Fix**: Parse output line by line:
```go
var currentFile string
scanner := bufio.NewScanner(strings.NewReader(outputStr))
for scanner.Scan() {
    line := scanner.Text()
    
    // Track current file being checked
    if strings.HasPrefix(line, "Checking patch ") {
        currentFile = extractFilename(line)
    }
    
    // Detect offset for current file
    if strings.Contains(line, "succeeded at") && strings.Contains(line, "offset") {
        offsetMatch := regexp.MustCompile(`offset (\d+) lines?`).FindStringSubmatch(line)
        if len(offsetMatch) >= 2 {
            offset, _ := strconv.Atoi(offsetMatch[1])
            result.OffsetFiles[currentFile] = offset
        }
    }
}
```

### Root Cause 2: State Pollution

We're re-applying the original patch every attempt instead of working with a clean state.

**Current approach (WRONG)**:
```
Attempt 1: Apply original → Extract context → LLM fix → Apply → Fail → Revert → Re-apply original
Attempt 2: Apply original → Extract SAME context → LLM SAME fix → Apply → Fail → Revert → Re-apply original
```

**Correct approach**:
```
ONCE: Apply original → Extract pristine context → Store it

Attempt 1: Use stored context → LLM fix → Apply → Fail → Parse ACTUAL error
Attempt 2: Use stored context + ACTUAL error → LLM fix → Apply → Fail → Parse ACTUAL error
Attempt 3: Use stored context + ACTUAL error → LLM fix → Apply
```

### Root Cause 3: Not Parsing LLM Patch Application Errors

When the LLM patch fails to apply, we don't parse which file actually failed.

**Current**: Show stale .rej file info (go.mod from original patch)
**Should**: Parse git apply error to see which file failed THIS time

## Fixes Needed

### Fix 1: Correct Offset Detection (HIGH PRIORITY)

```go
// Parse output line by line to detect offsets
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

### Fix 2: Extract Context ONCE, Reuse for All Attempts (CRITICAL)

**Modify `fixSinglePatch()`:**
```go
func fixSinglePatch(...) error {
    // Apply original patch ONCE to get pristine context
    rejFiles, patchResult, err := applySinglePatchWithReject(patchFile, projectPath, projectRepo)
    
    // Extract context ONCE from pristine content
    baseContext, err := ExtractPatchContext(rejFiles, patchFile, projectPath, 1, patchResult)
    
    // Iterative refinement loop
    for attempt := 1; attempt <= opts.MaxAttempts; attempt++ {
        // REUSE base context, just update error info
        ctx := baseContext
        ctx.BuildError = lastBuildError
        ctx.PreviousAttempts = previousAttempts
        
        // Call LLM
        fix, err := CallBedrockForPatchFix(ctx, opts.Model, attempt)
        
        // Apply LLM fix (NOT original patch)
        if err := ApplyPatchFix(fix, projectPath); err != nil {
            // Parse ACTUAL error from THIS attempt
            lastBuildError = parseGitApplyError(err.Error())
            previousAttempts = append(previousAttempts, fix.Patch)
            
            // Revert to pristine state (git reset --hard)
            RevertPatchFix(projectPath)
            continue
        }
        
        // Validate...
    }
}
```

### Fix 3: Parse LLM Patch Application Errors (MEDIUM PRIORITY)

```go
func parseGitApplyError(errorOutput string) string {
    // Parse: "error: patch failed: go.sum:933"
    // Extract which file actually failed
    
    lines := strings.Split(errorOutput, "\n")
    for _, line := range lines {
        if strings.Contains(line, "patch failed:") {
            // Extract filename and line number
            parts := strings.Split(line, ":")
            if len(parts) >= 3 {
                filename := strings.TrimSpace(parts[1])
                lineNum := strings.TrimSpace(parts[2])
                return fmt.Sprintf("Patch failed on %s at line %s", filename, lineNum)
            }
        }
    }
    
    return errorOutput
}
```

### Fix 4: Don't Re-apply Original Patch Between Attempts (CRITICAL)

**Remove this code:**
```go
// Re-apply original patch with --reject to regenerate .rej files for next attempt
if attempt < opts.MaxAttempts {
    logger.Info("Re-applying original patch with --reject to regenerate .rej files")
    _, _, reapplyErr := applySinglePatchWithReject(patchFile, projectPath, projectRepo)
}
```

**Why**: We should NOT re-apply the original patch between attempts. We should:
1. Apply original patch ONCE at the start
2. Extract context ONCE
3. For each attempt, apply LLM fix to CLEAN state
4. If it fails, revert to CLEAN state (not re-apply original)

## Implementation Priority

1. **Fix offset detection** (HIGH) - Without this, LLM doesn't know about offset files
2. **Remove re-application of original patch** (CRITICAL) - This is causing state pollution
3. **Extract context once, reuse** (CRITICAL) - Avoid redundant work and ensure consistency
4. **Parse LLM patch errors** (MEDIUM) - Better error tracking

## Expected Behavior After Fixes

### Logs will show:
```
Extracting pristine file content before applying patch
Captured pristine content    {"file": "go.mod", "size": 1234}
Captured pristine content    {"file": "go.sum", "size": 56789}
Detected offset hunk         {"file": "go.sum", "offset": 2}  ← NEW!
Patch has conflicts          {"rej_files": 1, "offset_files": 1}  ← FIXED!

Attempt 1:
  Using pristine content
  Calling Bedrock API
  Applying LLM-generated patch
  git apply failed: "error: patch failed: go.sum:933"  ← Actual error
  
Attempt 2:
  Using pristine content (REUSED, not re-extracted)
  Previous attempt failed on go.sum at line 933  ← Accurate
  Calling Bedrock API
```

### Prompt will show:
```markdown
### go.sum

**Status**: ⚠️ APPLIED WITH OFFSET (+2 lines)  ← FIXED!

**IMPORTANT**: This file applied successfully but at different line numbers than expected.
You MUST include this file in your fixed patch with updated line numbers.
The patch expected changes at certain lines, but they were found 2 lines later.

**Original content (BEFORE patch application):**
```
github.com/sigstore/timestamp-authority v1.2.8 ...  ← Pristine content ✅
```
```
