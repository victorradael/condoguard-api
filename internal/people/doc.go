// Package people manages who lives in and owns the units of a condominium:
// blocks, people (owners, co-owners, tenants, residents and dependents), the
// links between a person and a unit with their start and end days, the swap of
// an owner or tenant with its history, the invitation that turns a person into
// a user of the system, and the CSV import of units and residents.
//
// Personal data (CPF, e-mail, phone) is shown by level of need: a porteiro sees
// names and units, the conselho adds contact data and the masked CPF, the
// síndico and the administradora see everything, and a morador sees only the
// household of the units they belong to (and their own CPF). Whatever a caller
// may not see is reported as not found. The matrix is in docs/PAPEIS_E_PERMISSOES.md.
package people
