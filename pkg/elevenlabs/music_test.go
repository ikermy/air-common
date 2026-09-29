package elevenlabs

import "testing"

func TestValidateMusicRequest(t *testing.T) {
	length := 30000
	seed := 123
	inst := true

	cases := []struct {
		name    string
		req     MusicRequest
		wantErr bool
	}{
		{"prompt ok", MusicRequest{Prompt: "lo-fi chill beat"}, false},
		{"prompt with length ok", MusicRequest{Prompt: "rock", LengthMs: &length, ForceInstrumental: &inst}, false},
		{"prompt with seed forbidden", MusicRequest{Prompt: "rock", Seed: &seed}, true},
		{"plan ok", MusicRequest{CompositionPlan: &MusicCompositionPlan{Chunks: []MusicChunk{{Text: "[Verse]", DurationMs: 10000}}}, Seed: &seed}, false},
		{"plan with length forbidden", MusicRequest{CompositionPlan: &MusicCompositionPlan{Chunks: []MusicChunk{{Text: "x", DurationMs: 10000}}}, LengthMs: &length}, true},
		{"plan with instrumental forbidden", MusicRequest{CompositionPlan: &MusicCompositionPlan{Chunks: []MusicChunk{{Text: "x", DurationMs: 10000}}}, ForceInstrumental: &inst}, true},
		{"both forbidden", MusicRequest{Prompt: "x", CompositionPlan: &MusicCompositionPlan{Chunks: []MusicChunk{{Text: "y", DurationMs: 10000}}}}, true},
		{"neither forbidden", MusicRequest{}, true},
		{"length too small", MusicRequest{Prompt: "x", LengthMs: intPtr(1000)}, true},
		{"length too big", MusicRequest{Prompt: "x", LengthMs: intPtr(700000)}, true},
		{"chunk duration bad", MusicRequest{CompositionPlan: &MusicCompositionPlan{Chunks: []MusicChunk{{Text: "x", DurationMs: 100}}}}, true},
		{"seed negative", MusicRequest{CompositionPlan: &MusicCompositionPlan{Chunks: []MusicChunk{{Text: "x", DurationMs: 10000}}}, Seed: intPtr(-1)}, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateMusicRequest(tc.req)
			if tc.wantErr && err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func intPtr(v int) *int { return &v }
