package utils

import (
	"reflect"
	"strings"
	"testing"
)

// testFilter mirrors the tagged shape used by the service filter structs.
type testFilter struct {
	Department string `field:"a.department" op:"=" validate:"omitempty"`
	Reference  string `field:"a.reference" op:"ILIKE" validate:"omitempty,max=100"`
	Status     string `field:"a.status" op:"IN" validate:"omitempty"`
	Name       string `field:"ap.full_name_hash" hash:"true" op:"="`
	Age        *int   `field:"a.age" op:">="`
	Skip       string `validate:"omitempty"`
}

func TestBuildWhereClause_StringAndILike(t *testing.T) {
	f := testFilter{Department: "Engineering", Reference: "app"}

	where, having, params, err := BuildWhereClause(&f)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if having != "" {
		t.Errorf("expected no having clause, got %q", having)
	}

	wantWhere := " WHERE a.department = $1 AND lower(a.reference) ILIKE lower($2)"
	if where != wantWhere {
		t.Errorf("where clause mismatch\n got: %q\nwant: %q", where, wantWhere)
	}
	wantParams := []any{"Engineering", "%app%"}
	if !reflect.DeepEqual(params, wantParams) {
		t.Errorf("params mismatch\n got: %#v\nwant: %#v", params, wantParams)
	}
}

func TestBuildWhereClause_InCondition(t *testing.T) {
	f := testFilter{Status: "submitted, accepted ,rejected"}

	where, _, params, err := BuildWhereClause(&f)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if where != " WHERE a.status IN ($1,$2,$3)" {
		t.Errorf("unexpected where clause: %q", where)
	}
	wantParams := []any{"submitted", "accepted", "rejected"}
	if !reflect.DeepEqual(params, wantParams) {
		t.Errorf("params mismatch\n got: %#v\nwant: %#v", params, wantParams)
	}
}

func TestBuildWhereClause_HashColumn(t *testing.T) {
	f := testFilter{Name: "John Doe"}

	where, _, params, err := BuildWhereClause(&f)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if where != " WHERE ap.full_name_hash = encode(digest($1, 'sha256'), 'hex')" {
		t.Errorf("unexpected where clause: %q", where)
	}
	if !reflect.DeepEqual(params, []any{"John Doe"}) {
		t.Errorf("params mismatch: %#v", params)
	}
}

func TestBuildWhereClause_PointerAndSkip(t *testing.T) {
	// nil pointer is skipped, empty string is skipped, untagged field is skipped.
	f := testFilter{}
	where, _, params, err := BuildWhereClause(&f)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if where != "" || params != nil {
		t.Fatalf("expected empty clause and params, got %q / %#v", where, params)
	}

	age := 7
	f.Age = &age
	where, _, params, err = BuildWhereClause(&f)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if where != " WHERE a.age >= $1" {
		t.Errorf("unexpected where clause: %q", where)
	}
	if !reflect.DeepEqual(params, []any{float64(7)}) {
		t.Errorf("params mismatch: %#v", params)
	}
}

func TestBuildWhereClause_ValidationError(t *testing.T) {
	f := testFilter{Reference: strings.Repeat("x", 101)}

	_, _, _, err := BuildWhereClause(&f)
	if err == nil {
		t.Fatal("expected validation error for over-long reference")
	}
}

func TestAppendCondition(t *testing.T) {
	if got := AppendCondition("", ""); got != "" {
		t.Errorf("expected empty, got %q", got)
	}
	if got := AppendCondition("", "a.id = $1"); got != " WHERE a.id = $1" {
		t.Errorf("unexpected: %q", got)
	}
	if got := AppendCondition(" WHERE a.id = $1", "a.status = $2"); got != " WHERE a.id = $1 AND a.status = $2" {
		t.Errorf("unexpected: %q", got)
	}
}

func TestAppendSearchClause(t *testing.T) {
	// Empty term is a no-op.
	where, params := AppendSearchClause(" WHERE a.id = $1", []any{"x"}, []string{"a.reference"}, []string{"ap.email_hash"}, "")
	if where != " WHERE a.id = $1" || !reflect.DeepEqual(params, []any{"x"}) {
		t.Fatalf("empty term must be a no-op, got %q / %#v", where, params)
	}

	where, params = AppendSearchClause("", nil,
		[]string{"a.reference", "a.department"},
		[]string{"ap.full_name_hash", "ap.email_hash"},
		"eng",
	)

	wantWhere := " WHERE (lower(a.reference) LIKE lower($1) OR lower(a.department) LIKE lower($1) OR " +
		"ap.full_name_hash = encode(digest($2, 'sha256'), 'hex') OR " +
		"ap.email_hash = encode(digest($2, 'sha256'), 'hex'))"
	if where != wantWhere {
		t.Errorf("where clause mismatch\n got: %q\nwant: %q", where, wantWhere)
	}
	if !reflect.DeepEqual(params, []any{"%eng%", "eng"}) {
		t.Errorf("params mismatch: %#v", params)
	}
}

func TestAppendSearchClause_KeepsNumbering(t *testing.T) {
	// Existing params must shift the placeholder numbering of the search group.
	where, params := AppendSearchClause(" WHERE a.status = $1", []any{"accepted"},
		[]string{"a.reference"}, nil, "x")
	if where != " WHERE a.status = $1 AND (lower(a.reference) LIKE lower($2))" {
		t.Errorf("unexpected where clause: %q", where)
	}
	if !reflect.DeepEqual(params, []any{"accepted", "%x%"}) {
		t.Errorf("params mismatch: %#v", params)
	}
}

func TestGeneratePaginationData(t *testing.T) {
	// Middle page: has both next and prev.
	meta := GeneratePaginationData(25, 10, 2)
	if meta["total"] != 25 || meta["per_page"] != 10 || meta["current"] != 2 || meta["pages"] != 3 {
		t.Errorf("unexpected base meta: %#v", meta)
	}
	if meta["next"] != 3 || meta["prev"] != 1 {
		t.Errorf("unexpected next/prev: %#v", meta)
	}

	// First page: no prev, has next.
	meta = GeneratePaginationData(25, 10, 1)
	if meta["prev"] != nil || meta["next"] != 2 {
		t.Errorf("unexpected first page meta: %#v", meta)
	}

	// Last page: no next, has prev.
	meta = GeneratePaginationData(25, 10, 3)
	if meta["next"] != nil || meta["prev"] != 2 {
		t.Errorf("unexpected last page meta: %#v", meta)
	}

	// Empty result set: zero pages, no next/prev.
	meta = GeneratePaginationData(0, 10, 1)
	if meta["pages"] != 0 || meta["next"] != nil || meta["prev"] != nil {
		t.Errorf("unexpected empty meta: %#v", meta)
	}
}
