package blitz_test

import (
	"debug/elf"
	"debug/macho"
	"debug/pe"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// TestArchivesHaveNoLTOSections holds D29. Go 1.27.4 refuses to link an archive
// named in #cgo LDFLAGS if any member holds LLVM bitcode or GCC LTO data, and
// it says "disallowed LTO section". It reads every member of every archive.
// Go 1.27.3 did not, and the macOS archives of libblitz v0.1.5 held bitcode
// sections that only the ELF names were removed from.
//
// This repeats that check with the standard library, so that a Go older than
// 1.27.4 still finds the fault before a consumer does. It rejects a Mach-O
// segment called __LLVM or __GNU_LTO and any section in one, and an ELF or COFF
// section called .llvmbc, .llvmcmd, .llvm.lto* or .gnu.lto_*.
func TestArchivesHaveNoLTOSections(t *testing.T) {
	t.Parallel()
	archives, err := filepath.Glob(filepath.Join("libblitz", "*", "*.a"))
	if err != nil {
		t.Fatal(err)
	}
	if len(archives) < 10 {
		t.Fatalf("found %d archives under libblitz, expected at least 10", len(archives))
	}
	for _, path := range archives {
		t.Run(filepath.ToSlash(path), func(t *testing.T) {
			t.Parallel()
			var members, objects int
			err := walkArchive(path, func(name string, r io.ReaderAt, size int64) {
				members++
				sections, ok := objectSections(r, size)
				if !ok {
					return
				}
				objects++
				for _, s := range sections {
					if lto(s) {
						t.Errorf("%s: member %s holds %s. Remove it with llvm-objcopy in slim_archive. See D29",
							path, name, s)
						return
					}
				}
			})
			if err != nil {
				t.Fatalf("reading %s: %v", path, err)
			}
			if objects == 0 {
				t.Errorf("%s: none of its %d members is an object file, so this test guards nothing", path, members)
			}
		})
	}
}

// lto reports whether a section or segment, written as the objectSections
// function writes it, is one that Go 1.27.4 refuses.
func lto(s string) bool {
	if seg, _, ok := strings.Cut(s, ","); ok {
		return seg == "__LLVM" || seg == "__GNU_LTO"
	}
	switch s {
	case ".llvmbc", ".llvmcmd":
		return true
	}
	return strings.HasPrefix(s, ".llvm.lto") || strings.HasPrefix(s, ".gnu.lto_")
}

// objectSections lists the sections of one archive member as an ELF, a Mach-O
// or a COFF object. A Mach-O segment is listed as "<segment>," and each of its
// sections as "<segment>,<section>". It reports false for a member that is not
// an object file, such as the import stubs of a Windows archive.
func objectSections(r io.ReaderAt, size int64) ([]string, bool) {
	sr := io.NewSectionReader(r, 0, size)
	if f, err := elf.NewFile(sr); err == nil {
		defer f.Close()
		var out []string
		for _, s := range f.Sections {
			out = append(out, s.Name)
		}
		return out, true
	}
	if f, err := macho.NewFile(sr); err == nil {
		defer f.Close()
		var out []string
		for _, l := range f.Loads {
			if seg, ok := l.(*macho.Segment); ok {
				out = append(out, seg.Name+",")
			}
		}
		for _, s := range f.Sections {
			out = append(out, s.Seg+","+s.Name)
		}
		return out, true
	}
	if f, err := pe.NewFile(sr); err == nil {
		defer f.Close()
		var out []string
		for _, s := range f.Sections {
			out = append(out, s.Name)
		}
		return out, true
	}
	return nil, false
}

// walkArchive calls fn for each member of an ar archive that holds data, which
// is every member except the symbol table and the table of long names. It reads
// the System V and the BSD forms, where the BSD form puts a long name at the
// start of the member as "#1/<length>".
func walkArchive(path string, fn func(name string, r io.ReaderAt, size int64)) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	magic := make([]byte, 8)
	if _, err := f.ReadAt(magic, 0); err != nil || string(magic) != "!<arch>\n" {
		return fmt.Errorf("not an ar archive")
	}
	for off := int64(8); off < info.Size(); {
		hdr := make([]byte, 60)
		if _, err := f.ReadAt(hdr, off); err != nil {
			return fmt.Errorf("reading the member header at %d: %w", off, err)
		}
		if string(hdr[58:60]) != "`\n" {
			return fmt.Errorf("bad member header at %d", off)
		}
		name := strings.TrimRight(string(hdr[0:16]), " ")
		size, err := strconv.ParseInt(strings.TrimSpace(string(hdr[48:58])), 10, 64)
		if err != nil {
			return fmt.Errorf("bad member size at %d: %w", off, err)
		}
		data := off + 60
		if n, ok := strings.CutPrefix(name, "#1/"); ok {
			length, err := strconv.ParseInt(n, 10, 64)
			if err != nil || length > size {
				return fmt.Errorf("bad BSD member name at %d", off)
			}
			raw := make([]byte, length)
			if _, err := f.ReadAt(raw, data); err != nil {
				return fmt.Errorf("reading the member name at %d: %w", off, err)
			}
			name = strings.TrimRight(string(raw), "\x00")
			data += length
			size -= length
		}
		switch name {
		case "/", "//", "/SYM64/", "__.SYMDEF", "__.SYMDEF SORTED", "__.SYMDEF_64", "__.SYMDEF_64 SORTED":
		default:
			fn(name, io.NewSectionReader(f, data, size), size)
		}
		off = data + size
		off += off & 1
	}
	return nil
}
