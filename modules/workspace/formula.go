package workspace

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode"
)

const (
	MaxRecursionDepth = 32
	ErrCircularRef    = "#REF! CIRCULAR"
	ErrDivZero        = "#DIV/0!"
	ErrValue          = "#VALUE!"
	ErrFormulaName    = "#NAME?"
)

// CellCoord represents a 0-based column and row in the grid.
type CellCoord struct {
	Col int // 0 = A, 1 = B, 25 = Z, 26 = AA
	Row int // 0 = 1, 1 = 2
}

// String converts CellCoord back to standard spreadsheet notation (e.g. "B3").
func (c CellCoord) String() string {
	return ColToLetters(c.Col) + strconv.Itoa(c.Row+1)
}

// LettersToCol converts column letters (e.g. "A", "Z", "AA") to 0-based index.
func LettersToCol(letters string) (int, error) {
	letters = strings.ToUpper(strings.TrimSpace(letters))
	if letters == "" {
		return 0, errors.New("empty column letters")
	}
	col := 0
	for _, ch := range letters {
		if ch < 'A' || ch > 'Z' {
			return 0, fmt.Errorf("invalid column character: %c", ch)
		}
		col = col*26 + int(ch-'A'+1)
	}
	return col - 1, nil
}

// ColToLetters converts a 0-based column index to spreadsheet letters (0 -> "A", 25 -> "Z", 26 -> "AA").
func ColToLetters(col int) string {
	if col < 0 {
		return "A"
	}
	col++
	var result []byte
	for col > 0 {
		rem := (col - 1) % 26
		result = append([]byte{byte('A' + rem)}, result...)
		col = (col - 1) / 26
	}
	return string(result)
}

// ParseCoord parses a coordinate string like "A1", "b12", "AA3" into CellCoord.
func ParseCoord(s string) (CellCoord, error) {
	s = strings.ToUpper(strings.TrimSpace(s))
	if len(s) < 2 {
		return CellCoord{}, fmt.Errorf("invalid coordinate: %s", s)
	}

	colEnd := 0
	for colEnd < len(s) && s[colEnd] >= 'A' && s[colEnd] <= 'Z' {
		colEnd++
	}
	if colEnd == 0 || colEnd == len(s) {
		return CellCoord{}, fmt.Errorf("invalid coordinate: %s", s)
	}

	colStr := s[:colEnd]
	rowStr := s[colEnd:]

	col, err := LettersToCol(colStr)
	if err != nil {
		return CellCoord{}, err
	}

	rowNum, err := strconv.Atoi(rowStr)
	if err != nil || rowNum < 1 {
		return CellCoord{}, fmt.Errorf("invalid row number in %s: %w", s, err)
	}

	return CellCoord{Col: col, Row: rowNum - 1}, nil
}

// IsValidCoord checks whether the string is a valid cell coordinate.
func IsValidCoord(s string) bool {
	_, err := ParseCoord(s)
	return err == nil
}

// ExpandRange expands a range string like "A1:B3" into a slice of coordinates.
// If given a single coordinate like "A1", returns ["A1"].
func ExpandRange(r string) ([]string, error) {
	r = strings.ToUpper(strings.TrimSpace(r))
	parts := strings.Split(r, ":")
	if len(parts) == 1 {
		c, err := ParseCoord(parts[0])
		if err != nil {
			return nil, err
		}
		return []string{c.String()}, nil
	}
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid range syntax: %s", r)
	}

	start, err := ParseCoord(parts[0])
	if err != nil {
		return nil, err
	}
	end, err := ParseCoord(parts[1])
	if err != nil {
		return nil, err
	}

	minCol, maxCol := start.Col, end.Col
	if minCol > maxCol {
		minCol, maxCol = maxCol, minCol
	}
	minRow, maxRow := start.Row, end.Row
	if minRow > maxRow {
		minRow, maxRow = maxRow, minRow
	}

	// Guard against astronomically huge ranges that could exhaust memory
	totalCells := (maxCol - minCol + 1) * (maxRow - minRow + 1)
	if totalCells > 10000 {
		return nil, errors.New("range too large (max 10,000 cells)")
	}

	coords := make([]string, 0, totalCells)
	for col := minCol; col <= maxCol; col++ {
		for row := minRow; row <= maxRow; row++ {
			coords = append(coords, CellCoord{Col: col, Row: row}.String())
		}
	}
	return coords, nil
}

// FormulaEngine evaluates spreadsheet formulas with cycle detection and depth protection.
type FormulaEngine struct {
	cellGetter func(coord string) (string, any)
	visited    map[string]bool
	depth      int
}

// NewFormulaEngine constructs a formula evaluator given a function to read cell (raw, value).
func NewFormulaEngine(getter func(coord string) (string, any)) *FormulaEngine {
	return &FormulaEngine{
		cellGetter: getter,
		visited:    make(map[string]bool),
		depth:      0,
	}
}

// Evaluate evaluates a cell formula string (e.g. "=SUM(A1:A5)" or "=A1+B1").
func (fe *FormulaEngine) Evaluate(targetCoord string, formula string) any {
	targetCoord = strings.ToUpper(strings.TrimSpace(targetCoord))
	formula = strings.TrimSpace(formula)
	if !strings.HasPrefix(formula, "=") {
		return formula
	}

	// Reset evaluation tracking
	fe.visited = make(map[string]bool)
	fe.depth = 0

	return fe.evalCellFormula(targetCoord, strings.TrimPrefix(formula, "="))
}

func (fe *FormulaEngine) evalCellFormula(targetCoord string, exprStr string) any {
	if targetCoord != "" {
		if fe.visited[targetCoord] {
			return ErrCircularRef
		}
		fe.visited[targetCoord] = true
		defer func() {
			delete(fe.visited, targetCoord)
		}()
	}

	fe.depth++
	if fe.depth > MaxRecursionDepth {
		return ErrCircularRef
	}
	defer func() {
		fe.depth--
	}()

	p := newParser(exprStr, fe)
	val, err := p.parseExpression()
	if err != nil {
		if strings.HasPrefix(err.Error(), "#") {
			return err.Error()
		}
		return ErrValue
	}
	return val
}

// resolveCellValue resolves a coordinate to a numeric value or float64.
func (fe *FormulaEngine) resolveCellValue(coord string) (float64, error) {
	coord = strings.ToUpper(strings.TrimSpace(coord))
	if fe.visited[coord] {
		return 0, errors.New(ErrCircularRef)
	}

	raw, v := fe.cellGetter(coord)
	raw = strings.TrimSpace(raw)

	// If the referenced cell is itself a formula, evaluate it
	if strings.HasPrefix(raw, "=") {
		evaluated := fe.evalCellFormula(coord, strings.TrimPrefix(raw, "="))
		switch ev := evaluated.(type) {
		case float64:
			return ev, nil
		case int:
			return float64(ev), nil
		case string:
			if strings.HasPrefix(ev, "#") {
				return 0, errors.New(ev)
			}
			f, err := strconv.ParseFloat(ev, 64)
			if err != nil {
				return 0, nil // Non-numeric text defaults to 0 in math
			}
			return f, nil
		default:
			return 0, nil
		}
	}

	// Try using scalar v if available
	if v != nil {
		switch num := v.(type) {
		case float64:
			return num, nil
		case int:
			return float64(num), nil
		case int64:
			return float64(num), nil
		case string:
			f, err := strconv.ParseFloat(strings.TrimSpace(num), 64)
			if err == nil {
				return f, nil
			}
		}
	}

	if raw == "" {
		return 0, nil
	}

	f, err := strconv.ParseFloat(raw, 64)
	if err == nil {
		return f, nil
	}
	return 0, nil
}

// -----------------------------------------------------------------------------
// Recursive-Descent Expression Parser
// Grammar:
//   Expr     = Term (('+' | '-') Term)*
//   Term     = Factor (('*' | '/') Factor)*
//   Factor   = ('+' | '-')? Primary
//   Primary  = NUMBER | FUNC_CALL | RANGE_AGG | COORD | '(' Expr ')'
// -----------------------------------------------------------------------------

type parser struct {
	src    string
	pos    int
	engine *FormulaEngine
}

func newParser(src string, engine *FormulaEngine) *parser {
	return &parser{
		src:    src,
		pos:    0,
		engine: engine,
	}
}

func (p *parser) skipWhitespace() {
	for p.pos < len(p.src) && unicode.IsSpace(rune(p.src[p.pos])) {
		p.pos++
	}
}

func (p *parser) peek() byte {
	p.skipWhitespace()
	if p.pos >= len(p.src) {
		return 0
	}
	return p.src[p.pos]
}

func (p *parser) parseExpression() (float64, error) {
	left, err := p.parseTerm()
	if err != nil {
		return 0, err
	}

	for {
		p.skipWhitespace()
		ch := p.peek()
		if ch == '+' {
			p.pos++
			right, err := p.parseTerm()
			if err != nil {
				return 0, err
			}
			left += right
		} else if ch == '-' {
			p.pos++
			right, err := p.parseTerm()
			if err != nil {
				return 0, err
			}
			left -= right
		} else {
			break
		}
	}
	return left, nil
}

func (p *parser) parseTerm() (float64, error) {
	left, err := p.parseFactor()
	if err != nil {
		return 0, err
	}

	for {
		p.skipWhitespace()
		ch := p.peek()
		if ch == '*' {
			p.pos++
			right, err := p.parseFactor()
			if err != nil {
				return 0, err
			}
			left *= right
		} else if ch == '/' {
			p.pos++
			right, err := p.parseFactor()
			if err != nil {
				return 0, err
			}
			if right == 0 {
				return 0, errors.New(ErrDivZero)
			}
			left /= right
		} else {
			break
		}
	}
	return left, nil
}

func (p *parser) parseFactor() (float64, error) {
	p.skipWhitespace()
	ch := p.peek()
	if ch == '+' {
		p.pos++
		return p.parseFactor()
	}
	if ch == '-' {
		p.pos++
		val, err := p.parseFactor()
		if err != nil {
			return 0, err
		}
		return -val, nil
	}
	return p.parsePrimary()
}

func (p *parser) parsePrimary() (float64, error) {
	p.skipWhitespace()
	ch := p.peek()
	if ch == 0 {
		return 0, errors.New("unexpected end of formula")
	}

	// Parenthesized expression: '(' Expr ')'
	if ch == '(' {
		p.pos++
		val, err := p.parseExpression()
		if err != nil {
			return 0, err
		}
		p.skipWhitespace()
		if p.peek() != ')' {
			return 0, errors.New("missing closing parenthesis")
		}
		p.pos++
		return val, nil
	}

	// Numeric literal
	if unicode.IsDigit(rune(ch)) || ch == '.' {
		start := p.pos
		hasDot := ch == '.'
		p.pos++
		for p.pos < len(p.src) {
			c := p.src[p.pos]
			if c == '.' {
				if hasDot {
					break
				}
				hasDot = true
				p.pos++
			} else if unicode.IsDigit(rune(c)) {
				p.pos++
			} else {
				break
			}
		}
		val, err := strconv.ParseFloat(p.src[start:p.pos], 64)
		if err != nil {
			return 0, errors.New(ErrValue)
		}
		return val, nil
	}

	// Function call or Cell Coord or Identifier
	if unicode.IsLetter(rune(ch)) {
		start := p.pos
		for p.pos < len(p.src) && (unicode.IsLetter(rune(p.src[p.pos])) || unicode.IsDigit(rune(p.src[p.pos]))) {
			p.pos++
		}
		token := p.src[start:p.pos]

		p.skipWhitespace()
		// Check if it's a function call followed by '('
		if p.peek() == '(' {
			p.pos++
			return p.evalFunction(token)
		}

		// Check if it's part of a range (e.g. A1:B3) outside function call
		if p.peek() == ':' {
			p.pos++
			rangeStart := token
			rStartPos := p.pos
			for p.pos < len(p.src) && (unicode.IsLetter(rune(p.src[p.pos])) || unicode.IsDigit(rune(p.src[p.pos]))) {
				p.pos++
			}
			rangeEnd := p.src[rStartPos:p.pos]
			coords, err := ExpandRange(rangeStart + ":" + rangeEnd)
			if err != nil {
				return 0, err
			}
			// Standalone range without function defaults to sum of range
			sum := 0.0
			for _, c := range coords {
				v, err := p.engine.resolveCellValue(c)
				if err != nil {
					return 0, err
				}
				sum += v
			}
			return sum, nil
		}

		// Single cell coordinate (e.g. "A1", "B2")
		if IsValidCoord(token) {
			return p.engine.resolveCellValue(token)
		}

		return 0, fmt.Errorf("%s %s", ErrFormulaName, token)
	}

	return 0, fmt.Errorf("unexpected character: %c", ch)
}

func (p *parser) evalFunction(fnName string) (float64, error) {
	fn := strings.ToUpper(strings.TrimSpace(fnName))

	// Collect argument values
	var values []float64

	p.skipWhitespace()
	if p.peek() == ')' {
		p.pos++
	} else {
		for {
			p.skipWhitespace()

			// Check if argument is a range like A1:B5
			rangeStart := p.pos
			for p.pos < len(p.src) && (unicode.IsLetter(rune(p.src[p.pos])) || unicode.IsDigit(rune(p.src[p.pos]))) {
				p.pos++
			}
			token := p.src[rangeStart:p.pos]
			p.skipWhitespace()

			if p.peek() == ':' && IsValidCoord(token) {
				p.pos++ // consume ':'
				p.skipWhitespace()
				endStart := p.pos
				for p.pos < len(p.src) && (unicode.IsLetter(rune(p.src[p.pos])) || unicode.IsDigit(rune(p.src[p.pos]))) {
					p.pos++
				}
				endToken := p.src[endStart:p.pos]
				coords, err := ExpandRange(token + ":" + endToken)
				if err != nil {
					return 0, err
				}
				for _, coord := range coords {
					v, err := p.engine.resolveCellValue(coord)
					if err != nil {
						return 0, err
					}
					values = append(values, v)
				}
			} else {
				// Rewind and parse as normal expression
				p.pos = rangeStart
				val, err := p.parseExpression()
				if err != nil {
					return 0, err
				}
				values = append(values, val)
			}

			p.skipWhitespace()
			next := p.peek()
			if next == ',' {
				p.pos++
				continue
			} else if next == ')' {
				p.pos++
				break
			} else {
				return 0, errors.New("expected ',' or ')' in function arguments")
			}
		}
	}

	switch fn {
	case "SUM":
		sum := 0.0
		for _, v := range values {
			sum += v
		}
		return sum, nil

	case "AVERAGE", "AVG":
		if len(values) == 0 {
			return 0, errors.New(ErrDivZero)
		}
		sum := 0.0
		for _, v := range values {
			sum += v
		}
		return sum / float64(len(values)), nil

	case "COUNT":
		return float64(len(values)), nil

	case "MIN":
		if len(values) == 0 {
			return 0, nil
		}
		min := values[0]
		for _, v := range values[1:] {
			if v < min {
				min = v
			}
		}
		return min, nil

	case "MAX":
		if len(values) == 0 {
			return 0, nil
		}
		max := values[0]
		for _, v := range values[1:] {
			if v > max {
				max = v
			}
		}
		return max, nil

	default:
		return 0, fmt.Errorf("%s %s", ErrFormulaName, fn)
	}
}

// EvaluateFullSheet recalculates all formulas in a SheetData and updates cell.Value.
func EvaluateFullSheet(data *SheetData) {
	if data == nil || len(data.Cells) == 0 {
		return
	}

	getter := func(coord string) (string, any) {
		coord = strings.ToUpper(strings.TrimSpace(coord))
		if cell, exists := data.Cells[coord]; exists {
			return cell.Raw, cell.Value
		}
		return "", nil
	}

	engine := NewFormulaEngine(getter)

	for coord, cell := range data.Cells {
		raw := strings.TrimSpace(cell.Raw)
		if strings.HasPrefix(raw, "=") {
			res := engine.Evaluate(coord, raw)
			// Format float clean if whole number
			if f, ok := res.(float64); ok {
				if math.Trunc(f) == f && !math.IsNaN(f) && !math.IsInf(f, 0) {
					cell.Value = int64(f)
				} else {
					cell.Value = f
				}
			} else {
				cell.Value = res
			}
			data.Cells[coord] = cell
		}
	}
}
