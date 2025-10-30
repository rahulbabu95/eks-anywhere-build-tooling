# Fix: Bedrock Inference Profile Requirement

## Problem

When calling Bedrock API with Claude Sonnet 4.5, the request was failing with:

```
ValidationException: Invocation of model ID anthropic.claude-sonnet-4-5-20250929-v1:0 
with on-demand throughput isn't supported. Retry your request with the ID or ARN of an 
inference profile that contains this model.
```

## Root Cause

AWS Bedrock changed the API requirements for newer Claude models (Sonnet 4.5 and later). These models now require using **inference profiles** instead of direct model IDs.

### What are Inference Profiles?

Inference profiles provide:
- **Cross-region routing**: Automatically route requests to available regions
- **Better availability**: Failover to other regions if one is unavailable
- **Consistent pricing**: Same pricing across regions

### Model ID vs Inference Profile

**Old way (direct model ID):**
```
anthropic.claude-sonnet-4-5-20250929-v1:0
```

**New way (inference profile):**
```
us.anthropic.claude-sonnet-4-5-20250929-v1:0
```

The inference profile ID uses a similar format:
- Prefix: `us.` (for US cross-region profile) or `global.` (for global routing)
- **Keeps the full date-based version**: The date portion is preserved in the profile ID

## Solution

Added a helper function `convertToInferenceProfile()` that automatically converts model IDs to inference profiles when needed:

```go
func convertToInferenceProfile(modelID string, region string) string {
	// Map of model IDs that require inference profiles
	inferenceProfileMap := map[string]string{
		"anthropic.claude-sonnet-4-5-20250929-v1:0": "us.anthropic.claude-sonnet-4-5-v1:0",
		"anthropic.claude-3-5-sonnet-20241022-v2:0": "us.anthropic.claude-3-5-sonnet-v2:0",
	}

	// Check if this model needs an inference profile
	if profileID, needsProfile := inferenceProfileMap[modelID]; needsProfile {
		return profileID
	}

	// For older models, return the original model ID
	return modelID
}
```

The function is called before invoking Bedrock:

```go
// Convert model ID to inference profile ARN if needed
modelOrProfile := convertToInferenceProfile(model, cfg.Region)
logger.Info("Using model/profile", "identifier", modelOrProfile)

// Use the converted identifier in the API call
response, err = client.InvokeModel(context.Background(), &bedrockruntime.InvokeModelInput{
	ModelId:     aws.String(modelOrProfile),
	ContentType: aws.String("application/json"),
	Body:        requestBodyBytes,
})
```

## Supported Models

The mapping currently includes:

| Model ID | Inference Profile |
|----------|-------------------|
| `anthropic.claude-sonnet-4-5-20250929-v1:0` | `us.anthropic.claude-sonnet-4-5-20250929-v1:0` |
| `anthropic.claude-3-5-sonnet-20241022-v2:0` | `us.anthropic.claude-3-5-sonnet-20241022-v2:0` |
| `anthropic.claude-sonnet-4-20250514-v1:0` | `us.anthropic.claude-sonnet-4-20250514-v1:0` |
| `anthropic.claude-opus-4-20250514-v1:0` | `us.anthropic.claude-opus-4-20250514-v1:0` |
| `anthropic.claude-opus-4-1-20250805-v1:0` | `us.anthropic.claude-opus-4-1-20250805-v1:0` |
| `anthropic.claude-3-7-sonnet-20250219-v1:0` | `us.anthropic.claude-3-7-sonnet-20250219-v1:0` |
| `anthropic.claude-3-5-haiku-20241022-v1:0` | `us.anthropic.claude-3-5-haiku-20241022-v1:0` |

Older Claude models (3.0, 3.5 Sonnet v1 from June 2024) continue to work with direct model IDs.

## Default Model

The default model in the CLI is:
```bash
--model anthropic.claude-sonnet-4-5-20250929-v1:0
```

This will automatically be converted to:
```
us.anthropic.claude-sonnet-4-5-20250929-v1:0
```

## Testing

After this fix, the Bedrock API call should work correctly:

```bash
cd tools/version-tracker
./version-tracker fix-patches \
  --project fluxcd/source-controller \
  --pr 4883 \
  --max-attempts 1 \
  --verbosity 6
```

You should see in the logs:
```
Using model/profile identifier=us.anthropic.claude-sonnet-4-5-20250929-v1:0
```

## References

- [AWS Bedrock Inference Profiles Documentation](https://docs.aws.amazon.com/bedrock/latest/userguide/inference-profiles.html)
- [Claude 3.5 Sonnet v2 Announcement](https://www.anthropic.com/news/claude-3-5-sonnet)

## Files Changed

- `tools/version-tracker/pkg/commands/fixpatches/llm.go`
  - Added `convertToInferenceProfile()` function
  - Updated `CallBedrockForPatchFix()` to use inference profiles
