# Key Findings - Why the LLM Keeps Failing

## TL;DR

The LLM generates **identical incorrect patches** across all 3 attempts because:

1. **It's copying from the wrong example** - The "Original Complete Patch" section shows truncated hashes, and the LLM copies them
2. **No failure feedback** - We don't tell it WHY attempt 1 and 2 failed (git apply error details)
3. **Missing go.sum context** - We only show go.mod content, not go.sum actual content
4. **Misleading classification** - We say go.sum "succeeded" when it actually needs line number updates

---

## Your 4 Questions - Answered

### 1. Is the prompt introducing bias?

**YES - Critical bias from the "Original Complete Patch" section**

The prompt shows:
```diff
@@ -933,8 +933,8 @@ github.com/sigstore/sigstore/pkg/signature/kms/gcp v1.9.5 h1:7U0GsO0UGG1PdtgS6wB
```

The LLM generates (all 3 attempts):
```diff
@@ -933,8 +933,8 @@ github.com/sigstore/sigstore/pkg/signature/kms/gcp v1.9.5 h1:7U0GsO0UGG1PdtgS6wB
```

**Identical truncated hash!** The LLM is anchoring on this example and copying it verbatim.

**The bias:**
- The example comes AFTER our instructions
- It's presented as "the correct format"
- Our warnings get overridden by this prominent example
- "Don't prune the hash" is too generic and gets ignored

### 2. Is context being refined between attempts?

**NO - Context is identical**

- Attempt 1 → Attempt 2: **Identical prompts** (9.6K each)
- Attempt 2 → Attempt 3: **Slightly longer** (9.8K) but only adds "Reflection Required"
- **NO build error details** in any attempt
- **NO git apply failure message** shown
- **NO information about which line failed or why**

**What's missing:**
```
Previous attempt failed with:
error: patch failed: go.sum:933
error: go.sum: patch does not apply

Reason: Your context line was truncated:
  You used: h1:7U0GsO0UGG1PdtgS6wB
  Actual:   h1:7U0GsO0UGG1PdtgS6wBkRC0sMgq7BRVaFlPRwN4m1Qg=
```

### 3. Does attempt 3 provide failure context?

**NO - Only asks for reflection, doesn't provide facts**

Attempt 3 adds:
```
## Reflection Required
Before providing the fix, first explain:
1. Why the previous attempts failed
2. What needs to change
3. The specific lines that need modification
```

**The LLM's response shows it's guessing:**
```
Why the previous attempts failed:
1. The patch expects specific version numbers
2. The actual file has been updated
3. The patch tool cannot find the exact context match
```

**This is WRONG!** The real reason is the truncated hash, not version numbers.

**The LLM doesn't know the real error** because we didn't tell it.

### 4. Does the example introduce overfitting?

**YES - Severe overfitting to the wrong example**

The "Original Complete Patch" section causes **catastrophic overfitting**:

- The LLM sees the truncated hash as "the correct format"
- It reproduces the exact same format in all 3 attempts
- Our warnings are ignored because the example contradicts them
- The example should either be removed or heavily annotated with warnings

---

## Additional Critical Observations

### Observation 1: go.sum Context is Missing

The prompt shows:
- ✅ go.mod expected vs actual
- ✅ go.mod current file content  
- ❌ NO go.sum expected vs actual
- ❌ NO go.sum current file content

**The LLM has no way to know:**
- What line 933 actually looks like in go.sum
- That the hash is complete (not truncated)
- Where the timestamp-authority line is

### Observation 2: Misleading "Success" Classification

The prompt says:
```
- FAILED files (FIX these): go.mod
- SUCCESSFUL files (keep UNCHANGED): go.sum
```

**But the logs show:**
```
Hunk #1 succeeded at 935 (offset 2 lines).
```

go.sum applied with an **offset** - meaning the line numbers were wrong. We should treat this as "needs fixing" not "successful".

### Observation 3: The LLM CAN Fix Version Mismatches

In attempt 3, the LLM correctly:
- Identified that v0.8.0 → v0.9.0 and v1.56.1 → v1.57.0
- Updated the context lines to match the current file
- Understood the semantic intent of the patch

**This proves the LLM is capable** when given the right context.

### Observation 4: All 3 Attempts are Identical

The LLM generated the **exact same patch** 3 times because:
1. **Attempt 1**: Copies from "Original Complete Patch" example
2. **Attempt 2**: Same prompt → Same response (no new info)
3. **Attempt 3**: Asks for reflection, but LLM guesses wrong → Same patch

**The LLM is stuck in a loop** with no way to break out.

---

## Root Causes (Priority Order)

### P0 - Critical Issues

1. **Truncated hash in example**
   - Location: "Original Complete Patch" section
   - Impact: LLM copies the truncated format
   - Fix: Remove section or add strong warnings

2. **No failure feedback**
   - Location: Between attempts
   - Impact: LLM can't learn from mistakes
   - Fix: Add git apply error details and specific mismatch info

3. **Missing go.sum context**
   - Location: Context extraction
   - Impact: LLM can't see actual file content
   - Fix: Extract and show go.sum content like we do for go.mod

### P1 - High Priority

4. **Misleading success classification**
   - Location: Task instructions
   - Impact: LLM thinks go.sum doesn't need fixing
   - Fix: Treat "offset" as "needs line number update"

5. **Generic warnings**
   - Location: Throughout prompt
   - Impact: Gets ignored or misunderstood
   - Fix: Make warnings specific with examples

6. **No correct example**
   - Location: Missing
   - Impact: No good pattern to follow
   - Fix: Add side-by-side WRONG vs CORRECT example

---

## Recommended Fixes

### Fix #1: Remove or Annotate "Original Complete Patch"

**Option A - Remove it** (simplest):
- We already show the intent and failed hunks
- The original patch is causing more harm than good

**Option B - Add strong warnings**:
```markdown
## Original Complete Patch (⚠️  OUTDATED - FOR REFERENCE ONLY)

⚠️  WARNING: This patch was created against an OLD version.
⚠️  The context lines are TRUNCATED and line numbers are WRONG.
⚠️  DO NOT copy them. Use "What's ACTUALLY in the file" instead.

[patch content]
```

### Fix #2: Add Explicit Failure Context

```markdown
## Previous Attempt #N Failed

**Git Apply Error:**
```
error: patch failed: go.sum:933
error: go.sum: patch does not apply
```

**Root Cause:**
Your patch used a TRUNCATED context line at go.sum:933:
`github.com/.../gcp v1.9.5 h1:7U0GsO0UGG1PdtgS6wB` ← TRUNCATED

The actual file has the COMPLETE hash:
`github.com/.../gcp v1.9.5 h1:7U0GsO0UGG1PdtgS6wBkRC0sMgq7BRVaFlPRwN4m1Qg=` ← COMPLETE

**Fix:** Use the COMPLETE hash from the actual file.
```

### Fix #3: Extract go.sum Context

Treat ALL files equally:
- Extract context for go.sum (even if it "succeeded")
- Show "Expected vs Actual" for go.sum
- Let LLM see the complete hashes

### Fix #4: Reclassify "Offset" as "Needs Fixing"

```markdown
- FAILED files (FIX these): go.mod
- OFFSET files (UPDATE line numbers): go.sum (applied at 935, expected 933)
```

### Fix #5: Add Correct Example

```markdown
## Example: Correct vs Incorrect

**❌ WRONG (truncated context):**
```diff
-package v1.2.8 h1:ABC...
+package v1.2.0 h1:XYZ...
```

**✅ CORRECT (complete context):**
```diff
-package v1.2.8 h1:ABCdefghijklmnopqrstuvwxyz123456=
+package v1.2.0 h1:XYZabcdefghijklmnopqrstuvwxyz789012=
```

Notice: Complete hashes ending with `=`
```

---

## Impact Assessment

| Issue | Severity | Frequency | Fix Effort | Priority |
|-------|----------|-----------|------------|----------|
| Truncated hash example | CRITICAL | 100% (all attempts) | Low | P0 |
| No failure feedback | HIGH | 100% (attempts 2-3) | Medium | P0 |
| Missing go.sum context | HIGH | 100% (all attempts) | Medium | P0 |
| Misleading classification | MEDIUM | 100% (all attempts) | Low | P1 |
| Generic warnings | MEDIUM | 100% (all attempts) | Low | P1 |
| No correct example | MEDIUM | 100% (all attempts) | Low | P1 |

---

## Next Steps

1. **Immediate**: Fix the 3 P0 issues
   - Remove or annotate "Original Complete Patch"
   - Add failure feedback between attempts
   - Extract go.sum context

2. **Short-term**: Fix the 3 P1 issues
   - Reclassify offset as needs-fixing
   - Make warnings more specific
   - Add correct example

3. **Test**: Run a fresh test and verify:
   - LLM uses complete hashes
   - LLM learns from failure feedback
   - LLM generates different patches on retry

---

## Conclusion

The current prompt has **structural issues** that prevent the LLM from succeeding:

1. **Wrong example** (truncated hash) overrides our instructions
2. **No feedback loop** - LLM can't learn from failures
3. **Incomplete context** - LLM can't see what it needs to fix

These are **fixable** with targeted prompt improvements. The LLM has shown it CAN understand and fix version mismatches when given proper context (see go.mod fix in attempt 3).

**The good news**: These are prompt engineering issues, not fundamental LLM capability issues.
