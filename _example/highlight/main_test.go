package main

import (
	"context"
	"strings"
	"testing"

	"github.com/xo/transit/styles"
)

func TestHighlight(t *testing.T) {
	style, ok := styles.Get("github")
	if !ok {
		t.Fatal("no style github")
	}
	h, err := newHighlighter(context.Background(), style)
	if err != nil {
		t.Fatal(err)
	}

	got, ok := h.Highlight("go", "func main() {\n\tprintln(\"a < b\")\n}\n")
	if !ok {
		t.Fatal("no highlight for go")
	}
	// The keyword is colored with the color that github gives a keyword, and the
	// text of the code is escaped.
	if !strings.Contains(got, `<span style="color:#cf222e">func</span>`) {
		t.Errorf("the keyword func is not colored: %s", got)
	}
	if !strings.Contains(got, "a &lt; b") {
		t.Errorf("the text is not escaped: %s", got)
	}
	if strings.Contains(got, "<script") {
		t.Errorf("unexpected tag: %s", got)
	}

	// The alias of a language, in any case.
	if _, ok := h.Highlight("Golang", "var x int\n"); !ok {
		t.Error("no highlight for the alias Golang")
	}
	for _, lang := range []string{"python", "json", "bash", "sh"} {
		if _, ok := h.Highlight(lang, "x\n"); !ok {
			t.Errorf("no highlight for %s", lang)
		}
	}
	// A language with no grammar is declined, so the block stays plain.
	if _, ok := h.Highlight("nosuchlang", "x\n"); ok {
		t.Error("a language with no grammar was highlighted")
	}
}
