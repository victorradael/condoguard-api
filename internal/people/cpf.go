package people

import (
	"strings"
	"unicode"
)

// NormalizeCPF keeps the 11 digits of a CPF written with or without the usual
// punctuation and checks the verification digits.
func NormalizeCPF(s string) (string, bool) {
	var b strings.Builder
	for _, r := range s {
		switch {
		case unicode.IsDigit(r) && r < 128:
			b.WriteRune(r)
		case r == '.' || r == '-' || r == ' ':
		default:
			return "", false
		}
	}
	d := b.String()
	if !ValidCPF(d) {
		return "", false
	}
	return d, true
}

// ValidCPF reports whether d is 11 digits with correct verification digits and
// not a sequence of one repeated digit.
func ValidCPF(d string) bool {
	if len(d) != 11 {
		return false
	}
	same := true
	for i := range d {
		if d[i] < '0' || d[i] > '9' {
			return false
		}
		if d[i] != d[0] {
			same = false
		}
	}
	if same {
		return false
	}
	return cpfDigit(d[:9], 10) == int(d[9]-'0') && cpfDigit(d[:10], 11) == int(d[10]-'0')
}

func cpfDigit(d string, weight int) int {
	sum := 0
	for i := range d {
		sum += int(d[i]-'0') * (weight - i)
	}
	r := (sum * 10) % 11
	if r == 10 {
		return 0
	}
	return r
}

// CPFWithCheckDigits completes 9 digits with their two verification digits
// (used by the fictitious data generator).
func CPFWithCheckDigits(nine string) string {
	d1 := cpfDigit(nine, 10)
	ten := nine + string(rune('0'+d1))
	d2 := cpfDigit(ten, 11)
	return ten + string(rune('0'+d2))
}

// MaskCPF hides all but the middle digits: ***.456.789-**.
func MaskCPF(d string) string {
	if len(d) != 11 {
		return ""
	}
	return "***." + d[3:6] + "." + d[6:9] + "-**"
}

// FormatCPF writes 12345678909 as 123.456.789-09.
func FormatCPF(d string) string {
	if len(d) != 11 {
		return d
	}
	return d[:3] + "." + d[3:6] + "." + d[6:9] + "-" + d[9:]
}

// NormalizePhone keeps the digits of a phone number (area code required, country
// code 55 optional). ok is false for anything else.
func NormalizePhone(s string) (string, bool) {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '+' || r == '(' || r == ')' || r == '-' || r == ' ':
		default:
			return "", false
		}
	}
	d := b.String()
	if strings.HasPrefix(d, "55") && (len(d) == 12 || len(d) == 13) {
		return d, true
	}
	if len(d) == 10 || len(d) == 11 {
		return d, true
	}
	return "", false
}
