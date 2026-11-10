package people

import (
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/victorradael/condoguard/api/internal/audit"
	"github.com/victorradael/condoguard/api/internal/authz"
	"github.com/victorradael/condoguard/api/internal/db"
	"github.com/victorradael/condoguard/api/internal/db/dbgen"
	"github.com/victorradael/condoguard/api/internal/tenant"
	"github.com/victorradael/condoguard/api/pkg/civil"
)

// ImportHeader is the header of the import spreadsheet (the order does not
// matter when importing, and ';' or ',' both work as separator). One row is one
// person linked to a unit; a row with only block, unit and floor creates the
// unit alone. Roles: proprietario, coproprietario, inquilino, morador, dependente.
var ImportHeader = []string{"bloco", "unidade", "andar", "papel", "nome", "cpf", "email", "telefone", "inicio"}

// ErrImportInvalid is returned by CommitImport when the file has errors; the
// report says which. Nothing is saved.
var ErrImportInvalid = errors.New("people: the file has errors")

// RowIssue is one problem found in a line of the file.
type RowIssue struct {
	Line    int    `json:"line"`
	Field   string `json:"field,omitempty"`
	Message string `json:"message"`
}

// RowPreview is what a line of the file will do.
type RowPreview struct {
	Line   int    `json:"line"`
	Block  string `json:"block,omitempty"`
	Unit   string `json:"unit"`
	Kind   string `json:"kind,omitempty"`
	Person string `json:"person,omitempty"`
	// Action: create (new unit and/or person and link), link (existing ones),
	// unchanged (already registered), error.
	Action string `json:"action"`
}

// ImportReport is the preview or the result of an import.
type ImportReport struct {
	Valid      bool         `json:"valid"`
	Rows       int          `json:"rows"`
	NewBlocks  int          `json:"newBlocks"`
	NewUnits   int          `json:"newUnits"`
	NewPeople  int          `json:"newPeople"`
	NewLinks   int          `json:"newLinks"`
	Unchanged  int          `json:"unchanged"`
	ErrorCount int          `json:"errorCount"`
	Errors     []RowIssue   `json:"errors"`
	Preview    []RowPreview `json:"preview"`
	// Truncated: Errors and Preview hold only the first entries.
	Truncated bool `json:"truncated,omitempty"`
}

const (
	maxReportErrors  = 500
	maxReportPreview = 200
)

type csvRow struct {
	line                    int
	block, unit             string
	floor                   int
	kind                    string
	name, cpf, email, phone string
	starts                  time.Time
	hasPerson, dated        bool
}

const maxUnit = 40

var headerAliases = map[string]string{
	"bloco": "bloco", "torre": "bloco",
	"unidade": "unidade", "apartamento": "unidade", "apto": "unidade", "unit": "unidade",
	"andar": "andar", "floor": "andar",
	"papel": "papel", "vinculo": "papel", "tipo": "papel",
	"nome": "nome", "name": "nome",
	"cpf":   "cpf",
	"email": "email", "e-mail": "email",
	"telefone": "telefone", "celular": "telefone", "phone": "telefone",
	"inicio": "inicio", "desde": "inicio", "data_inicio": "inicio",
}

var accentFold = strings.NewReplacer("á", "a", "à", "a", "â", "a", "ã", "a", "é", "e", "ê", "e", "í", "i", "ó", "o", "ô", "o", "õ", "o", "ú", "u", "ç", "c")

func foldKey(s string) string {
	return accentFold.Replace(strings.ToLower(strings.Join(strings.Fields(s), "_")))
}

func foldKind(s string) string {
	return accentFold.Replace(strings.ToLower(strings.TrimSpace(s)))
}

// parseCSV reads the file into rows, reporting the problems it finds in the
// text itself (separator, header, malformed lines). Per-field problems are
// found later with the database at hand.
func parseCSV(data []byte) (rows []csvRow, issues []RowIssue, err error) {
	data = bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))
	if !utf8.Valid(data) {
		return nil, nil, validationf("the file must be UTF-8 text")
	}
	first, _, _ := bytes.Cut(data, []byte("\n"))
	r := csv.NewReader(bytes.NewReader(data))
	r.Comma = ','
	if bytes.Count(first, []byte(";")) > bytes.Count(first, []byte(",")) {
		r.Comma = ';'
	}
	r.FieldsPerRecord = -1
	r.TrimLeadingSpace = true

	head, rerr := r.Read()
	if rerr == io.EOF {
		return nil, nil, validationf("the file is empty")
	}
	if rerr != nil {
		return nil, nil, validationf("the header could not be read: %v", rerr)
	}
	col := map[string]int{}
	for i, h := range head {
		if name, ok := headerAliases[foldKey(h)]; ok {
			if _, dup := col[name]; dup {
				return nil, nil, validationf("column %q appears twice", name)
			}
			col[name] = i
		}
	}
	if _, ok := col["unidade"]; !ok {
		return nil, nil, validationf("the header needs a column \"unidade\" (found: %s)", strings.Join(head, ", "))
	}
	get := func(rec []string, name string) string {
		if i, ok := col[name]; ok && i < len(rec) {
			return strings.TrimSpace(rec[i])
		}
		return ""
	}
	for {
		rec, rerr := r.Read()
		if rerr == io.EOF {
			break
		}
		line, _ := r.FieldPos(0)
		if rerr != nil {
			var pe *csv.ParseError
			if errors.As(rerr, &pe) {
				issues = append(issues, RowIssue{Line: pe.StartLine, Message: "the line is malformed (quotes or separators)"})
				continue
			}
			return nil, nil, rerr
		}
		allEmpty := true
		for _, f := range rec {
			if strings.TrimSpace(f) != "" {
				allEmpty = false
				break
			}
		}
		if allEmpty {
			continue
		}
		if len(rows)+len(issues) >= maxImport {
			return nil, nil, validationf("the file has more than %d rows; split it", maxImport)
		}
		row := csvRow{line: line, block: get(rec, "bloco"), unit: get(rec, "unidade"), kind: foldKind(get(rec, "papel")),
			name: get(rec, "nome"), cpf: get(rec, "cpf"), email: get(rec, "email"), phone: get(rec, "telefone")}
		row.hasPerson = row.kind != "" || row.name != "" || row.cpf != "" || row.email != "" || row.phone != ""
		if v := get(rec, "andar"); v != "" {
			n, aerr := strconv.Atoi(v)
			if aerr != nil || n < -10 || n > 200 {
				issues = append(issues, RowIssue{Line: line, Field: "andar", Message: "andar must be a whole number"})
				continue
			}
			row.floor = n
		}
		if v := get(rec, "inicio"); v != "" {
			d, derr := parseDay(v, "inicio", time.Time{})
			if derr != nil {
				// Brazilian spreadsheets write dd/mm/yyyy.
				if t, e2 := time.Parse("02/01/2006", v); e2 == nil {
					d, derr = t, nil
				}
			}
			if derr != nil {
				issues = append(issues, RowIssue{Line: line, Field: "inicio", Message: "inicio must be YYYY-MM-DD or DD/MM/YYYY"})
				continue
			}
			row.starts, row.dated = d, true
		}
		rows = append(rows, row)
	}
	return rows, issues, nil
}

type unitKey struct{ block, number string }

type interval struct {
	kind, person string
	starts       time.Time
	ends         *time.Time
}

func (a interval) overlaps(starts time.Time, ends *time.Time) bool {
	if ends != nil && !ends.After(a.starts) {
		return false
	}
	return a.ends == nil || a.ends.After(starts)
}

type unitPlan struct {
	key      unitKey
	id       uuid.UUID
	isNew    bool
	floor    int
	existing []interval // links already in the database
	names    map[string]string
}

type personPlan struct {
	id         uuid.UUID
	isNew      bool
	name       string
	cpf, email *string
	phone      *string
}

type linkOp struct {
	line   int
	unit   *unitPlan
	person *personPlan
	kind   string
	starts time.Time
}

type importPlan struct {
	report    ImportReport
	newBlocks []string
	units     []*unitPlan
	people    []*personPlan
	links     []linkOp
}

func (rp *ImportReport) fail(line int, field, msg string) {
	rp.ErrorCount++
	if len(rp.Errors) < maxReportErrors {
		rp.Errors = append(rp.Errors, RowIssue{Line: line, Field: field, Message: msg})
	} else {
		rp.Truncated = true
	}
}

func (rp *ImportReport) add(p RowPreview) {
	if len(rp.Preview) < maxReportPreview {
		rp.Preview = append(rp.Preview, p)
	} else {
		rp.Truncated = true
	}
}

// analyze validates the file against the data of the condominium and works out
// what importing it would do. It writes nothing.
func (s *Service) analyze(ctx context.Context, condo uuid.UUID, data []byte) (*importPlan, error) {
	rows, issues, err := parseCSV(data)
	if err != nil {
		return nil, err
	}
	plan := &importPlan{report: ImportReport{Rows: len(rows) + len(issues), Errors: []RowIssue{}, Preview: []RowPreview{}}}
	for _, is := range issues {
		plan.report.fail(is.Line, is.Field, is.Message)
	}
	today := civil.Today()

	blocks := map[string]bool{} // name → exists in the database
	units := map[unitKey]*unitPlan{}
	byCPF := map[string]*personPlan{}
	byEmail := map[string]*personPlan{}

	for _, row := range rows {
		bad := func(field, msg string) {
			plan.report.fail(row.line, field, msg)
			plan.report.add(RowPreview{Line: row.line, Block: row.block, Unit: row.unit, Action: "error"})
		}

		// ── fields ──
		if row.unit == "" {
			bad("unidade", "unidade is required")
			continue
		}
		if utf8.RuneCountInString(row.unit) > maxUnit || utf8.RuneCountInString(row.block) > maxBlock {
			bad("unidade", "block or unit name is too long")
			continue
		}
		var c cleanedPerson
		if row.hasPerson {
			if row.kind == "" {
				bad("papel", "papel is required when the line has a person")
				continue
			}
			if !validKind(row.kind) {
				bad("papel", fmt.Sprintf("papel %q is unknown (use %s)", row.kind, strings.Join(Kinds, ", ")))
				continue
			}
			if c, err = cleanPersonRequest(PersonRequest{FullName: row.name, CPF: row.cpf, Email: row.email, Phone: row.phone}); err != nil {
				field := "nome"
				msg := strings.TrimPrefix(err.Error(), ErrValidation.Error()+": ")
				for _, f := range []string{"cpf", "email", "phone"} {
					if strings.HasPrefix(msg, f) {
						field = map[string]string{"cpf": "cpf", "email": "email", "phone": "telefone"}[f]
					}
				}
				bad(field, msg)
				continue
			}
		}

		// ── block and unit ──
		if row.block != "" {
			name, _ := cleanName(row.block, maxBlock, "bloco")
			row.block = name
			if _, seen := blocks[name]; !seen {
				_, ferr := s.q.FindBlockByName(ctx, dbgen.FindBlockByNameParams{CondominiumID: condo, Name: name})
				if ferr != nil && !db.IsNoRows(ferr) {
					return nil, ferr
				}
				blocks[name] = ferr == nil
				if ferr != nil {
					plan.newBlocks = append(plan.newBlocks, name)
				}
			}
		}
		key := unitKey{row.block, row.unit}
		up := units[key]
		if up == nil {
			up = &unitPlan{key: key, floor: row.floor, names: map[string]string{}}
			if err = s.loadUnit(ctx, condo, up); err != nil {
				return nil, err
			}
			units[key] = up
			plan.units = append(plan.units, up)
		}
		if !row.hasPerson {
			act := "unchanged"
			if up.isNew && up.floor == row.floor {
				act = "create"
			}
			plan.report.add(RowPreview{Line: row.line, Block: row.block, Unit: row.unit, Action: act})
			continue
		}

		// ── person ──
		var pp *personPlan
		fresh := false // pp was not planned before: add it once the line is accepted
		switch {
		case c.cpf != nil && byCPF[*c.cpf] != nil:
			pp = byCPF[*c.cpf]
			if !strings.EqualFold(pp.name, c.name) {
				bad("cpf", fmt.Sprintf("this CPF is already used on another line by %q", pp.name))
				continue
			}
		case c.cpf == nil && c.email != nil && byEmail[*c.email+"|"+strings.ToLower(c.name)] != nil:
			pp = byEmail[*c.email+"|"+strings.ToLower(c.name)]
		default:
			pp = &personPlan{id: uuid.New(), isNew: true, name: c.name, cpf: c.cpf, email: c.email, phone: c.phone}
			found, ferr := s.findPerson(ctx, condo, c)
			if ferr != nil {
				return nil, ferr
			}
			if found != nil {
				pp.id, pp.isNew = found.ID, false
			}
			fresh = true
		}

		// ── link ──
		starts := row.starts
		if !row.dated {
			starts = today
		}
		pid := pp.id.String()
		dup, clash := false, ""
		for _, iv := range up.existing {
			sameKind := iv.kind == row.kind
			if sameKind && iv.ends == nil && (iv.person == pid || (c.cpf == nil && strings.EqualFold(up.names[iv.person], c.name))) {
				dup = true
			}
			if row.kind == KindOwner && iv.kind == KindOwner && iv.overlaps(starts, nil) && iv.person != pid {
				clash = "the unit already has a principal owner in this period"
			}
		}
		if dup {
			plan.report.Unchanged++
			plan.report.add(RowPreview{Line: row.line, Block: row.block, Unit: row.unit, Kind: row.kind, Person: c.name, Action: "unchanged"})
			continue
		}
		if clash != "" {
			bad("papel", clash)
			continue
		}
		if fresh {
			if c.cpf != nil {
				byCPF[*c.cpf] = pp
			} else if c.email != nil {
				byEmail[*c.email+"|"+strings.ToLower(c.name)] = pp
			}
			plan.people = append(plan.people, pp)
		}
		up.existing = append(up.existing, interval{kind: row.kind, person: pid, starts: starts})
		up.names[pid] = c.name
		plan.links = append(plan.links, linkOp{line: row.line, unit: up, person: pp, kind: row.kind, starts: starts})
		act := "link"
		if up.isNew || pp.isNew {
			act = "create"
		}
		plan.report.add(RowPreview{Line: row.line, Block: row.block, Unit: row.unit, Kind: row.kind, Person: c.name, Action: act})
	}

	rp := &plan.report
	rp.NewBlocks = len(plan.newBlocks)
	for _, u := range plan.units {
		if u.isNew {
			rp.NewUnits++
		}
	}
	for _, p := range plan.people {
		if p.isNew {
			rp.NewPeople++
		}
	}
	rp.NewLinks = len(plan.links)
	rp.Valid = rp.ErrorCount == 0
	return plan, nil
}

// loadUnit fills in the unit if it exists, with its current links.
func (s *Service) loadUnit(ctx context.Context, condo uuid.UUID, up *unitPlan) error {
	var block *uuid.UUID
	if up.key.block != "" {
		id, err := s.q.FindBlockByName(ctx, dbgen.FindBlockByNameParams{CondominiumID: condo, Name: up.key.block})
		if db.IsNoRows(err) {
			up.id, up.isNew = uuid.New(), true
			return nil
		}
		if err != nil {
			return err
		}
		block = &id
	}
	id, err := s.q.FindResidentByNumber(ctx, dbgen.FindResidentByNumberParams{CondominiumID: condo, BlockID: block, UnitNumber: up.key.number})
	if db.IsNoRows(err) {
		up.id, up.isNew = uuid.New(), true
		return nil
	}
	if err != nil {
		return err
	}
	up.id = id
	links, err := s.q.ListLinksByUnit(ctx, dbgen.ListLinksByUnitParams{CondominiumID: condo, UnitID: id})
	if err != nil {
		return err
	}
	for _, l := range links {
		up.existing = append(up.existing, interval{kind: l.Kind, person: l.PersonID.String(), starts: l.StartsOn, ends: l.EndsOn})
		up.names[l.PersonID.String()] = l.PersonName
	}
	return nil
}

// findPerson looks for the registered person the line describes: same CPF, or
// same e-mail and name.
func (s *Service) findPerson(ctx context.Context, condo uuid.UUID, c cleanedPerson) (*dbgen.Person, error) {
	var (
		r   dbgen.Person
		err error
	)
	switch {
	case c.cpf != nil:
		r, err = s.q.FindPersonByCPF(ctx, dbgen.FindPersonByCPFParams{CondominiumID: condo, Cpf: c.cpf})
	case c.email != nil:
		r, err = s.q.FindPersonByEmailAndName(ctx, dbgen.FindPersonByEmailAndNameParams{CondominiumID: condo, Email: c.email, FullName: c.name})
	default:
		return nil, nil
	}
	if db.IsNoRows(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// PreviewImport validates the CSV and says what importing it would do, with a
// report by line. Nothing is saved. Needs units.write and people.write.
func (s *Service) PreviewImport(ctx context.Context, data []byte) (*ImportReport, error) {
	if err := requireImport(ctx); err != nil {
		return nil, err
	}
	condo, err := s.condo(ctx)
	if err != nil {
		return nil, err
	}
	var rep *ImportReport
	err = db.InTx(ctx, s.pool, func(ctx context.Context) error {
		plan, aerr := s.analyze(ctx, condo, data)
		if aerr != nil {
			return aerr
		}
		rep = &plan.report
		return nil
	})
	return rep, err
}

func requireImport(ctx context.Context) error {
	if _, err := authz.Require(ctx, authz.PermUnitsWrite); err != nil {
		return err
	}
	_, err := authz.Require(ctx, authz.PermPeopleWrite)
	return err
}

// CommitImport imports the CSV: all or nothing. A file with errors saves nothing
// and returns ErrImportInvalid with the report. Needs units.write and people.write.
func (s *Service) CommitImport(ctx context.Context, data []byte) (*ImportReport, error) {
	if err := requireImport(ctx); err != nil {
		return nil, err
	}
	condo, err := s.condo(ctx)
	if err != nil {
		return nil, err
	}
	var rep *ImportReport
	err = s.trail.Atomic(ctx, func(ctx context.Context) (audit.Entry, error) {
		plan, aerr := s.analyze(ctx, condo, data)
		if aerr != nil {
			return audit.Entry{}, aerr
		}
		rep = &plan.report
		if !rep.Valid {
			return audit.Entry{}, ErrImportInvalid
		}
		if aerr = s.apply(ctx, condo, plan); aerr != nil {
			return audit.Entry{}, aerr
		}
		return audit.Entry{Action: audit.ActionCreate, Resource: resImport, ResourceID: "csv",
			After: map[string]int{"rows": rep.Rows, "blocks": rep.NewBlocks, "units": rep.NewUnits, "people": rep.NewPeople, "links": rep.NewLinks, "unchanged": rep.Unchanged}}, nil
	})
	if errors.Is(err, ErrImportInvalid) {
		return rep, err
	}
	if db.IsExclusionViolation(err) || db.IsUniqueViolation(err) {
		return rep, conflictf("the data changed while importing; preview again")
	}
	return rep, err
}

func (s *Service) apply(ctx context.Context, condo uuid.UUID, plan *importPlan) error {
	blockIDs := map[string]uuid.UUID{}
	blockOf := func(name string) (*uuid.UUID, error) {
		if name == "" {
			return nil, nil
		}
		if id, ok := blockIDs[name]; ok {
			return &id, nil
		}
		id, err := s.q.FindBlockByName(ctx, dbgen.FindBlockByNameParams{CondominiumID: condo, Name: name})
		if db.IsNoRows(err) {
			id = uuid.New()
			_, err = s.q.InsertBlock(ctx, dbgen.InsertBlockParams{ID: id, CondominiumID: condo, Name: name})
		}
		if err != nil {
			return nil, err
		}
		blockIDs[name] = id
		return &id, nil
	}
	condoText, _ := tenant.CondominiumID(ctx)
	for _, u := range plan.units {
		if !u.isNew {
			continue
		}
		block, err := blockOf(u.key.block)
		if err != nil {
			return err
		}
		if _, err = s.q.InsertResident(ctx, dbgen.InsertResidentParams{ID: u.id, CondominiumID: condo, CondominiumRef: condoText,
			UnitNumber: u.key.number, Floor: int32(u.floor), OwnerID: "", BlockID: block}); err != nil { //nolint:gosec // floor is range-checked while parsing
			return err
		}
	}
	for _, p := range plan.people {
		if !p.isNew {
			continue
		}
		if err := s.q.InsertPerson(ctx, dbgen.InsertPersonParams{ID: p.id, CondominiumID: condo, FullName: p.name, Cpf: p.cpf, Email: p.email, Phone: p.phone}); err != nil {
			return err
		}
	}
	for _, l := range plan.links {
		if err := s.q.InsertLink(ctx, dbgen.InsertLinkParams{ID: uuid.New(), CondominiumID: condo, UnitID: l.unit.id, PersonID: l.person.id, Kind: l.kind, StartsOn: l.starts}); err != nil {
			return linkConflict(err)
		}
	}
	return nil
}
