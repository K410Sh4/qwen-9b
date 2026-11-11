package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strconv"
)

func objectSchema(properties map[string]any, required ...string) map[string]any {
	return map[string]any{"type": "object", "properties": properties, "required": required, "additionalProperties": false}
}

func responseSchema(out any) map[string]any {
	str := map[string]any{"type": "string"}
	num := map[string]any{"type": "number"}
	switch out.(type) {
	case *ProjectPlan:
		args := map[string]any{"type": "array", "items": str}
		item := objectSchema(map[string]any{"Name": str, "Args": args, "Stdin": str, "Stdout": str}, "Name", "Args", "Stdin", "Stdout")
		return objectSchema(map[string]any{"Title": str, "Summary": str, "Cases": map[string]any{"type": "array", "items": item}}, "Title", "Summary", "Cases")
	case *ProjectPatch:
		file := objectSchema(map[string]any{"Path": str, "Source": str}, "Path", "Source")
		return objectSchema(map[string]any{"Files": map[string]any{"type": "array", "items": file}}, "Files")
	case *Opportunity:
		return objectSchema(map[string]any{
			"Title": str, "Audience": str, "Problem": str, "Hypothesis": str, "Contradiction": str, "NextTask": str,
			"EvidenceIDs": map[string]any{"type": "array", "items": str},
			"Operation":   map[string]any{"type": "string", "enum": []string{"sum", "difference", "product", "ratio", "min", "max", "unsupported"}},
			"InputA":      str, "InputB": str,
		}, "Title", "Audience", "Problem", "Hypothesis", "Contradiction", "NextTask", "EvidenceIDs", "Operation", "InputA", "InputB")
	case *Tool:
		return objectSchema(map[string]any{
			"Name": str, "Operation": map[string]any{"type": "string", "enum": []string{"sum", "difference", "product", "ratio", "min", "max"}},
			"InputA": str, "InputB": str, "Result": str,
			"Tests": map[string]any{"type": "array", "items": objectSchema(map[string]any{"A": num, "B": num, "Expected": num}, "A", "B", "Expected")},
		}, "Name", "Operation", "InputA", "InputB", "Result", "Tests")
	case *Critique:
		return objectSchema(map[string]any{"Pass": map[string]any{"type": "boolean"}, "Reason": str, "Lesson": str, "Strategy": str},
			"Pass", "Reason", "Lesson", "Strategy")
	}
	return nil
}

var numericText = regexp.MustCompile(`^-?(0|[1-9][0-9]*)(\.[0-9]+)?([eE][+-]?[0-9]+)?$`)

func (e *Example) UnmarshalJSON(data []byte) error {
	var wire struct{ A, B, Expected json.RawMessage }
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	for _, field := range []struct {
		name string
		raw  json.RawMessage
		dst  *float64
	}{
		{"A", wire.A, &e.A}, {"B", wire.B, &e.B}, {"Expected", wire.Expected, &e.Expected},
	} {
		raw := bytes.TrimSpace(field.raw)
		value := string(raw)
		if len(raw) > 0 && raw[0] == '"' {
			if err := json.Unmarshal(raw, &value); err != nil {
				return fmt.Errorf("campo %s: %w", field.name, err)
			}
		}
		if !numericText.MatchString(value) {
			return fmt.Errorf("o teste requer número válido em %s", field.name)
		}
		number, err := strconv.ParseFloat(value, 64)
		if err != nil || math.IsInf(number, 0) || math.IsNaN(number) {
			return errors.New("número de teste fora do intervalo")
		}
		*field.dst = number
	}
	return nil
}
