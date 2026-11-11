// Command highlight renders a markdown file to a PNG with the fenced code blocks
// highlighted by transit. It is a sample program of blitz.
//
// blitz does not depend on a highlighter. It gives the Highlighter hook, and
// this program fills it: for the language of a block it parses the code with a
// grammar of github.com/xo/transit, runs the highlight query of the grammar, and
// colors each capture with a style of github.com/xo/transit/styles. The
// styles of GitHub are github and github-dark.
//
// Run it in the folder _example of the repository:
//
//	go run ./highlight -style github -o out.png ../README.md
//	go run ./highlight -dark -o out.png notes.md
//
// A block with a language that has no grammar here keeps the plain look.
package main

import (
	"context"
	"flag"
	"fmt"
	"html"
	"os"
	"strings"

	"github.com/xo/blitz"
	"github.com/xo/transit"
	"github.com/xo/transit/grammars/bash"
	golang "github.com/xo/transit/grammars/go"
	"github.com/xo/transit/grammars/json"
	"github.com/xo/transit/grammars/python"
	"github.com/xo/transit/styles"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

// run reads the flags in args, renders the markdown file that they name, and
// writes the PNG.
func run(args []string) error {
	fs := flag.NewFlagSet("highlight", flag.ContinueOnError)
	out := fs.String("o", "out.png", "the PNG to write")
	styleName := fs.String("style", "", "the style of the module styles (default github, or github-dark with -dark)")
	dark := fs.Bool("dark", false, "render with the dark color scheme")
	width := fs.Uint("w", 900, "the width in CSS pixels")
	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("reading the flags: %w", err)
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("give one markdown file")
	}
	if *styleName == "" {
		*styleName = "github"
		if *dark {
			*styleName = "github-dark"
		}
	}
	style, ok := styles.Get(*styleName)
	if !ok {
		return fmt.Errorf("no style %q: choose one of %s", *styleName, strings.Join(styles.Names(), ", "))
	}
	source, err := os.ReadFile(fs.Arg(0))
	if err != nil {
		return fmt.Errorf("reading %s: %w", fs.Arg(0), err)
	}

	c, err := blitz.New(0)
	if err != nil {
		return fmt.Errorf("making a context: %w", err)
	}
	defer c.Close()
	opts := blitz.DefaultOptions()
	opts.Width = uint32(*width)
	opts.Scale = 2
	if *dark {
		opts.ColorScheme = blitz.Dark
	}
	h, err := newHighlighter(context.Background(), style)
	if err != nil {
		return err
	}
	png, err := c.RenderMarkdownHighlightedPNG(string(source), "", opts, 144, h.Highlight)
	if err != nil {
		return fmt.Errorf("rendering: %w", err)
	}
	if err := os.WriteFile(*out, png, 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", *out, err)
	}
	fmt.Printf("wrote %s (%d KiB)\n", *out, len(png)/1024)
	return nil
}

// A grammar is a language of transit and its highlight query, compiled once.
type grammar struct {
	language *transit.Language
	query    *transit.Query
}

// highlighter highlights code with the grammars of transit. It keeps a parser
// and a query cursor, so it is not safe for use by several goroutines at once.
type highlighter struct {
	ctx      context.Context
	style    *styles.Style
	parser   *transit.Parser
	cursor   *transit.QueryCursor
	grammars map[string]*grammar
}

// aliases maps the info strings of a fenced block to a grammar.
var aliases = map[string]string{
	"go": "go", "golang": "go",
	"python": "python", "py": "python",
	"json": "json",
	"bash": "bash", "sh": "bash", "shell": "bash",
}

// newHighlighter makes a highlighter that colors with style.
func newHighlighter(ctx context.Context, style *styles.Style) (*highlighter, error) {
	h := &highlighter{
		ctx:      ctx,
		style:    style,
		parser:   transit.NewParser(),
		cursor:   transit.NewQueryCursor(),
		grammars: make(map[string]*grammar),
	}
	for name, lang := range map[string]*transit.Language{
		"bash":   bash.Language(),
		"go":     golang.Language(),
		"json":   json.Language(),
		"python": python.Language(),
	} {
		var source []byte
		var err error
		switch name {
		case "bash":
			source, err = bash.Queries.ReadFile("queries/highlights.scm")
		case "go":
			source, err = golang.Queries.ReadFile("queries/highlights.scm")
		case "json":
			source, err = json.Queries.ReadFile("queries/highlights.scm")
		case "python":
			source, err = python.Queries.ReadFile("queries/highlights.scm")
		}
		if err != nil {
			return nil, fmt.Errorf("reading the highlight query of %s: %w", name, err)
		}
		q, err := transit.NewQuery(lang, string(source))
		if err != nil {
			return nil, fmt.Errorf("compiling the highlight query of %s: %w", name, err)
		}
		h.grammars[name] = &grammar{language: lang, query: q}
	}
	return h, nil
}

// Highlight is a blitz.Highlighter. It returns the code as HTML, with a span
// for each run of text that one capture of the highlight query covers, and
// false for a language that it has no grammar for.
func (h *highlighter) Highlight(lang, code string) (string, bool) {
	name, ok := aliases[strings.ToLower(lang)]
	if !ok {
		return "", false
	}
	g := h.grammars[name]
	if err := h.parser.SetLanguage(g.language); err != nil {
		return "", false
	}
	src := []byte(code)
	tree, err := h.parser.Parse(h.ctx, src, nil)
	if err != nil {
		return "", false
	}

	// The capture name of each byte. The innermost capture wins, and of two
	// patterns on one node the last wins, as in the sample programs of transit.
	names := make([]string, len(src))
	root := tree.RootNode()
	captureNames := g.query.CaptureNames()
	h.cursor.SetByteRange(root.StartByte(), root.EndByte())
	for m, i := range h.cursor.Captures(h.ctx, g.query, root, src) {
		c := m.Captures[i]
		name := captureNames[c.Index]
		if strings.HasPrefix(name, "_") {
			continue // a predicate, and not a highlight
		}
		for b := c.Node.StartByte(); b < c.Node.EndByte() && b < len(names); b++ {
			names[b] = name
		}
	}

	var b strings.Builder
	for i := 0; i < len(src); {
		j := i + 1
		for j < len(src) && names[j] == names[i] {
			j++
		}
		text := html.EscapeString(string(src[i:j]))
		if css := h.css(names[i]); css != "" {
			fmt.Fprintf(&b, `<span style="%s">%s</span>`, css, text)
		} else {
			b.WriteString(text)
		}
		i = j
	}
	return b.String(), true
}

// css returns the inline style for a capture name, or "" for text that no
// capture names, which keeps the color of the page.
func (h *highlighter) css(name string) string {
	if name == "" {
		return ""
	}
	e := h.style.Lookup(name)
	var css []string
	if e.Fg.IsSet() {
		r, g, b := e.Fg.RGB()
		css = append(css, fmt.Sprintf("color:#%02x%02x%02x", r, g, b))
	}
	if e.Bold {
		css = append(css, "font-weight:bold")
	}
	if e.Italic {
		css = append(css, "font-style:italic")
	}
	return strings.Join(css, ";")
}
