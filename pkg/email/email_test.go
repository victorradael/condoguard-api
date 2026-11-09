package email_test

import (
	"testing"

	"github.com/victorradael/condoguard/api/pkg/email"
)

func TestNormalize(t *testing.T) {
	for in, want := range map[string]string{
		"ana@exemplo.test":       "ana@exemplo.test",
		"  Ana@Exemplo.TEST ":    "ana@exemplo.test",
		"a.b+tag@sub.exemplo.io": "a.b+tag@sub.exemplo.io",
	} {
		if got, err := email.Normalize(in); err != nil || got != want {
			t.Errorf("%q: got %q, %v", in, got, err)
		}
	}
	for _, in := range []string{"", "ana", "ana@", "@x.io", "Ana <ana@x.io>", "ana@localhost", "a b@x.io", "ana@x.io, bia@x.io"} {
		if _, err := email.Normalize(in); err == nil {
			t.Errorf("%q: expected an error", in)
		}
	}
}
