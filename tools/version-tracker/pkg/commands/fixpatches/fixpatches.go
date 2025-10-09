package fixpatches

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/aws/eks-anywhere-build-tooling/tools/version-tracker/pkg/constants"
	"github.com/aws/eks-anywhere-build-tooling/tools/version-tracker/pkg/types"
	"github.com/aws/eks-anywhere-build-tooling/tools/version-tracker/pkg/util/logger"
)

// Run executes the patch fixing workflow, processing each patch file sequentially.
func Run(opts *types.FixPatchesOptions) error {
	logger.Info("Starting patch fixing workflow", "project", opts.ProjectName, "pr", opts.PRNumber)

	// Get current working directory
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting current working directory: %v", err)
	}

	// Extract org and repo from project name
	projectRepo := strings.Split(opts.ProjectName, "/")[1]

	// Construct project path: projects/<org>/<repo>
	projectPath := filepath.Join(cwd, "projects", opts.ProjectName)
	if _, err := os.Stat(projectPath); os.IsNotExist(err) {
		return fmt.Errorf("project directory does not exist: %s", projectPath)
	}

	logger.Info("Project directory located", "path", projectPath)

	// Get sorted list of patch files
	patchesDir := filepath.Join(projectPath, constants.PatchesDirectory)
	patchFiles, err := filepath.Glob(filepath.Join(patchesDir, "*.patch"))
	if err != nil {
		return fmt.Errorf("finding patch files: %v", err)
	}

	if len(patchFiles) == 0 {
		logger.Info("No patch files found - nothing to fix")
		return nil
	}

	// Sort patch files to ensure sequential processing (0001, 0002, 0003...)
	sort.Strings(patchFiles)

	logger.Info("Found patch files", "count", len(patchFiles), "files", patchFiles)

	// Process each patch file sequentially
	for patchIndex, patchFile := range patchFiles {
		logger.Info("Processing patch", "index", patchIndex+1, "total", len(patchFiles), "file", filepath.Base(patchFile))

		// Try to fix this specific patch
		if err := fixSinglePatch(patchFile, projectPath, projectRepo, opts); err != nil {
			return fmt.Errorf("failed to fix patch %s: %v", filepath.Base(patchFile), err)
		}

		logger.Info("Patch processed successfully", "file", filepath.Base(patchFile))
	}

	logger.Info("All patches processed successfully")
	return nil
}

// fixSinglePatch processes a single patch file through the fix-validate cycle.
func fixSinglePatch(patchFile string, projectPath string, projectRepo string, opts *types.FixPatchesOptions) error {
	logger.Info("Fixing single patch", "patch", filepath.Base(patchFile))

	// Apply this specific patch with git apply --reject
	rejFiles, err := applySinglePatchWithReject(patchFile, projectPath, projectRepo)
	if err != nil {
		return fmt.Errorf("applying patch with reject: %v", err)
	}

	// If no .rej files, patch applied successfully
	if len(rejFiles) == 0 {
		logger.Info("Patch applied successfully without conflicts", "patch", filepath.Base(patchFile))
		return nil
	}

	logger.Info("Patch has conflicts", "patch", filepath.Base(patchFile), "rej_files", len(rejFiles))

	// Calculate complexity for this patch
	// TODO(Phase 2): Consider PR-level complexity gating instead of per-patch
	// If any single patch exceeds threshold, skip entire PR for better UX
	// Rationale: Avoid mixed state where some patches fixed, others need manual work
	complexity, err := calculateComplexity(rejFiles)
	if err != nil {
		return fmt.Errorf("calculating complexity: %v", err)
	}

	logger.Info("Calculated patch complexity", "score", complexity, "threshold", opts.ComplexityThreshold)

	// Check if complexity exceeds threshold
	// TODO(Phase 2): Refine complexity calculation based on PoC metrics
	// Current: complexity = hunks + files
	// Consider: weighted scoring based on hunk type, file type, lines changed
	// Track success rates by complexity level to optimize threshold
	if complexity > opts.ComplexityThreshold {
		logger.Info("Complexity exceeds threshold - skipping this patch",
			"complexity", complexity,
			"threshold", opts.ComplexityThreshold)
		return &types.PatchFixError{
			Code:    types.ErrorComplexityTooHigh,
			Message: fmt.Sprintf("Patch %s complexity (%d) exceeds threshold (%d)", filepath.Base(patchFile), complexity, opts.ComplexityThreshold),
			Details: map[string]interface{}{
				"patch":      filepath.Base(patchFile),
				"complexity": complexity,
				"threshold":  opts.ComplexityThreshold,
				"rej_files":  rejFiles,
			},
		}
	}

	// Iterative refinement loop for this patch
	for attempt := 1; attempt <= opts.MaxAttempts; attempt++ {
		logger.Info("Starting fix attempt for patch", "patch", filepath.Base(patchFile), "attempt", attempt, "max_attempts", opts.MaxAttempts)

		// Extract context from .rej files for THIS patch
		ctx, err := ExtractPatchContext(rejFiles, patchFile, projectPath, attempt)
		if err != nil {
			logger.Info("Failed to extract patch context", "error", err, "attempt", attempt)
			if attempt == opts.MaxAttempts {
				return fmt.Errorf("extracting patch context (attempt %d/%d): %v", attempt, opts.MaxAttempts, err)
			}
			continue
		}

		logger.Info("Extracted patch context", "token_count", ctx.TokenCount, "hunks", len(ctx.FailedHunks))

		// Call LLM to generate fix
		fix, err := CallBedrockForPatchFix(ctx, opts.Model, attempt)
		if err != nil {
			logger.Info("Bedrock API call failed", "error", err, "attempt", attempt)
			if attempt == opts.MaxAttempts {
				return &types.PatchFixError{
					Code:    types.ErrorBedrockAPI,
					Message: fmt.Sprintf("Bedrock API failed for patch %s after %d attempts: %v", filepath.Base(patchFile), opts.MaxAttempts, err),
					Details: map[string]interface{}{
						"patch":    filepath.Base(patchFile),
						"attempts": opts.MaxAttempts,
						"error":    err.Error(),
					},
				}
			}
			continue
		}

		logger.Info("LLM generated patch fix", "tokens_used", fix.TokensUsed, "cost", fix.Cost)

		// Apply the patch fix
		if err := ApplyPatchFix(fix, projectPath); err != nil {
			logger.Info("Failed to apply patch fix", "error", err, "attempt", attempt)
			// Revert changes
			if revertErr := RevertPatchFix(projectPath); revertErr != nil {
				logger.Info("Failed to revert patch", "error", revertErr)
			}
			// Store build error for next attempt
			ctx.BuildError = err.Error()
			ctx.PreviousAttempts = append(ctx.PreviousAttempts, fix.Patch)
			continue
		}

		logger.Info("Patch fix applied successfully")

		// Validate build
		if err := ValidateBuild(projectPath); err != nil {
			logger.Info("Build validation failed", "error", err, "attempt", attempt)
			// Revert changes
			if revertErr := RevertPatchFix(projectPath); revertErr != nil {
				logger.Info("Failed to revert patch", "error", revertErr)
			}
			// Store build error for next attempt
			ctx.BuildError = err.Error()
			ctx.PreviousAttempts = append(ctx.PreviousAttempts, fix.Patch)
			if attempt == opts.MaxAttempts {
				return &types.PatchFixError{
					Code:    types.ErrorBuildFailed,
					Message: fmt.Sprintf("Build validation failed for patch %s after %d attempts", filepath.Base(patchFile), opts.MaxAttempts),
					Details: map[string]interface{}{
						"patch":       filepath.Base(patchFile),
						"attempts":    opts.MaxAttempts,
						"build_error": err.Error(),
					},
				}
			}
			continue
		}

		logger.Info("Build validation passed")

		// Validate semantics
		if err := ValidateSemantics(fix, ctx); err != nil {
			logger.Info("Semantic validation failed", "error", err, "attempt", attempt)
			// Revert changes
			if revertErr := RevertPatchFix(projectPath); revertErr != nil {
				logger.Info("Failed to revert patch", "error", revertErr)
			}
			// Store error for next attempt
			ctx.BuildError = err.Error()
			ctx.PreviousAttempts = append(ctx.PreviousAttempts, fix.Patch)
			if attempt == opts.MaxAttempts {
				return &types.PatchFixError{
					Code:    types.ErrorSemanticDrift,
					Message: fmt.Sprintf("Semantic validation failed for patch %s after %d attempts", filepath.Base(patchFile), opts.MaxAttempts),
					Details: map[string]interface{}{
						"patch":    filepath.Base(patchFile),
						"attempts": opts.MaxAttempts,
						"error":    err.Error(),
					},
				}
			}
			continue
		}

		logger.Info("Semantic validation passed")

		// Success! This patch is fixed
		logger.Info("Patch fix successful", "patch", filepath.Base(patchFile), "attempt", attempt, "tokens_used", fix.TokensUsed, "cost", fix.Cost)

		// Clean up .rej files for this patch
		for _, rejFile := range rejFiles {
			os.Remove(rejFile)
		}

		return nil
	}

	// All attempts exhausted for this patch
	return &types.PatchFixError{
		Code:    types.ErrorMaxAttemptsExceeded,
		Message: fmt.Sprintf("Failed to fix patch %s after %d attempts", filepath.Base(patchFile), opts.MaxAttempts),
		Details: map[string]interface{}{
			"patch":    filepath.Base(patchFile),
			"attempts": opts.MaxAttempts,
		},
	}
}

// applyPatches attempts to apply patches using git apply --reject to generate .rej files.
// This function:
// 1. Ensures the upstream repo is checked out (via make checkout-repo)
// 2. Applies patches using git apply --reject to generate .rej files for conflicts
func applyPatches(projectPath string, repoName string) error {
	logger.Info("Checking out upstream repository", "path", projectPath)

	// First, ensure the repo is checked out (but don't apply patches yet)
	checkoutCmd := exec.Command("make", "-C", projectPath, "checkout-repo")
	checkoutOutput, err := checkoutCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("make checkout-repo failed: %v\nOutput: %s", err, checkoutOutput)
	}

	logger.Info("Repository checked out successfully")

	// Find the patches directory
	patchesDir := filepath.Join(projectPath, constants.PatchesDirectory)
	if _, err := os.Stat(patchesDir); os.IsNotExist(err) {
		logger.Info("No patches directory found - nothing to fix")
		return nil
	}

	// Get list of patch files
	patchFiles, err := filepath.Glob(filepath.Join(patchesDir, "*.patch"))
	if err != nil {
		return fmt.Errorf("finding patch files: %v", err)
	}

	if len(patchFiles) == 0 {
		logger.Info("No patch files found in patches directory")
		return nil
	}

	logger.Info("Found patch files", "count", len(patchFiles))

	// The cloned repo directory is named after the repository
	repoPath := filepath.Join(projectPath, repoName)

	// Check if repo was cloned
	if _, err := os.Stat(repoPath); os.IsNotExist(err) {
		return fmt.Errorf("cloned repository not found at %s", repoPath)
	}

	// Configure git in the cloned repo (same as Common.mk does for patch application)
	configEmailCmd := exec.Command("git", "-C", repoPath, "config", "user.email", constants.PatchApplyGitUserEmail)
	if err := configEmailCmd.Run(); err != nil {
		return fmt.Errorf("configuring git user.email: %v", err)
	}

	configNameCmd := exec.Command("git", "-C", repoPath, "config", "user.name", constants.PatchApplyGitUserName)
	if err := configNameCmd.Run(); err != nil {
		return fmt.Errorf("configuring git user.name: %v", err)
	}

	// Apply patches using git apply --reject
	// This will:
	// - Apply successful hunks
	// - Create .rej files for failed hunks
	// - Return error if any hunks fail
	logger.Info("Applying patches with git apply --reject", "repo", repoPath)

	for _, patchFile := range patchFiles {
		logger.Info("Applying patch", "file", filepath.Base(patchFile))

		cmd := exec.Command("git", "-C", repoPath, "apply", "--reject", "--whitespace=fix", patchFile)
		output, err := cmd.CombinedOutput()

		if err != nil {
			// Check if it's a patch conflict (expected) vs other error
			outputStr := string(output)
			if strings.Contains(outputStr, "patch does not apply") ||
				strings.Contains(outputStr, "Rejected hunk") ||
				strings.Contains(outputStr, "does not exist in index") {
				logger.Info("Patch application failed with conflicts (expected)",
					"patch", filepath.Base(patchFile),
					"output", outputStr)
				// Continue to next patch - we want to apply as many as possible
				continue
			}
			return fmt.Errorf("git apply failed for %s: %v\nOutput: %s", patchFile, err, output)
		}

		logger.Info("Patch applied successfully", "file", filepath.Base(patchFile))
	}

	// If we got here, at least one patch had conflicts (which is what we expect)
	// Return an error to signal that .rej files were created
	return fmt.Errorf("patch conflicts detected - .rej files generated")
}

// findRejectionFiles locates all .rej files in the cloned repository directory.
// .rej files are created by git am when patches fail to apply.
func findRejectionFiles(repoPath string) ([]string, error) {
	var rejFiles []string

	// Check if repo directory exists
	if _, err := os.Stat(repoPath); os.IsNotExist(err) {
		return rejFiles, nil // No repo directory means no .rej files
	}

	// Walk through the entire cloned repo to find .rej files
	err := filepath.Walk(repoPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		// Skip .git directory
		if info.IsDir() && info.Name() == ".git" {
			return filepath.SkipDir
		}
		if !info.IsDir() && strings.HasSuffix(info.Name(), ".rej") {
			rejFiles = append(rejFiles, path)
		}
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("walking repository directory: %v", err)
	}

	return rejFiles, nil
}

// calculateComplexity scores patch failure complexity.
// TODO(Phase 2): Refine complexity calculation based on PoC metrics
// Current formula: complexity = total_hunks + num_files
// Future considerations:
// - Weighted scoring: different weights for hunk types (context vs logic changes)
// - File type weights: go.mod (predictable) vs core logic (complex)
// - Lines changed: larger changes = higher complexity
// - Historical success rates: learn optimal weights from data
// Track metrics: success_rate_by_complexity, avg_attempts_by_complexity, cost_by_complexity
func calculateComplexity(rejFiles []string) (int, error) {
	// Complexity is based on number of failed hunks across all .rej files
	totalHunks := 0

	for _, rejFile := range rejFiles {
		content, err := os.ReadFile(rejFile)
		if err != nil {
			return 0, fmt.Errorf("reading rejection file %s: %v", rejFile, err)
		}

		// Count hunks by counting "@@" markers in the .rej file
		hunks := strings.Count(string(content), "@@")
		// Each hunk has 2 @@ markers (start and end), so divide by 2
		if hunks > 0 {
			totalHunks += hunks / 2
		}
	}

	// Complexity score = number of failed hunks + number of affected files
	complexity := totalHunks + len(rejFiles)

	return complexity, nil
}

// applySinglePatchWithReject applies a single patch file and returns any .rej files generated.
func applySinglePatchWithReject(patchFile string, projectPath string, repoName string) ([]string, error) {
	logger.Info("Applying single patch with reject", "patch", filepath.Base(patchFile))

	// Ensure the repo is checked out
	checkoutCmd := exec.Command("make", "-C", projectPath, "checkout-repo")
	checkoutOutput, err := checkoutCmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("make checkout-repo failed: %v\nOutput: %s", err, checkoutOutput)
	}

	logger.Info("Repository checked out successfully")

	// The cloned repo directory
	repoPath := filepath.Join(projectPath, repoName)

	// Check if repo was cloned
	if _, err := os.Stat(repoPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("cloned repository not found at %s", repoPath)
	}

	// Configure git in the cloned repo (same as Common.mk does for patch application)
	configEmailCmd := exec.Command("git", "-C", repoPath, "config", "user.email", constants.PatchApplyGitUserEmail)
	if err := configEmailCmd.Run(); err != nil {
		return nil, fmt.Errorf("configuring git user.email: %v", err)
	}

	configNameCmd := exec.Command("git", "-C", repoPath, "config", "user.name", constants.PatchApplyGitUserName)
	if err := configNameCmd.Run(); err != nil {
		return nil, fmt.Errorf("configuring git user.name: %v", err)
	}

	// Apply this specific patch using git apply --reject
	logger.Info("Applying patch with git apply --reject", "patch", filepath.Base(patchFile))

	cmd := exec.Command("git", "-C", repoPath, "apply", "--reject", "--whitespace=fix", patchFile)
	output, err := cmd.CombinedOutput()

	if err != nil {
		// Check if it's a patch conflict (expected) vs other error
		outputStr := string(output)
		if strings.Contains(outputStr, "patch does not apply") ||
			strings.Contains(outputStr, "Rejected hunk") ||
			strings.Contains(outputStr, "does not exist in index") {
			logger.Info("Patch application failed with conflicts (expected)",
				"patch", filepath.Base(patchFile),
				"output", outputStr)
			// Continue - we'll find the .rej files
		} else {
			return nil, fmt.Errorf("git apply failed for %s: %v\nOutput: %s", patchFile, err, output)
		}
	} else {
		logger.Info("Patch applied successfully without conflicts", "patch", filepath.Base(patchFile))
	}

	// Find .rej files generated for this patch
	rejFiles, err := findRejectionFiles(repoPath)
	if err != nil {
		return nil, fmt.Errorf("finding rejection files: %v", err)
	}

	return rejFiles, nil
}
