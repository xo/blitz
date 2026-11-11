package blitz

import (
	"html"
	"image"
	"regexp"
)

// A Highlighter highlights the code of one fenced block in a markdown document.
//
// lang is the language of the block: the first word of its info string, so
// "go" for a block that opens with ```go. It is exactly as the author wrote it.
// The case is kept, so ```Go gives "Go", and an attribute that touches the word
// stays, so ```rust,ignore gives "rust,ignore" and ```go{.numberLines} gives
// "go{.numberLines}". A Highlighter maps the name to a language itself: it
// lowercases the name, cuts it at a comma, a brace or a space, and looks it up
// in a table of its own, such as the names and aliases of GitHub's Linguist.
// code is the text of the block, and it
// is the plain text, with no HTML escapes. It ends with a newline, and a tab in
// it is already a run of spaces.
//
// A Highlighter returns the HTML to put inside the <code> element, and true.
// That HTML goes in as it is, so the Highlighter must escape the text of the
// code itself, as html.EscapeString does, and it writes a <span style="color:...">
// for each run that it colors. Inline styles need no stylesheet. A Highlighter
// returns false for a language that it does not know, and the block keeps its
// plain look.
//
// blitz does not depend on a highlighter, so any one can be used. For example,
// a Highlighter can parse code with the grammar of github.com/xo/transit for
// lang, and color each capture of the highlight query with a style of
// github.com/xo/transit/styles. _example/highlight does that.
type Highlighter func(lang, code string) (html string, ok bool)

// fencedBlock matches a fenced code block that has a language, as the markdown
// conversion writes it: a <pre> that holds a <code> with the class
// "language-<lang>". The code inside is HTML-escaped, so it holds no tag.
var fencedBlock = regexp.MustCompile(`(?s)<pre><code class="language-([^"]+)">(.*?)</code></pre>`)

// HighlightCodeBlocks rewrites the fenced code blocks of html, the output of
// MarkdownToHTML, with h. A block that has no language, a block that h does not
// highlight, and everything else in html stay as they are. The <pre><code> that
// wraps a block stays too, so the stylesheet of the document still styles it. A
// nil h returns html as it is.
//
// To use a stylesheet of your own, convert with MarkdownToHTML and a stylesheet,
// call HighlightCodeBlocks, and render the result with RenderHTML.
func HighlightCodeBlocks(doc string, h Highlighter) string {
	if h == nil {
		return doc
	}
	return fencedBlock.ReplaceAllStringFunc(doc, func(block string) string {
		m := fencedBlock.FindStringSubmatch(block)
		lang := html.UnescapeString(m[1])
		out, ok := h(lang, html.UnescapeString(m[2]))
		if !ok {
			return block
		}
		return `<pre><code class="language-` + m[1] + `">` + out + `</code></pre>`
	})
}

// RenderMarkdownHighlighted renders markdown like RenderMarkdown, with the
// fenced code blocks highlighted by h. See Highlighter and HighlightCodeBlocks.
// A nil h gives the same image as RenderMarkdown.
func (c *Context) RenderMarkdownHighlighted(markdown, baseURL string, opts Options, h Highlighter) (*image.RGBA, error) {
	doc, err := highlightedMarkdown(markdown, h)
	if err != nil {
		return nil, err
	}
	return c.RenderHTML(doc, baseURL, opts)
}

// RenderMarkdownHighlightedPNG renders markdown like RenderMarkdownPNG, with the
// fenced code blocks highlighted by h. dpi of 0 uses 144.
func (c *Context) RenderMarkdownHighlightedPNG(markdown, baseURL string, opts Options, dpi uint32, h Highlighter) ([]byte, error) {
	doc, err := highlightedMarkdown(markdown, h)
	if err != nil {
		return nil, err
	}
	return c.RenderHTMLPNG(doc, baseURL, opts, dpi)
}

// highlightedMarkdown converts markdown with the built-in stylesheet and
// highlights its code blocks.
func highlightedMarkdown(markdown string, h Highlighter) (string, error) {
	doc, err := MarkdownToHTML(markdown, nil)
	if err != nil {
		return "", err
	}
	return HighlightCodeBlocks(doc, h), nil
}
