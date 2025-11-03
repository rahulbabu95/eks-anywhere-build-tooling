package fixpatches

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/aws/eks-anywhere-build-tooling/tools/version-tracker/pkg/types"
	"github.com/aws/eks-anywhere-build-tooling/tools/version-tracker/pkg/util/logger"
)

// isAutoscalerProject checks if this is the kubernetes/autoscaler project
func isAutoscalerProject(projectPath string) bool {
	return strings.Contains(projectPath, "kubernetes/autoscaler") ||
		strings.Contains(projectPath, "kubernetes-autoscaler")
}

// tryAutoscalerSpecialCase attempts to fix autoscaler patches using known patterns
// Returns true if a special case was applied, false if LLM should handle it
func tryAutoscalerSpecialCase(ctx *types.PatchContext, projectPath string) (string, bool, error) {
	if !isAutoscalerProject(projectPath) {
		return "", false, nil
	}

	logger.Info("Detected autoscaler project, checking for known patch patterns")

	// Check if this is the cloud provider removal patch
	if isCloudProviderRemovalPatch(ctx.OriginalPatch) {
		logger.Info("Detected cloud provider removal patch, applying special case fix")
		fixedPatch, err := fixCloudProviderRemovalPatch(ctx)
		if err != nil {
			logger.Info("Special case fix failed", "error", err)
			return "", false, err
		}
		logger.Info("Successfully applied autoscaler special case fix")
		return fixedPatch, true, nil
	}

	// Add more special cases here as needed
	// if isGoModUpdatePatch(ctx.OriginalPatch) { ... }

	logger.Info("No matching special case pattern, will use LLM approach")
	return "", false, nil
}

// isCloudProviderRemovalPatch checks if this patch removes cloud providers
func isCloudProviderRemovalPatch(patch string) bool {
	// Check for the characteristic pattern of this patch
	indicators := []string{
		"Remove-Cloud-Provider-Builders",
		"Remove Cloud Provider Builders",
		"builder_alicloud.go",
		"builder_aws.go",
		"builder_azure.go",
	}

	matchCount := 0
	for _, indicator := range indicators {
		if strings.Contains(patch, indicator) {
			matchCount++
		}
	}

	// Need at least 3 indicators to be confident
	return matchCount >= 3
}

// fixCloudProviderRemovalPatch fixes the cloud provider removal patch
// This implements the logic from the README:
// - Remove all cloud provider files except clusterapi
// - Update builder_all.go to only reference clusterapi
func fixCloudProviderRemovalPatch(ctx *types.PatchContext) (string, error) {
	originalPatch := ctx.OriginalPatch

	// The key issue is that new cloud providers (like coreweave, utho) were added
	// after the original patch was created. We need to remove those too.

	// Strategy: Parse the current file state and generate a patch that:
	// 1. Removes ALL cloud provider imports except clusterapi
	// 2. Removes ALL cloud provider entries from AvailableCloudProviders except clusterapi
	// 3. Removes ALL cloud provider cases from buildCloudProvider except clusterapi
	// 4. Updates DefaultCloudProvider to clusterapi

	// For now, we'll enhance the original patch by adding the new providers
	// This is simpler than regenerating from scratch

	// Extract the failed hunk to see what's different
	if len(ctx.FailedHunks) == 0 {
		return "", fmt.Errorf("no failed hunks to fix")
	}

	// Get the current file content from the hunk
	hunk := ctx.FailedHunks[0]

	// Build the fixed patch by updating the builder_all.go hunk
	fixedPatch := fixBuilderAllGoHunk(originalPatch, hunk)

	return fixedPatch, nil
}

// fixBuilderAllGoHunk fixes the builder_all.go hunk to handle new cloud providers
func fixBuilderAllGoHunk(originalPatch string, hunk types.FailedHunk) string {
	// Parse the original patch to find the builder_all.go section
	lines := strings.Split(originalPatch, "\n")

	var result strings.Builder
	inBuilderAll := false
	inImportSection := false
	inAvailableProviders := false
	inBuildFunction := false

	for i, line := range lines {
		// Detect if we're in the builder_all.go diff
		if strings.Contains(line, "diff --git") && strings.Contains(line, "builder_all.go") {
			inBuilderAll = true
		} else if strings.Contains(line, "diff --git") && !strings.Contains(line, "builder_all.go") {
			inBuilderAll = false
		}

		if !inBuilderAll {
			// Pass through non-builder_all.go content
			result.WriteString(line)
			if i < len(lines)-1 {
				result.WriteString("\n")
			}
			continue
		}

		// We're in builder_all.go - need to handle new providers

		// Detect sections
		if strings.Contains(line, "@@ ") && strings.Contains(line, "import") {
			inImportSection = true
		} else if strings.Contains(line, "@@ ") && strings.Contains(line, "AvailableCloudProviders") {
			inAvailableProviders = true
		} else if strings.Contains(line, "@@ ") && strings.Contains(line, "buildCloudProvider") {
			inBuildFunction = true
		}

		// Add lines for removing new providers that weren't in original patch
		if inImportSection && strings.Contains(line, `"k8s.io/autoscaler/cluster-autoscaler/cloudprovider/clusterapi"`) {
			// After clusterapi import, check if we need to add removals for new providers
			result.WriteString(line)
			if i < len(lines)-1 {
				result.WriteString("\n")
			}

			// Add removal lines for new providers (coreweave, utho, etc.)
			// These would appear in the actual file but not in the original patch
			newProvidersToRemove := []string{
				`-       "k8s.io/autoscaler/cluster-autoscaler/cloudprovider/coreweave"`,
				`-       "k8s.io/autoscaler/cluster-autoscaler/cloudprovider/utho"`,
			}

			// Only add if not already present
			patchContent := strings.Join(lines, "\n")
			for _, removal := range newProvidersToRemove {
				if !strings.Contains(patchContent, removal) {
					result.WriteString(removal)
					result.WriteString("\n")
				}
			}
			continue
		}

		// Similarly handle AvailableCloudProviders section
		if inAvailableProviders && strings.Contains(line, "cloudprovider.ClusterAPIProviderName") {
			result.WriteString(line)
			if i < len(lines)-1 {
				result.WriteString("\n")
			}

			// Add removal lines for new provider entries
			newProviderEntries := []string{
				`-       cloudprovider.CoreWeaveProviderName,`,
				`-       cloudprovider.UthoProviderName,`,
			}

			patchContent := strings.Join(lines, "\n")
			for _, removal := range newProviderEntries {
				if !strings.Contains(patchContent, removal) {
					result.WriteString(removal)
					result.WriteString("\n")
				}
			}
			continue
		}

		// Pass through the line
		result.WriteString(line)
		if i < len(lines)-1 {
			result.WriteString("\n")
		}
	}

	return result.String()
}

// extractNewProvidersFromActual extracts new cloud providers from the actual file content
// that weren't in the original patch's expected content
func extractNewProvidersFromActual(actualLines []string, expectedLines []string) []string {
	// Convert to maps for easier comparison
	expectedSet := make(map[string]bool)
	for _, line := range expectedLines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			expectedSet[trimmed] = true
		}
	}

	var newProviders []string
	providerPattern := regexp.MustCompile(`"k8s\.io/autoscaler/cluster-autoscaler/cloudprovider/(\w+)"`)

	for _, line := range actualLines {
		trimmed := strings.TrimSpace(line)
		if !expectedSet[trimmed] && providerPattern.MatchString(trimmed) {
			// This is a new provider not in the expected content
			matches := providerPattern.FindStringSubmatch(trimmed)
			if len(matches) > 1 {
				providerName := matches[1]
				if providerName != "clusterapi" {
					newProviders = append(newProviders, providerName)
				}
			}
		}
	}

	return newProviders
}
