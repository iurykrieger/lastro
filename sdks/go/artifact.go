package lastro

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode"
)

// Intent artifact: creation, human review and freezing.
//
// Extraction is non-deterministic. Review is where a human catches "the AI
// misunderstood". Freeze pins the reviewed artifact with a content hash;
// everything downstream is deterministic and bound to that hash. The hash is
// the same one the TS SDK computes, so an artifact frozen by one SDK is intact
// for the other.

// IntentDraft is an intent as an extractor proposes it.
type IntentDraft struct {
	Statement         string
	Kind              string
	Provability       string
	BeyondProofReason string
	// Quote is the verbatim excerpt from the prose this intent comes from.
	Quote string
}

// DecisionDraft is a decision as an extractor proposes it.
type DecisionDraft struct {
	IntentIDs   []string
	Question    string
	Options     []DecisionOption
	Recommended int
}

// ArtifactError lists the problems that make an artifact invalid or block freezing.
type ArtifactError struct{ Problems []string }

func (e *ArtifactError) Error() string {
	return "Invalid intent artifact:\n - " + strings.Join(e.Problems, "\n - ")
}

func isReason(r string) bool { return contains(BeyondProofReasons, r) }

// CreateArtifact builds an artifact from extractor output. Quotes are anchored to the prose (intent map).
func CreateArtifact(prose string, drafts []IntentDraft, decisions []DecisionDraft, notPromised []string) (IntentArtifact, error) {
	var problems []string
	intents := make([]Intent, len(drafts))
	ids := map[string]bool{}
	for i, d := range drafts {
		id := fmt.Sprintf("I%d", i+1)
		start := locate(prose, d.Quote)
		if start < 0 {
			problems = append(problems, fmt.Sprintf("%s: quote not found in prose: \"%s\"", id, d.Quote))
		}
		if d.Provability == "beyond-proof" && d.BeyondProofReason == "" {
			problems = append(problems, id+": beyond-proof intent needs a beyondProofReason")
		}
		if d.BeyondProofReason != "" && !isReason(d.BeyondProofReason) {
			problems = append(problems, fmt.Sprintf("%s: unknown beyondProofReason '%s'", id, d.BeyondProofReason))
		}
		intents[i] = Intent{ID: id, Statement: d.Statement, Kind: d.Kind, Provability: d.Provability, BeyondProofReason: d.BeyondProofReason, Source: span(d.Quote, start), Status: "proposed"}
		ids[id] = true
	}
	cards := make([]DecisionCard, len(decisions))
	for i, d := range decisions {
		id := fmt.Sprintf("D%d", i+1)
		if len(d.Options) < 2 {
			problems = append(problems, id+": a decision needs at least two options")
		}
		if d.Recommended < 0 || d.Recommended >= len(d.Options) {
			problems = append(problems, id+": recommended out of range")
		}
		for _, ref := range d.IntentIDs {
			if !ids[ref] {
				problems = append(problems, fmt.Sprintf("%s: unknown intent %s", id, ref))
			}
		}
		cards[i] = DecisionCard{ID: id, IntentIDs: nonNil(d.IntentIDs), Question: d.Question, Options: nonNil(d.Options), Recommended: d.Recommended, Origin: "extraction"}
	}
	if len(problems) > 0 {
		return IntentArtifact{}, &ArtifactError{Problems: problems}
	}
	return IntentArtifact{Lastro: "intent-artifact/v0", Prose: prose, Intents: intents, Decisions: cards, NotPromised: nonNil(notPromised)}, nil
}

func span(quote string, start int) SourceSpan {
	if start < 0 {
		start = 0
	}
	return SourceSpan{Quote: quote, Start: start, End: start + len(utf16Units(quote))}
}

// normWS collapses whitespace runs, trims and lower-cases (the TS SDK's norm()).
func normWS(s string) string { return strings.ToLower(strings.Join(strings.Fields(s), " ")) }

// locate finds quote in prose (UTF-16 offset), ignoring whitespace differences, so a quote with
// reflowed lines still anchors. -1 when absent.
func locate(prose, quote string) int {
	p, q := utf16Units(prose), utf16Units(quote)
	if i := indexUnits(p, q); i >= 0 {
		return i
	}
	nq := normWS(quote)
	if nq == "" {
		return -1
	}
	for i := 0; i < len(p); i++ {
		if i > 0 && !unicode.IsSpace(rune(p[i-1])) {
			continue
		}
		end := i + len(q)*2
		if end > len(p) {
			end = len(p)
		}
		if strings.HasPrefix(normWS(fromUTF16(p[i:end])), nq) {
			return i
		}
	}
	return -1
}

func indexUnits(h, n []uint16) int {
	if len(n) == 0 {
		return 0
	}
outer:
	for i := 0; i+len(n) <= len(h); i++ {
		for j := range n {
			if h[i+j] != n[j] {
				continue outer
			}
		}
		return i
	}
	return -1
}

// clone deep-copies through JSON, like structuredClone in the TS SDK.
func clone[T any](v T) T {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	var out T
	if err := json.Unmarshal(b, &out); err != nil {
		panic(err)
	}
	return out
}

// ---------------------------------------------------------------------------
// Review
// ---------------------------------------------------------------------------

// Review is one person's review of an artifact. Chain Approve/Amend/Reject/Answer, then Done.
// The first error (unknown intent, option out of range...) is kept and returned by Done.
type Review struct {
	a        IntentArtifact
	reviewer string
	err      error
}

// NewReview starts a review. reviewer names who is reviewing; it is required when the policy asks for several approvals.
func NewReview(a IntentArtifact, reviewer string) *Review {
	r := &Review{a: clone(a), reviewer: reviewer}
	if a.Hash != "" {
		r.err = fmt.Errorf("artifact is frozen; review a copy created with Thaw()")
	}
	return r
}

func (r *Review) fail(format string, args ...any) *Review {
	if r.err == nil {
		r.err = fmt.Errorf(format, args...)
	}
	return r
}

func (r *Review) sign(it *Intent, reset bool) {
	if r.reviewer == "" {
		return
	}
	prev := it.Approvals
	if reset {
		prev = nil
	}
	if !contains(prev, r.reviewer) {
		prev = append(append([]string{}, prev...), r.reviewer)
	}
	it.Approvals = prev
}

func (r *Review) intent(id string) *Intent {
	for i := range r.a.Intents {
		if r.a.Intents[i].ID == id {
			return &r.a.Intents[i]
		}
	}
	r.fail("unknown intent %s", id)
	return nil
}

// Approve approves intents. An amended intent stays amended (amended intents count as approved).
func (r *Review) Approve(ids ...string) *Review {
	for _, id := range ids {
		if it := r.intent(id); it != nil {
			if it.Status != "amended" {
				it.Status = "approved"
			}
			r.sign(it, false)
		}
	}
	return r
}

// ApproveAll approves every intent this reviewer has not signed yet (rejected ones stay rejected).
func (r *Review) ApproveAll() *Review {
	for _, it := range r.a.Intents {
		if it.Status != "rejected" {
			r.Approve(it.ID)
		}
	}
	return r
}

// Reject marks an intent the AI got wrong: it must not be built at all.
func (r *Review) Reject(id, note string) *Review {
	if it := r.intent(id); it != nil {
		it.Status = "rejected"
		it.ReviewNote = note
		if r.reviewer != "" {
			it.ReviewNote = fmt.Sprintf("%s (%s)", note, r.reviewer)
		}
		it.Approvals = nil
	}
	return r
}

// IntentPatch changes an intent's classification while amending it. Empty fields are kept.
type IntentPatch struct {
	Kind, Provability, BeyondProofReason string
}

// Amend rewrites a statement the AI got partially wrong. Amended intents count as approved,
// and everyone else must approve the new wording.
func (r *Review) Amend(id, statement string) *Review {
	return r.AmendWith(id, statement, IntentPatch{}, "amended by reviewer")
}

// AmendWith is Amend with a classification patch and a note.
func (r *Review) AmendWith(id, statement string, patch IntentPatch, note string) *Review {
	it := r.intent(id)
	if it == nil {
		return r
	}
	if it.OriginalStatement == "" {
		it.OriginalStatement = it.Statement
	}
	it.Statement = statement
	if patch.Kind != "" {
		it.Kind = patch.Kind
	}
	if patch.Provability != "" {
		it.Provability = patch.Provability
	}
	if patch.BeyondProofReason != "" {
		it.BeyondProofReason = patch.BeyondProofReason
	}
	it.Status = "amended"
	it.ReviewNote = note
	r.sign(it, true)
	return r
}

// Add adds an intent the extractor missed (the "unstated" requirement made explicit).
func (r *Review) Add(d IntentDraft) *Review {
	start := locate(r.a.Prose, d.Quote)
	note := "added by reviewer"
	if start < 0 {
		note = "added by reviewer (not in prose)"
	}
	r.a.Intents = append(r.a.Intents, Intent{
		ID: fmt.Sprintf("I%d", len(r.a.Intents)+1), Statement: d.Statement, Kind: d.Kind, Provability: d.Provability,
		BeyondProofReason: d.BeyondProofReason, Source: span(d.Quote, start), Status: "approved", ReviewNote: note,
	})
	return r
}

// Answer picks an option of a decision. A decision raised after extraction (a contradiction in
// the spec) changes what is promised: its answer becomes an intent the team approves like any other.
func (r *Review) Answer(decisionID string, option int) *Review {
	var d *DecisionCard
	for i := range r.a.Decisions {
		if r.a.Decisions[i].ID == decisionID {
			d = &r.a.Decisions[i]
		}
	}
	if d == nil {
		return r.fail("unknown decision %s", decisionID)
	}
	if option < 0 || option >= len(d.Options) {
		return r.fail("option %d out of range for %s", option, decisionID)
	}
	chosen := option
	d.Chosen = &chosen
	if d.Origin != "extraction" {
		o := d.Options[option]
		statement := fmt.Sprintf("%s Decided: %s", d.Question, o.Label)
		if o.Consequence != "" {
			statement += fmt.Sprintf(" (%s)", o.Consequence)
		}
		statement += "."
		var it *Intent
		for i := range r.a.Intents {
			if r.a.Intents[i].DecisionID == d.ID {
				it = &r.a.Intents[i]
			}
		}
		if it == nil {
			r.a.Intents = append(r.a.Intents, Intent{
				ID: fmt.Sprintf("I%d", len(r.a.Intents)+1), Kind: "behavior", Provability: "formal",
				Source: SourceSpan{}, DecisionID: d.ID,
			})
			it = &r.a.Intents[len(r.a.Intents)-1]
		}
		it.Statement = statement
		it.Status = "proposed"
		it.ReviewNote = "answer to decision " + d.ID
		it.Approvals = nil
		it.OriginalStatement = ""
	}
	return r
}

// NotPromised records things the change explicitly does not promise.
func (r *Review) NotPromised(items ...string) *Review {
	r.a.NotPromised = append(r.a.NotPromised, items...)
	return r
}

// Done returns the reviewed artifact, or the first error the review hit.
func (r *Review) Done() (IntentArtifact, error) {
	if r.err != nil {
		return IntentArtifact{}, r.err
	}
	return clone(r.a), nil
}

// RequiredApprovals is the artifact's policy, at least 1.
func RequiredApprovals(a IntentArtifact) int {
	if a.Policy != nil && a.Policy.RequiredApprovals > 1 {
		return a.Policy.RequiredApprovals
	}
	return 1
}

// ApprovalCount is the approvals an intent has collected (a legacy approval without names counts as one).
func ApprovalCount(it Intent) int {
	if it.Status != "approved" && it.Status != "amended" {
		return 0
	}
	if it.Approvals == nil {
		return 1
	}
	return len(it.Approvals)
}

// PendingReview lists what blocks freezing: intents without enough approvals and unanswered decisions.
func PendingReview(a IntentArtifact) []string {
	p := []string{}
	need := RequiredApprovals(a)
	for _, it := range a.Intents {
		if it.Status == "proposed" && len(it.Approvals) == 0 {
			p = append(p, it.ID+" not reviewed")
		} else if it.Status != "rejected" && ApprovalCount(it) < need {
			p = append(p, fmt.Sprintf("%s needs %d more approval(s)", it.ID, need-ApprovalCount(it)))
		}
	}
	for _, d := range a.Decisions {
		if d.Chosen == nil {
			p = append(p, fmt.Sprintf("%s unanswered: %s", d.ID, d.Question))
		}
	}
	return p
}

// Freeze pins a fully reviewed artifact with its canonical hash.
func Freeze(a IntentArtifact) (IntentArtifact, error) {
	if pending := PendingReview(a); len(pending) > 0 {
		return IntentArtifact{}, &ArtifactError{Problems: pending}
	}
	out := clone(a)
	out.Hash = ""
	h, err := HashOf(out)
	if err != nil {
		return IntentArtifact{}, err
	}
	out.Hash = h
	return out, nil
}

// Thaw returns an unfrozen copy, ready for another review.
func Thaw(a IntentArtifact) IntentArtifact {
	out := clone(a)
	out.Hash = ""
	return out
}

// IsIntact re-computes the hash: true when the artifact was not modified after Freeze.
func IsIntact(a IntentArtifact) bool {
	if a.Hash == "" {
		return false
	}
	h, err := HashOf(Thaw(a))
	return err == nil && h == a.Hash
}

// ApprovedIntents are the intents with enough approvals for the policy.
func ApprovedIntents(a IntentArtifact) []Intent {
	need := RequiredApprovals(a)
	out := []Intent{}
	for _, it := range a.Intents {
		if ApprovalCount(it) >= need {
			out = append(out, it)
		}
	}
	return out
}

// RaiseDecisions sends spec problems back to the team: contradictions become unanswered
// decision cards on a thawed copy, so the artifact cannot be frozen until someone answers them.
func RaiseDecisions(a IntentArtifact, cards []DecisionCard) IntentArtifact {
	out := Thaw(a)
	known := map[string]bool{}
	for _, d := range out.Decisions {
		known[d.Question] = true
	}
	for _, c := range cards {
		if known[c.Question] {
			continue
		}
		cp := clone(c)
		cp.ID = fmt.Sprintf("D%d", len(out.Decisions)+1)
		cp.Chosen = nil
		out.Decisions = append(out.Decisions, cp)
	}
	return out
}

// CarryReview carries a previous review over to a fresh extraction of edited prose. An intent
// whose statement matches one already approved or rejected keeps that verdict; an amendment is
// reapplied when the AI repeats its old reading of the same quote; decisions with the same
// question keep their answer. Everything else stays proposed.
func CarryReview(previous, next IntentArtifact) IntentArtifact {
	out := clone(next)
	norm := normWS
	var reviewed []Intent
	for _, p := range previous.Intents {
		if p.Status != "proposed" {
			reviewed = append(reviewed, p)
		}
	}
	for i := range out.Intents {
		it := &out.Intents[i]
		var same, amended *Intent
		for j := range reviewed {
			p := &reviewed[j]
			if same == nil && norm(p.Statement) == norm(it.Statement) {
				same = p
			}
			if amended == nil && p.Status == "amended" && p.OriginalStatement != "" && norm(p.OriginalStatement) == norm(it.Statement) && norm(p.Source.Quote) == norm(it.Source.Quote) {
				amended = p
			}
		}
		if same != nil {
			it.Status = same.Status
			if same.Approvals != nil {
				it.Approvals = append([]string{}, same.Approvals...)
			}
			if same.ReviewNote != "" {
				it.ReviewNote = same.ReviewNote
			}
			if same.OriginalStatement != "" {
				it.OriginalStatement = same.OriginalStatement
			}
		} else if amended != nil {
			it.OriginalStatement = it.Statement
			it.Statement = amended.Statement
			it.Status = "amended"
			if amended.Approvals != nil {
				it.Approvals = append([]string{}, amended.Approvals...)
			}
			if amended.ReviewNote != "" {
				it.ReviewNote = amended.ReviewNote
			}
		}
	}
	for i := range out.Decisions {
		d := &out.Decisions[i]
		for _, p := range previous.Decisions {
			if norm(p.Question) == norm(d.Question) {
				if p.Chosen != nil && *p.Chosen < len(d.Options) {
					c := *p.Chosen
					d.Chosen = &c
				}
				break
			}
		}
	}
	if previous.Policy != nil {
		pol := *previous.Policy
		out.Policy = &pol
	}
	return out
}
