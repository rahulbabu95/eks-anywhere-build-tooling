package fixpatches

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"

	"github.com/aws/eks-anywhere-build-tooling/tools/version-tracker/pkg/types"
	"github.com/aws/eks-anywhere-build-tooling/tools/version-tracker/pkg/util/logger"
)

// BedrockResponse represents the response from Bedrock API.
type BedrockResponse struct {
	Content []struct {
		Text string `json:"text"`
	} `json:"content"`
	Usage struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

// CallBedrockForPatchFix invokes Bedrock with patch context to generate a fix.
func CallBedrockForPatchFix(ctx *types.PatchContext, model string, attempt int) (*types.PatchFix, error) {
	logger.Info("Calling Bedrock API", "model", model, "attempt", attempt)

	// Load AWS config (uses default credential chain)
	cfg, err := config.LoadDefaultConfig(context.Background())
	if err != nil {
		return nil, fmt.Errorf("loading AWS config: %v", err)
	}

	client := bedrockruntime.NewFromConfig(cfg)

	// Build the prompt
	prompt := BuildPrompt(ctx, attempt)

	logger.Info("Prompt built", "length", len(prompt), "estimated_tokens", len(prompt)/4)

	// Construct Bedrock request for Claude
	systemPrompt := `You are an expert at resolving Git patch conflicts. Your task is to fix failed patch hunks by analyzing the original intent and the current code state.

Rules:
1. Preserve the original patch intent exactly
2. Preserve the original patch metadata (From, Date, Subject) exactly
3. Only modify the diff content to resolve the conflict
4. Maintain code style and formatting
5. Output ONLY the corrected patch in unified diff format with complete headers
6. Do not add explanations or commentary`

	requestBody := map[string]interface{}{
		"anthropic_version": "bedrock-2023-05-31",
		"max_tokens":        4096,
		"messages": []map[string]string{
			{
				"role":    "user",
				"content": prompt,
			},
		},
		"system": systemPrompt,
	}

	requestBodyBytes, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("marshaling request body: %v", err)
	}

	// Invoke model with retry logic
	var response *bedrockruntime.InvokeModelOutput
	maxRetries := 3
	for i := 0; i < maxRetries; i++ {
		response, err = client.InvokeModel(context.Background(), &bedrockruntime.InvokeModelInput{
			ModelId:     aws.String(model),
			ContentType: aws.String("application/json"),
			Body:        requestBodyBytes,
		})

		if err == nil {
			break
		}

		logger.Info("Bedrock API call failed, retrying", "attempt", i+1, "error", err)
		if i < maxRetries-1 {
			time.Sleep(time.Second * time.Duration(i+1)) // Exponential backoff
		}
	}

	if err != nil {
		return nil, fmt.Errorf("invoking Bedrock after %d retries: %v", maxRetries, err)
	}

	// Parse response
	var result BedrockResponse
	if err := json.Unmarshal(response.Body, &result); err != nil {
		return nil, fmt.Errorf("unmarshaling Bedrock response: %v", err)
	}

	if len(result.Content) == 0 {
		return nil, fmt.Errorf("empty response from Bedrock")
	}

	responseText := result.Content[0].Text
	logger.Info("Received response from Bedrock",
		"response_length", len(responseText),
		"input_tokens", result.Usage.InputTokens,
		"output_tokens", result.Usage.OutputTokens)

	// Extract patch from response
	patch := extractPatchFromResponse(responseText)
	if patch == "" {
		return nil, fmt.Errorf("no patch found in Bedrock response")
	}

	// Validate patch format and metadata
	if err := validatePatchFormat(patch, ctx); err != nil {
		return nil, fmt.Errorf("invalid patch format: %v", err)
	}

	// Calculate cost (Claude Sonnet 4.5 pricing)
	// Input: $0.003 per 1K tokens, Output: $0.015 per 1K tokens
	inputCost := float64(result.Usage.InputTokens) / 1000.0 * 0.003
	outputCost := float64(result.Usage.OutputTokens) / 1000.0 * 0.015
	totalCost := inputCost + outputCost

	logger.Info("Bedrock API call complete",
		"input_cost", fmt.Sprintf("$%.4f", inputCost),
		"output_cost", fmt.Sprintf("$%.4f", outputCost),
		"total_cost", fmt.Sprintf("$%.4f", totalCost))

	return &types.PatchFix{
		Patch:      patch,
		TokensUsed: result.Usage.InputTokens + result.Usage.OutputTokens,
		Cost:       totalCost,
	}, nil
}

// extractPatchFromResponse extracts the patch content from LLM response.
// The LLM might wrap the patch in markdown code blocks or add explanations.
func extractPatchFromResponse(response string) string {
	// Look for patch content between ```diff or ``` markers
	if strings.Contains(response, "```") {
		// Extract content between code blocks
		parts := strings.Split(response, "```")
		for i, part := range parts {
			// Skip the first part (before first ```)
			if i == 0 {
				continue
			}
			// Remove language identifier if present (e.g., "diff\n")
			part = strings.TrimPrefix(part, "diff\n")
			part = strings.TrimPrefix(part, "diff ")
			part = strings.TrimSpace(part)

			// Check if this looks like a patch (starts with From or diff)
			if strings.HasPrefix(part, "From ") || strings.HasPrefix(part, "diff --git") {
				return part
			}
		}
	}

	// If no code blocks, look for patch markers
	lines := strings.Split(response, "\n")
	var patchLines []string
	inPatch := false

	for _, line := range lines {
		// Start of patch
		if strings.HasPrefix(line, "From ") || strings.HasPrefix(line, "diff --git") {
			inPatch = true
		}

		if inPatch {
			patchLines = append(patchLines, line)
		}
	}

	if len(patchLines) > 0 {
		return strings.Join(patchLines, "\n")
	}

	// Fallback: return the whole response if it looks like a patch
	if strings.Contains(response, "diff --git") || strings.Contains(response, "From ") {
		return strings.TrimSpace(response)
	}

	return ""
}

// BuildPrompt constructs the LLM prompt from context.
func BuildPrompt(ctx *types.PatchContext, attempt int) string {
	var prompt strings.Builder

	// Project information
	prompt.WriteString(fmt.Sprintf("## Project: %s\n\n", ctx.ProjectName))

	// Original patch metadata
	prompt.WriteString("## Original Patch Metadata\n")
	if ctx.PatchAuthor != "" {
		prompt.WriteString(fmt.Sprintf("From: %s\n", ctx.PatchAuthor))
	}
	if ctx.PatchDate != "" {
		prompt.WriteString(fmt.Sprintf("Date: %s\n", ctx.PatchDate))
	}
	if ctx.PatchSubject != "" {
		prompt.WriteString(fmt.Sprintf("Subject: %s\n", ctx.PatchSubject))
	}
	prompt.WriteString("\n")

	// Original patch intent
	if ctx.PatchIntent != "" {
		prompt.WriteString("## Original Patch Intent\n")
		prompt.WriteString(fmt.Sprintf("%s\n\n", ctx.PatchIntent))
	}

	// Failed hunks
	for i, hunk := range ctx.FailedHunks {
		prompt.WriteString(fmt.Sprintf("## Failed Hunk #%d in %s\n\n", hunk.HunkIndex, hunk.FilePath))

		// What the patch tried to do
		prompt.WriteString("### What the patch tried to do:\n")
		prompt.WriteString("```diff\n")
		for _, line := range hunk.OriginalLines {
			prompt.WriteString(line + "\n")
		}
		prompt.WriteString("```\n\n")

		// Current state of the file
		prompt.WriteString(fmt.Sprintf("### Current state of the file (around line %d):\n", hunk.LineNumber))
		prompt.WriteString("```\n")
		prompt.WriteString(hunk.Context)
		prompt.WriteString("\n```\n\n")

		// Why it failed
		prompt.WriteString("### Why it failed:\n")
		prompt.WriteString("The patch does not apply cleanly to the current file state. ")
		prompt.WriteString("The code structure or content has changed in the new version.\n\n")

		// Add separator between hunks
		if i < len(ctx.FailedHunks)-1 {
			prompt.WriteString("---\n\n")
		}
	}

	// Previous attempts (if any)
	if attempt > 1 && len(ctx.PreviousAttempts) > 0 {
		prompt.WriteString(fmt.Sprintf("## Previous Attempt #%d\n", attempt-1))
		prompt.WriteString("You tried this fix, but it failed validation:\n")
		prompt.WriteString("```diff\n")
		prompt.WriteString(ctx.PreviousAttempts[len(ctx.PreviousAttempts)-1])
		prompt.WriteString("\n```\n\n")

		if ctx.BuildError != "" {
			prompt.WriteString("Build error:\n")
			prompt.WriteString("```\n")
			// Limit build error to last 500 lines
			errorLines := strings.Split(ctx.BuildError, "\n")
			if len(errorLines) > 500 {
				prompt.WriteString("...(truncated)...\n")
				errorLines = errorLines[len(errorLines)-500:]
			}
			prompt.WriteString(strings.Join(errorLines, "\n"))
			prompt.WriteString("\n```\n\n")
		}
	}

	// Reflection for later attempts
	if attempt >= 3 {
		prompt.WriteString("## Reflection Required\n")
		prompt.WriteString("Before providing the fix, first explain:\n")
		prompt.WriteString("1. Why the previous attempts failed\n")
		prompt.WriteString("2. What needs to change in this attempt\n")
		prompt.WriteString("3. The specific lines that need modification\n\n")
		prompt.WriteString("Then provide the corrected patch.\n\n")
	}

	// Task instructions
	prompt.WriteString("## Task\n")
	prompt.WriteString("Generate a corrected patch that:\n")
	prompt.WriteString("1. Preserves the exact metadata (From, Date, Subject) from the original patch\n")
	prompt.WriteString("2. Achieves the same intent as the original patch\n")
	prompt.WriteString("3. Applies cleanly to the current file state\n")
	prompt.WriteString("4. Will compile successfully\n\n")

	prompt.WriteString("Output the corrected patch in unified diff format with complete headers:\n")
	prompt.WriteString("```\n")
	prompt.WriteString("From <commit-hash> Mon Sep 17 00:00:00 2001\n")
	if ctx.PatchAuthor != "" {
		prompt.WriteString(fmt.Sprintf("From: %s\n", ctx.PatchAuthor))
	}
	if ctx.PatchDate != "" {
		prompt.WriteString(fmt.Sprintf("Date: %s\n", ctx.PatchDate))
	}
	if ctx.PatchSubject != "" {
		prompt.WriteString(fmt.Sprintf("Subject: %s\n", ctx.PatchSubject))
	}
	prompt.WriteString("\n---\n")
	prompt.WriteString("<diff content>\n")
	prompt.WriteString("```\n")

	return prompt.String()
}

// validatePatchFormat validates that the patch has required metadata and format.
func validatePatchFormat(patch string, ctx *types.PatchContext) error {
	// Check for required patch headers
	if !strings.Contains(patch, "From ") && !strings.Contains(patch, "diff --git") {
		return fmt.Errorf("patch missing required headers (From or diff --git)")
	}

	// Validate patch metadata is preserved (if original had it)
	if ctx.PatchAuthor != "" {
		if !strings.Contains(patch, ctx.PatchAuthor) {
			logger.Info("Warning: patch author not preserved in LLM output",
				"expected", ctx.PatchAuthor)
			// Don't fail - this is a warning, not a hard error
		}
	}

	if ctx.PatchDate != "" {
		if !strings.Contains(patch, ctx.PatchDate) {
			logger.Info("Warning: patch date not preserved in LLM output",
				"expected", ctx.PatchDate)
		}
	}

	if ctx.PatchSubject != "" {
		// Check if subject is preserved (might be slightly reformatted)
		subjectCore := strings.TrimPrefix(ctx.PatchSubject, "[PATCH]")
		subjectCore = strings.TrimSpace(subjectCore)
		if !strings.Contains(patch, subjectCore) {
			logger.Info("Warning: patch subject not preserved in LLM output",
				"expected", subjectCore)
		}
	}

	// Check for diff content
	if !strings.Contains(patch, "@@") {
		return fmt.Errorf("patch missing diff hunks (no @@ markers found)")
	}

	// Check for basic diff structure
	hasMinus := strings.Contains(patch, "---")
	hasPlus := strings.Contains(patch, "+++")
	if !hasMinus || !hasPlus {
		return fmt.Errorf("patch missing file markers (--- or +++)")
	}

	logger.Info("Patch format validation passed")
	return nil
}
