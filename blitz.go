// Package blitz renders HTML, markdown and web pages to images using the Blitz
// engine (https://github.com/dioxuslabs/blitz), linked as a static archive.
//
// The archives are prebuilt and come from a companion module per platform,
// under github.com/xo/blitz/libblitz/. Each is pulled in by a build-tagged
// blank import in link_GOOS_GOARCH.go, so `go build` downloads only the one
// for the platform being built and the link flags live next to the archives
// they describe. There is no build step; ./build-blitz.sh is for regenerating
// the archives themselves, not for consuming this package.
//
// All exported operations are safe for concurrent use by multiple goroutines.
package blitz

/*
#cgo CFLAGS: -I${SRCDIR}/libblitz

#include <stdlib.h>
#include <string.h>
#include <errno.h>
#include "blitz.h"
*/
import "C"

import (
	"errors"
	"fmt"
	"image"
	"os"
	"runtime"
	"sync"
	"unsafe"
)

// ErrClosed is returned by operations on a Context that has been closed.
var ErrClosed = errors.New("blitz: context is closed")

// ColorScheme selects how `prefers-color-scheme` media queries resolve.
type ColorScheme uint32

const (
	Light ColorScheme = C.BLITZ_COLOR_SCHEME_LIGHT
	Dark  ColorScheme = C.BLITZ_COLOR_SCHEME_DARK
)

// MediaType selects which `@media` rules apply.
type MediaType uint8

const (
	// Screen is the default, and what a screenshot wants.
	Screen MediaType = C.BLITZ_MEDIA_TYPE_SCREEN
	// Print applies `@media print` rules: sites use these to drop nav and ad
	// chrome, switch to serif, and show link targets. Styling only — Blitz has
	// no fragmentation, so `@page` and `page-break-*` are parsed and ignored,
	// and the render is still one continuous image. See WritePDF.
	Print MediaType = C.BLITZ_MEDIA_TYPE_PRINT
)

// Error is a failure reported by the underlying library. Code is one of the
// BLITZ_ERR_* values.
type Error struct {
	Op      string
	Code    int
	Message string
}

func (e *Error) Error() string {
	return fmt.Sprintf("blitz: %s: %s (code %d)", e.Op, e.Message, e.Code)
}

// Status codes, mirroring blitz.h. Compare against Error.Code to branch on the
// kind of failure.
const (
	CodeInvalidArg  = int(C.BLITZ_ERR_INVALID_ARG)
	CodeInvalidUTF8 = int(C.BLITZ_ERR_INVALID_UTF8)
	CodeInvalidURL  = int(C.BLITZ_ERR_INVALID_URL)
	CodeNetwork     = int(C.BLITZ_ERR_NETWORK)
	CodeIO          = int(C.BLITZ_ERR_IO)
	CodeRender      = int(C.BLITZ_ERR_RENDER)
	CodePanic       = int(C.BLITZ_ERR_PANIC)
)

// Options controls a render. The zero value is not meaningful — start from
// DefaultOptions and adjust.
type Options struct {
	// Width of the viewport in CSS pixels.
	Width uint32
	// Minimum height of the viewport in CSS pixels. The render grows past this
	// to fit the document unless FitContentHeight is false.
	Height uint32
	// Device pixel ratio. 2 gives a "retina" render at twice the pixel
	// dimensions.
	Scale float32
	// Upper bound on the rendered height in CSS pixels.
	MaxHeight uint32
	// Light or Dark.
	ColorScheme ColorScheme
	// Backdrop painted under the document as 0xRRGGBBAA. An alpha of 0 leaves
	// the image transparent.
	BackgroundRGBA uint32
	// How long to keep resolving while sub-resources are still loading.
	NetTimeoutMillis uint32
	// Overrides the User-Agent header. Empty uses the library default.
	UserAgent string
	// Fetch sub-resources (images, stylesheets, fonts). Disable for hermetic
	// rendering of self-contained documents.
	EnableNet bool
	// Grow the rendered height to fit the document.
	FitContentHeight bool
	// Screen (default) or Print.
	MediaType MediaType
}

// DefaultOptions returns the library's defaults: 1200x800 CSS pixels at 1x,
// opaque white, light scheme, networking enabled, height grown to fit.
func DefaultOptions() Options {
	d := C.blitz_render_options_default()
	return Options{
		Width:            uint32(d.width),
		Height:           uint32(d.height),
		Scale:            float32(d.scale),
		MaxHeight:        uint32(d.max_height),
		ColorScheme:      ColorScheme(d.color_scheme),
		BackgroundRGBA:   uint32(d.background_rgba),
		NetTimeoutMillis: uint32(d.net_timeout_ms),
		EnableNet:        d.enable_net != 0,
		FitContentHeight: d.fit_content_height != 0,
		MediaType:        MediaType(d.media_type),
	}
}

// Version reports the version of the linked library.
func Version() string {
	return C.GoString(C.blitz_version())
}

// DefaultMarkdownStylesheet returns the built-in CSS used for markdown, so it
// can be extended rather than replaced.
func DefaultMarkdownStylesheet() string {
	return C.GoString(C.blitz_default_markdown_stylesheet())
}

// Context owns the worker threads that back sub-resource fetching.
//
// Creating one is expensive, so create a single Context and share it. All
// methods are safe to call concurrently from any number of goroutines; the
// library serialises the actual rendering internally, so concurrent calls queue
// rather than run in parallel.
type Context struct {
	// mu guards ptr against a concurrent Close. Renders take RLock so they
	// proceed in parallel up to the library's own internal serialisation;
	// Close takes the write lock and so waits for in-flight renders to finish
	// before freeing anything.
	mu  sync.RWMutex
	ptr *C.BlitzContext
}

// New creates a Context. workerThreads of 0 lets the runtime choose based on
// the number of CPUs.
func New(workerThreads uint32) (*Context, error) {
	// blitz_context_new records failures in thread-local storage, so the
	// creation and the error read have to happen on one OS thread. See the
	// comment on call() for why that isn't automatic.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	ptr := C.blitz_context_new(C.uint32_t(workerThreads))
	if ptr == nil {
		return nil, newError("context_new", -1)
	}

	c := &Context{ptr: ptr}
	// Backstop only. Callers should Close explicitly; a finalizer runs at the
	// GC's convenience and may never run at all.
	runtime.SetFinalizer(c, func(c *Context) { c.Close() })
	return c, nil
}

// Close releases the context and joins its worker threads. It blocks until any
// in-flight renders finish. Safe to call multiple times and from any goroutine.
func (c *Context) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.ptr == nil {
		return nil
	}
	C.blitz_context_free(c.ptr)
	c.ptr = nil
	runtime.SetFinalizer(c, nil)
	return nil
}

// call runs fn against the live context pointer with the goroutine pinned to
// one OS thread.
//
// The pinning is not optional. The library reports errors through
// blitz_last_error_message, which reads a *thread-local* slot. A cgo call keeps
// the goroutine on one OS thread for its own duration, but nothing stops the
// scheduler from moving the goroutine between the render call and the
// subsequent error read — which would return another thread's error, or none.
// LockOSThread keeps the pair together.
func (c *Context) call(op string, fn func(ptr *C.BlitzContext) C.int) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.ptr == nil {
		return ErrClosed
	}

	rc := fn(c.ptr)
	// Keep the Context reachable across the C call so the finalizer can't run
	// and free the pointer mid-render.
	runtime.KeepAlive(c)

	if rc != C.BLITZ_OK {
		return newError(op, int(rc))
	}
	return nil
}

// newError copies the thread-local message immediately; it is invalidated by
// the next library call on this thread.
func newError(op string, code int) error {
	msg := "no detail"
	if p := C.blitz_last_error_message(); p != nil {
		msg = C.GoString(p)
	}
	return &Error{Op: op, Code: code, Message: msg}
}

// RenderURL fetches a URL and renders it. A bare host is upgraded to https, and
// file:// URLs are read from disk.
func (c *Context) RenderURL(url string, opts Options) (*image.RGBA, error) {
	cURL := C.CString(url)
	defer C.free(unsafe.Pointer(cURL))

	copts, free := opts.toC()
	defer free()

	var img C.BlitzImage
	err := c.call("render_url", func(ptr *C.BlitzContext) C.int {
		return C.blitz_render_url(ptr, cURL, &copts, &img)
	})
	if err != nil {
		return nil, err
	}
	defer C.blitz_image_free(&img)

	return toImage(&img), nil
}

// RenderHTML renders markup directly. baseURL may be empty, in which case
// relative URLs in the document will not resolve.
func (c *Context) RenderHTML(html, baseURL string, opts Options) (*image.RGBA, error) {
	cHTML := C.CString(html)
	defer C.free(unsafe.Pointer(cHTML))

	cBase, freeBase := optionalCString(baseURL != "", baseURL)
	defer freeBase()

	copts, free := opts.toC()
	defer free()

	var img C.BlitzImage
	err := c.call("render_html", func(ptr *C.BlitzContext) C.int {
		return C.blitz_render_html(ptr, cHTML, cBase, &copts, &img)
	})
	if err != nil {
		return nil, err
	}
	defer C.blitz_image_free(&img)

	return toImage(&img), nil
}

// RenderMarkdown renders a markdown string using the built-in stylesheet.
// GFM tables, footnotes, strikethrough and task lists are enabled.
//
// baseURL may be empty; supply a directory file:// URL if the markdown
// references relative image paths.
func (c *Context) RenderMarkdown(markdown, baseURL string, opts Options) (*image.RGBA, error) {
	return c.renderMarkdown(markdown, baseURL, nil, opts)
}

// RenderMarkdownStyled renders markdown with a stylesheet of your own,
// replacing the built-in one. Pass an empty css string to render unstyled.
func (c *Context) RenderMarkdownStyled(markdown, baseURL, css string, opts Options) (*image.RGBA, error) {
	return c.renderMarkdown(markdown, baseURL, &css, opts)
}

func (c *Context) renderMarkdown(markdown, baseURL string, css *string, opts Options) (*image.RGBA, error) {
	cMD := C.CString(markdown)
	defer C.free(unsafe.Pointer(cMD))

	cBase, freeBase := optionalCString(baseURL != "", baseURL)
	defer freeBase()

	cCSS, freeCSS := optionalCString(css != nil, derefOr(css))
	defer freeCSS()

	copts, free := opts.toC()
	defer free()

	var img C.BlitzImage
	err := c.call("render_markdown", func(ptr *C.BlitzContext) C.int {
		return C.blitz_render_markdown(ptr, cMD, cBase, cCSS, &copts, &img)
	})
	if err != nil {
		return nil, err
	}
	defer C.blitz_image_free(&img)

	return toImage(&img), nil
}

// RenderURLPNG renders a URL straight to encoded PNG bytes, skipping the
// *image.RGBA round trip. dpi of 0 uses 144.
func (c *Context) RenderURLPNG(url string, opts Options, dpi uint32) ([]byte, error) {
	cURL := C.CString(url)
	defer C.free(unsafe.Pointer(cURL))

	copts, free := opts.toC()
	defer free()

	return c.renderToPNG("render_url", dpi, &copts, func(ptr *C.BlitzContext, o *C.BlitzRenderOptions, img *C.BlitzImage) C.int {
		return C.blitz_render_url(ptr, cURL, o, img)
	})
}

// RenderMarkdownPNG renders markdown straight to encoded PNG bytes.
func (c *Context) RenderMarkdownPNG(markdown, baseURL string, opts Options, dpi uint32) ([]byte, error) {
	cMD := C.CString(markdown)
	defer C.free(unsafe.Pointer(cMD))

	cBase, freeBase := optionalCString(baseURL != "", baseURL)
	defer freeBase()

	copts, free := opts.toC()
	defer free()

	return c.renderToPNG("render_markdown", dpi, &copts, func(ptr *C.BlitzContext, o *C.BlitzRenderOptions, img *C.BlitzImage) C.int {
		return C.blitz_render_markdown(ptr, cMD, cBase, nil, o, img)
	})
}

// RenderHTMLPNG renders markup straight to encoded PNG bytes.
func (c *Context) RenderHTMLPNG(html, baseURL string, opts Options, dpi uint32) ([]byte, error) {
	cHTML := C.CString(html)
	defer C.free(unsafe.Pointer(cHTML))

	cBase, freeBase := optionalCString(baseURL != "", baseURL)
	defer freeBase()

	copts, free := opts.toC()
	defer free()

	return c.renderToPNG("render_html", dpi, &copts, func(ptr *C.BlitzContext, o *C.BlitzRenderOptions, img *C.BlitzImage) C.int {
		return C.blitz_render_html(ptr, cHTML, cBase, o, img)
	})
}

// renderToPNG keeps the render and the encode inside one locked-thread window,
// so a failure in either reports the right error message.
func (c *Context) renderToPNG(
	op string,
	dpi uint32,
	copts *C.BlitzRenderOptions,
	render func(*C.BlitzContext, *C.BlitzRenderOptions, *C.BlitzImage) C.int,
) ([]byte, error) {
	var (
		img C.BlitzImage
		buf C.BlitzBuffer
		out []byte
	)

	err := c.call(op, func(ptr *C.BlitzContext) C.int {
		if rc := render(ptr, copts, &img); rc != C.BLITZ_OK {
			return rc
		}
		defer C.blitz_image_free(&img)

		if rc := C.blitz_image_encode_png(&img, C.uint32_t(dpi), &buf); rc != C.BLITZ_OK {
			return rc
		}
		defer C.blitz_buffer_free(&buf)

		out = C.GoBytes(unsafe.Pointer(buf.data), C.int(buf.len))
		return C.BLITZ_OK
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// WriteURLPNG renders a URL and writes the PNG to path.
func (c *Context) WriteURLPNG(url, path string, opts Options, dpi uint32) error {
	data, err := c.RenderURLPNG(url, opts, dpi)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// WriteMarkdownPNG renders markdown and writes the PNG to path.
func (c *Context) WriteMarkdownPNG(markdown, baseURL, path string, opts Options, dpi uint32) error {
	data, err := c.RenderMarkdownPNG(markdown, baseURL, opts, dpi)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// WriteHTMLPNG renders markup and writes the PNG to path.
func (c *Context) WriteHTMLPNG(html, baseURL, path string, opts Options, dpi uint32) error {
	data, err := c.RenderHTMLPNG(html, baseURL, opts, dpi)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// MarkdownToHTML converts markdown to a styled standalone HTML document without
// rendering it. Pass nil css for the built-in stylesheet. This does not touch a
// Context and is safe to call from any goroutine.
func MarkdownToHTML(markdown string, css *string) (string, error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	cMD := C.CString(markdown)
	defer C.free(unsafe.Pointer(cMD))

	cCSS, freeCSS := optionalCString(css != nil, derefOr(css))
	defer freeCSS()

	var buf C.BlitzBuffer
	if rc := C.blitz_markdown_to_html(cMD, cCSS, &buf); rc != C.BLITZ_OK {
		return "", newError("markdown_to_html", int(rc))
	}
	defer C.blitz_buffer_free(&buf)

	return C.GoStringN((*C.char)(unsafe.Pointer(buf.data)), C.int(buf.len)), nil
}

func (o Options) toC() (C.BlitzRenderOptions, func()) {
	copts := C.blitz_render_options_default()
	copts.width = C.uint32_t(o.Width)
	copts.height = C.uint32_t(o.Height)
	copts.scale = C.float(o.Scale)
	copts.max_height = C.uint32_t(o.MaxHeight)
	copts.color_scheme = C.uint32_t(o.ColorScheme)
	copts.background_rgba = C.uint32_t(o.BackgroundRGBA)
	copts.net_timeout_ms = C.uint32_t(o.NetTimeoutMillis)
	copts.enable_net = boolToU8(o.EnableNet)
	copts.fit_content_height = boolToU8(o.FitContentHeight)
	copts.media_type = C.uint8_t(o.MediaType)

	free := func() {}
	if o.UserAgent != "" {
		ua := C.CString(o.UserAgent)
		copts.user_agent = ua
		free = func() { C.free(unsafe.Pointer(ua)) }
	}
	return copts, free
}

func boolToU8(b bool) C.uint8_t {
	if b {
		return 1
	}
	return 0
}

// optionalCString distinguishes "absent" (NULL, which the library reads as "use
// the default") from "present but empty", which is a meaningful value for the
// markdown stylesheet.
func optionalCString(present bool, s string) (*C.char, func()) {
	if !present {
		return nil, func() {}
	}
	c := C.CString(s)
	return c, func() { C.free(unsafe.Pointer(c)) }
}

func derefOr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// toImage copies pixels out of the library's allocation into Go memory, so the
// result outlives the BlitzImage and is not tied to the C allocator.
func toImage(img *C.BlitzImage) *image.RGBA {
	w, h := int(img.width), int(img.height)
	stride := int(img.stride)
	src := C.GoBytes(unsafe.Pointer(img.data), C.int(img.len))

	out := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		copy(out.Pix[y*out.Stride:(y+1)*out.Stride], src[y*stride:(y+1)*stride])
	}
	return out
}
