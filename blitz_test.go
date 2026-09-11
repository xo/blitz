package blitz

import (
	"bytes"
	"errors"
	"flag"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
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
