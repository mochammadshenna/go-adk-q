package bridge_test

import (
	"context"
	"os"
	"testing"

	"github.com/firebase/genkit/go/genkit"
	"github.com/firebase/genkit/go/plugins/compat_oai"

	bridge "go-adk-q/sample/opencodezen/agents"
)

// TestAnalyzeFlow runs the full Genkit→ADK bridge end-to-end.
//
// Requires OPENCODE_API_KEY to be set; skipped otherwise.
//
//	OPENCODE_API_KEY=your_key go test ./sample/opencodezen/agents/ -v -run TestAnalyzeFlow
func TestAnalyzeFlow(t *testing.T) {
	if os.Getenv("OPENCODE_API_KEY") == "" {
		t.Skip("OPENCODE_API_KEY not set — skipping live test")
	}

	ctx := context.Background()

	g := genkit.Init(ctx,
		genkit.WithPlugins(&compat_oai.OpenAICompatible{
			Provider: "opencode",
			APIKey:   os.Getenv("OPENCODE_API_KEY"),
			BaseURL:  "https://opencode.ai/zen/v1",
		}),
	)

	// RegisterAnalysisFlow returns the *core.Flow — capture it.
	flow := bridge.RegisterAnalysisFlow(g)

	result, err := flow.Run(ctx, "Summarise this in 5 words: The quick brown fox jumps over the lazy dog")
	if err != nil {
		t.Fatalf("flow.Run: %v", err)
	}
	if result == "" {
		t.Fatal("got empty response")
	}
	t.Logf("response: %s", result)
}
