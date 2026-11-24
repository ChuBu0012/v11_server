package workspace

import (
	"reflect"
	"testing"
)

func TestFormula_Coordinates(t *testing.T) {
	tests := []struct {
		input string
		col   int
		row   int
		ok    bool
	}{
		{"A1", 0, 0, true},
		{"B2", 1, 1, true},
		{"Z10", 25, 9, true},
		{"AA1", 26, 0, true},
		{"ab12", 27, 11, true},
		{"", 0, 0, false},
		{"A", 0, 0, false},
		{"12", 0, 0, false},
		{"A0", 0, 0, false},
	}

	for _, tt := range tests {
		coord, err := ParseCoord(tt.input)
		if (err == nil) != tt.ok {
			t.Errorf("ParseCoord(%q) ok = %v, want %v (err: %v)", tt.input, err == nil, tt.ok, err)
			continue
		}
		if tt.ok {
			if coord.Col != tt.col || coord.Row != tt.row {
				t.Errorf("ParseCoord(%q) = (%d, %d), want (%d, %d)", tt.input, coord.Col, coord.Row, tt.col, tt.row)
			}
			restr := ColToLetters(coord.Col)
			if reCol, _ := LettersToCol(restr); reCol != coord.Col {
				t.Errorf("ColToLetters/LettersToCol roundtrip failed for %d: got %s -> %d", coord.Col, restr, reCol)
			}
		}
	}
}

func TestFormula_ExpandRange(t *testing.T) {
	expanded, err := ExpandRange("A1:B2")
	if err != nil {
		t.Fatalf("ExpandRange(A1:B2) failed: %v", err)
	}
	want := []string{"A1", "A2", "B1", "B2"}
	if !reflect.DeepEqual(expanded, want) {
		t.Errorf("ExpandRange(A1:B2) = %v, want %v", expanded, want)
	}

	single, err := ExpandRange("C5")
	if err != nil {
		t.Fatalf("ExpandRange(C5) failed: %v", err)
	}
	if len(single) != 1 || single[0] != "C5" {
		t.Errorf("ExpandRange(C5) = %v, want [C5]", single)
	}
}

func TestFormula_Arithmetic(t *testing.T) {
	engine := NewFormulaEngine(func(coord string) (string, any) {
		return "", nil
	})

	tests := []struct {
		formula string
		want    any
	}{
		{"=1+2*3", 7.0},
		{"=(10-2)/4", 2.0},
		{"=-5+10", 5.0},
		{"=100/4/5", 5.0},
		{"=10/0", ErrDivZero},
	}

	for _, tt := range tests {
		got := engine.Evaluate("", tt.formula)
		if got != tt.want {
			t.Errorf("Evaluate(%q) = %v, want %v", tt.formula, got, tt.want)
		}
	}
}

func TestFormula_CellReferencesAndAggregations(t *testing.T) {
	cells := map[string]SheetCell{
		"A1": {Raw: "10", Value: 10.0},
		"A2": {Raw: "20", Value: 20.0},
		"A3": {Raw: "30", Value: 30.0},
		"A4": {Raw: "40", Value: 40.0},
		"A5": {Raw: "50", Value: 50.0},
		"B1": {Raw: "=A1+5", Value: nil},
		"B2": {Raw: "=SUM(A1:A5)", Value: nil},
		"B3": {Raw: "=AVERAGE(A1:A5)", Value: nil},
		"B4": {Raw: "=COUNT(A1:A5)", Value: nil},
		"B5": {Raw: "=MIN(A1:A5)", Value: nil},
		"B6": {Raw: "=MAX(A1:A5)", Value: nil},
	}

	engine := NewFormulaEngine(func(coord string) (string, any) {
		c, ok := cells[coord]
		if !ok {
			return "", nil
		}
		return c.Raw, c.Value
	})

	if got := engine.Evaluate("B1", cells["B1"].Raw); got != 15.0 {
		t.Errorf("Evaluate(B1) = %v, want 15.0", got)
	}
	if got := engine.Evaluate("B2", cells["B2"].Raw); got != 150.0 {
		t.Errorf("Evaluate(B2: SUM) = %v, want 150.0", got)
	}
	if got := engine.Evaluate("B3", cells["B3"].Raw); got != 30.0 {
		t.Errorf("Evaluate(B3: AVERAGE) = %v, want 30.0", got)
	}
	if got := engine.Evaluate("B4", cells["B4"].Raw); got != 5.0 {
		t.Errorf("Evaluate(B4: COUNT) = %v, want 5.0", got)
	}
	if got := engine.Evaluate("B5", cells["B5"].Raw); got != 10.0 {
		t.Errorf("Evaluate(B5: MIN) = %v, want 10.0", got)
	}
	if got := engine.Evaluate("B6", cells["B6"].Raw); got != 50.0 {
		t.Errorf("Evaluate(B6: MAX) = %v, want 50.0", got)
	}
}

func TestFormula_CircularReferences(t *testing.T) {
	cells := map[string]SheetCell{
		"A1": {Raw: "=B1+1"},
		"B1": {Raw: "=A1+1"},
		"C1": {Raw: "=C1"}, // Self-reference
	}

	engine := NewFormulaEngine(func(coord string) (string, any) {
		return cells[coord].Raw, cells[coord].Value
	})

	gotA1 := engine.Evaluate("A1", cells["A1"].Raw)
	if gotA1 != ErrCircularRef {
		t.Errorf("Evaluate(A1 circular) = %v, want %q", gotA1, ErrCircularRef)
	}

	gotC1 := engine.Evaluate("C1", cells["C1"].Raw)
	if gotC1 != ErrCircularRef {
		t.Errorf("Evaluate(C1 self-ref) = %v, want %q", gotC1, ErrCircularRef)
	}
}

func TestFormula_EvaluateFullSheet(t *testing.T) {
	sheetData := &SheetData{
		Cells: map[string]SheetCell{
			"A1": {Raw: "100", Value: 100},
			"A2": {Raw: "200", Value: 200},
			"A3": {Raw: "=SUM(A1:A2)"},
		},
	}

	EvaluateFullSheet(sheetData)

	a3 := sheetData.Cells["A3"]
	if a3.Value != int64(300) && a3.Value != 300.0 {
		t.Errorf("EvaluateFullSheet A3 value = %v (%T), want 300", a3.Value, a3.Value)
	}
}
