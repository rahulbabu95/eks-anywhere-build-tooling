# Fix: git apply --reject Not Generating .rej Files

## Problem

When running `fix-patches`, the `.rej` files were not being generated, and the command was failing with:

```
error: projects/fluxcd/source-controller/source-controller/go.mod: No such file or directory
error: projects/fluxcd/source-controller/source-controller/go.sum: No such file or directory
```

Then later:
```
Warning: failed to parse rejection file
error: open /path/to/source-controller/go.mod.rej: no such file or directory
```

## Root Cause

The issue was with how we were passing the patch file path to `git apply`. 

When running:
```bash
git -C /path/to/cloned/repo apply --reject /relative/path/to/patch.patch
```

Git was interpreting the relative patch file path incorrectly, causing it to look for files in the wrong location.

## Solution

Convert the patch file path to an absolute path before passing it to `git apply`:

**Before:**
```go
cmd := exec.Command("git", "-C", repoPath, "apply", "--reject", "--whitespace=fix", patchFile)
```

**After:**
```go
// Need to use absolute path for patch file since we're running git from the repo directory
absPatchFile, err := filepath.Abs(patchFile)
if err != nil {
	return nil, fmt.Errorf("getting absolute path for patch file: %v", err)
}

cmd := exec.Command("git", "-C", repoPath, "apply", "--reject", "--whitespace=fix", absPatchFile)
```

## Why This Matters

When using `git -C <directory>`, Git changes to that directory before executing the command. If you pass a relative path to a patch file, Git will try to resolve it relative to the new directory, not the original working directory.

### Example

If you're in `/Users/user/eks-anywhere-build-tooling` and run:
```bash
git -C projects/fluxcd/source-controller/source-controller \
    apply --reject \
    projects/fluxcd/source-controller/patches/0001-foo.patch
```

Git will:
1. Change to `projects/fluxcd/source-controller/source-controller`
2. Try to find the patch at `projects/fluxcd/source-controller/patches/0001-foo.patch` **relative to that directory**
3. Fail because the path doesn't exist from there

### Solution

Use an absolute path:
```bash
git -C projects/fluxcd/source-controller/source-controller \
    apply --reject \
    /Users/user/eks-anywhere-build-tooling/projects/fluxcd/source-controller/patches/0001-foo.patch
```

Now Git can find the patch file regardless of which directory it's running from.

## Expected Behavior After Fix

1. `git apply --reject` runs successfully
2. When patches fail to apply, `.rej` files are created in the cloned repo
3. The `.rej` files contain the failed hunks
4. These `.rej` files are found and parsed for LLM context
5. LLM generates fixes based on the `.rej` file content

## Files Changed

- `tools/version-tracker/pkg/commands/fixpatches/fixpatches.go`
  - Updated `applySinglePatchWithReject()` to use absolute patch file paths
  - Added logging to show repo path and patch path for debugging

## Testing

After this fix, you should see in the logs:
```
Applying patch with git apply --reject
  patch=0001-Replace-timestamp-authority...patch
  repo_path=/path/to/cloned/repo
  patch_path=/absolute/path/to/patch.patch
```

And if the patch fails to apply:
```
Patch application failed with conflicts (expected)
  output=Rejected hunk #1.
```

Then `.rej` files should be created and found:
```
Found rejection files count=2
```
