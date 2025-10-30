# Ideal Test Candidates for LLM Patch Fixer

Based on analysis of the test repository, here are the best candidates for testing the LLM patch fixer, ordered from simplest to most complex.

## 1. kube-vip/kube-vip (RECOMMENDED - SIMPLEST)

**Why this is ideal:**
- ✅ Single patch file (1.9K)
- ✅ Only 2 Go files modified
- ✅ Clear, straightforward changes (remove code, modify function call)
- ✅ No dependency management (go.mod/go.sum)
- ✅ Easy to verify success/failure

**Patch details:**
- File: `0001-use-hostname-instead-of-kubernetes-to-contact-apiser.patch`
- Changes:
  - `pkg/kubevip/config_generator.go`: Remove 7 lines (HostAlias configuration)
  - `pkg/manager/manager.go`: Replace hardcoded "kubernetes" with `os.Hostname()`
- Total: 14 lines changed

**Test command:**
```bash
../bin/version-tracker fix-patches \
  --project kube-vip/kube-vip \
  --pr <PR_NUMBER> \
  --max-attempts 1 \
  --verbosity 6
```

---

## 2. kubernetes-sigs/kind - Patch 0004 (SIMPLE)

**Why this is good:**
- ✅ Single Go file modified
- ✅ Very small change (3 lines)
- ✅ Simple comment-out operation
- ✅ Clear context and reasoning

**Patch details:**
- File: `0004-Disable-cgroupns-private-to-fix-cluster-creation-on-.patch`
- Changes:
  - `pkg/cluster/internal/providers/docker/provision.go`: Comment out one line
- Total: 3 lines changed

**Test command:**
```bash
../bin/version-tracker fix-patches \
  --project kubernetes-sigs/kind \
  --pr <PR_NUMBER> \
  --max-attempts 1 \
  --verbosity 6
```

---

## 3. kubernetes-sigs/kind - Patch 0003 (MEDIUM)

**Why this is good:**
- ✅ Two files modified (config + Go code)
- ✅ Consistent change across files (maxconn value)
- ✅ Clear business logic (change 100000 to 10000)
- ✅ Good comments explaining the change

**Patch details:**
- File: `0003-Patch-haproxy-maxconn-value-to-avoid-ulimit-issue.patch`
- Changes:
  - `images/haproxy/haproxy.cfg`: Change maxconn from 100000 to 10000
  - `pkg/cluster/internal/loadbalancer/config.go`: Same change with comments
- Total: 10 lines changed

**Test command:**
```bash
../bin/version-tracker fix-patches \
  --project kubernetes-sigs/kind \
  --pr <PR_NUMBER> \
  --max-attempts 1 \
  --verbosity 6
```

---

## Projects to AVOID for Initial Testing

### fluxcd/source-controller
- ❌ Complex go.mod/go.sum whitespace issues
- ❌ Difficult to verify success
- ❌ Not representative of typical patch failures

### Projects with many patches
- ❌ kubernetes-sigs/kind (6 patches, 517 lines) - too complex for initial test
- ❌ Projects with >3 patches - harder to isolate issues

---

## Testing Strategy

### Phase 1: Validate Basic Functionality
1. Start with **kube-vip/kube-vip** (simplest)
2. Verify LLM can handle basic Go code changes
3. Confirm context gathering works correctly

### Phase 2: Test Multiple Files
1. Move to **kind patch 0004** (single file, simple)
2. Then **kind patch 0003** (multiple files, consistent change)
3. Verify LLM handles multi-file patches

### Phase 3: Complex Scenarios
1. Test with larger patches if Phase 1-2 succeed
2. Gradually increase complexity

---

## How to Find PR Numbers

To find open PRs with patch failures for these projects:

```bash
# Check for open PRs mentioning the project
gh pr list --repo aws/eks-anywhere-build-tooling \
  --state open \
  --search "kube-vip OR kind" \
  --json number,title,labels

# Or check our earlier analysis
cat tools/version-tracker/TESTING_SUMMARY.md
```

---

## Success Criteria

For each test:
1. ✅ LLM generates valid patch
2. ✅ Patch applies cleanly to new version
3. ✅ No syntax errors in generated code
4. ✅ Changes match intent of original patch
5. ✅ Process completes within reasonable time (<2 minutes)

---

## Next Steps

1. Identify open PR for kube-vip/kube-vip
2. Run test with max-attempts=1
3. Review generated patch
4. Document results
5. Iterate on prompt if needed
