// Package token provides token count estimation for mixed CJK/ASCII text.
package token

import (
	"unicode"
	"unicode/utf8"
)

// Coefficients holds the tunable parameters for token estimation.
type Coefficients struct {
	CJKRatio      float64 // tokens per CJK character (default: 2.0)
	ASCIIRatio    float64 // tokens per ASCII word (default: 1.3)
	CharsPerToken int     // chars per token for char-based estimate (default: 4)
}

// DefaultCoefficients returns the default estimation coefficients.
func DefaultCoefficients() Coefficients {
	return Coefficients{
		CJKRatio:      2.0,
		ASCIIRatio:    1.3,
		CharsPerToken: 4,
	}
}

var globalCoefficients = DefaultCoefficients()

// SetCoefficients updates the global estimation coefficients.
func SetCoefficients(c Coefficients) {
	if c.CJKRatio > 0 {
		globalCoefficients.CJKRatio = c.CJKRatio
	}
	if c.ASCIIRatio > 0 {
		globalCoefficients.ASCIIRatio = c.ASCIIRatio
	}
	if c.CharsPerToken > 0 {
		globalCoefficients.CharsPerToken = c.CharsPerToken
	}
}

// Estimate returns a conservative token-count estimate for mixed
// Japanese/English text.
//
// Uses the higher of word-based and char-based estimates to avoid
// undercount on structured data like JSON.
func Estimate(text string) int {
	return EstimateWith(text, globalCoefficients)
}

// EstimateWith returns a token estimate using the given coefficients.
func EstimateWith(text string, c Coefficients) int {
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
	wordBased := int(float64(cjkChars)*c.CJKRatio+0.5) +
		int(float64(asciiWords)*c.ASCIIRatio+0.5) +
		punctChars

	// Char-based estimate (good for JSON/structured data)
	cpt := c.CharsPerToken
	if cpt <= 0 {
		cpt = 4
	}
	charBased := (len(text) + cpt - 1) / cpt

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
