# Context Problems Identified from Actual Test

## What We Found in the Prompts

### Attempt 1 Prompt Analysis

#### Problem 1: go.sum Shows MODIFIED Content ❌

**What the prompt shows:**
```
### go.sum

**Status**: ✅ APPLIED CLEANLY

**Current content:**
Lines 924-951:
...
github.com/sigstore/timestamp-authority v1.2.0 h1:Ffk10QsHxu6aLwySQ7WuaoWkD63QkmcKtozlEFot/VI=
github.com/sigstore/timestamp-authority v1.2.0/go.mod h1:ojKaftH78Ovfow9DzuNl5WgTCEYSa4m5622UkKDHRXc=
...
```

**The Problem:**
- Status says "APPLIED CLEANLY" ✅
- But the content shows **v1.2.0** (the NEW version from the patch)
- This means we're reading the file AFTER `git apply --reject` already modified it
- The LLM sees v1.2.0 and thinks "it's already done, nothing to change"

**What we SHOULD show:**
```
### go.sum

**Status**: ⚠️ APPLIED WITH OFFSET (+2 lines)

**Original content in v1.7.2 (BEFORE patch):**
Lines 935-936:
github.com/sigstore/timestamp-authority v1.2.8 h1:BEV3fkphwU4zBp3allFAhCqQb99HkiyCXB853RIwuEE=
github.com/sigstore/timestamp-authority v1.2.8/go.mod h1:G2/0hAZmLPnevEwT1S9IvtNHUm9Ktzvso6xuRhl94ZY=

**What the patch wants to change it to:**
Lines 935-936:
github.com/sigstore/timestamp-authority v1.2.0 h1:Ffk10QsHxu6aLwySQ7WuaoWkD63QkmcKtozlEFot/VI=
github.com/sigstore/timestamp-authority v1.2.0/go.mod h1:ojKaftH78Ovfow9DzuNl5WgTCEYSa4m5622UkKDHRXc=

**Note:** Patch expected these at line 933, but they're at line 935 (offset +2)
```

#### Problem 2: Misleading "APPLIED CLEANLY" Status ❌

The status says "✅ APPLIED CLEANLY" but this is misleading because:
1. It applied with an OFFSET (+2 lines)
2. The LLM needs to know about this offset to generate the correct patch
3. "APPLIED CLEANLY" suggests no action needed, but the LLM must include this in the fixed patch

**What we SHOULD say:**
```
**Status**: ⚠️ APPLIED WITH OFFSET (+2 lines) - Include in fixed patch with updated line numbers
```

#### Problem 3: No Clear Instruction for Offset Files ❌

The prompt doesn't tell the LLM what to do with offset files. The LLM might think:
- "It applied cleanly, so I don't need to include it in my patch"
- "The content is already v1.2.0, so nothing to change"

**What we SHOULD say:**
```
**Task for this file:**
Even though this file applied with offset, you MUST include it in your fixed patch.
Update the line numbers from 933 to 935 to match where the lines actually are in v1.7.2.
```

### Attempt 2 & 3 Prompt Analysis

#### Problem 4: Same Stale Context Repeated ❌

Looking at the logs:
```
Attempt 1: "Extracted context for all files" {"count": 2}
Attempt 2: "Extracted context for all files" {"count": 2}
Attempt 3: "Extracted context for all files" {"count": 2}
```

But the error changes:
```
Attempt 1: git apply failed: "error: patch failed: go.sum:933"
Attempt 2: git apply failed: "error: patch failed: go.sum:933"
Attempt 3: git apply failed: "error: patch failed: go.sum:933"
```

**The Problem:**
- Every attempt re-applies the original patch with `--reject`
- This modifies go.sum AGAIN (applying v1.2.0)
- Then we extract context from the MODIFIED go.sum
- LLM sees v1.2.0 and generates a patch that doesn't change it
- The patch fails because it's trying to change v1.2.8 → v1.2.0, but v1.2.0 is already there

#### Problem 5: Error Message Doesn't Match Reality ❌

The error says:
```
error: patch failed: go.sum:933
error: go.sum: patch does not apply
```

But the prompt still shows:
```
## Failed Hunk #1 in go.mod
```

**The Problem:**
- Attempt 1: go.mod failed ✅ (correct)
- Attempt 2: go.sum failed ❌ (but prompt says go.mod)
- Attempt 3: go.sum failed ❌ (but prompt says go.mod)

The prompt is showing stale information from attempt 1, not the current failure.

## Root Causes

### Root Cause 1: Reading Files After Modification

**Current Flow:**
```
1. Checkout v1.7.2
2. Run: git apply --reject patch.patch
   → go.mod: FAILS, creates go.mod.rej
   → go.sum: SUCCEEDS with offset, MODIFIES the file (v1.2.8 → v1.2.0)
3. Extract context from files
   → go.mod: reads from go.mod (pristine) ✅
   → go.sum: reads from go.sum (MODIFIED with v1.2.0) ❌
4. LLM sees v1.2.0 in go.sum and thinks it's done
5. LLM generates patch that doesn't change go.sum
6. Apply LLM patch → FAILS because go.sum needs to be in the patch
```

**What Should Happen:**
```
1. Checkout v1.7.2
2. Extract context from PRISTINE files FIRST
   → go.mod: reads pristine ✅
   → go.sum: reads pristine (v1.2.8) ✅
3. Run: git apply --reject patch.patch
   → go.mod: FAILS, creates go.mod.rej
   → go.sum: SUCCEEDS with offset
4. Identify what failed and what succeeded
5. Pass PRISTINE context to LLM
6. LLM sees v1.2.8 in go.sum and knows to change it to v1.2.0
7. LLM generates correct patch
```

### Root Cause 2: Not Tracking Offset Files Properly

**Current Implementation:**
```go
// We parse git apply output to detect offset
offsetPattern := regexp.MustCompile(`Hunk #\d+ succeeded at \d+ \(offset (\d+) lines?\)`)
// Store in patchResult.OffsetFiles
```

But then we:
1. Don't pass this information clearly to the LLM
2. Don't explain what "offset" means
3. Don't show the ORIGINAL content (before offset was applied)
4. Mark it as "APPLIED CLEANLY" which is misleading

**What We Should Do:**
```go
// For offset files:
1. Store PRISTINE content (before git apply)
2. Store the offset amount
3. Store the original line numbers from patch
4. Store the actual line numbers in current file
5. Pass all this to LLM with clear explanation
```

### Root Cause 3: State Pollution Across Attempts

**Current Flow:**
```
Attempt 1:
  git apply --reject → go.sum modified (v1.2.0 applied)
  Extract context → sees v1.2.0
  LLM generates patch
  Apply fails
  Revert (git reset --hard)

Attempt 2:
  git apply --reject AGAIN → go.sum modified AGAIN (v1.2.0 applied)
  Extract context → sees v1.2.0 AGAIN
  LLM generates same wrong patch
  Apply fails
  Revert (git reset --hard)
```

**The Problem:**
We're repeating the same mistake because we extract context AFTER modification every time.

## The Fixes Needed

### Fix 1: Extract Context BEFORE Applying Patch (CRITICAL)

```go
func fixSinglePatch(patchFile string, projectPath string, projectRepo string, opts *types.FixPatchesOptions) error {
    // 1. Checkout repository
    repoPath := checkoutRepository(projectPath, projectRepo)
    
    // 2. Extract PRISTINE context BEFORE applying patch
    pristineContext := extractPristineContext(patchFile, repoPath)
    
    // 3. Apply patch with --reject
    rejFiles, patchResult, err := applySinglePatchWithReject(patchFile, projectPath, projectRepo)
    
    // 4. For each attempt, use PRISTINE context
    for attempt := 1; attempt <= opts.MaxAttempts; attempt++ {
        ctx := buildContextFromPristine(pristineContext, rejFiles, patchResult)
        // ... rest of logic
    }
}
```

### Fix 2: Better Offset File Handling

```go
type OffsetFileInfo struct {
    Filename          string
    OriginalLineStart int    // Line number from patch
    ActualLineStart   int    // Line number in current file
    OffsetAmount      int    // Difference
    PristineContent   string // Content BEFORE git apply
    ModifiedContent   string // Content AFTER git apply (for verification)
}

// In prompt:
for filename, info := range offsetFiles {
    prompt.WriteString(fmt.Sprintf("### %s\n\n", filename))
    prompt.WriteString(fmt.Sprintf("**Status**: ⚠️ APPLIED WITH OFFSET (+%d lines)\n\n", info.OffsetAmount))
    prompt.WriteString("**IMPORTANT**: This file applied successfully but at different line numbers.\n")
    prompt.WriteString("You MUST include this file in your fixed patch with updated line numbers.\n\n")
    prompt.WriteString(fmt.Sprintf("**Patch expected lines at**: %d\n", info.OriginalLineStart))
    prompt.WriteString(fmt.Sprintf("**Actually found at**: %d\n\n", info.ActualLineStart))
    prompt.WriteString("**Original content (BEFORE patch):**\n")
    prompt.WriteString("```\n")
    prompt.WriteString(info.PristineContent)
    prompt.WriteString("\n```\n\n")
}
```

### Fix 3: Accurate Failure Tracking

```go
// After each LLM attempt fails, parse the ACTUAL error
func parseGitApplyError(output string) *FailureInfo {
    // Parse: "error: patch failed: go.sum:933"
    // Extract: filename = "go.sum", line = 933
    return &FailureInfo{
        Filename: extractFilename(output),
        Line:     extractLine(output),
        Message:  output,
    }
}

// Update context with CURRENT failure, not stale .rej files
ctx.CurrentFailure = parseGitApplyError(applyError)
ctx.BuildError = applyError.Error()
```

### Fix 4: Clear Status Indicators

```markdown
## File Status Summary

### go.mod
- ❌ **FAILED** - Could not apply, needs fixing
- Error: patch failed: go.mod:8
- Reason: Expected blank line after replace statement, but found additional replace statements

### go.sum
- ⚠️ **APPLIED WITH OFFSET** - Applied successfully but at different line numbers
- Original line: 933
- Actual line: 935
- Offset: +2 lines
- **ACTION REQUIRED**: Include in fixed patch with updated line numbers (933 → 935)
```

## Summary: What's Wrong and What's Needed

### What's Wrong (Misleading Context):

1. ❌ **Reading modified files**: go.sum shows v1.2.0 (already applied) instead of v1.2.8 (original)
2. ❌ **Misleading status**: "APPLIED CLEANLY" suggests no action needed
3. ❌ **No offset explanation**: LLM doesn't understand what offset means or what to do
4. ❌ **Stale failure info**: Shows go.mod failed even when go.sum is the problem
5. ❌ **State pollution**: Same mistake repeated across attempts

### What's Needed (Missing Context):

1. ✅ **PRISTINE content**: Read files BEFORE git apply modifies them
2. ✅ **Offset details**: Original line vs actual line, with clear explanation
3. ✅ **Clear instructions**: Tell LLM to include offset files with updated line numbers
4. ✅ **Accurate failure tracking**: Show what failed THIS attempt, not previous
5. ✅ **State isolation**: Extract context once, reuse pristine version for all attempts

## Implementation Priority

### Priority 1 (CRITICAL): Extract Pristine Context First
Without this, the LLM will never see the correct content and will keep failing.

### Priority 2 (HIGH): Better Offset Handling
The LLM needs to understand offset files and include them in the fixed patch.

### Priority 3 (MEDIUM): Accurate Failure Tracking
Helps LLM understand what's actually wrong in each attempt.

### Priority 4 (LOW): Better Status Messages
Improves clarity but doesn't affect correctness.
