package blitz

import (
	"bytes"
	"strings"
	"testing"
)

// A document whose print stylesheet does something measurable: the banner is
// tall and display:none in print, so the print render must come out shorter.
var mediaDoc = `<html><head><style>
body { margin: 0; padding: 16px; font-family: serif; background: #fff; }
#banner { height: 200px; background: #c00; }
@media print { #banner { display: none; } }
p { font-size: 16px; line-height: 1.5; }
</style></head><body>
<div id="banner">navigation</div>
<h1>Report</h1>
<p>Body text that is present in both media types so neither render is blank.</p>
` + strings.Repeat("<p>Filler so the render clears the minimum viewport height, "+
	"otherwise both media types clamp to it and the hidden banner is invisible.</p>", 30) +
	`</body></html>`

func TestMediaTypePrintAppliesPrintStylesheet(t *testing.T) {
	c := ctx(t)
	o := testOptions()
	o.Width = 600
	o.FitContentHeight = true

	o.MediaType = Screen
	screen, err := c.RenderHTML(mediaDoc, "", o)
	if err != nil {
		t.Fatalf("render screen: %v", err)
	}

	o.MediaType = Print
	print_, err := c.RenderHTML(mediaDoc, "", o)
	if err != nil {
		t.Fatalf("render print: %v", err)
	}

	sh, ph := screen.Bounds().Dy(), print_.Bounds().Dy()
	if ph >= sh {
		t.Errorf("print render (%dpx) should be shorter than screen (%dpx): "+
			"@media print { #banner { display: none } } did not apply", ph, sh)
	}
	// The banner is 200 CSS px at scale 2.
	if want := 400; sh-ph != want {
		t.Errorf("height delta = %d, want %d (the hidden banner)", sh-ph, want)
	}
}

func TestMediaTypeDefaultsToScreen(t *testing.T) {
	if got := DefaultOptions().MediaType; got != Screen {
		t.Errorf("DefaultOptions().MediaType = %d, want Screen (%d)", got, Screen)
	}
	var zero Options
	if zero.MediaType != Screen {
		t.Errorf("zero Options.MediaType = %d, want Screen (%d)", zero.MediaType, Screen)
	}
}

func TestEncodePDFSinglePage(t *testing.T) {
	c := ctx(t)
	o := testOptions()
	o.Width = 600
	o.FitContentHeight = true

	img, err := c.RenderHTML(mediaDoc, "", o)
	if err != nil {
		t.Fatalf("render: %v", err)
	}

	data, err := EncodePDF(img, PDFOptions{})
	if err != nil {
		t.Fatalf("EncodePDF: %v", err)
	}
	checkPDF(t, data, 1)
	write(t, data, "single.pdf")
}

func TestEncodePDFPaginated(t *testing.T) {
	c := ctx(t)
	o := testOptions()
	o.Width = 600
	o.FitContentHeight = true
	o.MaxHeight = 8000

	body := mediaDoc
	for i := 0; i < 120; i++ {
		body += "<p>Filler line to push the document past a single A4 page.</p>"
	}

	img, err := c.RenderHTML(body, "", o)
	if err != nil {
		t.Fatalf("render: %v", err)
	}

	data, err := EncodePDF(img, PDFOptions{Page: A4, Paginate: true, MarginPt: 36})
	if err != nil {
		t.Fatalf("EncodePDF: %v", err)
	}
	n := checkPDF(t, data, 0)
	if n < 2 {
		t.Errorf("paginated output has %d page(s), want >= 2", n)
	}
	if !bytes.Contains(data, []byte("/MediaBox [0 0 595.2760 841.8900]")) {
		t.Error("pages are not A4")
	}
	write(t, data, "paginated.pdf")
}

func TestEncodePDFRejectsBadInput(t *testing.T) {
	if _, err := EncodePDF(nil, PDFOptions{}); err == nil {
		t.Error("EncodePDF(nil) should fail")
	}

	c := ctx(t)
	o := testOptions()
	o.Width = 600
	img, err := c.RenderHTML(mediaDoc, "", o)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	// A margin that swallows the page has no content area left.
	if _, err := EncodePDF(img, PDFOptions{Page: A4, Paginate: true, MarginPt: 999}); err == nil {
		t.Error("EncodePDF with an oversized margin should fail")
	}
}

// checkPDF asserts the bytes are a structurally plausible PDF and returns the
// page count. wantPages of 0 means "don't check".
func checkPDF(t *testing.T, data []byte, wantPages int) int {
	t.Helper()

	if !bytes.HasPrefix(data, []byte("%PDF-1.")) {
		t.Fatalf("not a PDF (%d bytes)", len(data))
	}
	if !bytes.HasSuffix(bytes.TrimRight(data, "\n"), []byte("%%EOF")) {
		t.Error("missing EOF trailer")
	}
	for _, want := range []string{"/Type /Catalog", "/Type /Pages", "startxref", "trailer"} {
		if !bytes.Contains(data, []byte(want)) {
			t.Errorf("PDF is missing %q", want)
		}
	}
	if len(data) < 2048 {
		t.Errorf("suspiciously small PDF (%d bytes)", len(data))
	}

	n := bytes.Count(data, []byte("/Type /Page\n")) + bytes.Count(data, []byte("/Type /Page "))
	if wantPages > 0 && n != wantPages {
		t.Errorf("page count = %d, want %d", n, wantPages)
	}
	return n
}
