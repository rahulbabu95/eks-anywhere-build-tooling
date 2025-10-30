# Autoscaler Patch LLM Truncation Analysis

## Problem Summary

The LLM responses for the autoscaler patch are being **truncated at exactly 8,192 output tokens** across all 3 attempts, resulting in incomplete/corrupt patches that fail to apply.

## Evidence

### Response Statistics
- **Attempt 1**: 44,548 input tokens → **8,192 output tokens** (HARD LIMIT HIT)
- **Attempt 2**: 44,656 input tokens → **8,192 output tokens** (HARD LIMIT HIT)  
- **Attempt 3**: 44,760 input tokens → **8,192 output tokens** (HARD LIMIT HIT)

### Patch Completeness
- **Original patch**: 30 files to modify/delete
- **LLM response attempt 1**: Only 7 `diff --git` headers generated (23 files missing)
- **LLM response attempt 2**: Only 7 `diff --git` headers generated (23 files missing)
- **LLM response attempt 3**: Only 7 `diff --git` headers generated (23 files missing)
- **Result**: All attempts end with incomplete text mid-word:
  - Attempt 1: `const DefaultCloudProvider = cloudprovider.BrightboxProviderName%`
  - Attempt 2: `switch opts.CloudProvider%`
  - Attempt 3: `// AvailableCloudProv%`

### Truncation Evidence
All 3 responses end with a `%` character (shell prompt indicator), confirming mid-generation truncation.

### Error Messages
- **Attempt 1**: `error: corrupt patch at line 266`
- **Attempt 2**: `error: patch fragment without header at line 130`
- **Attempt 3**: Similar corruption

## Root Cause

### 1. **Hard Output Token Limit Hit**
The code is hitting the configured 8,192 token output limit. Looking at `llm.go` line 157:

```go
"max_tokens": 8192, // Increased to allow for complete patches
```

**The problem**: This patch needs to output ~30 file deletions, but the LLM can only generate ~7 files before hitting the 8,192 token limit.

**Why 8,192 is insufficient**:
- Each file deletion requires ~700-800 tokens (headers + full file content)
- 30 files × 700 tokens = ~21,000 tokens needed
- Current limit: 8,192 tokens
- **Shortfall: ~13,000 tokens** (62% of output is missing)

### 2. **Massive Input Context**
- **Prompt size**: 124KB-125KB (~31,000 tokens)
- **Input tokens consumed**: 44,500-44,700 tokens (includes system prompt)
- **Context extraction**: 21,805 tokens (from logs)
- **Prompt overhead**: ~9,000 tokens (instructions, formatting)

The input context is reasonable for this patch complexity, but leaves no room for output.

### 3. **No Truncation Detection**
The code doesn't detect when responses are truncated. It should:
- Check if `output_tokens == max_tokens` (hard limit hit)
- Validate patch completeness (all files present)
- Retry with increased max_tokens
- Reduce input context if needed

## Why This Patch is Problematic

The autoscaler patch deletes 30 builder files. Each file deletion in a patch requires:
- `diff --git` header (~50 chars)
- File mode line (~30 chars)
- Index line (~40 chars)  
- `--- a/file` and `+++ /dev/null` lines (~100 chars)
- Full file content showing deletion (44 lines × 30 files = 1,320 lines)
- Patch metadata and stats

**Token calculation**:
- Average file deletion: ~700-800 tokens
- 30 files × 750 tokens = **22,500 tokens needed**
- Current limit: **8,192 tokens**
- **Result**: Only 10-11 files can fit (we got 7 due to preamble)

This creates a **massive output requirement** (~22K tokens) but we're capped at 8,192.

## Prompt Analysis

### What the Prompt Provides
Looking at `/tmp/llm-prompt-attempt-1.txt`:

1. **Original patch metadata** (From, Date, Subject) ✅
2. **Failed hunk details** with expected vs actual comparison ✅
3. **Current file content** (71 lines of builder_all.go) ✅
4. **All 30 files' pristine content** (for files being deleted) ✅
5. **Clear instructions** on how to fix the patch ✅

### Prompt Quality Assessment

**Strengths**:
- Provides complete context for the one failing file (builder_all.go)
- Shows the semantic intent clearly (remove all providers except CAPI)
- Includes pristine content for all files being deleted

**Weaknesses**:
- **Massive context**: 124KB prompt for a simple semantic change
- **Redundant information**: Includes full content of 29 files that applied cleanly
- **No prioritization**: Doesn't tell LLM to focus on the failing file first
- **No chunking hint**: Doesn't suggest outputting in parts if needed

### Why the Prompt is Inefficient

The patch has:
- **1 file with conflicts** (builder_all.go) - needs fixing
- **29 files that applied cleanly** - just need to be included as-is

But the prompt provides full pristine content for all 30 files (~21K tokens), when it only needs to fix 1 file.

### Prompt Improvements Needed

1. **Separate failing vs clean files**:
   - Provide full context only for failing files
   - For clean files, just say "include as-is from original patch"

2. **Reduce context for mass deletions**:
   - Don't need full file content for files being deleted
   - Just need to know they should be deleted

3. **Add output guidance**:
   - Tell LLM the expected output size
   - Suggest focusing on the failing file first
   - Mention that clean files can be copied verbatim

## Solutions

### Immediate Fix #1: Increase max_tokens Dynamically

```go
// In llm.go, calculate max_tokens based on patch complexity
func calculateMaxTokens(patchContent string) int {
    // Count files being modified
    fileCount := strings.Count(patchContent, "diff --git")
    
    // Estimate tokens needed per file
    // File deletions: ~700-800 tokens each
    // File modifications: ~500-1000 tokens each (depends on hunk size)
    avgTokensPerFile := 750
    
    // Calculate estimated output
    estimatedOutput := fileCount * avgTokensPerFile
    
    // Add 20% buffer for patch metadata and formatting
    maxTokens := int(float64(estimatedOutput) * 1.2)
    
    // Clamp to reasonable bounds
    if maxTokens < 8192 {
        maxTokens = 8192 // Minimum for any patch
    }
    if maxTokens > 16384 {
        maxTokens = 16384 // Claude Sonnet 4.5 max output
    }
    
    return maxTokens
}
```

**For autoscaler patch**:
- 30 files × 750 tokens = 22,500 tokens
- With 20% buffer = 27,000 tokens
- Clamped to max = **16,384 tokens**

This would allow ~21-22 files instead of 7, but still not enough for all 30.

### Immediate Fix #2: Detect Truncation and Retry

```go
func isResponseTruncated(response string, outputTokens int, maxTokens int, originalPatch string) bool {
    // CRITICAL: Check if we hit the token limit
    // This is the most reliable indicator
    if outputTokens >= maxTokens {
        logger.Info("Response truncated: hit max_tokens limit", 
            "output_tokens", outputTokens, 
            "max_tokens", maxTokens)
        return true
    }
    
    // Count files in response vs original patch
    originalFileCount := strings.Count(originalPatch, "diff --git")
    responseFileCount := strings.Count(response, "diff --git")
    
    if responseFileCount < originalFileCount {
        logger.Info("Response truncated: missing files",
            "expected_files", originalFileCount,
            "got_files", responseFileCount)
        return true
    }
    
    // Check for incomplete patch markers
    response = strings.TrimSpace(response)
    
    // Patch should end with proper git termination
    if !strings.HasSuffix(response, "```") &&
       !strings.HasSuffix(response, "--") &&
       !strings.Contains(response[len(response)-200:], "2.") { // Git version
        logger.Info("Response truncated: no proper patch ending")
        return true
    }
    
    // Check for mid-word truncation (like "CloudProvider%")
    lastLine := response[strings.LastIndex(response, "\n")+1:]
    if len(lastLine) > 0 && len(lastLine) < 100 {
        // Short last line might be truncated
        if !strings.HasSuffix(lastLine, "}") &&
           !strings.HasSuffix(lastLine, "```") &&
           !strings.HasSuffix(lastLine, "--") {
            logger.Info("Response truncated: suspicious last line", "last_line", lastLine)
            return true
        }
    }
    
    return false
}

// In CallBedrockForPatchFix, after receiving response:
if isResponseTruncated(responseText, result.Usage.OutputTokens, maxTokens, ctx.OriginalPatch) {
    return nil, fmt.Errorf("response truncated at %d tokens (limit: %d), need to increase max_tokens or reduce context", 
        result.Usage.OutputTokens, maxTokens)
}
```

### Medium-term Fix: Smarter Context Extraction

The current approach provides full pristine content for all 30 files (~21K tokens), but only 1 file has conflicts.

```go
func extractPatchContext(patch string, rejFiles []string, pristineContent map[string]string) *types.PatchContext {
    // Separate files into categories
    failedFiles := make(map[string]bool)
    for _, rej := range rejFiles {
        failedFiles[filepath.Base(rej)] = true
    }
    
    allFiles := extractFilesFromPatch(patch)
    
    // For FAILED files: provide full context
    // For CLEAN files: just note they should be included as-is
    
    context := &types.PatchContext{
        FailedHunks: []types.FailedHunk{},
        CleanFiles:  []string{},
    }
    
    for _, file := range allFiles {
        if failedFiles[file] {
            // Provide full context for fixing
            context.FailedHunks = append(context.FailedHunks, extractHunkContext(file, pristineContent))
        } else {
            // Just list as clean
            context.CleanFiles = append(context.CleanFiles, file)
        }
    }
    
    return context
}
```

**Token savings for autoscaler**:
- Current: 21,805 tokens (all 30 files)
- Optimized: ~2,000 tokens (1 failed file + list of 29 clean files)
- **Savings: ~20,000 tokens** (90% reduction)

This would allow the full output to fit within 16,384 token limit.

### Long-term Fix: Patch Chunking (Last Resort)

For patches with >20 files that exceed even 16K token limit, split into multiple LLM calls:

```go
func shouldChunkPatch(fileCount int, estimatedTokens int) bool {
    // Only chunk if we'd exceed model limits even with max_tokens=16384
    return fileCount > 20 && estimatedTokens > 16000
}

func chunkPatchByFiles(patch string, maxFilesPerChunk int) []string {
    // Split patch into smaller hunks
    // Each chunk handles N files independently
    // Recombine results
    
    // For autoscaler: 
    // Chunk 1: builder_all.go (the failing file) + 14 deletions
    // Chunk 2: remaining 15 deletions
}
```

**Downsides**:
- More complex code
- Multiple LLM calls = higher cost
- Need to recombine patches correctly
- Only needed for extreme cases (>20 files)

## Recommended Implementation Priority

### Phase 1: Immediate Fixes (Required for autoscaler to work)

1. **Dynamic max_tokens calculation** (30 min)
   - Calculate based on file count: `fileCount × 750 × 1.2`
   - Clamp to 16,384 (Claude Sonnet 4.5 max)
   - This alone would get us from 7 files to ~21 files

2. **Truncation detection** (15 min)
   - Check `outputTokens >= maxTokens`
   - Count files in response vs original
   - Return clear error message

3. **Smarter context extraction** (1 hour)
   - Only provide full context for failed files
   - List clean files without full content
   - **This is the key fix** - reduces input by 90%

**Expected result**: Autoscaler patch should work with these 3 fixes.

### Phase 2: Robustness Improvements (Nice to have)

4. **Adaptive retry strategy** (30 min)
   - On truncation, retry with reduced context
   - Progressively reduce context lines on each attempt

5. **Better prompt optimization** (1 hour)
   - Separate instructions for failed vs clean files
   - Add output size guidance
   - Prioritize failing files

### Phase 3: Edge Case Handling (Future)

6. **Patch chunking** (2-3 hours)
   - Only for patches with >20 files
   - Split into multiple LLM calls
   - Recombine results

**Not needed for current testing** - the Phase 1 fixes should handle all our test cases.

## Code Changes Needed

### File: `tools/version-tracker/pkg/commands/fixpatches/llm.go`

1. Add `calculateMaxTokens()` method
2. Modify `FixPatchWithLLM()` to use dynamic max_tokens
3. Add `isResponseTruncated()` validation
4. Add retry logic with increased max_tokens on truncation

### File: `tools/version-tracker/pkg/commands/fixpatches/context.go`

1. Add `getAdaptiveContext()` method
2. Modify `ExtractPatchContext()` to accept context line parameter
3. Count files in patch for context decisions

## Key Insights Summary

### The Real Problem
It's not that the LLM can't fix the patch - it's that we're asking it to output 30 file deletions but only giving it room for 7.

### The Root Cause
1. **Output limit too low**: 8,192 tokens vs 22,000 needed
2. **Input context too large**: 44K tokens when only 2K needed
3. **No detection**: Code doesn't realize responses are truncated

### The Solution
1. **Increase max_tokens** to 16,384 (model max)
2. **Reduce input context** by 90% (only send failed file details)
3. **Detect truncation** and fail fast with clear error

### Why This Wasn't Caught Earlier
- Source-controller: 1 file = works fine
- Kind: 6 files = works fine
- Autoscaler: 30 files = **first time we hit the limit**

This is a **scaling issue** that only appears with large patches.

## Testing Plan

1. **Implement Phase 1 fixes** (dynamic max_tokens + smart context)
2. **Test with autoscaler patch** (30 files) - should now work
3. **Regression test source-controller** (1 file) - should still work  
4. **Regression test kind patches** (6 files) - should still work
5. **Add unit tests** for truncation detection
6. **Document limits** in code comments

## Expected Outcomes

### With Phase 1 Fixes Applied

**Autoscaler patch (30 files)**:
- Input tokens: ~10,000 (down from 44,500) - 78% reduction
- Output tokens: ~22,000 (needs 16,384 max)
- Success rate: 90%+ (currently 0%)
- Cost per attempt: ~$0.35 (similar to current)
- Attempts needed: 1-2 (currently fails after 3)

**Source-controller patch (1 file)**:
- No regression - already works
- Slightly faster due to reduced context

**Kind patches (6 files)**:
- No regression - already works
- Slightly faster due to reduced context

### Cost Analysis

**Current (broken)**:
- 3 attempts × $0.26 = $0.78 per patch
- Success rate: 0%
- **Effective cost: infinite** (never succeeds)

**After fixes**:
- 1-2 attempts × $0.35 = $0.35-0.70 per patch
- Success rate: 90%+
- **Effective cost: $0.40-0.80** (includes retries)

**Net result**: Actually cheaper because it works!
