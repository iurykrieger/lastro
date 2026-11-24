package lastro

// Driving the application through its surfaces, over the protocols the market already uses (as the TS SDK's drive.ts):
//
//	screen / route  W3C WebDriver with any WebDriver server, or Appium for Android and iOS (appium:* capabilities)
//	api             HTTP, called directly
//	command         a process
//	any kind        Lastro's own JSON lines protocol, only when no standard protocol fits
//
// `.lastro/config.json` names them under `drivers`. Lastro ships no browser, device or Appium server. A driver that
// fails to start or times out (30 s per action by default) fails the scenario with the driver's error, never passes it.

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/png"
	"io"
	"math"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// DriverConfig is `drivers` in .lastro/config.json; each surface driver is kept as written.
type DriverConfig struct {
	Screen    json.RawMessage `json:"screen,omitempty"`
	Route     json.RawMessage `json:"route,omitempty"`
	API       json.RawMessage `json:"api,omitempty"`
	Command   json.RawMessage `json:"command,omitempty"`
	TimeoutMs int             `json:"timeoutMs,omitempty"`
	Data      *struct {
		Command string `json:"command"`
		Cwd     string `json:"cwd,omitempty"`
	} `json:"data,omitempty"`
}

type surfaceDriver struct {
	WebDriver    *string           `json:"webdriver"`
	Appium       *string           `json:"appium"`
	Capabilities json.RawMessage   `json:"capabilities"`
	BaseURL      *string           `json:"baseUrl"`
	Headers      map[string]string `json:"headers"`
	Process      bool              `json:"process"`
	JSONLines    *string           `json:"jsonlines"`
	Cwd          string            `json:"cwd"`
}

// DefaultTimeout is the time a driver has for each action.
const DefaultTimeout = 30 * time.Second

// DriverError is a driver that failed to start, broke or timed out.
type DriverError struct{ Msg string }

func (e *DriverError) Error() string { return e.Msg }

func (d *DriverConfig) driverFor(kind string) (surfaceDriver, bool) {
	var raw json.RawMessage
	if d != nil {
		switch kind {
		case "screen":
			raw = d.Screen
		case "route":
			raw = d.Route
			if len(raw) == 0 {
				raw = d.Screen
			}
		case "api":
			raw = d.API
		case "command":
			raw = d.Command
		}
	}
	var s surfaceDriver
	if len(raw) == 0 || json.Unmarshal(raw, &s) != nil {
		return s, false
	}
	return s, true
}

// ProtocolFor is the protocol a surface kind is driven over: webdriver, appium, http, process, jsonlines or none.
func ProtocolFor(kind string, d *DriverConfig) string {
	s, ok := d.driverFor(kind)
	switch {
	case !ok:
		return "none"
	case s.JSONLines != nil:
		return "jsonlines"
	case kind == "screen" || kind == "route":
		if s.Appium != nil {
			return "appium"
		}
		if s.WebDriver != nil {
			return "webdriver"
		}
	case kind == "api":
		if s.BaseURL != nil {
			return "http"
		}
	case s.Process:
		return "process"
	}
	return "none"
}

// Action is one input to a surface, and Observation what the driver observed after it.
type (
	Action      = map[string]any
	Observation = map[string]any
)

// Session is one open surface.
type Session interface {
	Act(a Action) (Observation, error)
	// Screenshot is a PNG of the screen (screen and route surfaces), nil otherwise.
	Screenshot() ([]byte, error)
	Close()
}

func (d *DriverConfig) timeout() time.Duration {
	if d != nil && d.TimeoutMs > 0 {
		return time.Duration(d.TimeoutMs) * time.Millisecond
	}
	return DefaultTimeout
}

func withTimeout[T any](ms time.Duration, what string, f func() (T, error)) (T, error) {
	type res struct {
		v   T
		err error
	}
	ch := make(chan res, 1)
	go func() {
		v, err := f()
		ch <- res{v, err}
	}()
	select {
	case r := <-ch:
		return r.v, r.err
	case <-time.After(ms):
		var zero T
		return zero, &DriverError{fmt.Sprintf("%s timed out after %d ms", what, ms.Milliseconds())}
	}
}

// OpenSession opens a session on a surface; root is the folder relative commands run in.
func OpenSession(kind, locator string, d *DriverConfig, root string) (Session, error) {
	protocol := ProtocolFor(kind, d)
	s, _ := d.driverFor(kind)
	ms := d.timeout()
	cwd := root
	if s.Cwd != "" {
		cwd = filepath.Join(root, s.Cwd)
		if filepath.IsAbs(s.Cwd) {
			cwd = s.Cwd
		}
	}
	switch protocol {
	case "webdriver", "appium":
		server := *s.WebDriver
		if protocol == "appium" {
			server = *s.Appium
		}
		base := ""
		if s.BaseURL != nil {
			base = *s.BaseURL
		}
		return withTimeout(ms, "starting the "+protocol+" driver", func() (Session, error) { return webdriverSession(server, s.Capabilities, base, locator, ms) })
	case "http":
		return &httpSession{base: *s.BaseURL, headers: s.Headers}, nil
	case "process":
		return &processSession{cwd: cwd, locator: locator, ms: ms}, nil
	case "jsonlines":
		return jsonLinesSession(*s.JSONLines, cwd, kind, locator, ms)
	}
	return nil, &DriverError{fmt.Sprintf("no driver configured for %s surfaces: add one under \"drivers\" in .lastro/config.json", kind)}
}

// ---------------------------------------------------------------------------
// W3C WebDriver (and Appium, which speaks the same protocol)
// ---------------------------------------------------------------------------

func wd(server, method, path string, body any) (any, error) {
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, strings.TrimSuffix(server, "/")+path, rd)
	if err != nil {
		return nil, err
	}
	req.Header.Set("content-type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, &DriverError{fmt.Sprintf("WebDriver %s %s: %s", method, path, err)}
	}
	defer res.Body.Close()
	var out struct {
		Value any `json:"value"`
	}
	_ = json.NewDecoder(res.Body).Decode(&out)
	if m, ok := out.Value.(map[string]any); res.StatusCode >= 300 || (ok && m["error"] != nil) {
		msg := fmt.Sprint(res.StatusCode)
		if ok && m["message"] != nil {
			msg = fmt.Sprint(m["message"])
		}
		return nil, &DriverError{fmt.Sprintf("WebDriver %s %s: %s", method, path, msg)}
	}
	return out.Value, nil
}

type webdriver struct {
	server, session, base string
	ms                    time.Duration
}

var httpURL = regexp.MustCompile(`^https?:`)

func webdriverSession(server string, caps json.RawMessage, base, locator string, ms time.Duration) (Session, error) {
	if len(caps) == 0 {
		caps = json.RawMessage("{}")
	}
	v, err := wd(server, "POST", "/session", map[string]any{"capabilities": map[string]any{"alwaysMatch": caps}})
	if err != nil {
		return nil, err
	}
	m, _ := v.(map[string]any)
	w := &webdriver{server: server, session: "/session/" + fmt.Sprint(m["sessionId"]), base: base, ms: ms}
	if base != "" || httpURL.MatchString(locator) {
		if _, err := wd(server, "POST", w.session+"/url", map[string]any{"url": w.url(locator)}); err != nil {
			w.Close()
			return nil, err
		}
	}
	return w, nil
}

func (w *webdriver) url(to string) string {
	if w.base != "" && strings.HasPrefix(to, "/") {
		return strings.TrimSuffix(w.base, "/") + to
	}
	return to
}

func (w *webdriver) element(selector string) (string, error) {
	using, value := "css selector", selector
	if strings.HasPrefix(selector, "~") {
		using, value = "accessibility id", selector[1:]
	} else if strings.HasPrefix(selector, "//") {
		using = "xpath"
	}
	v, err := wd(w.server, "POST", w.session+"/element", map[string]any{"using": using, "value": value})
	if err != nil {
		return "", err
	}
	for _, id := range v.(map[string]any) {
		return fmt.Sprint(id), nil
	}
	return "", &DriverError{"no element " + selector}
}

func (w *webdriver) Act(a Action) (Observation, error) {
	key, _ := CanonicalJSON(a)
	return withTimeout(w.ms, "action "+key, func() (Observation, error) {
		if to, ok := a["navigate"].(string); ok {
			if _, err := wd(w.server, "POST", w.session+"/url", map[string]any{"url": w.url(to)}); err != nil {
				return nil, err
			}
		}
		if sel, ok := a["click"].(string); ok {
			el, err := w.element(sel)
			if err == nil {
				_, err = wd(w.server, "POST", w.session+"/element/"+el+"/click", map[string]any{})
			}
			if err != nil {
				return nil, err
			}
		}
		if sel, ok := a["type"].(string); ok {
			el, err := w.element(sel)
			if err == nil {
				text := ""
				if a["text"] != nil {
					text = fmt.Sprint(a["text"])
				}
				_, err = wd(w.server, "POST", w.session+"/element/"+el+"/value", map[string]any{"text": text})
			}
			if err != nil {
				return nil, err
			}
		}
		obs := Observation{}
		if sel, ok := a["read"].(string); ok {
			el, err := w.element(sel)
			var t any
			if err == nil {
				t, err = wd(w.server, "GET", w.session+"/element/"+el+"/text", nil)
			}
			if err != nil {
				return nil, err
			}
			obs["text"] = t
		}
		u, _ := wd(w.server, "GET", w.session+"/url", nil)
		obs["url"] = u
		return obs, nil
	})
}

func (w *webdriver) Screenshot() ([]byte, error) {
	v, err := withTimeout(w.ms, "taking a screenshot", func() (any, error) { return wd(w.server, "GET", w.session+"/screenshot", nil) })
	if err != nil {
		return nil, err
	}
	return base64.StdEncoding.DecodeString(fmt.Sprint(v))
}

func (w *webdriver) Close() { _, _ = wd(w.server, "DELETE", w.session, nil) }

// ---------------------------------------------------------------------------
// HTTP and processes
// ---------------------------------------------------------------------------

type httpSession struct {
	base    string
	headers map[string]string
}

func (h *httpSession) Act(a Action) (Observation, error) {
	method, path := "GET", "/"
	if m, ok := a["method"].(string); ok {
		method = m
	}
	if p, ok := a["path"].(string); ok {
		path = p
	}
	var rd io.Reader
	if a["body"] != nil {
		b, _ := json.Marshal(a["body"])
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, strings.TrimSuffix(h.base, "/")+path, rd)
	if err != nil {
		return nil, &DriverError{err.Error()}
	}
	for k, v := range h.headers {
		req.Header.Set(k, v)
	}
	if rd != nil {
		req.Header.Set("content-type", "application/json")
	}
	if hs, ok := a["headers"].(map[string]any); ok {
		for k, v := range hs {
			req.Header.Set(k, fmt.Sprint(v))
		}
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, &DriverError{err.Error()}
	}
	defer res.Body.Close()
	text, _ := io.ReadAll(res.Body)
	var body any = string(text)
	var parsed any
	dec := json.NewDecoder(bytes.NewReader(text))
	dec.UseNumber()
	if dec.Decode(&parsed) == nil {
		body = normalizeValue(parsed)
	}
	return Observation{"status": float64(res.StatusCode), "body": body}, nil
}

func (h *httpSession) Screenshot() ([]byte, error) { return nil, nil }
func (h *httpSession) Close()                      {}

type processSession struct {
	cwd, locator string
	ms           time.Duration
}

func shell(command, cwd string) *exec.Cmd {
	cmd := exec.Command("sh", "-c", command)
	cmd.Dir = cwd
	return cmd
}

func (p *processSession) Act(a Action) (Observation, error) {
	line, ok := a["run"].(string)
	if !ok {
		parts := []string{p.locator}
		if args, ok := a["args"].([]any); ok {
			for _, x := range args {
				parts = append(parts, fmt.Sprint(x))
			}
		}
		line = strings.Join(parts, " ")
	}
	ctx, cancel := context.WithTimeout(context.Background(), p.ms)
	defer cancel()
	cmd := exec.CommandContext(ctx, "sh", "-c", line)
	cmd.Dir = p.cwd
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if in, ok := a["stdin"].(string); ok {
		cmd.Stdin = strings.NewReader(in)
	}
	err := cmd.Run()
	if ctx.Err() != nil {
		return nil, &DriverError{fmt.Sprintf("running %s timed out after %d ms", line, p.ms.Milliseconds())}
	}
	code := 0
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		code = exit.ExitCode()
	} else if err != nil {
		return nil, &DriverError{fmt.Sprintf("could not run %s: %s", line, err)}
	}
	return Observation{"exitCode": float64(code), "stdout": stdout.String(), "stderr": stderr.String()}, nil
}

func (p *processSession) Screenshot() ([]byte, error) { return nil, nil }
func (p *processSession) Close()                      {}

// ---------------------------------------------------------------------------
// Lastro's JSON lines protocol: only when no standard protocol fits
// ---------------------------------------------------------------------------

// The driver is a process. Lastro writes one JSON object per line and reads one back:
//
//	-> {"open": {"kind": "screen", "locator": "/chats"}}   <- {"ok": true}
//	-> {"act": {...action}}                                <- {"observed": {...}, "screenshot"?: "<base64 PNG>"}
//	-> {"screenshot": true}                                <- {"screenshot": "<base64 PNG>"}
//	-> {"close": true}                                     <- {"ok": true}
//
// Any answer with "error" fails the step with that error.
type jsonLines struct {
	cmd   *exec.Cmd
	in    io.WriteCloser
	lines chan string
	ms    time.Duration
	last  []byte
}

func jsonLinesSession(command, cwd, kind, locator string, ms time.Duration) (Session, error) {
	cmd := shell(command, cwd)
	in, _ := cmd.StdinPipe()
	out, _ := cmd.StdoutPipe()
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return nil, &DriverError{"the driver could not start: " + err.Error()}
	}
	j := &jsonLines{cmd: cmd, in: in, lines: make(chan string, 16), ms: ms}
	go func() {
		sc := bufio.NewScanner(out)
		sc.Buffer(make([]byte, 1<<20), 64<<20)
		for sc.Scan() {
			j.lines <- sc.Text()
		}
		close(j.lines)
	}()
	if _, err := j.ask(map[string]any{"open": map[string]any{"kind": kind, "locator": locator}}, "starting the JSON lines driver"); err != nil {
		j.stop()
		return nil, err
	}
	return j, nil
}

func (j *jsonLines) ask(msg any, what string) (map[string]any, error) {
	b, _ := json.Marshal(msg)
	if _, err := j.in.Write(append(b, '\n')); err != nil {
		return nil, &DriverError{"the driver exited"}
	}
	select {
	case line, ok := <-j.lines:
		if !ok {
			return nil, &DriverError{"the driver exited"}
		}
		var out map[string]any
		dec := json.NewDecoder(strings.NewReader(line))
		dec.UseNumber()
		if err := dec.Decode(&out); err != nil {
			return nil, &DriverError{"the driver answered with something that is not JSON: " + line}
		}
		if out["error"] != nil {
			return nil, &DriverError{fmt.Sprint(out["error"])}
		}
		return out, nil
	case <-time.After(j.ms):
		return nil, &DriverError{fmt.Sprintf("%s timed out after %d ms", what, j.ms.Milliseconds())}
	}
}

func (j *jsonLines) Act(a Action) (Observation, error) {
	key, _ := CanonicalJSON(a)
	out, err := j.ask(map[string]any{"act": a}, "action "+key)
	if err != nil {
		return nil, err
	}
	j.last = nil
	if s, ok := out["screenshot"].(string); ok {
		j.last, _ = base64.StdEncoding.DecodeString(s)
	}
	obs, _ := normalizeValue(out["observed"]).(map[string]any)
	if obs == nil {
		obs = Observation{}
	}
	return obs, nil
}

func (j *jsonLines) Screenshot() ([]byte, error) {
	if j.last != nil {
		return j.last, nil
	}
	out, err := j.ask(map[string]any{"screenshot": true}, "taking a screenshot")
	if err != nil {
		return nil, err
	}
	if s, ok := out["screenshot"].(string); ok {
		return base64.StdEncoding.DecodeString(s)
	}
	return nil, nil
}

func (j *jsonLines) Close() {
	_, _ = j.ask(map[string]any{"close": true}, "closing the driver")
	j.stop()
}

func (j *jsonLines) stop() {
	_ = j.in.Close()
	if j.cmd.Process != nil {
		_ = j.cmd.Process.Kill()
	}
	_ = j.cmd.Wait()
}

// normalizeValue turns json.Number into float64, recursively, as JSON.parse reads numbers.
func normalizeValue(v any) any {
	switch x := v.(type) {
	case json.Number:
		f, _ := x.Float64()
		return f
	case map[string]any:
		for k, y := range x {
			x[k] = normalizeValue(y)
		}
	case []any:
		for i, y := range x {
			x[i] = normalizeValue(y)
		}
	}
	return v
}

// ---------------------------------------------------------------------------
// Driven scenarios and properties
// ---------------------------------------------------------------------------

// Matches is true when every field of expected equals the observed one, recursively.
func Matches(expected, observed any) bool {
	switch e := expected.(type) {
	case map[string]any:
		o, ok := observed.(map[string]any)
		if !ok {
			return false
		}
		for k, v := range e {
			if !Matches(v, o[k]) {
				return false
			}
		}
		return true
	case []any:
		o, ok := observed.([]any)
		if !ok || len(o) != len(e) {
			return false
		}
		for i := range e {
			if !Matches(e[i], o[i]) {
				return false
			}
		}
		return true
	}
	return fmt.Sprint(normalizeValue(expected)) == fmt.Sprint(normalizeValue(observed)) && sameType(expected, observed)
}

func sameType(a, b any) bool {
	switch a.(type) {
	case nil:
		return b == nil
	case string:
		_, ok := b.(string)
		return ok
	case bool:
		_, ok := b.(bool)
		return ok
	}
	_, x := toFloat(a)
	_, y := toFloat(b)
	return x && y
}

func toFloat(v any) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case int:
		return float64(x), true
	case json.Number:
		f, err := x.Float64()
		return f, err == nil
	}
	return 0, false
}

// PixelShare is the share of pixels that differ between two PNGs (1 when their sizes differ or one is unreadable).
func PixelShare(a, b []byte) float64 {
	x, err1 := png.Decode(bytes.NewReader(a))
	y, err2 := png.Decode(bytes.NewReader(b))
	if err1 != nil || err2 != nil || x.Bounds().Size() != y.Bounds().Size() {
		return 1
	}
	bx, by := x.Bounds(), y.Bounds()
	n := bx.Dx() * bx.Dy()
	if n == 0 {
		return 0
	}
	changed := 0
	for j := 0; j < bx.Dy(); j++ {
		for i := 0; i < bx.Dx(); i++ {
			if !samePixel(x, y, bx.Min.X+i, bx.Min.Y+j, by.Min.X+i, by.Min.Y+j) {
				changed++
			}
		}
	}
	return float64(changed) / float64(n)
}

func samePixel(x, y image.Image, xi, xj, yi, yj int) bool {
	r1, g1, b1, a1 := x.At(xi, xj).RGBA()
	r2, g2, b2, a2 := y.At(yi, yj).RGBA()
	return r1>>8 == r2>>8 && g1>>8 == g2>>8 && b1>>8 == b2>>8 && a1>>8 == a2>>8
}

// Drivers runs driven scenarios and properties through the configured drivers; references are read from dir.
type Drivers struct {
	Config *DriverConfig
	// Dir is the workspace (.lastro/); Root the folder commands run in.
	Dir, Root string
	// Open opens a session (OpenSession by default; tests replace it).
	Open func(kind, locator string, d *DriverConfig, root string) (Session, error)
}

func (d Drivers) open(kind, locator string) (Session, error) {
	if d.Open != nil {
		return d.Open(kind, locator, d.Config, d.Root)
	}
	return OpenSession(kind, locator, d.Config, d.Root)
}

type drivenStep struct {
	Input     map[string]any `json:"input"`
	Expect    map[string]any `json:"expect"`
	Reference string         `json:"reference,omitempty"`
}

// Run runs one driven scenario or property.
func (d Drivers) Run(_ context.Context, pr ProofEntry) (DrivenOutcome, error) {
	var surface struct {
		Kind    string `json:"kind"`
		Locator string `json:"locator"`
	}
	_ = json.Unmarshal(pr.Check.Surface, &surface)
	screen := surface.Kind == "screen" || surface.Kind == "route"
	if pr.Check.Kind == "driven" {
		var steps []drivenStep
		dec := json.NewDecoder(bytes.NewReader(pr.Check.Steps))
		dec.UseNumber()
		_ = dec.Decode(&steps)
		s, err := d.open(surface.Kind, surface.Locator)
		if err != nil {
			return DrivenOutcome{Verdict: "violated", Note: "driver error: " + err.Error()}, nil
		}
		defer s.Close()
		for k, st := range steps {
			in := normalizeValue(st.Input).(map[string]any)
			obs, err := s.Act(in)
			if err != nil {
				return DrivenOutcome{Verdict: "violated", Note: "driver error: " + err.Error()}, nil
			}
			problem := ""
			if !Matches(normalizeValue(st.Expect), obs) {
				e, _ := CanonicalJSON(normalizeValue(st.Expect))
				o, _ := CanonicalJSON(obs)
				problem = fmt.Sprintf("expected %s, observed %s", e, o)
			}
			if st.Reference != "" && screen {
				if ref, err := os.ReadFile(filepath.Join(d.Dir, st.Reference)); err == nil {
					shot, _ := s.Screenshot()
					share := 1.0
					if shot != nil {
						share = PixelShare(ref, shot)
					}
					if share > pr.Check.Tolerance {
						p := fmt.Sprintf("%.2f%% of pixels changed, over the %.2f%% tolerance", share*100, pr.Check.Tolerance*100)
						if problem != "" {
							p = problem + "; " + p
						}
						problem = p
					}
				}
			}
			if problem != "" {
				input := Env{"step": float64(k + 1)}
				for key, v := range in {
					input[key] = v
				}
				return DrivenOutcome{Verdict: "violated", Note: fmt.Sprintf("step %d: %s", k+1, problem), Counterexample: &ProofCounterexample{Input: envJSON(input, append([]string{"step"}, keysOf(st.Input)...)), Output: mustJSON(obs)}}, nil
			}
		}
		return DrivenOutcome{Verdict: "evidenced", Passed: true}, nil
	}
	// A property: fixed-seed action sequences, the invariant checked after every action.
	var actions []map[string]any
	dec := json.NewDecoder(bytes.NewReader(pr.Check.Actions))
	dec.UseNumber()
	_ = dec.Decode(&actions)
	for i := range actions {
		actions[i] = normalizeValue(actions[i]).(map[string]any)
	}
	holds := func(obs Observation) bool {
		ok, err := Holds(*pr.Check.Invariant, EvalContext{Input: Env{}, Output: Env(obs)})
		return err == nil && ok
	}
	const sequences, length = 100, 10
	next := rng(42)
	seqs := make([][]map[string]any, sequences)
	for i := range seqs {
		seqs[i] = make([]map[string]any, length)
		for k := range seqs[i] {
			seqs[i][k] = actions[int(math.Floor(next()*float64(len(actions))))]
		}
	}
	count := 0
	run := func(seq []map[string]any) (int, error) {
		s, err := d.open(surface.Kind, surface.Locator)
		if err != nil {
			return 0, err
		}
		defer s.Close()
		for k, a := range seq {
			count++
			obs, err := s.Act(a)
			if err != nil {
				return 0, err
			}
			if !holds(obs) {
				return k + 1, nil
			}
		}
		return 0, nil
	}
	for _, seq := range seqs {
		failAt, err := run(seq)
		if err != nil {
			return DrivenOutcome{Verdict: "violated", Note: "driver error: " + err.Error()}, nil
		}
		if failAt == 0 {
			continue
		}
		prefix := seq[:failAt]
		text, _ := CanonicalJSON(prefix)
		again, err := run(prefix)
		if err != nil {
			return DrivenOutcome{Verdict: "violated", Note: "driver error: " + err.Error()}, nil
		}
		ce := &ProofCounterexample{Input: mustJSON(obj{{"prefix", text}}), Output: json.RawMessage("null")}
		if again == 0 {
			return DrivenOutcome{Verdict: "violated", Note: "nondeterministic: the shortest failing prefix passed on replay", Counterexample: ce}, nil
		}
		return DrivenOutcome{Verdict: "violated", Counterexample: ce}, nil
	}
	bound := math.Min(1, 3/math.Max(1, float64(count)))
	return DrivenOutcome{Verdict: "evidenced", Passed: true, Note: fmt.Sprintf("%d sequences, %d actions; failure rate below %s (95%%)", sequences, count, jsNumber(bound))}, nil
}

func keysOf(m map[string]any) []string {
	out := []string{}
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
