# Manual Testing Summary

## Clarifications on Testing Behavior

### 1. Dry-Run / Local-Only Behavior

**Q: Will changes be pushed to GitHub automatically?**

**A: NO.** All changes remain local:
- The tool modifies patch files in `projects/<org>/<repo>/patches/` directory
- Changes stay in your local git working directory (uncommitted)
- You review with `git diff` before deciding to commit/push
- No automatic PR creation or GitHub interaction
- Safe to test freely with your personal AWS account

### 2. Test Data Accuracy

**Q: Are the example PRs in the guide real?**

**A: Updated with real data.** The guide now includes:
- Actual project complexity analysis (43 patches, 35K lines for cluster-api)
- Real open PRs from the repository (as of Jan 2025)
- Helper script `analyze-open-prs.sh` to fetch current PRs
- Accurate complexity categorization based on actual patch files

### 3. Complexity Calculation

**Q: How is complexity computed?**

**A: Based on actual patch analysis:**

```
Complexity = Number of failed hunks + Number of .rej files
```

**Real project data:**
- **SIMPLE**: 1 patch, <100 lines
  - cluster-api-provider-vsphere: 1 patch, 30 lines
  - fluxcd/source-controller: 1 patch, 44 lines
  
- **MEDIUM**: 2-6 patches, 200-2000 lines
  - emissary-ingress/emissary: 2 patches, 7,090 lines
  - kubernetes-sigs/kind: 6 patches, 517 lines
  
- **HIGH**: 15+ patches or 2000+ lines
  - kubernetes-sigs/cluster-api: 43 patches, 35,244 lines
  - goharbor/harbor: 6 patches, 209,792 lines (!)

**Note**: The original example claiming cluster-api was "low complexity" was incorrect. It's actually one of the most complex projects with 43 patches and 35K lines.

## Updated Testing Workflow

### Step 1: Find Current Test Candidates (IMPORTANT!)

```bash
cd tools/version-tracker
./analyze-open-prs.sh
```

This script:
- Fetches open PRs from `eks-distro-pr-bot` only
- Checks for bot comments indicating patch failures
- Shows ONLY PRs with actual patch application failures
- Provides ready-to-use test commands

**Why this matters**: Only PRs with patch failures are eligible for LLM fixing. Testing PRs without failures wastes time and AWS credits.

### Step 2: Test with Simple Project

```bash
# Use output from analyze-open-prs.sh
git checkout -b test-patch-fixer
git fetch origin pull/<PR_NUMBER>/head:test-pr
git checkout test-pr

SKIP_VALIDATION=true ./version-tracker fix-patches \
  --project <project-name> \
  --pr <PR_NUMBER> \
  --max-attempts 1 \
  --verbosity 6

# Review (all local)
git diff projects/<project-name>/patches/

# Clean up
git reset --hard HEAD
```

### Step 3: Collect Metrics

Track these for each test:
- PR number and project
- Patch count and total lines
- Complexity score calculated by tool
- Success/failure
- Attempts needed
- Duration
- Cost and token usage

## Helper Scripts

1. **analyze-open-prs.sh** - Fetches current open PRs and suggests test candidates
2. **test-patch-fixer.sh** - Automated testing script (update TEST_CASES with current PRs)
3. **find-test-candidates.sh** - Analyzes all local projects by complexity

## Recommended Test Sequence

1. **Simple projects first** (1 patch, <100 lines)
   - Fast feedback on core LLM functionality
   - Low cost per test (~$0.50-$1.00)
   - Quick iteration on prompts

2. **Medium projects** (2-6 patches, 200-2000 lines)
   - Test multi-patch handling
   - Validate sequential processing
   - Measure success rates

3. **Complex projects last** (15+ patches or 2000+ lines)
   - Stress test token limits
   - Test Lambda timeout handling (20 min)
   - May require prompt optimization first

## Cost Estimates

Based on Claude Sonnet 4.5 pricing:
- **Simple patch** (30 lines): ~$0.50-$1.00 per attempt
- **Medium patch** (500 lines): ~$2.00-$4.00 per attempt
- **Complex patch** (2000+ lines): ~$8.00-$15.00 per attempt

**Budget recommendation**: Start with $50-100 for initial testing (50-100 simple tests or 10-20 medium tests)

## Next Steps After Testing

1. Analyze success rates by complexity
2. Refine complexity threshold based on data
3. Optimize prompts for common failure patterns
4. Adjust token limits if needed
5. Move to Lambda deployment once validated
