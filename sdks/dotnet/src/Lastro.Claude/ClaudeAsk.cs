using System.Text;
using System.Text.Json;
using System.Text.Json.Nodes;
using Anthropic;
using Anthropic.Models.Beta;
using Anthropic.Models.Beta.Messages;

namespace Lastro.Claude;

public sealed class ClaudeOptions
{
    /// <summary>A configured client. Default: <c>new AnthropicClient()</c> (ANTHROPIC_API_KEY or an <c>ant auth login</c> profile).</summary>
    public AnthropicClient? Client { get; init; }
    public string Model { get; init; } = "claude-opus-5-5";
    /// <summary>low, medium, high, xhigh or max. Default high.</summary>
    public string Effort { get; init; } = "high";
    public long MaxTokens { get; init; } = 64000;
}

/// <summary>
/// Lastro's two AI steps (and the gap review) on Claude, over the official Anthropic C# SDK:
/// adaptive thinking, structured outputs with the step's JSON Schema, streaming, and the
/// server-side refusal fallback.
/// </summary>
public static class ClaudeAsk
{
    public static AskJson Create(ClaudeOptions? options = null)
    {
        var o = options ?? new ClaudeOptions();
        return async (request, ct) =>
        {
            var client = o.Client ?? new AnthropicClient();
            var parameters = new MessageCreateParams
            {
                Model = o.Model,
                MaxTokens = o.MaxTokens,
                Betas = ["server-side-fallback-2026-07-01"],
                // On a policy decline, the API re-runs the request on a fallback model.
                Fallbacks = new Default(),
                Thinking = new BetaThinkingConfigAdaptive(),
                OutputConfig = new BetaOutputConfig
                {
                    Effort = o.Effort switch
                    {
                        "low" => Effort.Low,
                        "medium" => Effort.Medium,
                        "xhigh" => Effort.Xhigh,
                        "max" => Effort.Max,
                        _ => Effort.High,
                    },
                    Format = new BetaJsonOutputFormat
                    {
                        Schema = request.Schema.ToDictionary(kv => kv.Key, kv => JsonSerializer.SerializeToElement(kv.Value)),
                    },
                },
                System = request.System,
                Messages = [new BetaUserMessageParam { Content = request.User }],
            };
            var text = new StringBuilder();
            bool refused = false, truncated = false;
            await foreach (var ev in client.Beta.Messages.CreateStreaming(parameters, ct))
            {
                if (ev.TryPickContentBlockDelta(out var delta) && delta.Delta.TryPickText(out var t)) text.Append(t.Text);
                else if (ev.TryPickDelta(out var md))
                {
                    refused |= md.Delta.StopReason == "refusal";
                    truncated |= md.Delta.StopReason == "max_tokens";
                }
            }
            if (refused)
                throw new InvalidOperationException("Claude declined the request (stop_reason=refusal)");
            if (truncated)
                throw new InvalidOperationException("Claude output was truncated (max_tokens)");
            return JsonNode.Parse(text.ToString());
        };
    }

    public static LlmIntentExtractor Extractor(ClaudeOptions? options = null) => new(Create(options));
    public static LlmProofDeriver Deriver(ClaudeOptions? options = null) => new(Create(options));
    public static LlmGapReviewer GapReviewer(ClaudeOptions? options = null) => new(Create(options));
}
