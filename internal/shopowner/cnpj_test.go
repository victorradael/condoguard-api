package shopowner_test

import (
	"testing"

	"github.com/victorradael/condoguard/api/internal/shopowner"
)

// CNPJ válidos (dígitos verificadores corretos)
var validCNPJs = []struct {
	raw       string
	formatted string
}{
	{"11222333000181", "11.222.333/0001-81"},
	{"11.222.333/0001-81", "11.222.333/0001-81"},
	{"45997418000153", "45.997.418/0001-53"},
	{"45.997.418/0001-53", "45.997.418/0001-53"},
}

// CNPJ inválidos
var invalidCNPJs = []string{
	"",
	"00000000000000",  // todos zeros
	"11111111111111",  // todos iguais
	"1234567890123",   // menos de 14 dígitos
	"123456789012345", // mais de 14 dígitos
	"11222333000100",  // dígito verificador errado
	"abc.def.ghi/jkl-mn",
}

func TestCNPJ_Validate_ValidCNPJs(t *testing.T) {
	for _, tc := range validCNPJs {
		t.Run(tc.raw, func(t *testing.T) {
			if err := shopowner.ValidateCNPJ(tc.raw); err != nil {
				t.Errorf("expected valid CNPJ %q, got error: %v", tc.raw, err)
			}
		})
	}
}

func TestCNPJ_Validate_InvalidCNPJs(t *testing.T) {
	for _, raw := range invalidCNPJs {
		t.Run(raw, func(t *testing.T) {
			if err := shopowner.ValidateCNPJ(raw); err == nil {
				t.Errorf("expected invalid CNPJ %q to fail, but got nil error", raw)
			}
		})
	}
}

func TestCNPJ_Format_NormalizesRawDigits(t *testing.T) {
	got := shopowner.FormatCNPJ("11222333000181")
	want := "11.222.333/0001-81"
	if got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

func TestCNPJ_Format_PreservesAlreadyFormatted(t *testing.T) {
	got := shopowner.FormatCNPJ("11.222.333/0001-81")
	want := "11.222.333/0001-81"
	if got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

func TestCNPJ_Validate_AllSameDigits_Invalid(t *testing.T) {
	for d := '0'; d <= '9'; d++ {
		cnpj := ""
		for i := 0; i < 14; i++ {
			cnpj += string(d)
		}
		if err := shopowner.ValidateCNPJ(cnpj); err == nil {
			t.Errorf("CNPJ with all same digit %c should be invalid", d)
		}
	}
}
