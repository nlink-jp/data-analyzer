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
func Estimate(text string) int {
	var cjkChars, asciiWords int
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
		}
	}

	return cjkChars*2 + int(float64(asciiWords)*1.3+0.5)
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
