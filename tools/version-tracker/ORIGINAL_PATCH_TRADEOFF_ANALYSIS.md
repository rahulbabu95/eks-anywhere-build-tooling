# Original Patch in Subsequent Attempts - Tradeoff Analysis

## Current Situation

**Original patch size**: 62,843 bytes (~15,700 tokens)
**In prompt**: Takes up 1,635 lines (lines 966-2600 in attempt 2)

This is included in **every attempt** (1, 2, 3).

## Arguments FOR Including Original Patch

### 1. **Metadata Preservation**
The LLM needs to copy exact metadata:
```
From: Prow Bot <prow@amazonaws.com>
Date: Sun, 20 Apr 2025 20:16:04 -0700
Subject: [PATCH 1/3] Remove-Cloud-Provider-Builders-Except-CAPI
```

**Counter**: We already extract this separately as "Original Patch Metadata" (lines 3-7).

### 2. **File List Reference**
Shows all 30 files that need to be in the output.

**Counter**: We already list failed/clean files in the status section.

### 3. **Semantic Intent**
Shows what the patch is trying to do overall.

**Counter**: We already extract "Original Patch Intent" (line 8).

### 4. **Complete Context**
LLM can see the full scope of changes.

**Counter**: For failed files, we provide detailed "Expected vs Actual" context. For clean files, we just need to copy them.

## Arguments AGAINST Including Original Patch

### 1. **Token Waste** 
- **Cost**: ~15,700 tokens × 3 attempts = 47,100 tokens
- **At $3/M input tokens**: $0.14 wasted per patch
- **Across 100 patches**: $14 wasted

### 2. **Attention Dilution**
- 1,635 lines of patch content
- Buries critical error messages
- LLM focuses on semantic changes instead of format errors

### 3. **Redundancy**
We already provide:
- Metadata (extracted)
- Intent (extracted)
- Failed file context (detailed)
- Clean file list (names only)

### 4. **Context Window Pressure**
- Takes up 40% of the prompt
- Could use that space for better error context
- Limits how much file context we can provide

## What the LLM Actually Needs

Let me trace through what the LLM uses:

### Attempt 1 (No previous failure)
- ✅ Metadata: To copy headers
- ✅ Failed file context: To fix conflicts
- ✅ Original patch: To see all files and their changes
- **Verdict**: NEEDED

### Attempt 2+ (Has previous failure)
- ✅ Metadata: Already extracted separately
- ✅ Failed file context: Already provided
- ✅ Error message: "corrupt patch at line 276"
- ❓ Original patch: To... what exactly?

**The LLM should be fixing the ERROR, not re-doing the semantic change.**

## The Real Question

**What is attempt 2+ trying to fix?**

### Scenario A: Semantic Error (line numbers wrong)
- LLM needs to see current file state ✅ (we provide)
- LLM needs to understand intent ✅ (we provide)
- LLM needs original patch? ❌ (redundant)

### Scenario B: Format Error (corrupt patch)
- LLM needs to see the error ✅ (we provide)
- LLM needs to understand patch format ✅ (implicit knowledge)
- LLM needs original patch? ❌ (it just generated it!)

### Scenario C: Missing Files
- LLM needs to know which files ✅ (we list them)
- LLM needs original patch? ⚠️ (maybe, for file content)

## Proposed Solution

### Option 1: Remove Original Patch from Attempt 2+
```go
if attempt == 1 {
    // Include full original patch
    prompt.WriteString("## Original Patch (For Reference)\n\n")
    prompt.WriteString("```diff\n")
    prompt.WriteString(ctx.OriginalPatch)
    prompt.WriteString("\n```\n\n")
} else {
    // Just reference it
    prompt.WriteString("## Original Patch\n\n")
    prompt.WriteString("You already have the original patch from attempt 1.\n")
    prompt.WriteString("Focus on fixing the ERROR shown below, not re-implementing the semantic change.\n\n")
}
```

**Savings**: ~31,400 tokens (attempts 2+3)
**Risk**: LLM might forget file list

### Option 2: Include Only File List from Attempt 2+
```go
if attempt == 1 {
    // Full patch
    prompt.WriteString(ctx.OriginalPatch)
} else {
    // Just the file list and stats
    prompt.WriteString("## Original Patch Files\n\n")
    prompt.WriteString("Your patch must include these files:\n")
    for _, file := range extractFileList(ctx.OriginalPatch) {
        prompt.WriteString(fmt.Sprintf("- %s\n", file))
    }
}
```

**Savings**: ~15,000 tokens (most of the patch content)
**Risk**: Lower - still shows what files are needed

### Option 3: Include Only Failed File Portions
```go
if attempt > 1 {
    // Only show the parts that failed
    prompt.WriteString("## Original Patch (Failed Portions Only)\n\n")
    for _, failedFile := range failedFiles {
        // Extract just that file's diff from original patch
        prompt.WriteString(extractFileDiff(ctx.OriginalPatch, failedFile))
    }
}
```

**Savings**: ~14,000 tokens (29 clean files removed)
**Risk**: Lowest - still shows failed file's original intent

## Recommendation

**Use Option 3: Include only failed file portions from attempt 2+**

### Why?
1. **Keeps what's needed**: Failed file's original diff for reference
2. **Removes waste**: 29 clean files that applied successfully
3. **Low risk**: LLM still sees the file it needs to fix
4. **Token savings**: ~14,000 tokens per attempt 2+

### Implementation
```go
// After line 498 in llm.go
if attempt == 1 {
    // First attempt: include full original patch
    prompt.WriteString("**Full Original Patch:**\n")
    prompt.WriteString("```diff\n")
    prompt.WriteString(ctx.OriginalPatch)
    prompt.WriteString("\n```\n\n")
} else {
    // Subsequent attempts: only failed file portions
    prompt.WriteString("**Original Patch (Failed Files Only):**\n")
    prompt.WriteString("```diff\n")
    
    // Extract just the failed files from original patch
    for _, hunk := range ctx.FailedHunks {
        fileName := filepath.Base(hunk.FilePath)
        fileDiff := extractFileDiffFromPatch(ctx.OriginalPatch, fileName)
        if fileDiff != "" {
            prompt.WriteString(fileDiff)
            prompt.WriteString("\n")
        }
    }
    
    prompt.WriteString("```\n\n")
    prompt.WriteString(fmt.Sprintf("Note: %d other files applied successfully and are not shown here.\n\n", 
        len(extractAllFiles(ctx.OriginalPatch)) - len(ctx.FailedHunks)))
}
```

## Expected Impact

### Token Usage
- **Before**: 15,700 tokens × 3 attempts = 47,100 tokens
- **After**: 15,700 (attempt 1) + 1,700 (attempt 2) + 1,700 (attempt 3) = 19,100 tokens
- **Savings**: 28,000 tokens (60% reduction)

### Cost Savings
- Per patch: $0.08 saved
- Per 100 patches: $8 saved
- Per 1000 patches: $80 saved

### Quality Impact
- **Better**: LLM focuses on the error, not semantic changes
- **Better**: Critical error message more prominent
- **Same**: Still has all context needed to fix failed files
- **Risk**: Minimal - we're only removing successfully applied files

## Conclusion

**Yes, we should remove most of the original patch from attempt 2+.**

Keep only:
- Metadata (already extracted)
- Failed file diffs (for reference)
- File list (for completeness check)

Remove:
- All 29 successfully applied file diffs
- Redundant context

This saves tokens, improves focus, and reduces risk of the LLM ignoring errors.
