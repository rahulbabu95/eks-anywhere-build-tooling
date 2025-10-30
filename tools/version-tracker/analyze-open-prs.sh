#!/bin/bash

# Script to analyze current open PRs and suggest test candidates
# Only shows PRs with actual patch application failures (detected via bot comments)

echo "=== Analyzing Open PRs with Patch Failures ==="
echo ""

# Fetch open PRs from eks-distro-pr-bot only
echo "Fetching open PRs from eks-distro-pr-bot..."
OPEN_PRS=$(curl -s "https://api.github.com/repos/aws/eks-anywhere-build-tooling/pulls?state=open&per_page=100")

if [ $? -ne 0 ]; then
  echo "ERROR: Failed to fetch PRs from GitHub"
  exit 1
fi

# Filter to only bot PRs
BOT_PRS=$(echo "$OPEN_PRS" | jq '[.[] | select(.user.login == "eks-distro-pr-bot")]')
BOT_PR_COUNT=$(echo "$BOT_PRS" | jq 'length')

echo "✓ Found $BOT_PR_COUNT open PRs from eks-distro-pr-bot"
echo "Checking for patch application failures..."
echo ""

# Create temp file to store results
TEMP_RESULTS=$(mktemp)

# Parse PR data and check for patch failures
PR_COUNT=$(echo "$BOT_PRS" | jq 'length')
for ((i=0; i<$PR_COUNT; i++)); do
  pr_num=$(echo "$BOT_PRS" | jq -r ".[$i].number")
  title=$(echo "$BOT_PRS" | jq -r ".[$i].title")
  
  # Extract project name from title (e.g., "Bump kubernetes-sigs/cluster-api to...")
  if [[ "$title" =~ Bump[[:space:]]([a-zA-Z0-9_-]+/[a-zA-Z0-9_-]+) ]]; then
    project="${BASH_REMATCH[1]}"
    
    # Check if project exists locally (handle both running from tools/version-tracker and from repo root)
    if [ -d "projects/$project/patches" ]; then
      project_path="projects/$project"
    elif [ -d "../../projects/$project/patches" ]; then
      project_path="../../projects/$project"
    elif [ -d "test/eks-anywhere-build-tooling/projects/$project/patches" ]; then
      project_path="test/eks-anywhere-build-tooling/projects/$project"
    else
      continue
    fi
    
    if [ -d "$project_path/patches" ]; then
      # Check for bot comments indicating patch failure (get first 30 lines of comment)
      has_failure=$(curl -s "https://api.github.com/repos/aws/eks-anywhere-build-tooling/issues/$pr_num/comments" | \
        jq -r '.[] | select(.user.login == "eks-distro-pr-bot" and (.body | contains("Failed patch details"))) | .body' | head -30)
      
      if [ -n "$has_failure" ]; then
        # Extract failure details
        failed_patches=$(echo "$has_failure" | grep -o 'Only [0-9]*/[0-9]*' | head -1 | sed 's/Only //')
        failed_at=$(echo "$has_failure" | grep 'Patch failed at' | head -1 | sed 's/.*Patch failed at //')
        # Extract files between backticks
        failed_files=$(echo "$has_failure" | grep -o '`[^`]*`' | tr '\n' ',' | sed 's/`//g' | sed 's/,$//')
        
        # Get local project complexity
        patch_count=$(find "$project_path/patches" -name "*.patch" 2>/dev/null | wc -l | tr -d ' ')
        total_lines=$(cat "$project_path/patches"/*.patch 2>/dev/null | wc -l | tr -d ' ')
        
        # Categorize complexity
        if [ "$total_lines" -lt 100 ]; then
          complexity="SIMPLE"
        elif [ "$total_lines" -lt 2000 ]; then
          complexity="MEDIUM"
        else
          complexity="HIGH"
        fi
        
        # Store result
        echo "$pr_num|$project|$patch_count|$total_lines|$complexity|$failed_patches|$failed_at|$failed_files" >> "$TEMP_RESULTS"
      fi
    fi
  fi
done

# Display results
if [ ! -s "$TEMP_RESULTS" ]; then
  echo "No open PRs with patch failures found."
  echo ""
  echo "This could mean:"
  echo "  - All current PRs have patches applying successfully"
  echo "  - Bot hasn't commented yet on recent PRs"
  echo "  - No PRs are currently open"
  rm "$TEMP_RESULTS"
  exit 0
fi

echo "Found PRs with patch application failures:"
echo ""
printf "%-6s | %-45s | %8s | %6s | %-8s | %s\n" "PR#" "Project" "Patches" "Lines" "Complex" "Failed"
echo "-------|-----------------------------------------------|----------|--------|----------|------------------"

while IFS='|' read -r pr_num project patch_count total_lines complexity failed_patches failed_at failed_files; do
  printf "%-6s | %-45s | %2d patch | %6d | %-8s | %s\n" "$pr_num" "$project" "$patch_count" "$total_lines" "$complexity" "$failed_patches"
done < "$TEMP_RESULTS"

echo ""
echo "=== Recommended Test Commands ==="
echo ""

# Sort by complexity and show recommendations
echo "SIMPLE projects (good for initial testing):"
grep "SIMPLE" "$TEMP_RESULTS" | while IFS='|' read -r pr_num project patch_count total_lines complexity failed_patches failed_at failed_files; do
  echo "  ./test-patch-fixer.sh $project $pr_num"
  echo "    # Failed: $failed_patches patches, Files: $failed_files"
done | head -6

echo ""
echo "MEDIUM projects (for comprehensive testing):"
grep "MEDIUM" "$TEMP_RESULTS" | while IFS='|' read -r pr_num project patch_count total_lines complexity failed_patches failed_at failed_files; do
  echo "  ./test-patch-fixer.sh $project $pr_num"
  echo "    # Failed: $failed_patches patches, Files: $failed_files"
done | head -6

echo ""
echo "HIGH complexity projects (stress testing):"
grep "HIGH" "$TEMP_RESULTS" | while IFS='|' read -r pr_num project patch_count total_lines complexity failed_patches failed_at failed_files; do
  echo "  ./test-patch-fixer.sh $project $pr_num"
  echo "    # Failed: $failed_patches patches, Files: $failed_files"
done | head -6

echo ""
echo "=== Detailed Failure Information ==="
echo ""
while IFS='|' read -r pr_num project patch_count total_lines complexity failed_patches failed_at failed_files; do
  echo "PR #$pr_num - $project ($complexity)"
  echo "  Total patches: $patch_count ($total_lines lines)"
  echo "  Failed: $failed_patches"
  echo "  Failed at: $failed_at"
  echo "  Files: $failed_files"
  echo "  URL: https://github.com/aws/eks-anywhere-build-tooling/pull/$pr_num"
  echo ""
done < "$TEMP_RESULTS"

# Cleanup
rm "$TEMP_RESULTS"
