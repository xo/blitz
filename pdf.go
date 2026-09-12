package blitz

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"image"
	"io"
	"os"
)

// PageSize is a PDF page in points (1/72 inch).
type PageSize struct {
	WidthPt, HeightPt float64
}

// Common page sizes.
var (
	A4     = PageSize{595.276, 841.890}
	Letter = PageSize{612, 792}
)

// PDFOptions controls how a render is turned into a PDF.
//
// Blitz has no fragmentation, so nothing here can break pages at sensible
// boundaries — see Paginate.
type PDFOptions struct {
	// Page geometry. Zero uses the image's own aspect at 96 DPI, giving one
	// page exactly as tall as the render.
	Page PageSize

	// Paginate slices the render into Page-sized pages instead of emitting a
	// single tall one.
	//
	// The slicing is geometric, not semantic: Blitz cannot fragment layout, so
	// a cut lands wherever the arithmetic puts it, including through a line of
	// text, a table row, or an image. Use it when the alternative is one
	// unprintable 20,000px page; do not expect print-quality output.
	Paginate bool

	// DPI at which CSS pixels are mapped to PDF points when Page is set.
	// 0 -> 96, the CSS reference pixel.
	DPI float64

	// MarginPt is applied on all four sides when Paginate is set. Ignored
	// otherwise.
	MarginPt float64
}

// pdfPage is one emitted page: a source rect in image space, the page box, and
// where the slice is drawn on it. All Pt values are PDF points.
type pdfPage struct {
	x, y, w, h       int
	wPt, hPt         float64
	offXPt, offYPt   float64
	drawWPt, drawHPt float64
}

// EncodePDF renders img into a PDF document.
//
// The image is embedded as a Flate-compressed 24-bit RGB stream. That keeps
// this dependency-free, at the cost of a larger file than a JPEG-backed PDF
// would produce. Alpha is composited onto white, since PDF image XObjects
// have no alpha without a separate soft mask.
func EncodePDF(img *image.RGBA, opts PDFOptions) ([]byte, error) {
	if img == nil {
		return nil, fmt.Errorf("blitz: EncodePDF: nil image")
	}
	b := img.Bounds()
	if b.Dx() <= 0 || b.Dy() <= 0 {
		return nil, fmt.Errorf("blitz: EncodePDF: empty image %v", b)
	}

	dpi := opts.DPI
	if dpi <= 0 {
		dpi = 96
	}
	ptPerPx := 72 / dpi

	var pages []pdfPage

	switch {
	case !opts.Paginate:
		// One page sized to the render.
		wPt, hPt := float64(b.Dx())*ptPerPx, float64(b.Dy())*ptPerPx
		if opts.Page.WidthPt > 0 && opts.Page.HeightPt > 0 {
			// Fit the whole render inside the requested page, preserving aspect.
			wPt, hPt = opts.Page.WidthPt, opts.Page.HeightPt
			sx, sy := wPt/float64(b.Dx()), hPt/float64(b.Dy())
			s := sx
			if sy < s {
				s = sy
			}
			dw, dh := float64(b.Dx())*s, float64(b.Dy())*s
			pages = append(pages, pdfPage{
				x: 0, y: 0, w: b.Dx(), h: b.Dy(),
				wPt: wPt, hPt: hPt,
				offXPt: (wPt - dw) / 2, offYPt: hPt - dh,
				drawWPt: dw, drawHPt: dh,
			})
			break
		}
		pages = append(pages, pdfPage{
			x: 0, y: 0, w: b.Dx(), h: b.Dy(),
			wPt: wPt, hPt: hPt,
			offXPt: 0, offYPt: 0,
			drawWPt: wPt, drawHPt: hPt,
		})

	default:
		ps := opts.Page
		if ps.WidthPt <= 0 || ps.HeightPt <= 0 {
			ps = A4
		}
		m := opts.MarginPt
		contentWPt, contentHPt := ps.WidthPt-2*m, ps.HeightPt-2*m
		if contentWPt <= 0 || contentHPt <= 0 {
			return nil, fmt.Errorf("blitz: EncodePDF: margin %.1fpt leaves no content area on %.0fx%.0fpt", m, ps.WidthPt, ps.HeightPt)
		}
		// Scale so the render's width fills the content width, then cut the
		// height into content-height strips.
		scale := contentWPt / float64(b.Dx())
		stripPx := int(contentHPt / scale)
		if stripPx < 1 {
			stripPx = 1
		}
		for y := 0; y < b.Dy(); y += stripPx {
			h := stripPx
			if y+h > b.Dy() {
				h = b.Dy() - y
			}
			pages = append(pages, pdfPage{
				x: 0, y: y, w: b.Dx(), h: h,
				wPt: ps.WidthPt, hPt: ps.HeightPt,
				offXPt: m, offYPt: ps.HeightPt - m - float64(h)*scale,
				drawWPt: contentWPt, drawHPt: float64(h) * scale,
			})
		}
	}

	return buildPDF(img, pages)
}

// WritePDF renders img to a PDF at path.
func WritePDF(img *image.RGBA, path string, opts PDFOptions) error {
	data, err := EncodePDF(img, opts)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// rgbStream extracts a sub-rectangle of img as 24-bit RGB, compositing alpha
// onto white, and Flate-compresses it.
func rgbStream(img *image.RGBA, x, y, w, h int) ([]byte, error) {
	raw := make([]byte, 0, w*h*3)
	for row := y; row < y+h; row++ {
		i := img.PixOffset(img.Bounds().Min.X+x, img.Bounds().Min.Y+row)
		line := img.Pix[i : i+w*4]
		for c := 0; c < len(line); c += 4 {
			r, g, b, a := line[c], line[c+1], line[c+2], line[c+3]
			if a != 0xFF {
				// Go's RGBA is alpha-premultiplied, so un-premultiplying is not
				// needed to composite onto white: dst = src + white*(1-a).
				inv := 255 - uint32(a)
				r = uint8((uint32(r)*255 + 255*inv) / 255)
				g = uint8((uint32(g)*255 + 255*inv) / 255)
				b = uint8((uint32(b)*255 + 255*inv) / 255)
			}
			raw = append(raw, r, g, b)
		}
	}
	var zb bytes.Buffer
	zw, err := zlib.NewWriterLevel(&zb, zlib.BestSpeed)
	if err != nil {
		return nil, err
	}
	if _, err := zw.Write(raw); err != nil {
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return zb.Bytes(), nil
}

func buildPDF(img *image.RGBA, pages []pdfPage) ([]byte, error) {
	var buf bytes.Buffer
	// Object 0 is the free-list head; offsets[n] is the byte offset of object n.
	offsets := []int{0}
	obj := func(body func(w io.Writer)) int {
		n := len(offsets)
		offsets = append(offsets, buf.Len())
		fmt.Fprintf(&buf, "%d 0 obj\n", n)
		body(&buf)
		buf.WriteString("\nendobj\n")
		return n
	}

	buf.WriteString("%PDF-1.4\n%\xE2\xE3\xCF\xD3\n")

	// Reserve 1=Catalog, 2=Pages; emit them last, once the kids are known.
	offsets = append(offsets, 0, 0)

	pageIDs := make([]int, 0, len(pages))
	for _, pg := range pages {
		data, err := rgbStream(img, pg.x, pg.y, pg.w, pg.h)
		if err != nil {
			return nil, err
		}
		imgID := obj(func(w io.Writer) {
			fmt.Fprintf(w, "<< /Type /XObject /Subtype /Image /Width %d /Height %d "+
				"/ColorSpace /DeviceRGB /BitsPerComponent 8 /Filter /FlateDecode /Length %d >>\nstream\n",
				pg.w, pg.h, len(data))
			w.Write(data)
			io.WriteString(w, "\nendstream")
		})
		content := fmt.Sprintf("q\n%.4f 0 0 %.4f %.4f %.4f cm\n/Im0 Do\nQ\n",
			pg.drawWPt, pg.drawHPt, pg.offXPt, pg.offYPt)
		contentID := obj(func(w io.Writer) {
			fmt.Fprintf(w, "<< /Length %d >>\nstream\n%s\nendstream", len(content), content)
		})
		pageID := obj(func(w io.Writer) {
			fmt.Fprintf(w, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 %.4f %.4f] "+
				"/Resources << /XObject << /Im0 %d 0 R >> >> /Contents %d 0 R >>",
				pg.wPt, pg.hPt, imgID, contentID)
		})
		pageIDs = append(pageIDs, pageID)
	}

	offsets[2] = buf.Len()
	buf.WriteString("2 0 obj\n<< /Type /Pages /Count ")
	fmt.Fprintf(&buf, "%d /Kids [", len(pageIDs))
	for i, id := range pageIDs {
		if i > 0 {
			buf.WriteByte(' ')
		}
		fmt.Fprintf(&buf, "%d 0 R", id)
	}
	buf.WriteString("] >>\nendobj\n")

	offsets[1] = buf.Len()
	buf.WriteString("1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n")

	start := buf.Len()
	fmt.Fprintf(&buf, "xref\n0 %d\n", len(offsets))
	buf.WriteString("0000000000 65535 f \n")
	for _, off := range offsets[1:] {
		fmt.Fprintf(&buf, "%010d 00000 n \n", off)
	}
	fmt.Fprintf(&buf, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n",
		len(offsets), start)
	return buf.Bytes(), nil
}
