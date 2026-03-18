package shopowner

import (
	"errors"
	"strings"
	"unicode"
)

// ValidateCNPJ validates a CNPJ string (formatted or raw digits).
// It checks length, rejects all-same-digit sequences, and verifies
// both check digits using the standard Brazilian algorithm.
func ValidateCNPJ(cnpj string) error {
	digits := onlyDigits(cnpj)

	if len(digits) != 14 {
		return errors.New("cnpj: must contain exactly 14 digits")
	}

	if allSameDigits(digits) {
		return errors.New("cnpj: all digits are the same")
	}

	if !checkDigit(digits, 12) || !checkDigit(digits, 13) {
		return errors.New("cnpj: invalid check digits")
	}

	return nil
}

// FormatCNPJ returns the CNPJ in the standard XX.XXX.XXX/XXXX-XX format.
// It strips any existing formatting first.
func FormatCNPJ(cnpj string) string {
	d := onlyDigits(cnpj)
	if len(d) != 14 {
		return cnpj
	}
	return d[0:2] + "." + d[2:5] + "." + d[5:8] + "/" + d[8:12] + "-" + d[12:14]
}

// ── helpers ───────────────────────────────────────────────────────────────────

func onlyDigits(s string) string {
	var b strings.Builder
	for _, r := range s {
		if unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func allSameDigits(s string) bool {
	for i := 1; i < len(s); i++ {
		if s[i] != s[0] {
			return false
		}
	}
	return true
}

// checkDigit verifies the check digit at position pos (12 or 13).
// Weights for pos=12: 5,4,3,2,9,8,7,6,5,4,3,2
// Weights for pos=13: 6,5,4,3,2,9,8,7,6,5,4,3,2
func checkDigit(digits string, pos int) bool {
	weights := make([]int, pos)
	weight := 2
	for i := pos - 1; i >= 0; i-- {
		weights[i] = weight
		weight++
		if weight > 9 {
			weight = 2
		}
	}

	sum := 0
	for i := 0; i < pos; i++ {
		sum += int(digits[i]-'0') * weights[i]
	}

	remainder := sum % 11
	expected := 0
	if remainder >= 2 {
		expected = 11 - remainder
	}

	return int(digits[pos]-'0') == expected
}
