# Reverse Engineering: What Context is Needed to Fix the Patch?

## The Scenario

We have a patch that was created against an OLD version of source-controller, and we're trying to apply it to a NEW version (v1.7.2).

**Original Patch Intent:**
```diff
diff --git a/go.mod b/go.mod
--- a/go.mod
+++ b/go.mod
@@ -8,6 +8,8 @@ replace github.com/fluxcd/source-controller/api => ./api
 // xref: https://github.com/opencontainers/go-digest/pull/66
 replace github.com/opencontainers/go-digest => github.com/opencontainers/go-digest v1.0.1-0.20220411205349-bde1400a84be
 
+replace github.com/sigstore/timestamp-authority => github.com/sigstore/timestamp-authority v1.2.0
+
 require (
 	cloud.google.com/go/compute/metadata v0.8.0
 	cloud.google.com/go/storage v1.56.1

diff --git a/go.sum b/go.sum
--- a/go.sum
+++ b/go.sum
@@ -933,8 +933,8 @@ github.com/sigstore/sigstore/pkg/signature/kms/gcp v1.9.5 h1:...
-github.com/sigstore/timestamp-authority v1.2.8 h1:BEV3fkphwU4zBp3allFAhCqQb99HkiyCXB853RIwuEE=
-github.com/sigstore/timestamp-authority v1.2.8/go.mod h1:G2/0hAZmLPnevEwT1S9IvtNHUm9Ktzvso6xuRhl94ZY=
+github.com/sigstore/timestamp-authority v1.2.0 h1:Ffk10QsHxu6aLwySQ7WuaoWkD63QkmcKtozlEFot/VI=
+github.com/sigstore/timestamp-authority v1.2.0/go.mod h1:ojKaftH78Ovfow9DzuNl5WgTCEYSa4m5622UkKDHRXc=
```

**What Actually Happened:**
```
Checking patch go.mod...
error: patch failed: go.mod:8
Applying patch go.mod with 1 reject...
Rejected hunk #1.

Checking patch go.sum...
Hunk #1 succeeded at 935 (offset 2 lines).
Applied patch go.sum cleanly.
```

## As a Human, What Would I Need to Know?

### 1. **Understanding the Conflict**

**Question**: Why did go.mod fail?
- The patch expects to find a blank line after the `replace github.com/opencontainers/go-digest` line
- But in v1.7.2, there might be additional replace statements or different formatting

**What I'd want to see:**
```
Expected (from OLD version):
---
// xref: https://github.com/opencontainers/go-digest/pull/66
replace github.com/opencontainers/go-digest => github.com/opencontainers/go-digest v1.0.1-0.20220411205349-bde1400a84be

require (
---

Actual (in NEW version v1.7.2):
---
// xref: https://github.com/opencontainers/go-digest/pull/66
replace github.com/opencontainers/go-digest => github.com/opencontainers/go-digest v1.0.1-0.20220411205349-bde1400a84be

replace github.com/something/else => github.com/something/else v1.0.0

require (
---
```

### 2. **Understanding the Offset**

**Question**: Why did go.sum succeed with offset +2?
- The patch expects to find the timestamp-authority lines at line 933
- But in v1.7.2, they're actually at line 935 (2 lines later)
- This means 2 new dependencies were added between the OLD and NEW versions

**What I'd want to see:**
```
Original patch expected at line 933:
---
github.com/sigstore/timestamp-authority v1.2.8 h1:BEV3fkphwU4zBp3allFAhCqQb99HkiyCXB853RIwuEE=
github.com/sigstore/timestamp-authority v1.2.8/go.mod h1:G2/0hAZmLPnevEwT1S9IvtNHUm9Ktzvso6xuRhl94ZY=
---

Actually found at line 935 in v1.7.2:
---
github.com/sigstore/timestamp-authority v1.2.8 h1:BEV3fkphwU4zBp3allFAhCqQb99HkiyCXB853RIwuEE=
github.com/sigstore/timestamp-authority v1.2.8/go.mod h1:G2/0hAZmLPnevEwT1S9IvtNHUm9Ktzvso6xuRhl94ZY=
---

Context around line 935:
---
930: github.com/sigstore/sigstore/pkg/signature/kms/gcp v1.9.5 h1:...
931: github.com/sigstore/sigstore/pkg/signature/kms/gcp v1.9.5/go.mod h1:...
932: github.com/sigstore/sigstore/pkg/signature/kms/hashivault v1.9.5 h1:...
933: github.com/sigstore/sigstore/pkg/signature/kms/hashivault v1.9.5/go.mod h1:...
934: [NEW LINE ADDED IN v1.7.2]
935: github.com/sigstore/timestamp-authority v1.2.8 h1:BEV3fkphwU4zBp3allFAhCqQb99HkiyCXB853RIwuEE=
936: github.com/sigstore/timestamp-authority v1.2.8/go.mod h1:G2/0hAZmLPnevEwT1S9IvtNHUm9Ktzvso6xuRhl94ZY=
937: github.com/sirupsen/logrus v1.2.0/go.mod h1:...
---
```

### 3. **What the Fix Should Be**

**For go.mod:**
- Find where the replace statements are in v1.7.2
- Insert the new replace statement in the correct location (after the last replace, before require)
- Use the ACTUAL formatting and spacing from v1.7.2

**For go.sum:**
- The lines are at 935, not 933
- Replace the v1.2.8 lines with v1.2.0 lines at the CORRECT location (935-936)

## Critical Insight: The State Problem

**Your speculation is CORRECT!** Here's the issue:

### Attempt 1:
1. Checkout fresh v1.7.2 ✅
2. Apply patch with --reject
   - go.mod: FAILS → creates go.mod.rej ✅
   - go.sum: SUCCEEDS with offset → **go.sum is now MODIFIED** ⚠️
3. Extract context from v1.7.2 files
   - go.mod: reads ORIGINAL v1.7.2 ✅
   - go.sum: reads **MODIFIED** v1.7.2 (with v1.2.0 already applied!) ❌
4. LLM generates patch
5. Apply LLM patch → FAILS on go.sum because v1.2.0 is already there!

### Attempt 2:
1. Revert changes (git reset --hard)
2. Checkout fresh v1.7.2 again ✅
3. Apply patch with --reject AGAIN
   - go.mod: FAILS → creates go.mod.rej ✅
   - go.sum: SUCCEEDS with offset → **go.sum is MODIFIED AGAIN** ⚠️
4. Extract context
   - go.sum: reads **MODIFIED** v1.7.2 (with v1.2.0 already applied!) ❌
5. Same problem repeats!

## The Root Cause

**We're extracting context AFTER the partial patch has been applied!**

When `git apply --reject` runs:
- Failed hunks → .rej files created
- Successful hunks → **ACTUALLY APPLIED TO THE FILES** ⚠️

So when we read go.sum to extract context, we're reading the ALREADY-MODIFIED version, not the ORIGINAL v1.7.2.

## What Context We SHOULD Provide

### For Failed Files (go.mod):
```markdown
## go.mod - FAILED

### Original Patch Intent:
Add this replace statement:
```
replace github.com/sigstore/timestamp-authority => github.com/sigstore/timestamp-authority v1.2.0
```

### What the patch expected (from OLD version):
```
// xref: https://github.com/opencontainers/go-digest/pull/66
replace github.com/opencontainers/go-digest => github.com/opencontainers/go-digest v1.0.1-0.20220411205349-bde1400a84be

require (
```

### What's actually in v1.7.2 (BEFORE any modifications):
```
// xref: https://github.com/opencontainers/go-digest/pull/66
replace github.com/opencontainers/go-digest => github.com/opencontainers/go-digest v1.0.1-0.20220411205349-bde1400a84be

replace github.com/something/else => github.com/something/else v1.0.0

require (
```

### Task:
Insert the new replace statement in the correct location in v1.7.2
```

### For Offset Files (go.sum):
```markdown
## go.sum - APPLIED WITH OFFSET (+2 lines)

### Original Patch Intent:
Replace these lines:
```
-github.com/sigstore/timestamp-authority v1.2.8 h1:BEV3fkphwU4zBp3allFAhCqQb99HkiyCXB853RIwuEE=
-github.com/sigstore/timestamp-authority v1.2.8/go.mod h1:G2/0hAZmLPnevEwT1S9IvtNHUm9Ktzvso6xuRhl94ZY=
+github.com/sigstore/timestamp-authority v1.2.0 h1:Ffk10QsHxu6aLwySQ7WuaoWkD63QkmcKtozlEFot/VI=
+github.com/sigstore/timestamp-authority v1.2.0/go.mod h1:ojKaftH78Ovfow9DzuNl5WgTCEYSa4m5622UkKDHRXc=
```

### Patch expected these at line 933, but they're actually at line 935 in v1.7.2

### Current content in v1.7.2 (BEFORE any modifications) around line 935:
```
933: github.com/sigstore/sigstore/pkg/signature/kms/hashivault v1.9.5/go.mod h1:...
934: [some new dependency added in v1.7.2]
935: github.com/sigstore/timestamp-authority v1.2.8 h1:BEV3fkphwU4zBp3allFAhCqQb99HkiyCXB853RIwuEE=
936: github.com/sigstore/timestamp-authority v1.2.8/go.mod h1:G2/0hAZmLPnevEwT1S9IvtNHUm9Ktzvso6xuRhl94ZY=
937: github.com/sirupsen/logrus v1.2.0/go.mod h1:...
```

### Task:
Generate a patch that replaces lines 935-936 (not 933-934) with the v1.2.0 versions
```

## What We're ACTUALLY Providing (Current Implementation)

### Problem 1: Reading Modified Files
```markdown
## go.sum context

**Status**: ⚠️ APPLIED WITH OFFSET (+2 lines)

**Current content:**
```
935: github.com/sigstore/timestamp-authority v1.2.0 h1:Ffk10QsHxu6aLwySQ7WuaoWkD63QkmcKtozlEFot/VI=  ← ALREADY MODIFIED!
936: github.com/sigstore/timestamp-authority v1.2.0/go.mod h1:ojKaftH78Ovfow9DzuNl5WgTCEYSa4m5622UkKDHRXc=  ← ALREADY MODIFIED!
```
```

**LLM sees**: "Oh, v1.2.0 is already there, nothing to do!"
**LLM generates**: Patch that doesn't change go.sum (or removes it thinking it's wrong)

### Problem 2: Misleading Status
The prompt says "go.mod FAILED" even on attempt 2 and 3, but:
- Attempt 1: go.mod failed ✅
- Attempt 2: go.sum failed (because v1.2.0 already applied) ❌
- Attempt 3: go.sum failed again ❌

## The Solution

### Fix 1: Extract Context BEFORE Applying Patch
```
1. Checkout fresh v1.7.2
2. Extract context from PRISTINE files ✅
3. Apply patch with --reject
4. Identify what failed
5. Pass PRISTINE context to LLM
```

### Fix 2: Track What Actually Failed in Each Attempt
```
Attempt 1:
- Applied: go.sum (offset +2)
- Failed: go.mod
- Error: "patch failed: go.mod:8"

Attempt 2:
- Applied: go.mod (from LLM fix)
- Failed: go.sum
- Error: "patch failed: go.sum:933" (because v1.2.0 already there)

Attempt 3:
- Applied: go.mod (from LLM fix)
- Failed: go.sum
- Error: "patch failed: go.sum:933"
```

### Fix 3: For Offset Files, Show ORIGINAL Content
```markdown
## go.sum - APPLIED WITH OFFSET (+2 lines)

**What the patch tried to change:**
Line 933 (OLD version): github.com/sigstore/timestamp-authority v1.2.8 ...
Line 933 (OLD version): github.com/sigstore/timestamp-authority v1.2.8/go.mod ...

**Where it actually is in v1.7.2 (BEFORE modification):**
Line 935: github.com/sigstore/timestamp-authority v1.2.8 ...  ← ORIGINAL v1.2.8
Line 936: github.com/sigstore/timestamp-authority v1.2.8/go.mod ...  ← ORIGINAL v1.2.8

**What it should become:**
Line 935: github.com/sigstore/timestamp-authority v1.2.0 ...
Line 936: github.com/sigstore/timestamp-authority v1.2.0/go.mod ...
```

## Summary: Misleading vs Missing Context

### Misleading Context (What's Wrong):
1. ❌ **Reading modified files**: go.sum shows v1.2.0 already applied
2. ❌ **Stale status**: Says "go.mod FAILED" on attempt 2, but go.sum is what failed
3. ❌ **Wrong line numbers**: Shows line 933 but file has changed
4. ❌ **No indication of what was already applied**: LLM doesn't know go.sum was partially successful

### Missing Context (What's Needed):
1. ✅ **PRISTINE file content**: Read files BEFORE applying patch
2. ✅ **Accurate failure tracking**: Track what failed in THIS attempt, not previous
3. ✅ **Offset explanation**: Clearly explain "patch expected line X, found at line Y"
4. ✅ **Applied vs Failed distinction**: Show which files were already modified successfully
5. ✅ **Original vs Modified comparison**: For offset files, show BEFORE and AFTER

## Recommended Fixes

### Priority 1: Extract Context Before Applying Patch
- Read all files BEFORE `git apply --reject`
- Store pristine content
- Use pristine content for LLM prompts

### Priority 2: Track Actual Failures Per Attempt
- Parse git apply output to see what failed THIS time
- Update BuildError with current attempt's error
- Don't reuse stale .rej files from previous attempts

### Priority 3: Better Offset Handling
- For offset files, show:
  - Original line number from patch
  - Actual line number in current file
  - Content at BOTH locations
  - Clear instruction to update line numbers

### Priority 4: State Management
- After each failed attempt, fully reset to pristine state
- Re-extract context from pristine files
- Don't accumulate modifications across attempts
