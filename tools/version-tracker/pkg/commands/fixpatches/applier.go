package fixpatches

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/aws/eks-anywhere-build-tooling/tools/version-tracker/pkg/types"
	"github.com/aws/eks-anywhere-build-tooling/tools/version-tracker/pkg/util/logger"
)

// ApplyPatchFix applies the LLM-generated patch to files.
func ApplyPatchFix(fix *types.PatchFix, projectPath string) error {
	logger.Info("Applying LLM-generated patch", "path", projectPath)

	// Get the repo directory (e.g., "trivy" from "projects/aquasecurity/trivy")
	repoName := filepath.Base(projectPath)
	repoPath := filepath.Join(projectPath, repoName)

	// Check if repo exists
	if _, err := os.Stat(repoPath); os.IsNotExist(err) {
		return fmt.Errorf("repository not found at %s", repoPath)
	}

	// Save patch to temporary file
	tmpPatchFile := filepath.Join(projectPath, ".llm-patch.tmp")
	if err := os.WriteFile(tmpPatchFile, []byte(fix.Patch), 0644); err != nil {
		return fmt.Errorf("writing temporary patch file: %v", err)
	}
	defer os.Remove(tmpPatchFile) // Clean up temp file

	logger.Info("Saved patch to temporary file", "file", tmpPatchFile)

	// Apply patch using git apply
	// Note: We use git apply instead of git am because we're applying to an already-cloned repo
	cmd := exec.Command("git", "-C", repoPath, "apply", "--whitespace=fix", tmpPatchFile)
	output, err := cmd.CombinedOutput()

	if err != nil {
		outputStr := string(output)
		logger.Info("git apply failed", "error", err, "output", outputStr)
		return fmt.Errorf("git apply failed: %v\nOutput: %s", err, outputStr)
	}

	logger.Info("Patch applied successfully")

	// Stage the changes
	cmd = exec.Command("git", "-C", repoPath, "add", "-A")
	output, err = cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git add failed: %v\nOutput: %s", err, string(output))
	}

	logger.Info("Changes staged successfully")

	return nil
}

// RevertPatchFix reverts a failed patch application.
func RevertPatchFix(projectPath string) error {
	logger.Info("Reverting patch changes", "path", projectPath)

	// Get the repo directory
	repoName := filepath.Base(projectPath)
	repoPath := filepath.Join(projectPath, repoName)

	// Check if repo exists
	if _, err := os.Stat(repoPath); os.IsNotExist(err) {
		return fmt.Errorf("repository not found at %s", repoPath)
	}

	// Reset any staged changes
	cmd := exec.Command("git", "-C", repoPath, "reset", "--hard", "HEAD")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git reset failed: %v\nOutput: %s", err, string(output))
	}

	// Clean any untracked files
	cmd = exec.Command("git", "-C", repoPath, "clean", "-fd")
	output, err = cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git clean failed: %v\nOutput: %s", err, string(output))
	}

	logger.Info("Patch changes reverted successfully")

	return nil
}

// CommitPatchFix commits the successfully applied patch.
func CommitPatchFix(projectPath string, commitMessage string) error {
	logger.Info("Committing patch fix", "path", projectPath, "message", commitMessage)

	// Get the repo directory
	repoName := filepath.Base(projectPath)
	repoPath := filepath.Join(projectPath, repoName)

	// Check if repo exists
	if _, err := os.Stat(repoPath); os.IsNotExist(err) {
		return fmt.Errorf("repository not found at %s", repoPath)
	}

	// Commit the changes
	cmd := exec.Command("git", "-C", repoPath, "commit", "-m", commitMessage)
	output, err := cmd.CombinedOutput()
	if err != nil {
		// Check if there's nothing to commit
		if strings.Contains(string(output), "nothing to commit") {
			logger.Info("No changes to commit")
			return nil
		}
		return fmt.Errorf("git commit failed: %v\nOutput: %s", err, string(output))
	}

	logger.Info("Patch fix committed successfully")

	return nil
}
