package fixpatches

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
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

// convertToInferenceProfile converts a model ID to an inference profile ID if needed.
// Claude Sonnet 4.5 and newer models require using inference profiles instead of direct model IDs.
// Inference profiles provide cross-region routing and better availability.
func convertToInferenceProfile(modelID string, region string) string {
	// Map of model IDs that require inference profiles
	// Format: model-id -> inference-profile-id
	// Note: Inference profile IDs keep the full date-based version, just add "us." or "global." prefix
	inferenceProfileMap := map[string]string{
		"anthropic.claude-sonnet-4-5-20250929-v1:0": "us.anthropic.claude-sonnet-4-5-20250929-v1:0",
		"anthropic.claude-3-7-sonnet-20250219-v1:0": "us.anthropic.claude-3-7-sonnet-20250219-v1:0", // 1M tokens/min default!
		"anthropic.claude-3-5-sonnet-20241022-v2:0": "us.anthropic.claude-3-5-sonnet-20241022-v2:0",
		"anthropic.claude-sonnet-4-20250514-v1:0":   "us.anthropic.claude-sonnet-4-20250514-v1:0",
		"anthropic.claude-opus-4-20250514-v1:0":     "us.anthropic.claude-opus-4-20250514-v1:0",
		"anthropic.claude-opus-4-1-20250805-v1:0":   "us.anthropic.claude-opus-4-1-20250805-v1:0",
		"anthropic.claude-3-5-haiku-20241022-v1:0":  "us.anthropic.claude-3-5-haiku-20241022-v1:0",
	}

	// Check if this model needs an inference profile
	if profileID, needsProfile := inferenceProfileMap[modelID]; needsProfile {
		return profileID
	}

	// For older models (Claude 3.0, 3.5 v1) that work with direct model IDs, return as-is
	return modelID
}

// Global client to reuse across calls (avoids recreating client on every retry)
var globalBedrockClient *bedrockruntime.Client
var globalModelOrProfile string
var lastRequestTime time.Time
var requestMutex sync.Mutex

// initBedrockClient initializes the Bedrock client once and reuses it.
func initBedrockClient(model string) (*bedrockruntime.Client, string, error) {
	// Convert model to profile first to check if we need to reinitialize
	cfg, err := config.LoadDefaultConfig(context.Background(),
		config.WithRetryMaxAttempts(1),
	)
	if err != nil {
		return nil, "", fmt.Errorf("loading AWS config: %v", err)
	}

	modelOrProfile := convertToInferenceProfile(model, cfg.Region)

	// Reuse client if model hasn't changed
	if globalBedrockClient != nil && globalModelOrProfile == modelOrProfile {
		return globalBedrockClient, globalModelOrProfile, nil
	}

	// Model changed or first initialization
	logger.Info("Initializing Bedrock client", "model", model, "profile", modelOrProfile, "region", cfg.Region)

	// Create new client
	globalBedrockClient = bedrockruntime.NewFromConfig(cfg)
	globalModelOrProfile = modelOrProfile

	return globalBedrockClient, globalModelOrProfile, nil
}

// waitForRateLimit ensures we don't exceed Bedrock's rate limits.
// Bedrock has a 4 requests/min limit for cross-region inference profiles.
// This means we need at least 15 seconds between requests.
func waitForRateLimit() {
	requestMutex.Lock()
	defer requestMutex.Unlock()

	// Calculate time since last request
	timeSinceLastRequest := time.Since(lastRequestTime)

	// Bedrock limit: 4 requests/min = 15 seconds between requests
	minTimeBetweenRequests := 15 * time.Second

	if timeSinceLastRequest < minTimeBetweenRequests {
		waitTime := minTimeBetweenRequests - timeSinceLastRequest
		logger.Info("Rate limiting: waiting to respect Bedrock limits",
			"wait_seconds", waitTime.Seconds(),
			"time_since_last_request", timeSinceLastRequest.Seconds())
		time.Sleep(waitTime)
	}

	// Update last request time
	lastRequestTime = time.Now()
}

// CallBedrockForPatchFix invokes Bedrock with patch context to generate a fix.
func CallBedrockForPatchFix(ctx *types.PatchContext, model string, attempt int) (*types.PatchFix, error) {
	logger.Info("Calling Bedrock API", "model", model, "attempt", attempt)

	// Initialize or reuse existing client
	client, modelOrProfile, err := initBedrockClient(model)
	if err != nil {
		return nil, err
	}

	logger.Info("Initialized Bedrock client", "model", model, "profile", modelOrProfile, "region", "us-west-2")

	// Build the prompt
	prompt := BuildPrompt(ctx, attempt)

	logger.Info("Prompt built", "length", len(prompt), "estimated_tokens", len(prompt)/4)

	// Write prompt to debug file for inspection
	promptDebugFile := fmt.Sprintf("/tmp/llm-prompt-attempt-%d.txt", attempt)
	if err := os.WriteFile(promptDebugFile, []byte(prompt), 0644); err != nil {
		logger.Info("Warning: failed to write prompt debug file", "error", err)
	} else {
		logger.Info("Wrote prompt to debug file", "file", promptDebugFile)
	}

	// Construct Bedrock request for Claude
	systemPrompt := `You are an expert at resolving Git patch conflicts. Your task is to fix failed patch hunks by analyzing the original intent and the current code state.

Rules:
1. Preserve the original patch intent exactly
2. Preserve the original patch metadata (From, Date, Subject) exactly
3. Only modify the diff content to resolve the conflict
4. Maintain code style and formatting
5. Output ONLY the corrected patch in unified diff format with complete headers
6. Do not add explanations or commentary`

	// Calculate max_tokens based on patch size
	// Use patch size as proxy: larger patches need more output tokens
	// Conservative estimate: patch size in chars / 3 * 2 (for output expansion)
	patchSize := len(ctx.OriginalPatch)
	maxTokens := (patchSize / 3) * 2

	// Clamp to reasonable bounds
	// With extended output feature enabled, we can use up to 128K tokens
	if maxTokens < 8192 {
		maxTokens = 8192 // Minimum for any patch
	}
	if maxTokens > 100000 {
		maxTokens = 100000 // Stay well under 128K limit for safety
	}

	logger.Info("Calculated max_tokens for patch",
		"patch_size_bytes", patchSize,
		"max_tokens", maxTokens)

	requestBody := map[string]any{
		"anthropic_version": "bedrock-2023-05-31",
		"max_tokens":        maxTokens, // Dynamic based on patch size
		"messages": []map[string]string{
			{
				"role":    "user",
				"content": prompt,
			},
		},
		"system": systemPrompt,
		// Enable extended output feature for Claude models
		// This allows up to 128K output tokens instead of the default 8K limit
		"anthropic_beta": []string{"output-128k-2025-02-19"},
	}

	requestBodyBytes, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("marshaling request body: %v", err)
	}

	// Invoke model with retry logic and exponential backoff
	// Bedrock rate limits for Claude Sonnet 4.5 (cross-region inference profile):
	// - Requests per minute: 4 (L-4A6BFAB1)
	// - Tokens per minute: 4,000 (L-F4DDD3EB)
	// - Max tokens per day: 144M (L-381AD9EE)
	//
	// With 4 requests/min, we need at least 15 seconds between requests (60s / 4 = 15s)
	// To be safe and account for clock skew, we use 20s as the minimum wait time
	var response *bedrockruntime.InvokeModelOutput
	maxRetries := 5 // Give multiple chances with proper backoff

	for i := 0; i < maxRetries; i++ {
		// Log the attempt
		if i > 0 {
			logger.Info("Retrying Bedrock API call", "attempt", i+1, "max_retries", maxRetries)
		}

		// CRITICAL: Wait for rate limit before making request
		// This ensures we never exceed 4 requests/min
		waitForRateLimit()

		response, err = client.InvokeModel(context.Background(), &bedrockruntime.InvokeModelInput{
			ModelId:     aws.String(modelOrProfile),
			ContentType: aws.String("application/json"),
			Body:        requestBodyBytes,
		})

		if err == nil {
			logger.Info("Bedrock API call succeeded", "attempt", i+1)
			break
		}

		// Log the error
		logger.Info("Bedrock API call failed", "attempt", i+1, "max_retries", maxRetries, "error", err.Error())

		if i < maxRetries-1 {
			// Exponential backoff starting at 20s to respect 4 requests/min limit
			// Wait times: 20s, 40s, 80s, 160s
			// This ensures we stay well under the 4 requests/min limit (15s minimum)
			waitTime := time.Duration(20*(1<<uint(i))) * time.Second
			logger.Info("Waiting before retry to respect rate limits",
				"wait_seconds", waitTime.Seconds(),
				"rate_limit", "4 requests/min for Claude Sonnet 4.5")
			time.Sleep(waitTime)
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

	// Write response to debug file for inspection
	responseDebugFile := fmt.Sprintf("/tmp/llm-response-attempt-%d.txt", attempt)
	if err := os.WriteFile(responseDebugFile, []byte(responseText), 0644); err != nil {
		logger.Info("Warning: failed to write response debug file", "error", err)
	} else {
		logger.Info("Wrote response to debug file", "file", responseDebugFile)
	}

	// Check if response was truncated
	if result.Usage.OutputTokens >= maxTokens {
		logger.Info("Response truncated: hit max_tokens limit",
			"output_tokens", result.Usage.OutputTokens,
			"max_tokens", maxTokens)
		return nil, fmt.Errorf("LLM response truncated at %d tokens (limit: %d) - patch output too large, consider reducing input context",
			result.Usage.OutputTokens, maxTokens)
	}

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

		// Expected vs Actual comparison (NEW)
		if len(hunk.ExpectedContext) > 0 || len(hunk.ActualContext) > 0 {
			prompt.WriteString("### Expected vs Actual File State:\n\n")

			prompt.WriteString("**What the original patch expected (from OLD version):**\n")
			prompt.WriteString("```\n")
			if len(hunk.ExpectedContext) > 0 {
				for _, line := range hunk.ExpectedContext {
					prompt.WriteString(line + "\n")
				}
			} else {
				prompt.WriteString("(No expected context extracted)\n")
			}
			prompt.WriteString("```\n\n")

			prompt.WriteString("**What's actually in the file now (CURRENT version):**\n")
			prompt.WriteString("```\n")
			if len(hunk.ActualContext) > 0 {
				for _, line := range hunk.ActualContext {
					prompt.WriteString(line + "\n")
				}
			} else {
				prompt.WriteString("(No actual context extracted)\n")
			}
			prompt.WriteString("```\n\n")

			if len(hunk.Differences) > 0 {
				prompt.WriteString("**Differences:**\n")
				for _, diff := range hunk.Differences {
					prompt.WriteString(fmt.Sprintf("- %s\n", diff))
				}
				prompt.WriteString("\n")
			}
		}

		// Current state of the file (broader context)
		prompt.WriteString(fmt.Sprintf("### Current file content (around line %d):\n", hunk.LineNumber))
		prompt.WriteString("```\n")
		prompt.WriteString(hunk.Context)
		prompt.WriteString("\n```\n\n")

		// Instructions for this file
		prompt.WriteString("### Instructions:\n")
		prompt.WriteString("- Use the ACTUAL CURRENT content shown above as your starting point\n")
		prompt.WriteString("- Match the exact formatting and whitespace from the current file\n")
		prompt.WriteString("- Use current line numbers, not the original patch's line numbers\n\n")

		// Add separator between hunks
		if i < len(ctx.FailedHunks)-1 {
			prompt.WriteString("---\n\n")
		}
	}

	// Show context for ALL files in patch (NEW: Approach 2)
	if len(ctx.AllFileContexts) > 0 {
		prompt.WriteString("## Current File States\n\n")
		prompt.WriteString("Here is the current content of all files being modified:\n\n")

		for filename, context := range ctx.AllFileContexts {
			prompt.WriteString(fmt.Sprintf("### %s\n\n", filename))

			// Check if this file has a .rej (failed)
			hasFailed := false
			for _, hunk := range ctx.FailedHunks {
				if strings.Contains(hunk.FilePath, filename) {
					hasFailed = true
					break
				}
			}

			// Check if this file has offset
			hasOffset := false
			offsetAmount := 0
			if ctx.ApplicationResult != nil {
				if offset, ok := ctx.ApplicationResult.OffsetFiles[filename]; ok {
					hasOffset = true
					offsetAmount = offset
				}
			}

			// Show status
			if hasFailed {
				prompt.WriteString("**Status**: ❌ FAILED (see detailed context above)\n\n")
				prompt.WriteString("**Action Required**: Fix this file to resolve the conflict\n\n")
			} else if hasOffset {
				prompt.WriteString(fmt.Sprintf("**Status**: ⚠️ APPLIED WITH OFFSET (+%d lines)\n\n", offsetAmount))
				prompt.WriteString("**IMPORTANT**: This file applied successfully but at different line numbers than expected.\n")
				prompt.WriteString("You MUST include this file in your fixed patch with updated line numbers.\n")
				prompt.WriteString(fmt.Sprintf("The patch expected changes at certain lines, but they were found %d lines later.\n\n", offsetAmount))
			} else {
				prompt.WriteString("**Status**: ✅ APPLIED CLEANLY\n\n")
				prompt.WriteString("**Action Required**: Include this file in your fixed patch (no changes needed to line numbers)\n\n")
			}

			// Show pristine content (BEFORE git apply modified it)
			prompt.WriteString("**Original content (BEFORE patch application):**\n")
			prompt.WriteString("```\n")
			prompt.WriteString(context)
			prompt.WriteString("\n```\n\n")
		}
	}

	// Original patch for reference (with warnings about what needs fixing)
	// NOTE: We put this BEFORE the error so the error is closer to the task
	prompt.WriteString("## Original Patch (For Reference)\n\n")

	// Identify which files failed and which succeeded with offset
	failedFiles := make(map[string]bool)
	for _, hunk := range ctx.FailedHunks {
		failedFiles[filepath.Base(hunk.FilePath)] = true
	}

	// Show status of each file
	if ctx.ApplicationResult != nil {
		prompt.WriteString("**Patch Application Status:**\n")

		// List failed files
		if len(failedFiles) > 0 {
			failedList := make([]string, 0, len(failedFiles))
			for file := range failedFiles {
				failedList = append(failedList, file)
			}
			prompt.WriteString(fmt.Sprintf("- ❌ FAILED (needs fixing): %s\n", strings.Join(failedList, ", ")))
		}

		// List offset files
		if len(ctx.ApplicationResult.OffsetFiles) > 0 {
			for file, offset := range ctx.ApplicationResult.OffsetFiles {
				prompt.WriteString(fmt.Sprintf("- ⚠️  APPLIED WITH OFFSET (needs line number update): %s (offset: %d lines)\n", file, offset))
			}
		}
		prompt.WriteString("\n")
	}

	// For attempt 1: include full original patch
	// For attempt 2+: include only failed file portions to save tokens
	if attempt == 1 {
		prompt.WriteString("**Full Original Patch:**\n")
		prompt.WriteString("```diff\n")
		prompt.WriteString(ctx.OriginalPatch)
		prompt.WriteString("\n```\n\n")
	} else {
		// Extract only the failed files from the original patch
		prompt.WriteString("**Original Patch (Failed Files Only):**\n")
		prompt.WriteString("```diff\n")

		// Get list of failed files
		failedFileNames := make(map[string]bool)
		for _, hunk := range ctx.FailedHunks {
			fileName := filepath.Base(hunk.FilePath)
			failedFileNames[fileName] = true
		}

		// Extract diffs for failed files only
		if len(failedFileNames) > 0 {
			failedDiffs := extractFileDiffsFromPatch(ctx.OriginalPatch, failedFileNames)
			if failedDiffs != "" {
				prompt.WriteString(failedDiffs)
			} else {
				// Fallback: include full patch if extraction fails
				prompt.WriteString(ctx.OriginalPatch)
			}
		}

		prompt.WriteString("\n```\n\n")

		// Count total files vs failed files
		totalFiles := strings.Count(ctx.OriginalPatch, "diff --git")
		prompt.WriteString(fmt.Sprintf("ℹ️  Note: Showing only %d failed file(s). %d other files applied successfully.\n\n",
			len(failedFileNames), totalFiles-len(failedFileNames)))
	}

	prompt.WriteString("⚠️  **Important**: This patch was created against an OLD version of the code.\n")
	prompt.WriteString("Some files may have changed (version bumps, line shifts, etc.).\n")
	prompt.WriteString("Use the 'Expected vs Actual' sections above to see what changed.\n\n")

	// CRITICAL: Put error information HERE, right before the task
	// This ensures the LLM sees the error immediately before generating the fix
	if attempt > 1 && ctx.BuildError != "" {
		prompt.WriteString("---\n\n")
		prompt.WriteString(fmt.Sprintf("# 🚨 CRITICAL: Attempt #%d Failed With This Error\n\n", attempt-1))

		prompt.WriteString("**Your previous patch failed to apply with this error:**\n")
		prompt.WriteString("```\n")
		// Limit build error to last 500 lines
		errorLines := strings.Split(ctx.BuildError, "\n")
		if len(errorLines) > 500 {
			prompt.WriteString("...(truncated)...\n")
			errorLines = errorLines[len(errorLines)-500:]
		}
		prompt.WriteString(strings.Join(errorLines, "\n"))
		prompt.WriteString("\n```\n\n")

		prompt.WriteString("**🎯 Your primary goal:**\n")
		prompt.WriteString("Fix the SPECIFIC error shown above. The error message tells you:\n")
		prompt.WriteString("- Which line number failed (e.g., 'corrupt patch at line 276')\n")
		prompt.WriteString("- What type of error occurred (corrupt patch, missing header, etc.)\n")
		prompt.WriteString("- This is the MOST IMPORTANT context for your fix\n\n")

		prompt.WriteString("**Common causes of these errors:**\n")
		prompt.WriteString("- 'corrupt patch at line X': Patch format is malformed (missing newlines, truncated content)\n")
		prompt.WriteString("- 'patch fragment without header': Missing 'diff --git' or file headers\n")
		prompt.WriteString("- 'does not apply': Line numbers or content don't match current file\n\n")

		prompt.WriteString("---\n\n")
	}

	// Reflection for later attempts
	if attempt >= 3 {
		prompt.WriteString(fmt.Sprintf("## 🤔 Reflection Required (Attempt #%d)\n\n", attempt))
		prompt.WriteString(fmt.Sprintf("This is your %s attempt. Before providing the fix, first explain:\n", ordinal(attempt)))
		prompt.WriteString("1. What SPECIFIC error occurred (see the error above)\n")
		prompt.WriteString("2. Why that error happened (patch format issue? line number mismatch?)\n")
		prompt.WriteString("3. What SPECIFIC changes you'll make to fix it\n\n")
		prompt.WriteString("Then provide the corrected patch.\n\n")
	}

	// Task instructions
	prompt.WriteString("## Task\n")
	prompt.WriteString("Generate a corrected patch that:\n")
	prompt.WriteString("1. Preserves the exact metadata (From, Date, Subject) from the original patch\n")
	prompt.WriteString("2. Includes ALL files from the original patch (both failed and offset files)\n")
	prompt.WriteString("3. For FAILED files: Fix them using the 'Expected vs Actual' context above\n")
	prompt.WriteString("4. For OFFSET files: Update line numbers to match current file state\n")
	prompt.WriteString("5. Uses RELATIVE file paths NOT absolute paths\n")
	prompt.WriteString("6. Will compile successfully\n\n")

	prompt.WriteString("## How to Generate the Fix\n\n")

	prompt.WriteString("**Step 1: Understand the Intent**\n")
	prompt.WriteString("Look at 'What the patch tried to do' to understand the semantic change being made.\n\n")

	prompt.WriteString("**Step 2: Use Current File State**\n")
	prompt.WriteString("The 'Expected vs Actual' sections show you:\n")
	prompt.WriteString("- What the original patch expected (OLD version)\n")
	prompt.WriteString("- What's actually in the file NOW (NEW version)\n")
	prompt.WriteString("- The specific differences between them\n\n")
	prompt.WriteString("You MUST use the ACTUAL CURRENT content as your starting point, not the expected content.\n\n")

	prompt.WriteString("**Step 3: Find the Semantic Location**\n")
	prompt.WriteString("Find where in the CURRENT file the change should be applied:\n")
	prompt.WriteString("- Use the 'Current file content' section to see the broader context\n")
	prompt.WriteString("- Match based on semantic meaning (package names, function names, etc.)\n")
	prompt.WriteString("- Don't rely on line numbers from the original patch - they may have shifted\n\n")

	prompt.WriteString("**Step 4: Generate the Patch**\n")
	prompt.WriteString("Create a patch that:\n")
	prompt.WriteString("- Uses context lines from the CURRENT file (complete, not truncated)\n")
	prompt.WriteString("- Uses CURRENT line numbers\n")
	prompt.WriteString("- Makes the SAME semantic change as the original patch intended\n")
	prompt.WriteString("- Preserves exact formatting and whitespace from the current file\n\n")

	prompt.WriteString("Output format (unified diff with complete headers):\n")
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
	prompt.WriteString(" file1.ext | X +/-\n")
	prompt.WriteString(" file2.ext | Y +/-\n")
	prompt.WriteString(" N files changed, X insertions(+), Y deletions(-)\n\n")
	prompt.WriteString("diff --git a/file1.ext b/file1.ext\n")
	prompt.WriteString("...\n")
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

// ordinal returns the ordinal string for a number (1st, 2nd, 3rd, etc.)
func ordinal(n int) string {
	suffix := "th"
	switch n % 10 {
	case 1:
		if n%100 != 11 {
			suffix = "st"
		}
	case 2:
		if n%100 != 12 {
			suffix = "nd"
		}
	case 3:
		if n%100 != 13 {
			suffix = "rd"
		}
	}
	return fmt.Sprintf("%d%s", n, suffix)
}

// extractFileDiffsFromPatch extracts only the diffs for specified files from a patch.
// This is used to reduce token usage in retry attempts by only including failed files.
func extractFileDiffsFromPatch(patch string, fileNames map[string]bool) string {
	if len(fileNames) == 0 {
		return ""
	}

	var result strings.Builder
	lines := strings.Split(patch, "\n")

	inTargetFile := false
	currentFileName := ""
	var currentFileDiff strings.Builder

	for i, line := range lines {
		// Check for new file diff
		if strings.HasPrefix(line, "diff --git") {
			// Save previous file if it was a target
			if inTargetFile && currentFileDiff.Len() > 0 {
				result.WriteString(currentFileDiff.String())
				result.WriteString("\n")
			}

			// Reset for new file
			currentFileDiff.Reset()
			inTargetFile = false

			// Extract filename from "diff --git a/path/to/file.go b/path/to/file.go"
			parts := strings.Fields(line)
			if len(parts) >= 4 {
				// Get the b/ path (destination)
				filePath := strings.TrimPrefix(parts[3], "b/")
				currentFileName = filepath.Base(filePath)

				// Check if this is a file we want
				if fileNames[currentFileName] {
					inTargetFile = true
					currentFileDiff.WriteString(line)
					currentFileDiff.WriteString("\n")
				}
			}
		} else if inTargetFile {
			// Include all lines for target files
			currentFileDiff.WriteString(line)
			if i < len(lines)-1 {
				currentFileDiff.WriteString("\n")
			}
		}
	}

	// Don't forget the last file
	if inTargetFile && currentFileDiff.Len() > 0 {
		result.WriteString(currentFileDiff.String())
	}

	return result.String()
}
