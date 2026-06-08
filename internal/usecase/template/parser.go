package template

import "fmt"

// ParseError is returned for any malformed input. It carries Position so
// upstream validators can surface line/col in their error messages.
type ParseError struct {
	Pos Position
	Msg string
}

func (e *ParseError) Error() string {
	return fmt.Sprintf("template parse error at line %d col %d: %s", e.Pos.Line, e.Pos.Col, e.Msg)
}

// Parse turns a string into a slice of Segments. The slice may be empty
// (empty input) or all-literal (no `${{ }}` blocks).
func Parse(input string) ([]Segment, error) {
	p := &parser{input: input, line: 1, col: 1}
	return p.parse()
}

type parser struct {
	input string
	pos   int
	line  int
	col   int
}

func (p *parser) parse() ([]Segment, error) {
	var out []Segment
	var lit []byte

	flushLiteral := func() {
		if len(lit) > 0 {
			out = append(out, Literal{Text: string(lit)})
			lit = lit[:0]
		}
	}

	for p.pos < len(p.input) {
		if p.peekOpen() {
			flushLiteral()
			seg, err := p.parseRef()
			if err != nil {
				return nil, err
			}
			out = append(out, seg)
			continue
		}
		lit = append(lit, p.input[p.pos])
		p.advance(1)
	}
	flushLiteral()
	return out, nil
}

func (p *parser) peek2(s string) bool {
	return p.pos+2 <= len(p.input) && p.input[p.pos:p.pos+2] == s
}

// peekOpen reports whether the cursor is at the "${{" open sentinel.
func (p *parser) peekOpen() bool {
	return p.pos+3 <= len(p.input) && p.input[p.pos:p.pos+3] == "${{"
}

func (p *parser) advance(n int) {
	for i := 0; i < n && p.pos < len(p.input); i++ {
		if p.input[p.pos] == '\n' {
			p.line++
			p.col = 1
		} else {
			p.col++
		}
		p.pos++
	}
}

func (p *parser) here() Position {
	return Position{Line: p.line, Col: p.col, Offset: p.pos}
}

func (p *parser) parseRef() (Segment, error) {
	p.advance(3) // consume "${{"
	p.skipWS()

	if p.peekOpen() {
		return nil, &ParseError{Pos: p.here(), Msg: "nested ${{ inside template"}
	}

	nsStart := p.here()
	ns := p.readIdent(isIdentByte)
	if ns == "" {
		return nil, &ParseError{Pos: p.here(), Msg: "expected namespace after ${{"}
	}

	if !p.peekByte('.') {
		return nil, &ParseError{Pos: p.here(), Msg: "expected '.' after namespace"}
	}
	p.advance(1)

	switch ns {
	case "fixtures":
		return p.parseFixtureTail(nsStart)
	case "entry_points":
		return p.parseEntryPointTail(nsStart)
	case "inputs":
		return p.parseInputTail(nsStart)
	case "steps":
		return p.parseStepOutputTail(nsStart)
	default:
		return nil, &ParseError{Pos: nsStart, Msg: "unknown namespace: " + ns}
	}
}

func (p *parser) parseFixtureTail(refPos Position) (Segment, error) {
	id, ok := p.readKebabID()
	if !ok {
		return nil, &ParseError{Pos: p.here(), Msg: "expected fixture id"}
	}
	ref := FixtureRef{ID: id, Pos: refPos}
	for p.peekByte('.') {
		p.advance(1)
		key, ok := p.readJSONKey()
		if !ok {
			return nil, &ParseError{Pos: p.here(), Msg: "expected JSON key after '.'"}
		}
		ref.JSONPath = append(ref.JSONPath, key)
	}
	if err := p.expectClose(); err != nil {
		return nil, err
	}
	return ref, nil
}

func (p *parser) parseEntryPointTail(refPos Position) (Segment, error) {
	id, ok := p.readKebabID()
	if !ok {
		return nil, &ParseError{Pos: p.here(), Msg: "expected entry_point id"}
	}
	ref := EntryPointRef{ID: id, Pos: refPos}
	if p.peekByte('.') {
		p.advance(1)
		field := p.readIdent(isIdentByte)
		if field != "spec" {
			return nil, &ParseError{Pos: p.here(), Msg: "entry_points.<id> only accepts '.spec.<key>'; got '" + field + "'"}
		}
		if !p.peekByte('.') {
			return nil, &ParseError{Pos: p.here(), Msg: "expected '.' after 'spec'"}
		}
		p.advance(1)
		key, ok := p.readJSONKey()
		if !ok {
			return nil, &ParseError{Pos: p.here(), Msg: "expected spec field name"}
		}
		ref.SpecKey = key
		if p.peekByte('.') {
			return nil, &ParseError{Pos: p.here(), Msg: "entry_points spec access is single-key only"}
		}
	}
	if err := p.expectClose(); err != nil {
		return nil, err
	}
	return ref, nil
}

func (p *parser) parseInputTail(refPos Position) (Segment, error) {
	name, ok := p.readInputName()
	if !ok {
		return nil, &ParseError{Pos: p.here(), Msg: "expected input name"}
	}
	if p.peekByte('.') {
		return nil, &ParseError{Pos: p.here(), Msg: "inputs.<name> takes no further keys"}
	}
	if err := p.expectClose(); err != nil {
		return nil, err
	}
	return InputRef{Name: name, Pos: refPos}, nil
}

func (p *parser) parseStepOutputTail(refPos Position) (Segment, error) {
	stepID, ok := p.readKebabID()
	if !ok {
		return nil, &ParseError{Pos: p.here(), Msg: "expected step id"}
	}
	if !p.peekByte('.') {
		return nil, &ParseError{Pos: p.here(), Msg: "expected '.outputs.' after step id"}
	}
	p.advance(1)
	if seg := p.readIdent(isIdentByte); seg != "outputs" {
		return nil, &ParseError{Pos: p.here(), Msg: "steps.<id> only accepts '.outputs.<name>'; got '" + seg + "'"}
	}
	if !p.peekByte('.') {
		return nil, &ParseError{Pos: p.here(), Msg: "expected '.' after 'outputs'"}
	}
	p.advance(1)
	name, ok := p.readKebabID()
	if !ok {
		return nil, &ParseError{Pos: p.here(), Msg: "expected output name"}
	}
	if err := p.expectClose(); err != nil {
		return nil, err
	}
	return StepOutputRef{StepID: stepID, Name: name, Pos: refPos}, nil
}

func (p *parser) expectClose() error {
	p.skipWS()
	if !p.peek2("}}") {
		return &ParseError{Pos: p.here(), Msg: "expected '}}'"}
	}
	p.advance(2)
	return nil
}

func (p *parser) skipWS() {
	for p.pos < len(p.input) {
		c := p.input[p.pos]
		if c == ' ' || c == '\t' {
			p.advance(1)
			continue
		}
		return
	}
}

func (p *parser) peekByte(c byte) bool {
	return p.pos < len(p.input) && p.input[p.pos] == c
}

func (p *parser) readIdent(pred func(byte) bool) string {
	start := p.pos
	for p.pos < len(p.input) && pred(p.input[p.pos]) {
		p.advance(1)
	}
	return p.input[start:p.pos]
}

// readKebabID reads ID matching ^[a-z][a-z0-9-]{0,127}$. Returns ("", false)
// if the first byte is not [a-z] or the id is over 128 chars.
func (p *parser) readKebabID() (string, bool) {
	if p.pos >= len(p.input) {
		return "", false
	}
	c := p.input[p.pos]
	if !(c >= 'a' && c <= 'z') {
		return "", false
	}
	start := p.pos
	for p.pos < len(p.input) {
		c := p.input[p.pos]
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-' {
			p.advance(1)
			continue
		}
		break
	}
	id := p.input[start:p.pos]
	if len(id) > 128 {
		return "", false
	}
	return id, true
}

// readInputName reads an input name matching ^[a-z][a-z0-9_-]{0,127}$.
// Unlike readKebabID (used for schema Ids), input names are snake_case by
// convention (e.g. `base_url`, `expect_status`), so the underscore is part
// of the token. The env binder normalizes '-'/'_' uniformly, so both
// separators are accepted here. Returns ("", false) if the first byte is not
// [a-z] or the name exceeds 128 chars.
func (p *parser) readInputName() (string, bool) {
	if p.pos >= len(p.input) {
		return "", false
	}
	c := p.input[p.pos]
	if !(c >= 'a' && c <= 'z') {
		return "", false
	}
	start := p.pos
	for p.pos < len(p.input) {
		c := p.input[p.pos]
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-' || c == '_' {
			p.advance(1)
			continue
		}
		break
	}
	name := p.input[start:p.pos]
	if len(name) > 128 {
		return "", false
	}
	return name, true
}

// readJSONKey reads JSONKEY matching ^[a-zA-Z_][a-zA-Z0-9_-]*$.
func (p *parser) readJSONKey() (string, bool) {
	if p.pos >= len(p.input) {
		return "", false
	}
	c := p.input[p.pos]
	if !(isAlpha(c) || c == '_') {
		return "", false
	}
	start := p.pos
	for p.pos < len(p.input) {
		c := p.input[p.pos]
		if isAlpha(c) || isDigit(c) || c == '_' || c == '-' {
			p.advance(1)
			continue
		}
		break
	}
	return p.input[start:p.pos], true
}

func isAlpha(c byte) bool     { return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') }
func isDigit(c byte) bool     { return c >= '0' && c <= '9' }
func isIdentByte(c byte) bool { return isAlpha(c) || c == '_' }
