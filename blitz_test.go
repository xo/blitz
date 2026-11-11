package blitz

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"html"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

// Output lands in testdata/output/ so the renders can actually be looked at
// after a run. The directory is gitignored.
const outputDir = "testdata/output"

var network = flag.Bool("network", false, "run tests that fetch over the network")

// One shared Context for the whole suite: creating one spins up worker threads,
// and sharing it is also the configuration most likely to expose a data race.
var (
	shared     *Context
	sharedOnce sync.Once
)

func ctx(t *testing.T) *Context {
	t.Helper()
	sharedOnce.Do(func() {
		c, err := New(0)
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		shared = c
	})
	if shared == nil {
		t.Fatal("context unavailable")
	}
	return shared
}

func TestMain(m *testing.M) {
	flag.Parse()
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		panic(err)
	}
	code := m.Run()
	if shared != nil {
		shared.Close()
	}
	os.Exit(code)
}

func testOptions() Options {
	o := DefaultOptions()
	o.Width = 900
	o.Scale = 2
	// Hermetic by default. Individual tests turn this back on.
	o.EnableNet = false
	return o
}

// checkPNG verifies the bytes really are a decodable PNG of a plausible size.
// A status code alone doesn't prove anything was painted: with no fonts
// installed the library happily produces a blank page and returns success.
func checkPNG(t *testing.T, data []byte, name string) image.Image {
	t.Helper()

	if !bytes.HasPrefix(data, []byte("\x89PNG\r\n\x1a\n")) {
		t.Fatalf("%s: not a PNG (%d bytes)", name, len(data))
	}
	if len(data) < 2048 {
		t.Fatalf("%s: suspiciously small (%d bytes) — are fonts installed?", name, len(data))
	}

	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("%s: decode: %v", name, err)
	}

	b := img.Bounds()
	if b.Dx() == 0 || b.Dy() == 0 {
		t.Fatalf("%s: zero-sized image", name)
	}
	return img
}

func write(t *testing.T, data []byte, name string) {
	t.Helper()
	path := filepath.Join(outputDir, name)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	t.Logf("wrote %s (%d KiB)", path, len(data)/1024)
}

func TestVersion(t *testing.T) {
	v := Version()
	if v == "" {
		t.Fatal("empty version")
	}
	t.Logf("libblitz %s", v)
}

func TestDefaultOptions(t *testing.T) {
	o := DefaultOptions()
	if o.Width == 0 || o.Height == 0 || o.Scale <= 0 {
		t.Fatalf("implausible defaults: %+v", o)
	}
	if !o.EnableNet || !o.FitContentHeight {
		t.Fatalf("expected net and fit-content on by default: %+v", o)
	}
}

func TestRenderMarkdownToDisk(t *testing.T) {
	source, err := os.ReadFile("testdata/sample.md")
	if err != nil {
		t.Fatal(err)
	}

	data, err := ctx(t).RenderMarkdownPNG(string(source), "", testOptions(), 144)
	if err != nil {
		t.Fatalf("RenderMarkdownPNG: %v", err)
	}

	img := checkPNG(t, data, "markdown.png")
	if got := img.Bounds().Dx(); got != 1800 {
		t.Errorf("width = %d, want 1800 (900 CSS px at 2x)", got)
	}
	write(t, data, "markdown.png")
}

func TestRenderHTMLToDisk(t *testing.T) {
	source, err := os.ReadFile("testdata/sample.html")
	if err != nil {
		t.Fatal(err)
	}

	data, err := ctx(t).RenderHTMLPNG(string(source), "", testOptions(), 144)
	if err != nil {
		t.Fatalf("RenderHTMLPNG: %v", err)
	}

	checkPNG(t, data, "html.png")
	write(t, data, "html.png")
}

func TestWriteMarkdownPNG(t *testing.T) {
	path := filepath.Join(outputDir, "write-helper.png")
	os.Remove(path)

	err := ctx(t).WriteMarkdownPNG("# Written\n\nStraight to disk.", "", path, testOptions(), 144)
	if err != nil {
		t.Fatalf("WriteMarkdownPNG: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	checkPNG(t, data, "write-helper.png")
}

func TestRenderRGBA(t *testing.T) {
	opts := testOptions()
	opts.Width = 400

	img, err := ctx(t).RenderMarkdown("# Pixels\n\nSome text to rasterise.", "", opts)
	if err != nil {
		t.Fatalf("RenderMarkdown: %v", err)
	}

	if img.Bounds().Dx() != 800 {
		t.Errorf("width = %d, want 800", img.Bounds().Dx())
	}
	if len(img.Pix) != img.Bounds().Dx()*img.Bounds().Dy()*4 {
		t.Error("pixel buffer size does not match bounds")
	}

	// The default background is opaque white, so every pixel should have full
	// alpha. Catches a stride or row-copy mistake in toImage.
	for i := 3; i < len(img.Pix); i += 4 {
		if img.Pix[i] != 0xFF {
			t.Fatalf("pixel %d has alpha %d, want 255", i/4, img.Pix[i])
		}
	}
}

func TestTransparentBackground(t *testing.T) {
	opts := testOptions()
	opts.Width = 200
	opts.BackgroundRGBA = 0x00000000 // alpha 0

	img, err := ctx(t).RenderMarkdownStyled("# Clear", "", "body{margin:0}", opts)
	if err != nil {
		t.Fatalf("RenderMarkdownStyled: %v", err)
	}

	transparent := 0
	for i := 3; i < len(img.Pix); i += 4 {
		if img.Pix[i] == 0 {
			transparent++
		}
	}
	if transparent == 0 {
		t.Error("no transparent pixels with alpha-0 background")
	}
}

func TestDarkColorScheme(t *testing.T) {
	opts := testOptions()
	opts.ColorScheme = Dark
	opts.Width = 600

	data, err := ctx(t).RenderMarkdownPNG("# Dark mode\n\nThe built-in sheet honours `prefers-color-scheme`.", "", opts, 144)
	if err != nil {
		t.Fatalf("RenderMarkdownPNG: %v", err)
	}
	checkPNG(t, data, "dark.png")
	write(t, data, "dark.png")
}

func TestCustomStylesheet(t *testing.T) {
	css := DefaultMarkdownStylesheet()
	if len(css) < 100 {
		t.Fatalf("built-in stylesheet looks empty (%d bytes)", len(css))
	}

	extended := css + "\nbody { padding: 80px; background: #fffbe6; }"
	img, err := ctx(t).RenderMarkdownStyled("# Extended\n\nWith extra padding.", "", extended, testOptions())
	if err != nil {
		t.Fatalf("RenderMarkdownStyled: %v", err)
	}
	if img.Bounds().Dy() == 0 {
		t.Fatal("zero-height render")
	}

	// The custom background should show up somewhere in the corner padding.
	if r, g, b, _ := img.At(4, 4).RGBA(); r>>8 != 0xff || g>>8 != 0xfb || b>>8 != 0xe6 {
		t.Errorf("corner pixel = %02x%02x%02x, want fffbe6", r>>8, g>>8, b>>8)
	}
}

func TestMarkdownToHTML(t *testing.T) {
	html, err := MarkdownToHTML("# Title\n\n| a | b |\n| - | - |\n| 1 | 2 |", nil)
	if err != nil {
		t.Fatalf("MarkdownToHTML: %v", err)
	}
	for _, want := range []string{"<!DOCTYPE html>", "<h1>Title</h1>", "<table>", "<style>"} {
		if !bytes.Contains([]byte(html), []byte(want)) {
			t.Errorf("output missing %q", want)
		}
	}

	// Empty (not nil) stylesheet means "no CSS at all".
	empty := ""
	bare, err := MarkdownToHTML("# Title", &empty)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains([]byte(bare), []byte("<style>")) {
		t.Error("empty stylesheet should omit the <style> element")
	}
}

// A tab in a code block. Blitz draws the missing-glyph box for a tab, so the
// markdown conversion writes the spaces that reach the next tab stop, which is
// every 8 columns. See D32.
func TestMarkdownTabInCodeBlockIsSpaces(t *testing.T) {
	html, err := MarkdownToHTML("```\na\tb\n\tc\n```", nil)
	if err != nil {
		t.Fatal(err)
	}
	_, body, _ := strings.Cut(html, "<body>")
	if strings.Contains(body, "\t") {
		t.Errorf("a tab is left in the body: %q", body)
	}
	for _, want := range []string{"a       b", "\n        c"} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %q: %q", want, body)
		}
	}

	// The stylesheet does not matter: the spaces are in the HTML.
	empty := ""
	bare, err := MarkdownToHTML("```\n\tx\n```", &empty)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(bare, "\t") {
		t.Errorf("a tab is left with no stylesheet: %q", bare)
	}

	// A tab outside a code block is left alone, because it collapses to a space.
	para, err := MarkdownToHTML("a\tb", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(para, "a\tb") {
		t.Errorf("a tab in a paragraph changed: %q", para)
	}
}

func TestRenderedTabEqualsEightSpaces(t *testing.T) {
	c := ctx(t)
	tab, err := c.RenderMarkdown("```\n\tx\n```", "", testOptions())
	if err != nil {
		t.Fatal(err)
	}
	spaces, err := c.RenderMarkdown("```\n        x\n```", "", testOptions())
	if err != nil {
		t.Fatal(err)
	}
	if tab.Bounds() != spaces.Bounds() || !bytes.Equal(tab.Pix, spaces.Pix) {
		t.Error("a tab in a code block does not render as 8 spaces. Blitz draws a box for a tab. See D32")
	}
}

// Blitz draws the marker of an ordered list item with no space after it, so
// "1." touches the text. The stylesheet pads the item, and that makes a gap.
// This measures the first line of the render at scale 2. It finds the runs of
// ink columns, and takes the first wide one, which is the "O" of "Ordered"
// (about 22 pixels). The marker is the narrow runs before it, "1" and ".". The
// gap between the last of them and the "O" is 3 pixels without the padding and
// 15 with it, so the test wants 8 or more. See D32.
func TestOrderedListMarkerHasAGap(t *testing.T) {
	img, err := ctx(t).RenderMarkdown("1. Ordered item\n2. Ordered item", "", testOptions())
	if err != nil {
		t.Fatal(err)
	}
	b := img.Bounds()
	ink := func(x, y int) bool { return img.RGBAAt(x, y).R < 160 }

	// The first line is the rows from the first row with ink, for 28 pixels. That
	// reaches the baseline, where the period of "1." sits.
	top := -1
	for y := b.Min.Y; y < b.Max.Y && top < 0; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if ink(x, y) {
				top = y
				break
			}
		}
	}
	if top < 0 {
		t.Fatal("the render has no ink, so no fonts were found")
	}

	var runs [][2]int
	start := -1
	for x := b.Min.X; x < b.Max.X; x++ {
		on := false
		for y := top; y < top+28 && y < b.Max.Y; y++ {
			if ink(x, y) {
				on = true
				break
			}
		}
		switch {
		case on && start < 0:
			start = x
		case !on && start >= 0:
			runs = append(runs, [2]int{start, x - 1})
			start = -1
		}
	}

	for i, r := range runs {
		if r[1]-r[0]+1 < 16 {
			continue
		}
		if i == 0 {
			t.Fatalf("the first run of ink is wide, so there is no marker: %v", runs[:min(4, len(runs))])
		}
		if gap := r[0] - runs[i-1][1] - 1; gap < 8 {
			t.Errorf("the gap between the marker and the text is %dpx, want 8 or more. "+
				"Blitz puts no space after the marker, and the stylesheet pads the item. See D32", gap)
		}
		return
	}
	t.Fatalf("found no wide run of ink for the first letter: %v", runs)
}

// The CPU renderer counts the pixels of an image in 16 bits, so a render over
// 65535 pixels in one direction used to wrap around and fail with "renderer
// produced N bytes, expected M", and one just under it panicked. Both are now an
// error that names the limit. See D33.
func TestRenderOverThePixelLimitIsAClearError(t *testing.T) {
	c := ctx(t)
	// No margin on the body, so that the size of the content is the size asked for.
	tall := func(h int) string {
		return fmt.Sprintf(`<body style="margin:0"><div style="height:%dpx"></div></body>`, h)
	}

	check := func(name string, err error, want ...string) {
		t.Helper()
		var be *Error
		if !errors.As(err, &be) {
			t.Fatalf("%s: want an *Error, got %v", name, err)
		}
		if be.Code != CodeInvalidArg {
			t.Errorf("%s: code %d, want %d: %v", name, be.Code, CodeInvalidArg, err)
		}
		for _, w := range want {
			if !strings.Contains(be.Message, w) {
				t.Errorf("%s: message %q does not say %q", name, be.Message, w)
			}
		}
		for _, bad := range []string{"renderer produced", "panic"} {
			if strings.Contains(be.Message, bad) {
				t.Errorf("%s: the message is the old failure, %q: %q", name, bad, be.Message)
			}
		}
	}

	// Too wide: known from the options, so it fails before any layout.
	o := testOptions()
	o.Width = 40000
	o.Scale = 2
	_, err := c.RenderHTML("<p>x</p>", "", o)
	check("wide", err, "80000 pixels wide", "at most 65408 pixels wide", "Lower width or scale")

	// Too tall because the height is fixed.
	o = testOptions()
	o.Width = 100
	o.Height = 40000
	o.Scale = 2
	o.FitContentHeight = false
	_, err = c.RenderHTML("<p>x</p>", "", o)
	check("fixed height", err, "80000 pixels tall", "at most 65532 pixels tall")

	// Too tall because the content is: known only after layout. This is the
	// case that iv hit with a long README at a Scale of 2.
	o = testOptions()
	o.Width = 100
	o.Scale = 2
	o.MaxHeight = 70000
	o.FitContentHeight = true
	_, err = c.RenderHTML(tall(36257), "", o)
	check("content", err, "72514 pixels tall", "36257 CSS px at scale 2", "at most 65532 pixels tall", "lower max_height")

	// The sizes between the real limit and 65535 are errors too, and they are
	// not the panic that the renderer gives for them.
	for _, h := range []int{32767, 32768} {
		o.MaxHeight = uint32(h)
		_, err = c.RenderHTML(tall(40000), "", o)
		check(fmt.Sprintf("max_height %d at scale 2", h), err, "at most 65532 pixels tall")
	}

	// MaxHeight cuts a long page short, and the render is not too tall then.
	o.MaxHeight = 32766
	img, err := c.RenderHTML(tall(36257), "", o)
	if err != nil {
		t.Fatalf("a MaxHeight that fits should render: %v", err)
	}
	if got := img.Bounds().Dy(); got != 65532 {
		t.Errorf("cut to MaxHeight 32766 at scale 2: height %d, want 65532", got)
	}
}

// Each limit is inclusive: the last size that the renderer draws must render, at
// exactly that size with no wrap, and the next one must be the clear error.
func TestRenderAtThePixelLimitWorks(t *testing.T) {
	c := ctx(t)
	doc := func(w, h int) string {
		return fmt.Sprintf(`<body style="margin:0"><div style="width:%dpx;height:%dpx;background:#ccc"></div></body>`, w, h)
	}
	opts := func(w, h int) Options {
		o := testOptions()
		o.Width = uint32(w)
		o.Height = uint32(h)
		o.Scale = 1
		o.MaxHeight = uint32(h)
		o.FitContentHeight = true
		return o
	}

	for _, tc := range []struct {
		name string
		w, h int
	}{
		{"the tallest", 8, 65532},
		{"the widest", 65408, 8},
	} {
		img, err := c.RenderHTML(doc(tc.w, tc.h), "", opts(tc.w, tc.h))
		if err != nil {
			t.Errorf("%s (%dx%d) should render: %v", tc.name, tc.w, tc.h, err)
			continue
		}
		if b := img.Bounds(); b.Dx() != tc.w || b.Dy() != tc.h {
			t.Errorf("%s: size %dx%d, want %dx%d", tc.name, b.Dx(), b.Dy(), tc.w, tc.h)
		}
	}

	for _, tc := range []struct {
		name string
		w, h int
	}{
		{"one row too tall", 8, 65533},
		{"one column too wide", 65409, 8},
	} {
		_, err := c.RenderHTML(doc(tc.w, tc.h), "", opts(tc.w, tc.h))
		var be *Error
		if !errors.As(err, &be) || be.Code != CodeInvalidArg {
			t.Errorf("%s (%dx%d): want a CodeInvalidArg error, got %v", tc.name, tc.w, tc.h, err)
		}
	}
}

// The hook for a syntax highlighter. See D34.
func TestHighlightCodeBlocks(t *testing.T) {
	md := "intro\n\n```go\nif a < b && c {\n\treturn \"x\"\n}\n```\n\n```sql\nSELECT 1\n```\n\n" +
		"```x&y\nodd name\n```\n\n```Go\nupper\n```\n\n```rust,ignore\nattr\n```\n\n" +
		"```\nno language\n```\n\n`inline` code\n"
	doc, err := MarkdownToHTML(md, nil)
	if err != nil {
		t.Fatal(err)
	}

	var langs []string
	h := func(lang, code string) (string, bool) {
		langs = append(langs, lang)
		if lang == "sql" {
			return "", false
		}
		// The code is the plain text of the block, with a newline at the end and
		// a tab already written as spaces.
		if !strings.HasSuffix(code, "\n") {
			t.Errorf("%s: code does not end with a newline: %q", lang, code)
		}
		if strings.Contains(code, "\t") || strings.Contains(code, "&lt;") || strings.Contains(code, "&amp;") {
			t.Errorf("%s: code is not plain text: %q", lang, code)
		}
		return `<span class="hl">` + html.EscapeString(code) + `</span>`, true
	}
	out := HighlightCodeBlocks(doc, h)

	// Only a block with a language reaches the Highlighter, and the language is
	// the plain text of the info string.
	// The name is exactly as the author wrote it: its case and its attribute stay.
	if want := []string{"go", "sql", "x&y", "Go", "rust,ignore"}; !slices.Equal(langs, want) {
		t.Errorf("languages = %q, want %q", langs, want)
	}
	for _, want := range []string{
		`<pre><code class="language-go"><span class="hl">if a &lt; b &amp;&amp; c {`,
		"        return &#34;x&#34;\n}\n</span></code></pre>",
		// A block that the Highlighter declines is not changed, and neither is a
		// block with no language, an inline code, or the document around them.
		`<pre><code class="language-sql">SELECT 1`,
		`<pre><code>no language`,
		`<code>inline</code>`,
		`<p>intro</p>`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output does not hold %q:\n%s", want, out)
		}
	}
	if !strings.Contains(out, `class="language-x&amp;y"><span class="hl">odd name`) {
		t.Errorf("the wrapper of a block with a special language changed:\n%s", out)
	}

	// A nil Highlighter changes nothing.
	if got := HighlightCodeBlocks(doc, nil); got != doc {
		t.Error("a nil Highlighter changed the document")
	}
	// A Highlighter that declines every block changes nothing.
	if got := HighlightCodeBlocks(doc, func(string, string) (string, bool) { return "", false }); got != doc {
		t.Error("a Highlighter that declines every block changed the document")
	}
}

func TestRenderMarkdownHighlighted(t *testing.T) {
	c := ctx(t)
	const md = "A block:\n\n```go\nfunc main() {}\n```\n"
	red := func(lang, code string) (string, bool) {
		return `<span style="color:#cc0000">` + html.EscapeString(code) + `</span>`, true
	}
	redPixels := func(img *image.RGBA) (n int) {
		for i := 0; i+3 < len(img.Pix); i += 4 {
			if r, g, b := img.Pix[i], img.Pix[i+1], img.Pix[i+2]; r > 150 && g < 60 && b < 60 {
				n++
			}
		}
		return n
	}

	plain, err := c.RenderMarkdown(md, "", testOptions())
	if err != nil {
		t.Fatal(err)
	}
	hl, err := c.RenderMarkdownHighlighted(md, "", testOptions(), red)
	if err != nil {
		t.Fatal(err)
	}
	if n := redPixels(plain); n != 0 {
		t.Fatalf("the plain render has %d red pixels, so the test says nothing", n)
	}
	if n := redPixels(hl); n < 50 {
		t.Errorf("the highlighted render has %d red pixels, want the code in red", n)
	}
	if plain.Bounds() != hl.Bounds() {
		t.Errorf("highlighting changed the size: %v, was %v", hl.Bounds(), plain.Bounds())
	}

	// A nil Highlighter is RenderMarkdown.
	same, err := c.RenderMarkdownHighlighted(md, "", testOptions(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(same.Pix, plain.Pix) {
		t.Error("a nil Highlighter does not give the render of RenderMarkdown")
	}

	data, err := c.RenderMarkdownHighlightedPNG(md, "", testOptions(), 144, red)
	if err != nil {
		t.Fatal(err)
	}
	checkPNG(t, data, "highlighted")
}

// --- error handling ---------------------------------------------------------

func TestInvalidURLReportsError(t *testing.T) {
	_, err := ctx(t).RenderURL("http://%zz-not-a-host", testOptions())
	if err == nil {
		t.Fatal("expected an error")
	}

	var be *Error
	if !errors.As(err, &be) {
		t.Fatalf("expected *blitz.Error, got %T", err)
	}
	if be.Message == "" || be.Message == "no detail" {
		t.Errorf("error carries no message: %+v", be)
	}
	if be.Code != CodeInvalidURL && be.Code != CodeNetwork {
		t.Errorf("unexpected code %d", be.Code)
	}
	t.Logf("got expected error: %v", err)
}

func TestClosedContext(t *testing.T) {
	c, err := New(0)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	// Idempotent.
	if err := c.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}

	if _, err := c.RenderMarkdown("# after close", "", testOptions()); !errors.Is(err, ErrClosed) {
		t.Fatalf("want ErrClosed, got %v", err)
	}
}

// --- concurrency ------------------------------------------------------------

// Renders from many goroutines at once. Run with -race; the point is to catch
// unsynchronised access, not to measure throughput (the library serialises
// rendering internally, so these queue).
func TestConcurrentRenders(t *testing.T) {
	if testing.Short() {
		t.Skip("slow under -short")
	}

	c := ctx(t)
	const goroutines = 8

	var wg sync.WaitGroup
	errs := make(chan error, goroutines)

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()

			opts := testOptions()
			opts.Width = uint32(300 + n*20) // distinct sizes, distinct buffers

			img, err := c.RenderMarkdown("# Goroutine\n\nConcurrent render.", "", opts)
			if err != nil {
				errs <- err
				return
			}
			if want := int(opts.Width) * 2; img.Bounds().Dx() != want {
				errs <- errors.New("wrong width for this goroutine's options")
			}
		}(i)
	}

	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
}

// Errors are reported through thread-local storage in the library. If the
// goroutine were allowed to migrate between the render call and the error read,
// concurrent failures would report each other's messages — or none.
func TestConcurrentErrorsAreNotCrossed(t *testing.T) {
	c := ctx(t)
	const goroutines = 8

	var wg sync.WaitGroup
	bad := make(chan string, goroutines)

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := c.RenderURL("http://%zz-not-a-host", testOptions())
			if err == nil {
				bad <- "expected an error"
				return
			}
			var be *Error
			if errors.As(err, &be) && (be.Message == "" || be.Message == "no detail") {
				bad <- "lost the error message across threads"
			}
		}()
	}

	wg.Wait()
	close(bad)
	for msg := range bad {
		t.Error(msg)
	}
}

// Close must wait for in-flight renders rather than freeing under them. Without
// the RWMutex this is a use-after-free, which -race reports and which otherwise
// shows up as an occasional segfault.
func TestCloseDuringRender(t *testing.T) {
	c, err := New(0)
	if err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// Either a successful render or ErrClosed is fine; a crash is not.
			_, err := c.RenderMarkdown("# Racing close", "", testOptions())
			if err != nil && !errors.Is(err, ErrClosed) {
				t.Errorf("unexpected error: %v", err)
			}
		}()
	}

	time.Sleep(10 * time.Millisecond)
	if err := c.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
	wg.Wait()
}

func TestManyContextsSequentially(t *testing.T) {
	if testing.Short() {
		t.Skip("slow under -short")
	}
	// Creating and destroying contexts leaks worker threads if Close is wrong.
	before := runtime.NumGoroutine()
	for i := 0; i < 3; i++ {
		c, err := New(2)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := c.RenderMarkdown("# Cycle", "", testOptions()); err != nil {
			t.Fatal(err)
		}
		if err := c.Close(); err != nil {
			t.Fatal(err)
		}
	}
	if after := runtime.NumGoroutine(); after > before+2 {
		t.Errorf("goroutine count grew from %d to %d", before, after)
	}
}

// --- network ----------------------------------------------------------------

func TestRenderURLToDisk(t *testing.T) {
	if !*network {
		t.Skip("needs network; run with -network")
	}

	opts := DefaultOptions()
	opts.Width = 1200
	opts.Scale = 2

	data, err := ctx(t).RenderURLPNG("https://www.google.com", opts, 144)
	if err != nil {
		t.Fatalf("RenderURLPNG: %v", err)
	}

	img := checkPNG(t, data, "google.png")
	if got := img.Bounds().Dx(); got != 2400 {
		t.Errorf("width = %d, want 2400", got)
	}
	write(t, data, "google.png")
}

func TestRenderFileURL(t *testing.T) {
	abs, err := filepath.Abs("testdata/sample.html")
	if err != nil {
		t.Fatal(err)
	}

	// file:// goes through the URL path rather than being read by the caller.
	url := "file://" + filepath.ToSlash(abs)
	if runtime.GOOS == "windows" {
		url = "file:///" + filepath.ToSlash(abs)
	}

	data, err := ctx(t).RenderURLPNG(url, testOptions(), 144)
	if err != nil {
		t.Fatalf("RenderURLPNG(%s): %v", url, err)
	}
	checkPNG(t, data, "file-url.png")
	write(t, data, "file-url.png")
}

// --- benchmarks -------------------------------------------------------------

func BenchmarkRenderMarkdown(b *testing.B) {
	c, err := New(0)
	if err != nil {
		b.Fatal(err)
	}
	defer c.Close()

	source, err := os.ReadFile("testdata/sample.md")
	if err != nil {
		b.Fatal(err)
	}
	opts := DefaultOptions()
	opts.EnableNet = false
	opts.Width = 900

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := c.RenderMarkdown(string(source), "", opts); err != nil {
			b.Fatal(err)
		}
	}
}
