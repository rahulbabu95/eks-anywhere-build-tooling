package fixpatches

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/aws/eks-anywhere-build-tooling/tools/version-tracker/pkg/types"
	"github.com/aws/eks-anywhere-build-tooling/tools/version-tracker/pkg/util/logger"
)

// ValidateBuild runs make build and make checksums.
func ValidateBuild(projectPath string) error {
	// Check if SKIP_VALIDATION env var is set (for testing)
	if os.Getenv("SKIP_VALIDATION") == "true" {
		logger.Info("Skipping build validation (SKIP_VALIDATION=true)")
		return nil
	}

	logger.Info("Running build validation", "path", projectPath)

	// Run make build
	buildCmd := exec.Command("make", "-C", projectPath, "build")
	buildOutput, err := buildCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("build failed: %v\nOutput: %s", err, string(buildOutput))
	}

	logger.Info("Build succeeded")

	// Run make checksums
	checksumCmd := exec.Command("make", "-C", projectPath, "checksums")
	checksumOutput, err := checksumCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("checksums failed: %v\nOutput: %s", err, string(checksumOutput))
	}

	logger.Info("Checksums validation passed")

	return nil
}

// ValidateSemantics checks if fix preserves original intent.
func ValidateSemantics(fix *types.PatchFix, ctx *types.PatchContext) error {
	logger.Info("Running semantic validation")

	// Validate patch metadata is preserved
	if ctx.PatchAuthor != "" && !strings.Contains(fix.Patch, ctx.PatchAuthor) {
		logger.Info("Warning: patch author not preserved", "expected", ctx.PatchAuthor)
		// Don't fail - this is a warning
	}

	if ctx.PatchDate != "" && !strings.Contains(fix.Patch, ctx.PatchDate) {
		logger.Info("Warning: patch date not preserved", "expected", ctx.PatchDate)
	}

	if ctx.PatchSubject != "" {
		subjectCore := strings.TrimPrefix(ctx.PatchSubject, "[PATCH]")
		subjectCore = strings.TrimSpace(subjectCore)
		if !strings.Contains(fix.Patch, subjectCore) {
			logger.Info("Warning: patch subject not preserved", "expected", subjectCore)
		}
	}

	// Count lines changed in original patch
	originalLines := countChangedLines(ctx.OriginalPatch)
	fixLines := countChangedLines(fix.Patch)

	// Check for excessive drift (>50% more changes)
	if fixLines > originalLines*3/2 {
		return fmt.Errorf("semantic drift: fix changes %d lines vs %d in original (>50%% increase)",
			fixLines, originalLines)
	}

	logger.Info("Semantic validation passed", "original_lines", originalLines, "fix_lines", fixLines)

	return nil
}

// countChangedLines counts the number of changed lines in a patch (+ and - lines).
func countChangedLines(patch string) int {
	lines := strings.Split(patch, "\n")
	count := 0
	for _, line := range lines {
		if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
			count++
		}
		if strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---") {
			count++
		}
	}
	return count
}
