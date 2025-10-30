# Pre-Test Checklist

Before running the next test, verify all changes are in place.

## 1. Rebuild Binary

```bash
cd tools/version-tracker
make build
```

**Verify**: Binary timestamp should be recent
```bash
ls -lh bin/version-tracker
```

## 2. Verify Code Changes

### A. Check Default Model (cmd/fixpatches.go)
```bash
grep -A 2 '"model"' tools/version-tracker/cmd/fixpatches.go
```

**Expected output**:
```go
fixPatchesCmd.Flags().StringVar(&fixPatchesOptions.Model, "model", "anthropic.claude-sonnet-4-5-20250929-v1:0", "Bedrock model ID to use (Claude Sonnet 4.5 - 200K tokens/min approved)")
```

### B. Check Context Enhancement (pkg/types/fixpatches.go)
```bash
grep -A 3 "ExpectedContext" tools/version-tracker/pkg/types/fixpatches.go
```

**Expected output**:
```go
ExpectedContext []string // What the patch expects to find
ActualContext   []string // What's actually in the file  
Differences     []string // Human-readable differences
```

### C. Check extractExpectedVsActual Function (pkg/commands/fixpatches/context.go)
```bash
grep -c "func extractExpectedVsActual" tools/version-tracker/pkg/commands/fixpatches/context.go
```

**Expected output**: `1` (function exists)

### D. Check Debug Logging (pkg/commands/fixpatches/llm.go)
```bash
grep -c "llm-prompt-attempt" tools/version-tracker/pkg/commands/fixpatches/llm.go
```

**Expected output**: `2` (prompt and response debug files)

### E. Check Enhanced Prompt (pkg/commands/fixpatches/llm.go)
```bash
grep -c "Expected vs Actual File State" tools/version-tracker/pkg/commands/fixpatches/llm.go
```

**Expected output**: `1` (section exists in prompt)

## 3. Clean Up Previous Test Artifacts

```bash
# Remove old debug files
rm -f /tmp/llm-prompt-attempt-*.txt
rm -f /tmp/llm-response-attempt-*.txt

# Remove old patch debug files
find test/eks-anywhere-build-tooling/projects -name ".llm-patch-debug.txt" -delete

# Remove old .rej files (if any)
find test/eks-anywhere-build-tooling/projects -name "*.rej" -delete

# Clean up test repo
cd test/eks-anywhere-build-tooling/projects/fluxcd/source-controller
if [ -d "source-controller" ]; then
    cd source-controller
    git reset --hard HEAD
    git clean -fd
    cd ..
fi
cd -
```

## 4. Verify AWS Credentials

```bash
aws sts get-caller-identity
```

**Expected**: Should show your AWS account info without errors.

## 5. Verify Bedrock Access

```bash
aws bedrock list-foundation-models --region us-west-2 --query 'modelSummaries[?contains(modelId, `claude-sonnet-4-5`)].modelId' --output text
```

**Expected**: Should show `anthropic.claude-sonnet-4-5-20250929-v1:0`

## 6. Check Quota Status

```bash
aws service-quotas get-service-quota \
    --service-code bedrock \
    --quota-code L-F4DDD3EB \
    --region us-west-2 \
    --query 'Quota.Value' \
    --output text
```

**Expected**: Should show `200000` (200K tokens/min) or higher

## 7. Prepare Test Environment

```bash
# Navigate to test directory
cd test/eks-anywhere-build-tooling

# Ensure we're on the right branch
git status

# Check patch file exists
ls -lh projects/fluxcd/source-controller/patches/0001-*.patch
```

## 8. Review Test Command

```bash
# Basic test command
./tools/version-tracker/bin/version-tracker fix-patches \
    --project fluxcd/source-controller \
    --pr 1234 \
    --max-attempts 3 \
    2>&1 | tee auto-patch-source-controller.log
```

**Or with explicit model**:
```bash
./tools/version-tracker/bin/version-tracker fix-patches \
    --project fluxcd/source-controller \
    --pr 1234 \
    --model anthropic.claude-sonnet-4-5-20250929-v1:0 \
    --max-attempts 3 \
    2>&1 | tee auto-patch-source-controller.log
```

## 9. What to Monitor During Test

### Console Output
Watch for these log messages:
- ✅ `Initialized Bedrock client` with `profile: us.anthropic.claude-sonnet-4-5-20250929-v1:0`
- ✅ `Extracted patch context` with token count
- ✅ `Wrote prompt to debug file` with `/tmp/llm-prompt-attempt-1.txt`
- ✅ `Rate limiting: waiting to respect Bedrock limits`
- ✅ `Bedrock API call succeeded`
- ✅ `Wrote response to debug file` with `/tmp/llm-response-attempt-1.txt`
- ✅ `Generated patch preview`
- ✅ `Saved debug patch file`

### Red Flags
- ❌ Model shows `claude-3-7-sonnet` instead of `claude-sonnet-4-5`
- ❌ `503 ServiceUnavailableException: Too many connections`
- ❌ `No patch found in Bedrock response`
- ❌ `git apply failed`

## 10. Post-Test Verification

### Check Debug Files Were Created
```bash
ls -lh /tmp/llm-prompt-attempt-*.txt
ls -lh /tmp/llm-response-attempt-*.txt
ls -lh test/eks-anywhere-build-tooling/projects/fluxcd/source-controller/.llm-patch-debug.txt
```

### Inspect Prompt File
```bash
cat /tmp/llm-prompt-attempt-1.txt | grep -A 20 "Expected vs Actual File State"
```

**Should show**:
- "What the patch expects to find:" section
- "What's actually in the file:" section  
- "Key differences:" section

### Inspect Response File
```bash
cat /tmp/llm-response-attempt-1.txt | head -50
```

**Should show**:
- Patch headers (From, Date, Subject)
- diff --git lines
- Complete hashes (not truncated)

### Check Generated Patch
```bash
cat test/eks-anywhere-build-tooling/projects/fluxcd/source-controller/.llm-patch-debug.txt
```

**Verify**:
- Has complete patch headers
- go.sum hashes are complete (end with `=`)
- Line numbers match current file state

### Verify Patch Was Applied (if successful)
```bash
cd test/eks-anywhere-build-tooling/projects/fluxcd/source-controller/source-controller
git diff
```

**Should show**: Changes from the fixed patch applied to the repo

## 11. Common Issues and Quick Fixes

### Issue: Still seeing Claude 3.7 in logs
**Fix**: 
```bash
cd tools/version-tracker
make clean
make build
```

### Issue: 503 errors persist
**Fix**: Check if another process is using Bedrock API
```bash
# Wait 60 seconds between tests to ensure rate limit resets
sleep 60
```

### Issue: No debug files created
**Fix**: Check /tmp permissions
```bash
ls -ld /tmp
# Should be drwxrwxrwt
```

### Issue: Prompt doesn't have Expected vs Actual
**Fix**: Verify extractExpectedVsActual is being called
```bash
grep -n "extractExpectedVsActual" tools/version-tracker/pkg/commands/fixpatches/context.go
```

## 12. Quick Verification Script

Save this as `verify-changes.sh`:

```bash
#!/bin/bash

echo "=== Verifying LLM Patch Fixer Changes ==="
echo

echo "1. Checking binary..."
if [ -f "tools/version-tracker/bin/version-tracker" ]; then
    echo "✅ Binary exists"
    ls -lh tools/version-tracker/bin/version-tracker
else
    echo "❌ Binary not found - run 'make build'"
    exit 1
fi

echo
echo "2. Checking default model..."
if grep -q "claude-sonnet-4-5-20250929" tools/version-tracker/cmd/fixpatches.go; then
    echo "✅ Default model is Sonnet 4.5"
else
    echo "❌ Default model not updated"
    exit 1
fi

echo
echo "3. Checking context enhancement..."
if grep -q "ExpectedContext" tools/version-tracker/pkg/types/fixpatches.go; then
    echo "✅ Context enhancement fields added"
else
    echo "❌ Context enhancement missing"
    exit 1
fi

echo
echo "4. Checking extractExpectedVsActual function..."
if grep -q "func extractExpectedVsActual" tools/version-tracker/pkg/commands/fixpatches/context.go; then
    echo "✅ extractExpectedVsActual function exists"
else
    echo "❌ extractExpectedVsActual function missing"
    exit 1
fi

echo
echo "5. Checking debug logging..."
if grep -q "llm-prompt-attempt" tools/version-tracker/pkg/commands/fixpatches/llm.go; then
    echo "✅ Debug logging added"
else
    echo "❌ Debug logging missing"
    exit 1
fi

echo
echo "6. Checking enhanced prompt..."
if grep -q "Expected vs Actual File State" tools/version-tracker/pkg/commands/fixpatches/llm.go; then
    echo "✅ Enhanced prompt section added"
else
    echo "❌ Enhanced prompt section missing"
    exit 1
fi

echo
echo "=== All checks passed! Ready to test. ==="
```

Run it:
```bash
chmod +x verify-changes.sh
./verify-changes.sh
```

## Ready to Test!

Once all checks pass, you're ready to run the test:

```bash
cd test/eks-anywhere-build-tooling
./tools/version-tracker/bin/version-tracker fix-patches \
    --project fluxcd/source-controller \
    --pr 1234 \
    --max-attempts 3 \
    2>&1 | tee auto-patch-source-controller.log
```

Then immediately check:
```bash
# View prompt
cat /tmp/llm-prompt-attempt-1.txt | less

# View response  
cat /tmp/llm-response-attempt-1.txt | less

# View generated patch
cat projects/fluxcd/source-controller/.llm-patch-debug.txt | less
```
