# Patch Fixing Strategy Analysis

## The Core Problem

We have a patch with multiple files:
- **Some files apply cleanly** (e.g., go.sum - offset 2 lines but succeeded)
- **Some files fail** (e.g., go.mod - rejected)

How do we handle this without introducing bugs or bias?

---

## Option 1: Selective Prompting (Current Approach)

### Strategy
- Show LLM ONLY the failed files
- Ask it to generate patches ONLY for failed files
- Manually merge: successful parts (unchanged) + LLM-fixed parts

### Pros
✅ Focused prompt - LLM only sees what needs fixing
✅ Smaller token usage
✅ Less chance of LLM changing successful parts
✅ Clear separation of concerns

### Cons
❌ **Complex merging logic** - need to combine patches correctly
❌ **Loss of context** - LLM doesn't see successful changes that might inform the fix
❌ **Patch format issues** - need to reconstruct proper patch headers, line counts, etc.
❌ **Edge case**: What if successful file provides context for failed file?

### Implementation Complexity
**HIGH** - Requires:
1. Parse original patch to extract successful vs failed hunks
2. Generate new patch for failed parts only
3. Merge patches while preserving:
   - Correct file headers
   - Correct line numbers
   - Correct hunk counts
   - Proper diff format

### Example: source-controller Case
```
Original patch:
- go.mod (FAILED)
- go.sum (SUCCEEDED with offset)

Selective approach:
1. Show LLM only go.mod context
2. LLM generates patch for go.mod only
3. Manually add go.sum changes from original patch
4. Reconstruct complete patch with both files
```

**Problem**: go.sum succeeded with "offset 2 lines" - do we use original line numbers or adjusted ones?

---

## Option 2: Full Patch with Focused Instructions (Recommended)

### Strategy
- Show LLM the COMPLETE original patch
- Show LLM ALL failed hunks with context
- Explicitly tell LLM which files succeeded vs failed
- Ask LLM to generate COMPLETE patch with:
  - Failed files: FIXED
  - Successful files: UNCHANGED (copy from original)

### Pros
✅ **LLM has full context** - can see relationships between files
✅ **Simpler merging** - LLM returns complete patch, use as-is
✅ **Proper patch format** - LLM generates correct headers, line counts
✅ **Context preservation** - successful changes inform failed fixes
✅ **Less custom code** - no complex patch parsing/merging

### Cons
❌ More tokens used (but we have 1M tokens/min now)
❌ Risk of LLM modifying successful parts (need validation)
❌ Need to verify LLM doesn't hallucinate

### Implementation Complexity
**MEDIUM** - Requires:
1. Clear prompt instructions about which files failed
2. Validation that successful files weren't changed
3. Fallback if LLM modifies successful parts

### Example: source-controller Case
```
Prompt:
"Original patch has 2 files:
- go.mod: FAILED (needs fixing)
- go.sum: SUCCEEDED (keep unchanged)

Generate complete patch:
1. Fix go.mod to match current file state
2. Keep go.sum changes EXACTLY as in original patch
3. Use correct line numbers for both files"

LLM returns complete patch with both files.
```

---

## Option 3: Hybrid Approach (Best of Both)

### Strategy
- **Primary**: Use Option 2 (full patch)
- **Validation**: Check if LLM changed successful parts
- **Fallback**: If validation fails, use Option 1 (selective)

### Implementation
```go
1. Generate prompt with full context + explicit instructions
2. LLM returns complete patch
3. Validate:
   - Extract successful file hunks from LLM patch
   - Compare with original patch successful hunks
   - If different: REJECT and retry with selective approach
4. If validation passes: Use LLM patch as-is
5. If validation fails 3 times: Fall back to selective approach
```

### Pros
✅ Best of both worlds
✅ Safety through validation
✅ Fallback for edge cases
✅ Context preservation when it works

### Cons
❌ Most complex implementation
❌ Need robust validation logic
❌ Potential for multiple LLM calls

---

## Detailed Analysis of Edge Cases

### Case 1: Partial Success (go.mod fails, go.sum succeeds)

**Current source-controller situation**

#### Option 1 (Selective)
```python
# Pseudocode
original_patch = parse_patch(original_file)
failed_files = ["go.mod"]
successful_files = ["go.sum"]

# Generate prompt for failed files only
llm_prompt = build_prompt(failed_files, context)
llm_patch = call_llm(llm_prompt)  # Returns only go.mod changes

# Merge patches
final_patch = merge_patches(
    successful_hunks=extract_hunks(original_patch, successful_files),
    fixed_hunks=llm_patch
)
```

**Challenges**:
- `merge_patches()` is non-trivial
- Need to handle:
  - File ordering
  - Line number adjustments
  - Hunk counts in headers
  - Offset calculations

#### Option 2 (Full Patch)
```python
# Pseudocode
llm_prompt = f"""
Original patch (2 files):
{original_patch}

Status:
- go.mod: FAILED (fix this)
- go.sum: SUCCEEDED (keep unchanged)

Failed hunk context:
{failed_hunk_context}

Generate COMPLETE patch with:
1. go.mod: FIXED to match current state
2. go.sum: UNCHANGED from original
"""

llm_patch = call_llm(llm_prompt)  # Returns complete patch

# Validate
if validate_successful_files_unchanged(llm_patch, original_patch):
    return llm_patch
else:
    # Retry or fallback
```

**Challenges**:
- Need robust validation
- LLM might still modify successful parts

### Case 2: Successful Part Provides Context

**Example**: go.sum changes show which version is being used, helps fix go.mod

#### Option 1 (Selective) - LOSES CONTEXT
```
Prompt only shows:
- go.mod failed hunk
- go.mod current state

Missing: go.sum shows we're updating to v1.2.0
```

**Result**: LLM might not know which version to use

#### Option 2 (Full Patch) - PRESERVES CONTEXT
```
Prompt shows:
- Complete original patch (both files)
- go.sum changes visible: v1.2.8 → v1.2.0
- go.mod failed hunk
- go.mod current state

LLM can see: "Oh, we're downgrading to v1.2.0, let me use that in go.mod"
```

**Result**: LLM has full context to make correct fix

### Case 3: LLM Hallucination Risk

**Scenario**: LLM modifies successful parts or adds unrelated changes

#### Detection Strategy
```go
func validateLLMPatch(llmPatch, originalPatch string, successfulFiles []string) error {
    // Parse both patches
    llmHunks := parsePatch(llmPatch)
    originalHunks := parsePatch(originalPatch)
    
    // For each successful file
    for _, file := range successfulFiles {
        llmFileHunks := extractFileHunks(llmHunks, file)
        originalFileHunks := extractFileHunks(originalHunks, file)
        
        // Compare hunks (ignoring line number offsets)
        if !hunksEquivalent(llmFileHunks, originalFileHunks) {
            return fmt.Errorf("LLM modified successful file: %s", file)
        }
    }
    
    return nil
}
```

#### Mitigation
1. **Strict validation**: Reject if successful files changed
2. **Explicit instructions**: "DO NOT modify go.sum"
3. **Retry with stronger prompt**: Add "CRITICAL: Keep go.sum EXACTLY as shown"
4. **Fallback**: Use selective approach if validation fails repeatedly

---

## Recommended Implementation Strategy

### Phase 1: Full Patch Approach (Immediate)

**Why**: Simpler, preserves context, works for most cases

```go
func BuildPrompt(ctx *types.PatchContext, attempt int) string {
    // Identify failed vs successful files
    failedFiles := getFailedFiles(ctx.FailedHunks)
    successfulFiles := getSuccessfulFiles(ctx.OriginalPatch, failedFiles)
    
    prompt := `
## Files Status
Failed files (need fixing): ` + strings.Join(failedFiles, ", ") + `
Successful files (keep unchanged): ` + strings.Join(successfulFiles, ", ") + `

## Failed Hunks
[Show detailed context for failed files]

## Original Complete Patch
[Show full patch for context]

## Task
Generate COMPLETE patch that:
1. FIXES failed files: ` + strings.Join(failedFiles, ", ") + `
2. KEEPS successful files UNCHANGED: ` + strings.Join(successfulFiles, ", ") + `
3. Preserves all metadata

CRITICAL: Do NOT modify successful files. Copy them exactly from original patch.
`
    
    return prompt
}
```

### Phase 2: Add Validation (Next)

```go
func ApplyLLMPatch(llmPatch string, ctx *types.PatchContext) error {
    // Identify successful files
    successfulFiles := getSuccessfulFiles(ctx.OriginalPatch, ctx.FailedHunks)
    
    // Validate LLM didn't modify successful files
    if err := validateSuccessfulFilesUnchanged(llmPatch, ctx.OriginalPatch, successfulFiles); err != nil {
        logger.Warn("LLM modified successful files", "error", err)
        // Add to context for retry
        ctx.BuildError = "Previous attempt modified files that applied successfully. Do not modify: " + strings.Join(successfulFiles, ", ")
        return err
    }
    
    // Apply patch
    return applyPatch(llmPatch)
}
```

### Phase 3: Selective Fallback (Future)

If validation fails repeatedly, fall back to selective approach with manual merging.

---

## Avoiding source-controller Bias

### Generic Principles

1. **File-agnostic**: Don't hardcode "go.mod" or "go.sum"
2. **Format-agnostic**: Works for any patch format
3. **Language-agnostic**: Not specific to Go projects
4. **Offset-aware**: Handle "succeeded with offset" cases
5. **Multi-file**: Handle patches with 1-N files

### Test Cases to Validate

```
1. Single file, single hunk (simplest)
2. Single file, multiple hunks
3. Multiple files, all fail
4. Multiple files, all succeed (shouldn't call LLM)
5. Multiple files, partial success (source-controller case)
6. Multiple files, one succeeds with offset
7. Multiple files, interdependent changes
8. Dockerfile + code changes
9. Config file + implementation
10. Test file + source file
```

### Implementation Checklist

```go
// Generic helper functions
func getFailedFiles(failedHunks []FailedHunk) []string
func getSuccessfulFiles(originalPatch string, failedFiles []string) []string
func extractFileHunks(patch string, filename string) []Hunk
func hunksEquivalent(hunk1, hunk2 []Hunk) bool
func validateSuccessfulFilesUnchanged(llmPatch, originalPatch string, successfulFiles []string) error
```

---

## Decision Matrix

| Scenario | Option 1 (Selective) | Option 2 (Full) | Option 3 (Hybrid) |
|----------|---------------------|-----------------|-------------------|
| Simple case (1 file fails) | ✅ Works | ✅ Works | ✅ Works |
| Partial success | ⚠️ Complex merge | ✅ Simple | ✅ Best |
| Context needed | ❌ Loses context | ✅ Preserves | ✅ Preserves |
| LLM hallucination | ✅ Can't happen | ❌ Risk | ✅ Detected |
| Implementation | ❌ Complex | ✅ Simple | ⚠️ Medium |
| Token usage | ✅ Low | ⚠️ Higher | ⚠️ Higher |
| Maintenance | ❌ High | ✅ Low | ⚠️ Medium |

---

## Recommendation

### Start with Option 2 (Full Patch) + Basic Validation

**Rationale**:
1. Simpler implementation
2. Preserves context (critical for complex patches)
3. LLM is good at following instructions
4. We can add validation incrementally
5. Fallback to Option 1 if needed (Phase 2)

### Implementation Plan

**Immediate** (Current PR):
1. ✅ Update prompt to show failed vs successful files
2. ✅ Add explicit instructions not to modify successful files
3. ✅ Test with source-controller case

**Next** (Phase 2):
1. Add validation for successful files
2. Add retry with stronger prompt if validation fails
3. Test with multiple patch types

**Future** (Phase 3):
1. Implement selective approach as fallback
2. Add patch merging logic
3. Comprehensive test suite

---

## Current Implementation Status

### What We Just Fixed
✅ Prompt now identifies failed vs successful files
✅ Explicit instruction: "ONLY includes changes for FAILED files"
✅ Per-file instructions added

### What We Need to Add
⏳ Validation that successful files weren't modified
⏳ Helper function to identify successful files
⏳ Retry logic if validation fails

### Testing Needed
⏳ source-controller (go.mod fails, go.sum succeeds)
⏳ kind (multiple files, all fail)
⏳ Simple case (single file fails)

---

## Conclusion

**Option 2 (Full Patch) is the right starting point** because:
1. Simpler to implement correctly
2. Preserves critical context
3. LLMs are good at following structured instructions
4. We can add validation and fallbacks incrementally
5. Less risk of introducing bugs in patch merging logic

The key is **clear, explicit instructions** and **validation**, not complex code.
