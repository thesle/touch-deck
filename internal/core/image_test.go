package core

// Image_Manager and Command_Runner tests (tasks 3.3, 3.4, 3.5). These are
// white-box tests in package core so they can construct
// &Backend{configRoot: t.TempDir()} (or reuse newTestBackend from
// config_test.go) and redirect ImagesDir() under a temp directory — no test
// here ever touches the real ~/.config.
//
// Generators and helpers defined here are LOCAL to the image tests and use an
// "img"/"cmd" prefix to avoid colliding with the shared identifiers already in
// gen_test.go and config_test.go (newTestBackend, genValidConfig,
// genLegacyConfig, legacyButton, legacyConfig, keyNoFontSize,
// sortButtonsByOrder, sortPagesByIndex, buttonKeyNoFontSize).

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
)

// reflectTypeBytes is the reflect.Type the byte-content FlatMap produces, so
// gopter knows the generated value's type. reflectTypeConfig is already defined
// in gen_test.go and is not redefined here.
var reflectTypeBytes = reflect.TypeOf([]byte(nil))

// ---------------------------------------------------------------------------
// Local generators for the image tests
// ---------------------------------------------------------------------------

// imgBasenames is a small frozen pool of source basenames used by Property 8.
// It mixes image extensions, varied case, and names with spaces/unicode so the
// naming assertion (<digits>_<basename>) is exercised broadly. The extension
// is irrelevant to CopyImageToConfig (it copies any source), so these vary
// freely.
var imgBasenames = []string{
	"img.png",
	"a.jpg",
	"weird name.jpeg",
	"photo.PNG",
	"icon.svg",
	"anim.gif",
	"pic.webp",
	"no-ext",
	"multi.dot.name.png",
	"дом.jpg",
}

// genImageBasename draws one basename from the frozen pool.
func genImageBasename() gopter.Gen {
	return gen.OneConstOf(imgAsAnySlice(imgBasenames)...)
}

// genImageBytes produces arbitrary content of bounded length (0..2048 bytes),
// including the empty-content edge case.
func genImageBytes() gopter.Gen {
	return gen.SliceOfN(2048, gen.UInt8Range(0, 255)).
		FlatMap(func(v interface{}) gopter.Gen {
			full := v.([]uint8)
			return gen.IntRange(0, len(full)).Map(func(n int) []byte {
				out := make([]byte, n)
				for i := 0; i < n; i++ {
					out[i] = byte(full[i])
				}
				return out
			})
		}, reflectTypeBytes)
}

// imgAsAnySlice converts a []string into []interface{} for gen.OneConstOf.
func imgAsAnySlice(ss []string) []interface{} {
	out := make([]interface{}, len(ss))
	for i, s := range ss {
		out[i] = s
	}
	return out
}

// ---------------------------------------------------------------------------
// Task 3.3 — Property 8: Image copy preserves content and naming
// ---------------------------------------------------------------------------

// Feature: wails-to-gio-migration, Property 8: Image copy preserves content and naming
//
// For any source file with arbitrary bytes and a basename from the frozen pool,
// CopyImageToConfig creates a file in ImagesDir() whose bytes EQUAL the source
// bytes and whose name matches <digits>_<basename> (numeric timestamp prefix +
// underscore + original basename), and returns that destination path (which
// exists and lives inside ImagesDir).
//
// Choices (documented):
//   - The source file is written into a SEPARATE temp source directory, never
//     ImagesDir, so listing/copy semantics are not confounded.
//   - A fresh &Backend{configRoot: t.TempDir()} is used per iteration. This
//     both isolates iterations and sidesteps the (unlikely) chance that
//     time.Now().UnixNano() collides within a tight loop and overwrites a
//     previous dest — the assertion only concerns the returned dest path.
//   - Timestamp-prefix validation splits the base name at the FIRST underscore:
//     the prefix must be non-empty and all ASCII digits, and the remainder must
//     equal the original basename EXACTLY (basenames containing underscores are
//     handled because SplitN(_, 2) keeps the rest intact; none in the pool have
//     leading underscores, so the numeric prefix is unambiguous).
func TestProperty8_ImageCopyContentAndNaming(t *testing.T) {
	params := gopter.DefaultTestParameters()
	params.MinSuccessfulTests = 200

	properties := gopter.NewProperties(params)
	properties.Property("copy preserves bytes and names dest <digits>_<basename>", prop.ForAll(
		func(content []byte, basename string) bool {
			// Fresh backend (temp config root) per iteration.
			b := &Backend{configRoot: t.TempDir()}

			imagesDir, err := b.ImagesDir()
			if err != nil {
				t.Logf("ImagesDir error: %v", err)
				return false
			}

			// Write the source into a SEPARATE temp source dir (not ImagesDir).
			srcDir := t.TempDir()
			srcPath := filepath.Join(srcDir, basename)
			if err := os.WriteFile(srcPath, content, 0644); err != nil {
				t.Logf("write source error: %v", err)
				return false
			}

			dest, err := b.CopyImageToConfig(srcPath)
			if err != nil {
				t.Logf("CopyImageToConfig error: %v", err)
				return false
			}

			// Dest must live inside ImagesDir.
			if filepath.Dir(dest) != imagesDir {
				t.Logf("dest dir = %q, want %q", filepath.Dir(dest), imagesDir)
				return false
			}

			// Name must match <digits>_<basename>.
			gotBase := filepath.Base(dest)
			prefix, rest, found := strings.Cut(gotBase, "_")
			if !found {
				t.Logf("dest base %q has no underscore separator", gotBase)
				return false
			}
			if prefix == "" || !isAllDigits(prefix) {
				t.Logf("dest prefix %q is not a non-empty digit string", prefix)
				return false
			}
			if rest != basename {
				t.Logf("dest suffix %q != original basename %q", rest, basename)
				return false
			}

			// Dest must exist and its bytes must equal the source bytes.
			gotBytes, err := os.ReadFile(dest)
			if err != nil {
				t.Logf("read dest error: %v", err)
				return false
			}
			if !bytes.Equal(gotBytes, content) {
				t.Logf("content mismatch: len(dest)=%d len(src)=%d", len(gotBytes), len(content))
				return false
			}
			return true
		},
		genImageBytes(),
		genImageBasename(),
	))

	properties.TestingRun(t)
}

// isAllDigits reports whether s is non-empty and consists solely of ASCII
// digits 0-9.
func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// ---------------------------------------------------------------------------
// Task 3.4 — Property 9: Image listing extension filter
// ---------------------------------------------------------------------------

// imgImageExts are the frozen image extensions (lower-cased) that
// ListConfigImages must include.
var imgImageExts = []string{".png", ".jpg", ".jpeg", ".gif", ".webp", ".svg"}

// imgExtPool mixes image extensions in varied case with non-image extensions,
// so the generated filenames exercise both "kept" and "excluded" outcomes and
// the case-insensitive matching.
var imgExtPool = []string{
	".png", ".PNG", ".jpg", ".Jpg", ".JPEG", ".jpeg",
	".gif", ".GIF", ".webp", ".WEBP", ".svg", ".SVG",
	".txt", ".md", ".exe", ".", "", ".pngx", ".jpg.bak",
}

// genImageFilename builds a filename as <stem><ext> where stem is a short
// identifier (kept simple/unique-ish) and ext is drawn from the mixed pool.
func genImageFilename() gopter.Gen {
	return gopter.CombineGens(
		gen.Identifier().SuchThat(func(s string) bool { return s != "" }),
		gen.OneConstOf(imgAsAnySlice(imgExtPool)...),
	).Map(func(vals []interface{}) string {
		return vals[0].(string) + vals[1].(string)
	})
}

// genImageFileSet produces a bounded, variable-length set of filenames to place
// in ImagesDir, plus a boolean deciding whether to also create a subdirectory
// containing an image-named entry (which must be excluded).
func genImageFileSet() gopter.Gen {
	return gopter.CombineGens(
		gen.SliceOf(genImageFilename()), // variable-length list of filenames
		gen.Bool(),                      // create a subdirectory with an image inside
	).Map(func(vals []interface{}) imgFileSet {
		return imgFileSet{
			Names:      vals[0].([]string),
			WithSubdir: vals[1].(bool),
		}
	})
}

// imgFileSet describes the files (and optional subdir) to seed into ImagesDir
// for a Property 9 iteration.
type imgFileSet struct {
	Names      []string
	WithSubdir bool
}

// Feature: wails-to-gio-migration, Property 9: Image listing extension filter
//
// For any set of files placed in ImagesDir (mix of image extensions with varied
// case, non-image extensions, and optionally a subdirectory containing an
// image-named entry), ListConfigImages returns EXACTLY the paths of files whose
// lower-cased extension is in {.png,.jpg,.jpeg,.gif,.webp,.svg} — including
// every such file, excluding every non-image file and all subdirectories.
//
// Choices (documented):
//   - Fresh &Backend{configRoot: t.TempDir()} per iteration for a clean dir.
//   - Duplicate generated names collapse to a single file on disk (WriteFile
//     overwrites); the expected set is computed from the actual on-disk entries
//     (os.ReadDir) rather than from the raw generated names, so this is exact.
//   - Comparison is order-insensitive: both expected and actual paths are
//     collected into sorted slices and compared element-wise (a set compare).
//   - A subdirectory is (sometimes) created with an image extension in its name;
//     ListConfigImages skips dir entries, so it must be absent from the result.
func TestProperty9_ImageListingExtensionFilter(t *testing.T) {
	params := gopter.DefaultTestParameters()
	params.MinSuccessfulTests = 200

	properties := gopter.NewProperties(params)
	properties.Property("lists exactly image-extension files, excluding others and subdirs", prop.ForAll(
		func(fs imgFileSet) bool {
			b := &Backend{configRoot: t.TempDir()}
			imagesDir, err := b.ImagesDir()
			if err != nil {
				t.Logf("ImagesDir error: %v", err)
				return false
			}

			// Seed the files (touch empty files).
			for _, name := range fs.Names {
				p := filepath.Join(imagesDir, name)
				if err := os.WriteFile(p, []byte{}, 0644); err != nil {
					t.Logf("write %q error: %v", name, err)
					return false
				}
			}
			// Optionally create a subdirectory with an image-named entry; it
			// must be excluded because ReadDir dir entries are skipped.
			if fs.WithSubdir {
				sub := filepath.Join(imagesDir, "sub.png")
				if err := os.MkdirAll(sub, 0755); err != nil {
					t.Logf("mkdir subdir error: %v", err)
					return false
				}
				if err := os.WriteFile(filepath.Join(sub, "nested.png"), []byte{}, 0644); err != nil {
					t.Logf("write nested error: %v", err)
					return false
				}
			}

			// Compute expected set from the ACTUAL on-disk entries: files
			// (non-dir) whose lower-cased ext is in the frozen set.
			entries, err := os.ReadDir(imagesDir)
			if err != nil {
				t.Logf("ReadDir error: %v", err)
				return false
			}
			expected := []string{}
			for _, e := range entries {
				if e.IsDir() {
					continue
				}
				if imgIsImageExt(e.Name()) {
					expected = append(expected, filepath.Join(imagesDir, e.Name()))
				}
			}

			got, err := b.ListConfigImages()
			if err != nil {
				t.Logf("ListConfigImages error: %v", err)
				return false
			}

			return imgSameSet(expected, got)
		},
		genImageFileSet(),
	))

	properties.TestingRun(t)
}

// imgIsImageExt reports whether name's lower-cased extension is in the frozen
// image set.
func imgIsImageExt(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	for _, ie := range imgImageExts {
		if ext == ie {
			return true
		}
	}
	return false
}

// imgSameSet reports whether a and b contain the same paths, treated as sets
// (order-insensitive, and — given unique on-disk paths — duplicate-free).
func imgSameSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	ac := append([]string(nil), a...)
	bc := append([]string(nil), b...)
	sort.Strings(ac)
	sort.Strings(bc)
	for i := range ac {
		if ac[i] != bc[i] {
			return false
		}
	}
	return true
}

// ---------------------------------------------------------------------------
// Task 3.5 — Integration tests: Command_Runner (real bash) and image copy
// ---------------------------------------------------------------------------
//
// These are example-based integration tests that shell out to the real `bash`
// on the Linux target (Requirements 5.4, 5.5, 13.2). The timeout subtest is a
// genuine >10s check (the timeout is hard-coded to 10 seconds), so it is
// skipped under `go test -short`.

// RunCommandSync captures stdout on a successful command (Req 5.4).
func TestRunCommandSync_EchoStdout(t *testing.T) {
	b := newTestBackend(t)
	out, err := b.RunCommandSync("echo hello")
	if err != nil {
		t.Fatalf("RunCommandSync error: %v (output=%q)", err, out)
	}
	if !strings.Contains(out, "hello") {
		t.Errorf("output = %q, want it to contain %q", out, "hello")
	}
}

// RunCommandSync captures stderr and returns an error on a non-zero exit
// (Req 5.4 — combined output includes stderr).
func TestRunCommandSync_StderrAndNonZeroExit(t *testing.T) {
	b := newTestBackend(t)
	out, err := b.RunCommandSync("echo oops >&2; exit 3")
	if err == nil {
		t.Errorf("expected non-nil error for non-zero exit, got nil (output=%q)", out)
	}
	if !strings.Contains(out, "oops") {
		t.Errorf("combined output = %q, want it to contain stderr text %q", out, "oops")
	}
}

// RunCommandSync cancels and returns a timeout error when the command exceeds
// the 10 second deadline (Req 5.5). This is a real >10s test, so it is skipped
// in -short mode. It takes ~10s when run.
func TestRunCommandSync_Timeout(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping ~10s real timeout test in -short mode")
	}
	b := newTestBackend(t)
	out, err := b.RunCommandSync("sleep 11")
	if err == nil {
		t.Fatalf("expected timeout error, got nil (output=%q)", out)
	}
	if !strings.Contains(strings.ToLower(err.Error()), "timed out") {
		t.Errorf("error = %v, want it to mention a timeout", err)
	}
}

// RunCommandAsync returns a nil start error for a command that starts fine
// (Req 5.1). bash -c always starts, so a genuine start failure is not triggered
// here.
func TestRunCommandAsync_StartsSuccessfully(t *testing.T) {
	b := newTestBackend(t)
	if err := b.RunCommandAsync("true"); err != nil {
		t.Errorf("RunCommandAsync(\"true\") returned error: %v", err)
	}
}

// CopyImageToConfig round trip: a concrete example complementing Property 8
// (Req 13.2). Copies a small source file and verifies the dest exists, its
// bytes equal the source, and its name carries a numeric timestamp prefix.
func TestCopyImageToConfig_RoundTrip(t *testing.T) {
	b := newTestBackend(t)

	srcDir := t.TempDir()
	srcPath := filepath.Join(srcDir, "example.png")
	content := []byte("fake-png-bytes-\x00\x01\x02")
	if err := os.WriteFile(srcPath, content, 0644); err != nil {
		t.Fatalf("write source error: %v", err)
	}

	dest, err := b.CopyImageToConfig(srcPath)
	if err != nil {
		t.Fatalf("CopyImageToConfig error: %v", err)
	}

	// Dest exists and bytes match.
	gotBytes, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("read dest error: %v", err)
	}
	if !bytes.Equal(gotBytes, content) {
		t.Errorf("dest bytes != source bytes")
	}

	// Name has <digits>_example.png form.
	base := filepath.Base(dest)
	prefix, rest, found := strings.Cut(base, "_")
	if !found || !isAllDigits(prefix) || rest != "example.png" {
		t.Errorf("dest name = %q, want <digits>_example.png", base)
	}

	// Empty source is a no-op returning ("", nil).
	emptyDest, err := b.CopyImageToConfig("")
	if err != nil || emptyDest != "" {
		t.Errorf("CopyImageToConfig(\"\") = (%q, %v), want (\"\", nil)", emptyDest, err)
	}
}
