package page_test

import (
	"net/http/httptest"
	"testing"

	"github.com/victorradael/condoguard/api/pkg/page"
)

func TestFromRequest(t *testing.T) {
	ok := map[string]page.Params{
		"/x":                     {Page: 1, Size: 20},
		"/x?page=3":              {Page: 3, Size: 20},
		"/x?page=2&pageSize=100": {Page: 2, Size: 100},
		"/x?pageSize=1":          {Page: 1, Size: 1},
	}
	for url, want := range ok {
		got, err := page.FromRequest(httptest.NewRequest("GET", url, nil))
		if err != nil || got != want {
			t.Errorf("%s: got %+v, %v; want %+v", url, got, err, want)
		}
	}
	for _, url := range []string{"/x?page=0", "/x?page=-1", "/x?page=a", "/x?pageSize=0", "/x?pageSize=101", "/x?page=99999999999"} {
		if _, err := page.FromRequest(httptest.NewRequest("GET", url, nil)); err == nil {
			t.Errorf("%s: expected an error", url)
		}
	}
}

func TestParams_LimitOffset(t *testing.T) {
	p := page.Params{Page: 3, Size: 25}
	if p.Limit() != 25 || p.Offset() != 50 {
		t.Errorf("got limit %d offset %d", p.Limit(), p.Offset())
	}
}

func TestSlice(t *testing.T) {
	all := []int{1, 2, 3, 4, 5}
	if got := page.Slice(all, page.Params{Page: 2, Size: 2}); len(got) != 2 || got[0] != 3 {
		t.Errorf("page 2: %v", got)
	}
	if got := page.Slice(all, page.Params{Page: 3, Size: 2}); len(got) != 1 || got[0] != 5 {
		t.Errorf("page 3: %v", got)
	}
	if got := page.Slice(all, page.Params{Page: 9, Size: 2}); got == nil || len(got) != 0 {
		t.Errorf("past the end: %v", got)
	}
}

func TestNew_NilItemsBecomeEmpty(t *testing.T) {
	r := page.New[int](nil, page.Default, 0)
	if r.Items == nil {
		t.Error("items must not be nil")
	}
}
