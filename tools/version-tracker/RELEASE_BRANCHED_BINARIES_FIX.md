# Release-Branched Binaries Support

## Problem

Some projects in the build tooling repo have **release-branched binaries**, where the project structure is organized by Kubernetes release version:

```
projects/kubernetes/autoscaler/
├── 1-28/
│   ├── GIT_TAG
│   ├── patches/
│   │   ├── 0001-*.patch
│   │   └── 0002-*.patch
├── 1-29/
│   ├── GIT_TAG
│   └── patches/
├── 1-34/
│   ├── GIT_TAG
│   └── patches/
└── Makefile
```

This is different from normal projects where patches are at the project root:

```
projects/kubernetes-sigs/kind/
├── GIT_TAG
├── patches/
│   ├── 0001-*.patch
│   └── 0002-*.patch
└── Makefile
```

## Projects Affected

Based on investigation, the following projects have release-branched binaries:

1. ✅ **kubernetes/autoscaler** - HAS PATCHES
   - Patches in: `projects/kubernetes/autoscaler/1-34/patches/`
   - 3 patches per release branch

2. ❌ **kubernetes/cloud-provider-aws** - NO PATCHES
   - Structure exists but no patches directory

3. ❌ **kubernetes/cloud-provider-vsphere** - NO PATCHES
   - Structure exists but no patches directory

4. ❌ **containerd/containerd** - NO PATCHES
   - Structure exists but no patches directory

**Currently, only `kubernetes/autoscaler` needs this fix.**

## How to Identify Release-Branched Binaries

The Makefile variable `BINARIES_ARE_RELEASE_BRANCHED` indicates this:

```makefile
# In projects/kubernetes/autoscaler/Makefile
BINARIES_ARE_RELEASE_BRANCHED=true
```

When this is true:
- GIT_TAG is at: `./$(RELEASE_BRANCH)/GIT_TAG`
- Patches are at: `./$(RELEASE_BRANCH)/patches/`

## The Fix

### 1. Detect Release-Branched Binaries

Added logic to check `BINARIES_ARE_RELEASE_BRANCHED`:

```go
binariesReleaseBranchedCmd := exec.Command("make", "-C", projectPath, "var-value-BINARIES_ARE_RELEASE_BRANCHED")
binariesReleaseBranchedCmd.Env = append(os.Environ(), "RELEASE_BRANCH=dummy")
binariesReleaseBranchedOutput, _ := binariesReleaseBranchedCmd.CombinedOutput()
binariesReleaseBranched := strings.TrimSpace(string(binariesReleaseBranchedOutput)) == "true"
```

### 2. Adjust Patches Directory

```go
var patchesDir string
if binariesReleaseBranched {
    // Get latest release branch
    releaseBranch := getLatestReleaseBranch()
    patchesDir = filepath.Join(projectPath, releaseBranch, "patches")
} else {
    patchesDir = filepath.Join(projectPath, "patches")
}
```

### 3. Adjust GIT_TAG Location

```go
var gitTagPath string
if binariesReleaseBranched {
    releaseBranch := getLatestReleaseBranch()
    gitTagPath = filepath.Join(projectPath, releaseBranch, "GIT_TAG")
} else {
    gitTagPath = filepath.Join(projectPath, "GIT_TAG")
}
```

## Key Differences

### Normal Projects (e.g., kind)
- `HAS_RELEASE_BRANCHES=true` - Build system uses release branches
- `BINARIES_ARE_RELEASE_BRANCHED=false` - Binaries are NOT release-branched
- GIT_TAG: `projects/kubernetes-sigs/kind/GIT_TAG`
- Patches: `projects/kubernetes-sigs/kind/patches/`
- Checkout: Needs `RELEASE_BRANCH=1-34` for build system

### Release-Branched Binaries (e.g., autoscaler)
- `HAS_RELEASE_BRANCHES=true` - Build system uses release branches
- `BINARIES_ARE_RELEASE_BRANCHED=true` - Binaries ARE release-branched
- GIT_TAG: `projects/kubernetes/autoscaler/1-34/GIT_TAG`
- Patches: `projects/kubernetes/autoscaler/1-34/patches/`
- Checkout: Needs `RELEASE_BRANCH=1-34` for build system

## Implementation Details

### Changes in `FixPatches` function

1. Check `BINARIES_ARE_RELEASE_BRANCHED` early
2. Determine patches directory based on project structure
3. Log the detected structure for debugging

### Changes in `applySinglePatchWithReject` function

1. Check `BINARIES_ARE_RELEASE_BRANCHED` to find GIT_TAG
2. Read GIT_TAG from correct location
3. Still check `HAS_RELEASE_BRANCHES` for build system
4. Pass `RELEASE_BRANCH` to checkout command

## Testing

### Test with autoscaler:

```bash
cd test/eks-anywhere-build-tooling
SKIP_VALIDATION=true ../../bin/version-tracker fix-patches \
  --project kubernetes/autoscaler \
  --pr 4858 \
  --max-attempts 3 \
  --verbosity 6
```

Expected behavior:
- ✅ Detects `BINARIES_ARE_RELEASE_BRANCHED=true`
- ✅ Finds patches in `projects/kubernetes/autoscaler/1-34/patches/`
- ✅ Reads GIT_TAG from `projects/kubernetes/autoscaler/1-34/GIT_TAG`
- ✅ Checks out repo with `RELEASE_BRANCH=1-34`
- ✅ Applies and fixes patches

### Test with kind (regression test):

```bash
cd test/eks-anywhere-build-tooling
SKIP_VALIDATION=true ../../bin/version-tracker fix-patches \
  --project kubernetes-sigs/kind \
  --pr 4789 \
  --max-attempts 3 \
  --verbosity 6
```

Expected behavior:
- ✅ Detects `BINARIES_ARE_RELEASE_BRANCHED=false` (or not set)
- ✅ Finds patches in `projects/kubernetes-sigs/kind/patches/`
- ✅ Reads GIT_TAG from `projects/kubernetes-sigs/kind/GIT_TAG`
- ✅ Checks out repo with `RELEASE_BRANCH=1-34`
- ✅ Applies and fixes patches (same as before)

## Edge Cases Handled

1. **Project has both HAS_RELEASE_BRANCHES and BINARIES_ARE_RELEASE_BRANCHED**
   - Use release branch for both GIT_TAG location and checkout

2. **Project has only HAS_RELEASE_BRANCHES (like kind)**
   - GIT_TAG at project root
   - Patches at project root
   - Still pass RELEASE_BRANCH to checkout

3. **Project has neither (like source-controller)**
   - GIT_TAG at project root
   - Patches at project root
   - No RELEASE_BRANCH needed

## Files Modified

- `tools/version-tracker/pkg/commands/fixpatches/fixpatches.go`
  - Added `BINARIES_ARE_RELEASE_BRANCHED` detection in `FixPatches()`
  - Added conditional patches directory logic
  - Added conditional GIT_TAG path logic in `applySinglePatchWithReject()`

## Future Considerations

If more projects adopt release-branched binaries:
- The fix is generic and will work automatically
- Just need to ensure `BINARIES_ARE_RELEASE_BRANCHED=true` in their Makefile
- No code changes needed

## Verification Checklist

- [ ] Test with kubernetes/autoscaler (release-branched binaries)
- [ ] Test with kubernetes-sigs/kind (normal with HAS_RELEASE_BRANCHES)
- [ ] Test with fluxcd/source-controller (normal without HAS_RELEASE_BRANCHES)
- [ ] Verify patches are found in correct location
- [ ] Verify GIT_TAG is read from correct location
- [ ] Verify checkout works with correct RELEASE_BRANCH
- [ ] Verify LLM fixes work correctly
