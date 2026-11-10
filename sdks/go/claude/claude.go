// Package claude is an AskJSON transport over the Anthropic Messages API, for
// Lastro's two AI steps (intent extraction and proof derivation) and the gap
// review. It mirrors the TS SDK's `lastro/claude` entry point: structured
// outputs, adaptive thinking, and the server-side refusal fallback.
//
// The core package never imports this one, so a program that plugs in its
// own transport (or the CLI's handoff) does not use the Anthropic SDK.
package claude

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/anthropics/anthropic-sdk-go/v2"
	"github.com/anthropics/anthropic-sdk-go/v2/option"

	lastro "github.com/iurykrieger/lastro/sdks/go"
)

// DefaultModel is the model the transport uses unless Options.Model says otherwise.
const DefaultModel = "claude-opus-5-5"

// Options configures Ask.
type Options struct {
	// Client defaults to anthropic.NewClient(), which reads ANTHROPIC_API_KEY (or an `ant auth login` profile).
	Client *anthropic.Client
	// Model defaults to DefaultModel.
	Model string
	// Effort is low | medium | high | xhigh | max (default high).
	Effort string
	// RequestOptions are passed to every request (timeouts, base URL...).
	RequestOptions []option.RequestOption
}

// Ask returns an AskJSON transport over the Messages API, with structured outputs.
func Ask(o Options) lastro.AskJSON {
	return func(ctx context.Context, req lastro.AskRequest) (json.RawMessage, error) {
		client := o.Client
		if client == nil {
			c := anthropic.NewClient()
			client = &c
		}
		model := o.Model
		if model == "" {
			model = DefaultModel
		}
		effort := o.Effort
		if effort == "" {
			effort = "high"
		}
		params := anthropic.BetaMessageNewParams{
			Model:     anthropic.Model(model),
			MaxTokens: 64000,
			// On a policy decline, the API re-runs the request on a fallback model.
			Betas:     []anthropic.AnthropicBeta{anthropic.AnthropicBetaServerSideFallback2026_07_01},
			Fallbacks: anthropic.BetaFallbacksParamOfDefault(),
			Thinking:  anthropic.BetaThinkingConfigParamUnion{OfAdaptive: &anthropic.BetaThinkingConfigAdaptiveParam{}},
			OutputConfig: anthropic.BetaOutputConfigParam{
				Effort: anthropic.BetaOutputConfigEffort(effort),
				Format: anthropic.BetaJSONOutputFormatParam{Schema: req.Schema},
			},
			System:   []anthropic.BetaTextBlockParam{{Text: req.System}},
			Messages: []anthropic.BetaMessageParam{anthropic.NewBetaUserMessage(anthropic.NewBetaTextBlock(req.User))},
		}
		stream := client.Beta.Messages.NewStreaming(ctx, params, o.RequestOptions...)
		msg := anthropic.BetaMessage{}
		for stream.Next() {
			if err := msg.Accumulate(stream.Current()); err != nil {
				return nil, err
			}
		}
		if err := stream.Err(); err != nil {
			return nil, err
		}
		switch msg.StopReason {
		case anthropic.BetaStopReasonRefusal:
			return nil, fmt.Errorf("Claude declined the request (stop_reason=refusal)")
		case anthropic.BetaStopReasonMaxTokens:
			return nil, fmt.Errorf("Claude output was truncated (max_tokens)")
		}
		var text strings.Builder
		for _, b := range msg.Content {
			if b.Type == "text" {
				text.WriteString(b.Text)
			}
		}
		raw := json.RawMessage(text.String())
		if !json.Valid(raw) {
			return nil, fmt.Errorf("Claude answered with invalid JSON for the %s step", req.Step)
		}
		return raw, nil
	}
}
