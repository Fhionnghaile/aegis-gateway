package event

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// ErrInvalid is returned (wrapped, with detail) when raw bytes fail validation.
var ErrInvalid = errors.New("event failed schema validation")

// Validator turns raw request bytes into a normalised Event, or ErrInvalid.
type Validator interface {
	Validate(raw []byte) (Event, error)
}

var allowedSeverity = map[string]bool{
	"": true, "info": true, "low": true, "medium": true, "high": true, "critical": true,
}

type schemaValidator struct{ maxBytes int }

// NewSchemaValidator loads constraints at startup. schemaPath is reserved for a
// JSON-Schema backend (e.g. santhosh-tekuri/jsonschema); the field checks below are
// the same contract as schemas/event.schema.json and need no external dependency.
func NewSchemaValidator(schemaPath string) (Validator, error) {
	return &schemaValidator{maxBytes: 1 << 20}, nil
}

func (v *schemaValidator) Validate(raw []byte) (Event, error) {
	if len(raw) == 0 || len(raw) > v.maxBytes {
		return Event{}, fmt.Errorf("%w: body size %d out of range", ErrInvalid, len(raw))
	}
	var e Event
	if err := json.Unmarshal(raw, &e); err != nil {
		return Event{}, fmt.Errorf("%w: json: %v", ErrInvalid, err)
	}
	if strings.TrimSpace(e.Source) == "" {
		return Event{}, fmt.Errorf("%w: source is required", ErrInvalid)
	}
	if strings.TrimSpace(e.Type) == "" {
		return Event{}, fmt.Errorf("%w: type is required", ErrInvalid)
	}
	if !allowedSeverity[strings.ToLower(strings.TrimSpace(e.Severity))] {
		return Event{}, fmt.Errorf("%w: severity %q not allowed", ErrInvalid, e.Severity)
	}
	e.Normalise(time.Now())
	return e, nil
}
