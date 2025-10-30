# Context Enhancement Flow Diagram

## Before (Original Implementation)

```
.rej file → Extract hunk → Build prompt → LLM
                ↓
         [Original Lines]
         [Context (50 lines)]
                ↓
            Prompt:
         "Here's what failed"
         "Here's the file"
         "Fix it"
```

**Problem**: LLM doesn't see the SPECIFIC differences between what patch expected vs what's actually there.

---

## After (Enhanced Implementation)

```
.rej file → Extract hunk → extractExpectedVsActual() → Build prompt → LLM
                ↓                      ↓
         [Original Lines]    [Compare Expected vs Actual]
         [Context]                     ↓
                                  ExpectedContext: ["line 933: ...v1.2.8 h1:BEV3..."]
                                  ActualContext:   ["line 933: ...v1.2.8 h1:BEV3fkphwU4...="]
                                  Differences:     ["Hash is complete in actual file"]
                                       ↓
                                  Enhanced Prompt:
                              "What patch expects: ..."
                              "What's actually there: ..."
                              "Key differences: ..."
                              "Fix using ACTUAL state"
```

**Benefit**: LLM sees EXACTLY what's different and can make precise fixes.

---

## Detailed Flow with Debug Points

```
┌─────────────────────────────────────────────────────────────────┐
│ 1. Patch Application Fails                                       │
│    git apply --reject → Creates .rej files                       │
└────────────────┬────────────────────────────────────────────────┘
                 ↓
┌─────────────────────────────────────────────────────────────────┐
│ 2. Extract Context (context.go)                                  │
│    - Read .rej file                                              │
│    - Parse failed hunks                                          │
│    - Read actual file content                                    │
│    - Call extractExpectedVsActual()                              │
│      ├─ Extract context lines from .rej (Expected)               │
│      ├─ Find matching section in actual file (Actual)            │
│      └─ Identify differences                                     │
│                                                                   │
│    DEBUG: Log token count, hunk count                            │
└────────────────┬────────────────────────────────────────────────┘
                 ↓
┌─────────────────────────────────────────────────────────────────┐
│ 3. Build Enhanced Prompt (llm.go)                                │
│    - Project info                                                │
│    - Original patch metadata                                     │
│    - For each failed hunk:                                       │
│      ├─ What patch tried to do                                   │
│      ├─ Expected vs Actual comparison ← NEW!                     │
│      │   ├─ Expected context                                     │
│      │   ├─ Actual context                                       │
│      │   └─ Differences                                          │
│      ├─ Current file content (broader context)                   │
│      └─ Explicit instructions                                    │
│    - Original complete patch                                     │
│    - Task instructions                                           │
│                                                                   │
│    DEBUG: Write to /tmp/llm-prompt-attempt-N.txt                 │
└────────────────┬────────────────────────────────────────────────┘
                 ↓
┌─────────────────────────────────────────────────────────────────┐
│ 4. Call Bedrock API (llm.go)                                     │
│    - Initialize client (with inference profile)                  │
│    - Wait for rate limit (15s between requests)                  │
│    - Send request with enhanced prompt                           │
│    - Retry with exponential backoff if needed                    │
│                                                                   │
│    DEBUG: Log model, profile, region                             │
│    DEBUG: Log rate limiting waits                                │
└────────────────┬────────────────────────────────────────────────┘
                 ↓
┌─────────────────────────────────────────────────────────────────┐
│ 5. Process Response (llm.go)                                     │
│    - Extract patch from response                                 │
│    - Validate patch format                                       │
│    - Calculate cost                                              │
│                                                                   │
│    DEBUG: Write to /tmp/llm-response-attempt-N.txt               │
│    DEBUG: Log tokens used, cost, patch length                    │
└────────────────┬────────────────────────────────────────────────┘
                 ↓
┌─────────────────────────────────────────────────────────────────┐
│ 6. Apply Generated Patch (applier.go)                            │
│    - Write to temp file                                          │
│    - Apply with git apply                                        │
│    - Stage changes                                               │
│                                                                   │
│    DEBUG: Write to .llm-patch-debug.txt                          │
│    DEBUG: Log patch preview (first 500 chars)                    │
└────────────────┬────────────────────────────────────────────────┘
                 ↓
┌─────────────────────────────────────────────────────────────────┐
│ 7. Validate (validator.go)                                       │
│    - Build validation                                            │
│    - Semantic validation                                         │
│                                                                   │
│    If fails: Revert and retry with build error in context        │
│    If succeeds: Write to original patch file                     │
│                                                                   │
│    DEBUG: Log validation results                                 │
└─────────────────────────────────────────────────────────────────┘
```

---

## Example: Expected vs Actual in Prompt

### For go.sum Hash Issue

**Before (No explicit comparison)**:
```
### Current file content (around line 933):
github.com/sigstore/sigstore/pkg/signature/kms/gcp v1.9.5 h1:7U0GsO0UGG1PdtgS6wBkRC0sMgq7BRVaFlPRwN4m1Qg=
github.com/sigstore/sigstore/pkg/signature/kms/gcp v1.9.5/go.mod h1:/2qrI0nnCy/DTIPOMFaZlFnNPWEn5UeS70P37XEM88o=
github.com/sigstore/timestamp-authority v1.2.8 h1:BEV3fkphwU4zBp3allFAhCqQb99HkiyCXB853RIwuEE=
```

**After (With explicit comparison)**:
```
### Expected vs Actual File State:

**What the patch expects to find:**
github.com/sigstore/sigstore/pkg/signature/kms/gcp v1.9.5 h1:7U0GsO0UGG1PdtgS6wB
github.com/sigstore/timestamp-authority v1.2.8 h1:BEV3fkphwU4zBp3allFAhCqQb99HkiyCXB853RIwuEE=

**What's actually in the file:**
github.com/sigstore/sigstore/pkg/signature/kms/gcp v1.9.5 h1:7U0GsO0UGG1PdtgS6wBkRC0sMgq7BRVaFlPRwN4m1Qg=
github.com/sigstore/timestamp-authority v1.2.8 h1:BEV3fkphwU4zBp3allFAhCqQb99HkiyCXB853RIwuEE=

**Key differences:**
- Line 933: Expected hash is truncated, actual has complete hash ending with "=Qg="
- The actual file has the COMPLETE hash - use this exact format

### Current file content (around line 933):
[broader context...]
```

This makes it crystal clear to the LLM:
1. The expected context from .rej is truncated
2. The actual file has the complete hash
3. Use the actual file's format (complete hash)

---

## Debug File Locations

```
/tmp/
├── llm-prompt-attempt-1.txt      ← Full prompt sent to LLM (attempt 1)
├── llm-response-attempt-1.txt    ← Full response from LLM (attempt 1)
├── llm-prompt-attempt-2.txt      ← Second attempt (if needed)
└── llm-response-attempt-2.txt

projects/<org>/<repo>/
├── .llm-patch-debug.txt          ← Last generated patch
└── patches/
    └── 0001-*.patch              ← Updated with fixed patch (on success)
```

---

## Key Improvements Summary

1. **Explicit Comparison**: LLM sees Expected vs Actual side-by-side
2. **Difference Highlighting**: Specific differences are called out
3. **Complete Context**: Full file content shown (no truncation)
4. **Debug Visibility**: All prompts and responses saved to files
5. **Better Logging**: Track model, tokens, costs, patch lengths
6. **Rate Limiting**: Prevent 503 errors with proper throttling

---

## Testing Checklist

- [ ] Rebuild binary: `make build`
- [ ] Run test case
- [ ] Check `/tmp/llm-prompt-attempt-1.txt` exists
- [ ] Verify "Expected vs Actual" section in prompt
- [ ] Check `/tmp/llm-response-attempt-1.txt` exists
- [ ] Verify response has complete patch
- [ ] Check `.llm-patch-debug.txt` in project dir
- [ ] Verify hashes are complete (not truncated)
- [ ] Check logs show correct model (Sonnet 4.5)
- [ ] Verify rate limiting messages appear
