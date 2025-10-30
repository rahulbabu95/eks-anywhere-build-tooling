# Critical Context Issue - Missing go.sum Context

## The Problem You Identified

After analyzing the latest prompts, you found **critical missing context**:

### Issue 1: Wrong Status in Later Attempts

**Attempt 2 shows**:
```
**Patch Application Status:**
- ❌ FAILED (needs fixing): go.mod
```

**But the reality**:
- ✅ go.mod actually SUCCEEDED in attempt 1
- ❌ go.sum FAILED when applying the LLM-generated patch

**The status is stale** - it's showing the original patch application status, not the status from the previous LLM attempt.

### Issue 2: No go.sum Context

**What we show**:
- ✅ go.mod: Full "Expected vs Actual" with current file content
- ❌ go.sum: Only shown in the original patch, NO current file content
- ❌ go.sum: No context around line 933 where it's failing

**What the LLM needs**:
- Current go.sum content around line 933
- What's actually at that line
- Why the patch doesn't apply there

### Issue 3: Error Message Not Used

**The git apply error says**:
```
error: patch failed: go.sum:933
error: go.sum: patch does not apply
```

**But we don't**:
- Parse this to know go.sum is the problem
- Extract context from go.sum around line 933
- Show this context to the LLM

## Root Cause

### The Flow Problem

```
1. Original patch applied:
   - go.mod: FAILED (created .rej file)
   - go.sum: SUCCEEDED with offset

2. We extract context:
   - go.mod: YES (has .rej file)
   - go.sum: NO (no .rej file, treated as success)

3. LLM generates fix:
   - Fixes go.mod ✓
   - Includes go.sum from original patch (but with wrong line numbers)

4. We try to apply LLM patch:
   - go.mod: SUCCEEDS ✓
   - go.sum: FAILS ✗ (line 933 doesn't match)

5. Next attempt:
   - We still show go.mod as failed (stale status)
   - We still don't show go.sum context
   - LLM has no new information
```

## What We Need To Do

### Solution 1: Parse Git Apply Error and Extract Context

When the LLM-generated patch fails to apply:

```go
// Parse error: "error: patch failed: go.sum:933"
failedFile := "go.sum"
failedLine := 933

// Extract context from that file around that line
context := extractFileContext(failedFile, failedLine, projectPath)

// Add to BuildError for next attempt
ctx.BuildError = fmt.Sprintf(`
Git apply failed:
%s

Failed at: %s line %d

Current content around that line:
%s
`, gitError, failedFile, failedLine, context)
```

### Solution 2: Always Extract Context for ALL Files in Patch

Don't just extract for .rej files. Extract for:
- Files with .rej (complete failures)
- Files mentioned in offset messages
- Files mentioned in git apply errors

```go
// Parse original patch to find all files
allFiles := parseFilesFromPatch(originalPatch)

// Extract context for each file
for _, file := range allFiles {
    // Find the lines being changed
    lines := extractChangedLines(originalPatch, file)
    
    // Get current file content around those lines
    context := extractFileContext(file, lines, projectPath)
    
    // Add to context
    ctx.FileContexts[file] = context
}
```

### Solution 3: Update Status After Each Attempt

Track what failed in THIS attempt, not the original:

```go
type AttemptResult struct {
    AttemptNumber int
    PatchGenerated string
    ApplyError string
    FailedFiles map[string]int // file -> line number
}

// After each attempt
result := applyPatchAndGetResult(llmPatch)
ctx.AttemptHistory = append(ctx.AttemptHistory, result)

// Show in prompt
for _, attempt := range ctx.AttemptHistory {
    prompt.WriteString(fmt.Sprintf("Attempt %d failed:\n", attempt.AttemptNumber))
    for file, line := range attempt.FailedFiles {
        prompt.WriteString(fmt.Sprintf("- %s at line %d\n", file, line))
    }
}
```

## Recommended Approach

### Phase 1: Parse Git Apply Errors (Quick Win)

1. When `ApplyPatchFix()` fails, parse the error message
2. Extract file name and line number from "error: patch failed: FILE:LINE"
3. Read that file and get context around that line
4. Include in `BuildError` for next attempt

**Impact**: LLM will see actual file content where it's failing

### Phase 2: Extract Context for All Files (Better)

1. Parse original patch to find all files being modified
2. For each file, extract context around the lines being changed
3. Show this context in the prompt (not just for .rej files)

**Impact**: LLM has complete picture of all files

### Phase 3: Track Per-Attempt Status (Best)

1. After each LLM attempt, track what failed
2. Update status to show current failures, not original
3. Show progression: "Attempt 1 fixed go.mod, but go.sum still failing"

**Impact**: LLM understands progress and can focus on remaining issues

## Example of What Prompt Should Look Like

### Current (Broken)

```markdown
## Failed Hunk #1 in go.mod
[go.mod context]

## Original Patch
- ❌ FAILED: go.mod
[patch with go.sum changes but no go.sum context]
```

### Fixed (Phase 1)

```markdown
## Failed Hunk #1 in go.mod
[go.mod context]

## Previous Attempt #1 Failed

**Error:**
```
error: patch failed: go.sum:933
error: go.sum: patch does not apply
```

**Current content at go.sum:933:**
```
github.com/sigstore/sigstore/pkg/signature/kms/gcp v1.9.5 h1:7U0GsO0UGG1PdtgS6wBkRC0sMgq7BRVaFlPRwN4m1Qg=
github.com/sigstore/sigstore/pkg/signature/kms/gcp v1.9.5/go.mod h1:/2qrI0nnCy/DTIPOMFaZlFnNPWEn5UeS70P37XEM88o=
github.com/sigstore/sigstore/pkg/signature/kms/hashivault v1.9.5 h1:S2ukEfN1orLKw2wEQIUHDDlzk0YcylhcheeZ5TGk8LI=
github.com/sigstore/sigstore/pkg/signature/kms/hashivault v1.9.5/go.mod h1:m7sQxVJmDa+rsmS1m6biQxaLX83pzNS7ThUEyjOqkCU=
github.com/sigstore/timestamp-authority v1.2.8 h1:BEV3fkphwU4zBp3allFAhCqQb99HkiyCXB853RIwuEE=
```

**Your patch tried to change line 933, but the context doesn't match.**
```

### Fixed (Phase 2)

```markdown
## File Contexts

### go.mod (FAILED in original patch)
[Expected vs Actual for go.mod]

### go.sum (needs line number update)
**Lines being changed**: 933-935

**Current content:**
```
[lines 930-940 from current go.sum]
```

**Original patch expects line 933, but file has changed.**

## Previous Attempt #1 Failed
[error with context]
```

## Next Steps

I recommend implementing **Phase 1** first as it's the quickest win:

1. Parse git apply error in `ApplyPatchFix()`
2. Extract file and line number
3. Read file content around that line
4. Include in `BuildError`

This will immediately give the LLM the missing go.sum context it needs.

Would you like me to implement Phase 1 now?
