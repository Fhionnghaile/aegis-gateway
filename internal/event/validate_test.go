package event

import (
	"errors"
	"testing"
)

func mustVal(t *testing.T) Validator {
	v, err := NewSchemaValidator("")
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func TestValidateOK(t *testing.T) {
	e, err := mustVal(t).Validate([]byte(`{"source":"edr","type":"Login_Fail","severity":"HIGH"}`))
	if err != nil {
		t.Fatalf("want ok, got %v", err)
	}
	if e.Severity != "high" || e.Type != "login_fail" || e.ID == "" {
		t.Fatalf("not normalised: %+v", e)
	}
}

func TestValidateRejects(t *testing.T) {
	for _, raw := range []string{
		`{"type":"x"}`,                       // no source
		`{"source":"x"}`,                     // no type
		`{"source":"x","type":"y","severity":"nope"}`, // bad severity
		`not json`,
		``,
	} {
		if _, err := mustVal(t).Validate([]byte(raw)); !errors.Is(err, ErrInvalid) {
			t.Fatalf("want ErrInvalid for %q, got %v", raw, err)
		}
	}
}
