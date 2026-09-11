// Command blitz-render renders a URL, an HTML file or a markdown file to a PNG.
//
//	blitz-render https://example.com
//	blitz-render README.md -o readme.png -w 900
//	blitz-render page.html -dark -scale 2
package main

import (
	"flag"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/xo/blitz"
)

func main() {
	var (
		out       = flag.String("o", "", "output path (default: derived from the input)")
		width     = flag.Uint("w", 1200, "viewport width in CSS pixels")
		height    = flag.Uint("h", 800, "minimum viewport height in CSS pixels")
		scale     = flag.Float64("scale", 2, "device pixel ratio")
		dpi       = flag.Uint("dpi", 144, "PNG pixel density")
		dark      = flag.Bool("dark", false, "render with prefers-color-scheme: dark")
		noNet     = flag.Bool("no-net", false, "don't fetch sub-resources")
		clear     = flag.Bool("transparent", false, "transparent background")
		timeout   = flag.Uint("timeout", 10000, "sub-resource timeout in ms")
		userAgent = flag.String("ua", "", "override the User-Agent header")
	)

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: %s [flags] <url|file.html|file.md>\n\n", os.Args[0])
		flag.PrintDefaults()
	}
	flag.Parse()

	if flag.NArg() != 1 {
		flag.Usage()
		os.Exit(2)
	}
	input := flag.Arg(0)

	opts := blitz.DefaultOptions()
	opts.Width = uint32(*width)
	opts.Height = uint32(*height)
	opts.Scale = float32(*scale)
	opts.NetTimeoutMillis = uint32(*timeout)
	opts.UserAgent = *userAgent
	opts.EnableNet = !*noNet
	if *dark {
		opts.ColorScheme = blitz.Dark
	}
	if *clear {
		opts.BackgroundRGBA = 0x00000000
	}

	dest := *out
	if dest == "" {
		dest = deriveOutput(input)
	}

	ctx, err := blitz.New(0)
	if err != nil {
		fail(err)
	}
	defer ctx.Close()

	switch {
	case isURL(input):
		fmt.Printf("url      %s\n", input)
		err = ctx.WriteURLPNG(input, dest, opts, uint32(*dpi))

	case isMarkdown(input):
		fmt.Printf("markdown %s\n", input)
		source, readErr := os.ReadFile(input)
		if readErr != nil {
			fail(readErr)
		}
		err = ctx.WriteMarkdownPNG(string(source), baseURL(input), dest, opts, uint32(*dpi))

	default:
		fmt.Printf("html     %s\n", input)
		source, readErr := os.ReadFile(input)
		if readErr != nil {
			fail(readErr)
		}
		err = ctx.WriteHTMLPNG(string(source), baseURL(input), dest, opts, uint32(*dpi))
	}

	if err != nil {
		fail(err)
	}

	info, err := os.Stat(dest)
	if err != nil {
		fail(err)
	}
	fmt.Printf("wrote    %s (%d KiB)\n", dest, info.Size()/1024)
}

func isURL(s string) bool {
	u, err := url.Parse(s)
	return err == nil && u.Scheme != "" && u.Host != "" || strings.HasPrefix(s, "file://")
}

func isMarkdown(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".md", ".markdown", ".mdown", ".mkd":
		return true
	}
	return false
}

// baseURL gives relative asset paths inside a local document something to
// resolve against.
func baseURL(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		return ""
	}
	dir := filepath.ToSlash(filepath.Dir(abs))
	if !strings.HasPrefix(dir, "/") {
		dir = "/" + dir // Windows: C:/x -> /C:/x
	}
	return "file://" + dir + "/"
}

func deriveOutput(input string) string {
	if isURL(input) {
		u, err := url.Parse(input)
		if err == nil && u.Host != "" {
			return strings.NewReplacer(".", "", ":", "").Replace(u.Host) + ".png"
		}
		return "output.png"
	}
	base := filepath.Base(input)
	return strings.TrimSuffix(base, filepath.Ext(base)) + ".png"
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}
