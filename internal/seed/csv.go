package seed

import (
	"encoding/csv"
	"fmt"
	"io"
	"strings"

	"github.com/victorradael/condoguard/api/internal/people"
)

// WriteImportCSV writes the spreadsheet a síndico would hand over to start the
// condominium: one row per person living in or owning a unit now (the people
// who only appear in the history are not in it), plus the units that have nobody.
// The format is the one POST /imports reads (people.ImportHeader, ';' separated).
func (d *Dataset) WriteImportCSV(w io.Writer) error {
	cw := csv.NewWriter(w)
	cw.Comma = ';'
	if err := cw.Write(people.ImportHeader); err != nil {
		return err
	}
	withLink := make([]bool, len(d.Units))
	for _, l := range d.Links {
		if l.Ends != nil {
			continue
		}
		u, p := d.Units[l.Unit], d.People[l.Person]
		withLink[l.Unit] = true
		if err := cw.Write([]string{d.Blocks[u.Block].Name, u.Number, fmt.Sprint(u.Floor), l.Kind, p.Name,
			formatCPF(p.CPF), p.Email, formatPhone(p.Phone), l.Starts.Format("2006-01-02")}); err != nil {
			return err
		}
	}
	for i, u := range d.Units {
		if !withLink[i] {
			if err := cw.Write([]string{d.Blocks[u.Block].Name, u.Number, fmt.Sprint(u.Floor), "", "", "", "", "", ""}); err != nil {
				return err
			}
		}
	}
	cw.Flush()
	return cw.Error()
}

// WriteBrokenCSV writes the same spreadsheet with a few lines spoiled in
// different ways (bad CPF, unknown role, no name, bad e-mail, no unit, second
// principal owner, impossible date), to try the line-by-line error report.
func (d *Dataset) WriteBrokenCSV(w io.Writer) error {
	var good strings.Builder
	if err := d.WriteImportCSV(&good); err != nil {
		return err
	}
	lines := strings.Split(strings.TrimRight(good.String(), "\n"), "\n")
	spoil := map[int]func([]string) []string{
		3:  func(f []string) []string { f[5] = "123.456.789-00"; return f },
		9:  func(f []string) []string { f[3] = "sindico"; return f },
		15: func(f []string) []string { f[4] = ""; return f },
		22: func(f []string) []string { f[6] = "email-sem-arroba"; return f },
		30: func(f []string) []string { f[1] = ""; return f },
		41: func(f []string) []string { f[8] = "31/02/2021"; return f },
	}
	for n, fn := range spoil {
		if n < len(lines) {
			lines[n] = strings.Join(fn(strings.Split(lines[n], ";")), ";")
		}
	}
	// A second principal owner for a unit that already has one.
	for i := 1; i < len(lines); i++ {
		f := strings.Split(lines[i], ";")
		if f[3] == people.KindOwner {
			f[4], f[5], f[6] = "Segundo Dono Fictício", "", ""
			lines = append(lines, strings.Join(f, ";"))
			break
		}
	}
	_, err := io.WriteString(w, strings.Join(lines, "\n")+"\n")
	return err
}

func formatCPF(d string) string {
	if d == "" {
		return ""
	}
	return people.FormatCPF(d)
}

func formatPhone(d string) string {
	if len(d) != 11 {
		return d
	}
	return fmt.Sprintf("(%s) %s-%s", d[:2], d[2:7], d[7:])
}
