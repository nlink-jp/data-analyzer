package token

import "testing"

func TestEstimate(t *testing.T) {
	tests := []struct {
		name    string
		text    string
		wantMin int // minimum expected tokens
		wantMax int // maximum expected tokens
	}{
		{
			name: "empty", text: "",
			wantMin: 0, wantMax: 0,
		},
		{
			name: "ascii prose", text: "hello world",
			wantMin: 3, wantMax: 5,
		},
		{
			name: "cjk only", text: "日本語テスト",
			wantMin: 5, wantMax: 15,
		},
		{
			name: "mixed", text: "Hello 世界 test",
			wantMin: 4, wantMax: 10,
		},
		{
			name: "json structure",
			text: `{"timestamp":"2026-04-14T08:01:12Z","user":"tanaka","action":"login"}`,
			// wordBased: 8 words*1.3 + 18 punct = ~28; charBased: 68/4 = 17
			wantMin: 17, wantMax: 40,
		},
		{
			name: "json with cjk",
			text: `{"user":"田中","action":"ログイン"}`,
			wantMin: 10, wantMax: 35,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Estimate(tt.text)
			if got < tt.wantMin || got > tt.wantMax {
				t.Errorf("Estimate(%q) = %d, want [%d, %d]", tt.text, got, tt.wantMin, tt.wantMax)
			}
		})
	}
}

func TestEstimateJSONNotUndercount(t *testing.T) {
	// Ensure JSON punctuation is not undercounted
	json := `{"a":"b","c":"d"}`
	prose := "a b c d"

	jsonTokens := Estimate(json)
	proseTokens := Estimate(prose)

	if jsonTokens <= proseTokens {
		t.Errorf("JSON (%d tokens) should be more than prose (%d tokens) due to punctuation",
			jsonTokens, proseTokens)
	}
}

func TestIsCJK(t *testing.T) {
	tests := []struct {
		r    rune
		want bool
	}{
		{'A', false},
		{'1', false},
		{' ', false},
		{'漢', true},
		{'あ', true},
		{'カ', true},
		{'Ａ', true}, // fullwidth A
	}

	for _, tt := range tests {
		t.Run(string(tt.r), func(t *testing.T) {
			if got := isCJK(tt.r); got != tt.want {
				t.Errorf("isCJK(%q) = %v, want %v", tt.r, got, tt.want)
			}
		})
	}
}
