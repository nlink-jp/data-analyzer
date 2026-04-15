// Package token provides token count estimation for mixed CJK/ASCII text.
package token

import (
	"unicode"
	"unicode/utf8"
)

// Estimate returns a conservative token-count estimate for mixed
// Japanese/English text.
//
// Coefficients:
//   - CJK characters (Kanji, Hiragana, Katakana, etc.): 1 char ≈ 2 tokens
//   - ASCII/Latin words: 1 word ≈ 1.3 tokens
//   - Punctuation/symbols: 1 char ≈ 1 token (important for JSON structure)
//
// Uses the higher of word-based and char-based estimates to avoid
// undercount on structured data like JSON.
func Estimate(text string) int {
	var cjkChars, asciiWords, punctChars int
	inWord := false

	for i := 0; i < len(text); {
		r, size := utf8.DecodeRuneInString(text[i:])
		i += size

		if isCJK(r) {
			cjkChars++
			inWord = false
		} else if unicode.IsLetter(r) || unicode.IsDigit(r) {
			if !inWord {
				asciiWords++
				inWord = true
			}
		} else {
			inWord = false
			if !unicode.IsSpace(r) {
				punctChars++
			}
		}
	}

	// Word-based estimate (good for prose)
	wordBased := cjkChars*2 + int(float64(asciiWords)*1.3+0.5) + punctChars

	// Char-based estimate (good for JSON/structured data)
	// ~4 chars per token for ASCII, ~2 chars per token for CJK
	totalChars := len(text)
	charBased := (totalChars + 3) / 4 // conservative: 1 token per 4 chars

	// Use the higher estimate to avoid undercount
	if charBased > wordBased {
		return charBased
	}
	return wordBased
}

// isCJK reports whether r is a CJK unified ideograph, Hiragana, Katakana,
// or other East Asian script character.
func isCJK(r rune) bool {
	return unicode.Is(unicode.Han, r) ||
		unicode.Is(unicode.Hiragana, r) ||
		unicode.Is(unicode.Katakana, r) ||
		(r >= 0xFF01 && r <= 0xFF60) || // Fullwidth forms
		(r >= 0x3000 && r <= 0x303F) // CJK symbols and punctuation
}
