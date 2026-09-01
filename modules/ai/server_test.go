package ai

import "testing"

// TestProbeResultHas covers the model-id spellings a local server can report
// for what the user typed, since the "Test connection" check must not claim a
// model is missing just because the server names it differently.
func TestProbeResultHas(t *testing.T) {
	res := probeResult{models: []string{
		"unsloth/Qwen3-0.6B-Q8_0",
		"llama3.2:latest",
		"gemma3:4b",
	}}

	for _, tt := range []struct {
		want  string // what the user typed
		match string // id the server reports, "" for no match
	}{
		{"unsloth/Qwen3-0.6B-Q8_0", "unsloth/Qwen3-0.6B-Q8_0"}, // exact
		{"Qwen3-0.6B-Q8_0", "unsloth/Qwen3-0.6B-Q8_0"},         // org prefix dropped
		{"unsloth/qwen3-0.6b-q8_0", "unsloth/Qwen3-0.6B-Q8_0"}, // case
		{"llama3.2", "llama3.2:latest"},                        // implicit ollama tag
		{"gemma3:4b", "gemma3:4b"},
		{"gemma3", ""}, // a different tag is a different model
		{"mistral", ""},
		{"", ""},
	} {
		name, ok := res.has(tt.want)
		if ok != (tt.match != "") || name != tt.match {
			t.Errorf("has(%q) = %q, %v; want %q, %v", tt.want, name, ok, tt.match, tt.match != "")
		}
	}
}

// TestProbeResultHasEmptyServer confirms a reachable but model-less server
// matches nothing rather than erroring.
func TestProbeResultHasEmptyServer(t *testing.T) {
	if name, ok := (probeResult{}).has("llama3.2"); ok {
		t.Errorf("empty server matched %q", name)
	}
}
