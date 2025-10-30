# Fix: checkout-repo vs GIT_CHECKOUT_TARGET

## Problem

When running `fix-patches`, the command was failing with:

```
Error fixing patches: failed to fix patch 0001-Replace-timestamp-authority-and-go-fuzz-headers-revi.patch: 
applying patch with reject: make checkout-repo failed: exit status 2
```

## Root Cause

The code was calling `make checkout-repo`, which has this dependency chain:

```makefile
checkout-repo: $(if $(PATCHES_DIR),$(GIT_PATCH_TARGET),$(GIT_CHECKOUT_TARGET))
```

When `PATCHES_DIR` exists (which it always does for projects with patches), `checkout-repo` depends on `GIT_PATCH_TARGET`, which:

1. Checks out the repo at the specified tag (`GIT_CHECKOUT_TARGET`)
2. **Applies all patches** using `git am`

```makefile
$(GIT_PATCH_TARGET): $(GIT_CHECKOUT_TARGET)
	git -C $(REPO) config user.email prow@amazonaws.com
	git -C $(REPO) config user.name "Prow Bot"
	if [ -n "$(PATCHES_DIR)" ]; then git -C $(REPO) am --committer-date-is-author-date $(PATCHES_DIR)/*; fi
```

This fails when patches don't apply cleanly (which is exactly the case we're trying to fix!).

## Solution

Instead of calling `checkout-repo`, we now call the `GIT_CHECKOUT_TARGET` directly:

```go
// Get the GIT_TAG from the project's Makefile
gitTagCmd := exec.Command("make", "-C", projectPath, "var-value-GIT_TAG")
gitTagOutput, err := gitTagCmd.CombinedOutput()
gitTag := strings.TrimSpace(string(gitTagOutput))

// Build the GIT_CHECKOUT_TARGET: $(REPO)/eks-anywhere-checkout-$(GIT_TAG)
checkoutTarget := fmt.Sprintf("%s/eks-anywhere-checkout-%s", repoName, gitTag)

// Ensure the repo is checked out (but don't apply patches)
checkoutCmd := exec.Command("make", "-C", projectPath, checkoutTarget)
```

This:
1. Clones the repo (if not already cloned)
2. Checks out the specified tag
3. **Does NOT apply patches**
4. Creates a marker file `$(REPO)/eks-anywhere-checkout-$(GIT_TAG)`

## What GIT_CHECKOUT_TARGET Does

From `Common.mk`:

```makefile
$(GIT_CHECKOUT_TARGET): | $(REPO)
	@rm -f $(REPO)/eks-anywhere-*
	(cd $(REPO) && $(BASE_DIRECTORY)/build/lib/wait_for_tag.sh $(GIT_TAG))
	git -C $(REPO) checkout --quiet -f $(GIT_TAG)
	@touch $@
```

It simply:
- Ensures the repo is cloned
- Waits for the tag to be available
- Checks out the tag
- Creates a marker file

## Why This Matters

For the LLM patch fixer workflow:

1. We need the repo at the correct tag
2. We need to apply patches **ourselves** with `git apply --reject`
3. The `--reject` flag generates `.rej` files for conflicts
4. These `.rej` files are what we send to the LLM for fixing

If we let `checkout-repo` apply the patches with `git am`, it will fail before we can generate `.rej` files.

## Files Changed

- `tools/version-tracker/pkg/commands/fixpatches/fixpatches.go`
  - Updated `applyPatches()` function
  - Updated `applySinglePatchWithReject()` function
  - Both now use `GIT_CHECKOUT_TARGET` instead of `checkout-repo`

## Testing

After this fix, the command should successfully:
1. Clone and checkout the repo
2. Apply patches with `--reject` to generate `.rej` files
3. Send `.rej` files to LLM for fixing
4. Apply LLM-generated fixes

Test with:
```bash
cd tools/version-tracker
./version-tracker fix-patches \
  --project fluxcd/source-controller \
  --pr 4883 \
  --max-attempts 1 \
  --verbosity 6
```
