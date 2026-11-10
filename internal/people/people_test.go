package people

import (
	"strings"
	"testing"
)

func TestNormalizeCPF(t *testing.T) {
	for in, want := range map[string]string{
		"529.982.247-25":  "52998224725",
		"52998224725":     "52998224725",
		" 529 982 247-25": "52998224725",
	} {
		if got, ok := NormalizeCPF(in); !ok || got != want {
			t.Errorf("%q: got %q, %v", in, got, ok)
		}
	}
	for _, in := range []string{"", "123", "529.982.247-26", "111.111.111-11", "00000000000", "529.982.247-2a", "529,982,247-25", "５２９９８２２４７２５"} {
		if _, ok := NormalizeCPF(in); ok {
			t.Errorf("%q must be refused", in)
		}
	}
}

func TestCPFWithCheckDigits(t *testing.T) {
	if got := CPFWithCheckDigits("529982247"); got != "52998224725" {
		t.Errorf("got %s", got)
	}
}

func TestMaskAndFormatCPF(t *testing.T) {
	if got := MaskCPF("52998224725"); got != "***.982.247-**" {
		t.Errorf("mask: %q", got)
	}
	if got := FormatCPF("52998224725"); got != "529.982.247-25" {
		t.Errorf("format: %q", got)
	}
	if MaskCPF("123") != "" {
		t.Error("a malformed CPF is not echoed")
	}
}

func TestNormalizePhone(t *testing.T) {
	for in, want := range map[string]string{"(11) 90000-1234": "11900001234", "11 3000-1234": "1130001234", "+55 11 90000-1234": "5511900001234"} {
		if got, ok := NormalizePhone(in); !ok || got != want {
			t.Errorf("%q: got %q, %v", in, got, ok)
		}
	}
	for _, in := range []string{"", "1234", "11 abc", "123456789012345"} {
		if _, ok := NormalizePhone(in); ok {
			t.Errorf("%q must be refused", in)
		}
	}
}

func TestParseCSV(t *testing.T) {
	rows, issues, err := parseCSV([]byte("\xef\xbb\xbfTorre;Apto;Andar;Vínculo;Nome;CPF;E-mail;Celular;Desde\n" +
		"A;101;1;Proprietário;Ana;;;;15/03/2021\n\n" +
		"A;102;x;morador;Bia;;;;\n" + // bad floor
		"A;103;1;morador;Caio;;;;ontem\n" + // bad date
		"A;104;1;;;;;;\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || len(issues) != 2 {
		t.Fatalf("rows %d, issues %d: %v", len(rows), len(issues), issues)
	}
	if rows[0].block != "A" || rows[0].unit != "101" || rows[0].kind != "proprietario" || rows[0].starts.Format("2006-01-02") != "2021-03-15" || rows[0].line != 2 {
		t.Errorf("row 0: %+v", rows[0])
	}
	if rows[1].hasPerson || rows[1].line != 6 {
		t.Errorf("a unit-only row: %+v", rows[1])
	}
	if issues[0].Line != 4 || issues[0].Field != "andar" || issues[1].Line != 5 || issues[1].Field != "inicio" {
		t.Errorf("issues: %+v", issues)
	}

	// Comma separated and quoted.
	rows, _, err = parseCSV([]byte("unidade,nome,papel\n\"101\",\"Souza, Ana\",morador\n"))
	if err != nil || len(rows) != 1 || rows[0].name != "Souza, Ana" {
		t.Errorf("comma csv: %+v %v", rows, err)
	}

	for name, in := range map[string]string{
		"empty":         "",
		"no unit":       "nome;cpf\nAna;1\n",
		"duplicated":    "unidade;apto\n1;2\n",
		"not utf-8":     "unidade\n\xff\n",
		"too many rows": "unidade\n" + strings.Repeat("1\n", maxImport+1),
	} {
		if _, _, err := parseCSV([]byte(in)); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}
