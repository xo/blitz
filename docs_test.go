package blitz_test

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"unicode"
)

// These tests read the repository and not the package. They hold the layout
// that D22 set up: four documents in the root, the decisions in docs/PLAN.md
// with an index at the top, and an amendment that both decisions name.

// plan is the document that holds every decision (dbmeta D111).
var plan = filepath.Join("docs", "PLAN.md")

// TestTheRootHoldsFourDocuments holds D22. A document that appears in the
// root is a document that nobody filed.
func TestTheRootHoldsFourDocuments(t *testing.T) {
	t.Parallel()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	allowed := map[string]bool{
		"README.md": true, "AGENTS.md": true, "CLAUDE.md": true, "CONTRIBUTING.md": true,
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		if !allowed[e.Name()] {
			t.Errorf("%s is in the repository root. Only README, AGENTS, CLAUDE and CONTRIBUTING "+
				"go there, and every other document goes in docs/. See D22", e.Name())
		}
	}
	for name := range allowed {
		if _, err := os.Stat(name); err != nil {
			t.Errorf("expected %s in the repository root", name)
		}
	}
}

// TestEveryDocumentIsInBothTables checks that AGENTS.md and README.md name
// each document in docs/. A document that nobody can find is a document that
// nobody reads.
func TestEveryDocumentIsInBothTables(t *testing.T) {
	t.Parallel()
	entries, err := os.ReadDir("docs")
	if err != nil {
		t.Fatal(err)
	}
	var found int
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		found++
		for _, name := range []string{"AGENTS.md", "README.md"} {
			if !strings.Contains(read(t, name), "docs/"+e.Name()) {
				t.Errorf("%s does not name docs/%s. Add it to the list of documents", name, e.Name())
			}
		}
	}
	if found == 0 {
		t.Error("docs/ holds no document, so this test guards nothing")
	}
}

// decision is one decision heading in docs/PLAN.md.
type decision struct {
	num    int
	title  string
	status string
	anchor string
}

// statusClause is one part of a status. A status joins one or more of them
// with a full stop, as in "Decided. Amends D4".
const statusClause = `(?:Decided|Proposed|Amends D[1-9][0-9]*|Amended by D[1-9][0-9]*|` +
	`Supersedes D[1-9][0-9]*|Superseded by D[1-9][0-9]*)`

var (
	// decisionHeading matches every heading that starts a decision.
	decisionHeading = regexp.MustCompile(`(?m)^### D([0-9]+)\. (.*)$`)
	// titleAndStatus splits the rest of a heading into its title and its
	// status. The title is the shortest text that leaves a valid status.
	titleAndStatus = regexp.MustCompile(`^(.+?)\. (` + statusClause + `(?:\. ` + statusClause + `)*)\.$`)
)

// decisions reads every decision heading in docs/PLAN.md, in order:
//
//	### D5. Each platform is its own Go module. Decided. Amends D4.
func decisions(t *testing.T) []decision {
	t.Helper()
	var out []decision
	for _, m := range decisionHeading.FindAllStringSubmatch(read(t, plan), -1) {
		num, _ := strconv.Atoi(m[1])
		parts := titleAndStatus.FindStringSubmatch(m[2])
		if parts == nil {
			t.Errorf("%s: the heading of D%d is %q. A decision heading is \"### D<n>. <title>. <status>.\", "+
				"where the status is Decided, Proposed, Amends D<n> or Amended by D<n>, "+
				"or two of them joined by a full stop", plan, num, m[0])
			continue
		}
		if want := len(out) + 1; num != want {
			t.Errorf("%s: D%d follows D%d. Number each decision one more than the one before it",
				plan, num, want-1)
		}
		out = append(out, decision{
			num:    num,
			title:  parts[1],
			status: parts[2],
			anchor: anchor(strings.TrimPrefix(m[0], "### ")),
		})
	}
	if len(out) == 0 {
		t.Fatalf("%s holds no decision heading", plan)
	}
	return out
}

// anchor returns the fragment that GitHub gives a heading: the text in lower
// case, a hyphen for each space, and no punctuation other than a hyphen or an
// underscore.
func anchor(heading string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(heading) {
		switch {
		case r == ' ':
			b.WriteRune('-')
		case r == '-' || r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
		}
	}
	return b.String()
}

// TestTheDecisionIndexIsComplete checks the table at the top of docs/PLAN.md
// against the decision headings. A reader finds a decision by its row, so a
// missing row, a stale title or a stale status hides it.
func TestTheDecisionIndexIsComplete(t *testing.T) {
	t.Parallel()
	rows := make(map[int]string)
	for _, m := range regexp.MustCompile(`(?m)^\| \[D([0-9]+)\].*$`).FindAllStringSubmatch(read(t, plan), -1) {
		num, _ := strconv.Atoi(m[1])
		if _, ok := rows[num]; ok {
			t.Errorf("%s: the index has two rows for D%d", plan, num)
		}
		rows[num] = m[0]
	}
	written := make(map[int]bool)
	for _, d := range decisions(t) {
		written[d.num] = true
		want := fmt.Sprintf("| [D%d](#%s) | %s | %s |", d.num, d.anchor, d.title, d.status)
		switch got, ok := rows[d.num]; {
		case !ok:
			t.Errorf("D%d has no row in the index of %s. Add:\n%s", d.num, plan, want)
		case got != want:
			t.Errorf("D%d: the row in the index of %s is\n%s\nand the heading says\n%s", d.num, plan, got, want)
		}
	}
	for num := range rows {
		if !written[num] {
			t.Errorf("the index of %s has a row for D%d, and no heading holds it", plan, num)
		}
	}
}

// inverse names the clause that the other decision of an amendment must hold.
var inverse = map[string]string{
	"Amends":        "Amended by",
	"Amended by":    "Amends",
	"Supersedes":    "Superseded by",
	"Superseded by": "Supersedes",
}

// TestAnAmendmentPointsBothWays checks that a decision that names another one
// in its status is named back. If D5 amends D4, then D4 says that D5 amends
// it. Otherwise a reader who finds the older decision gets a rule that no
// longer holds.
func TestAnAmendmentPointsBothWays(t *testing.T) {
	t.Parallel()
	all := decisions(t)
	clauses := make(map[int][]string)
	for _, d := range all {
		clauses[d.num] = strings.Split(d.status, ". ")
	}
	naming := regexp.MustCompile(`^(Amends|Amended by|Supersedes|Superseded by) D([0-9]+)$`)
	var found int
	for _, d := range all {
		for _, clause := range clauses[d.num] {
			m := naming.FindStringSubmatch(clause)
			if m == nil {
				continue
			}
			found++
			other, _ := strconv.Atoi(m[2])
			theirs, ok := clauses[other]
			switch want := inverse[m[1]] + " D" + strconv.Itoa(d.num); {
			case !ok:
				t.Errorf("D%d says %q, and %s holds no D%d", d.num, clause, plan, other)
			case other == d.num:
				t.Errorf("D%d says %q, which names itself", d.num, clause)
			case !slices.Contains(theirs, want):
				t.Errorf("D%d says %q, and the status of D%d does not say %q. "+
					"Write the amendment in both headings, and in both rows of the index", d.num, clause, other, want)
			}
		}
	}
	if found == 0 {
		t.Errorf("%s has no amendment, so this test guards nothing", plan)
	}
}

// otherRepos names the repositories whose decisions a document here can
// cite. A number after one of these names, as in "dbmeta D110", is a decision
// of that repository and not of this one.
var otherRepos = map[string]bool{
	"blitz-c": true, "cql": true, "dbimp": true, "dbmeta": true, "dbtpl": true,
	"dburl": true, "n1ql": true, "tblfmt": true, "usql": true,
}

// TestEveryDecisionReferenceExists checks that a bare decision number, such
// as D3, in a document or a Go file names a decision in docs/PLAN.md. A
// reference to a number that nobody wrote leads a reader to trust a rule that
// does not exist. A reference to another repository names that repository
// first.
func TestEveryDecisionReferenceExists(t *testing.T) {
	t.Parallel()
	written := make(map[string]bool)
	for _, d := range decisions(t) {
		written[strconv.Itoa(d.num)] = true
	}
	ref := regexp.MustCompile(`\bD([1-9][0-9]*)\b`)
	before := regexp.MustCompile(`([a-z][a-z0-9-]*) $`)
	for _, path := range repoFiles(t, ".md", ".go") {
		body := read(t, path)
		for _, m := range ref.FindAllStringSubmatchIndex(body, -1) {
			if w := before.FindStringSubmatch(body[max(0, m[0]-32):m[0]]); w != nil && otherRepos[w[1]] {
				continue
			}
			if num := body[m[2]:m[3]]; !written[num] {
				t.Errorf("%s: refers to D%s, which is not in %s. If it is a decision of another "+
					"repository, write the name of that repository before it, such as dbmeta D110",
					path, num, plan)
			}
		}
	}
}

// repoFiles returns every file with one of the given extensions. It skips a
// hidden directory, which holds the agent skills or git itself, and the
// renders that the tests write.
func repoFiles(t *testing.T, exts ...string) []string {
	t.Helper()
	var out []string
	err := filepath.WalkDir(".", func(path string, d os.DirEntry, err error) error {
		switch {
		case err != nil:
			return err
		case d.IsDir() && path != "." && (strings.HasPrefix(d.Name(), ".") || path == filepath.Join("testdata", "output")):
			return filepath.SkipDir
		case d.IsDir():
			return nil
		}
		for _, ext := range exts {
			if strings.HasSuffix(path, ext) {
				out = append(out, path)
				break
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// read returns the content of a file in the repository.
func read(t *testing.T, path string) string {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}
