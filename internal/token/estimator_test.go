package token

import "testing"

func TestEstimate(t *testing.T) {
	tests := []struct {
		name string
		text string
		want int
	}{
		{
			name: "empty",
			text: "",
			want: 0,
		},
		{
			name: "ascii only",
			text: "hello world",
			want: 3, // 2 words * 1.3 = 2.6 → 3
		},
		{
			name: "cjk only",
			text: "日本語テスト",
			want: 12, // 6 chars * 2 = 12
		},
		{
			name: "mixed",
			text: "Hello 世界 test",
			want: 8, // 2 words * 1.3 = 2.6 → 3, 2 CJK * 2 = 4, total = 7... let's check
			// "Hello" = 1 word, "世界" = 2 CJK, "test" = 1 word
			// 2 CJK * 2 + 2 words * 1.3 = 4 + 2.6 = 6.6 → 4 + 3 = 7
		},
		{
			name: "numbers",
			text: "test123 abc456",
			want: 3, // 2 words * 1.3 = 2.6 → 3 (digits continue words)
		},
		{
			name: "single word",
			text: "hello",
			want: 1, // 1 word * 1.3 = 1.3 → 1
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Estimate(tt.text)
			if tt.name == "mixed" {
				// Recalculate: "Hello"=1word, "世界"=2CJK, "test"=1word
				// 2*2 + int(2*1.3+0.5) = 4 + int(3.1) = 4 + 3 = 7
				tt.want = 7
			}
			if got != tt.want {
				t.Errorf("Estimate(%q) = %d, want %d", tt.text, got, tt.want)
			}
		})
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
