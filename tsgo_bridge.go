package main

/*
#include <stdlib.h>
*/
import "C"
import (
	"context"
	"os"
	"strings"
	"time"

	"github.com/microsoft/typescript-go/internal/ast"
	"github.com/microsoft/typescript-go/internal/compiler"
	"github.com/microsoft/typescript-go/internal/core"
	"github.com/microsoft/typescript-go/internal/diagnostics"
	"github.com/microsoft/typescript-go/internal/parser"
	"github.com/microsoft/typescript-go/internal/tsoptions"
	"github.com/microsoft/typescript-go/internal/tspath"
	"github.com/microsoft/typescript-go/internal/vfs"
)

// --- FS and Host implementations (Keep as before) ---
type fsWrapper struct{ files map[string]string }

func (w *fsWrapper) UseCaseSensitiveFileNames() bool               { return false }
func (w *fsWrapper) FileExists(path string) bool                   { _, ok := w.files[path]; return ok }
func (w *fsWrapper) ReadFile(path string) (string, bool)           { s, ok := w.files[path]; return s, ok }
func (w *fsWrapper) WriteFile(path string, data string) error      { w.files[path] = data; return nil }
func (w *fsWrapper) AppendFile(path string, data string) error     { w.files[path] += data; return nil }
func (w *fsWrapper) Remove(path string) error                      { delete(w.files, path); return nil }
func (w *fsWrapper) Chtimes(path string, a, m time.Time) error     { return nil }
func (w *fsWrapper) DirectoryExists(path string) bool              { return false }
func (w *fsWrapper) GetAccessibleEntries(path string) vfs.Entries  { return vfs.Entries{} }
func (w *fsWrapper) Stat(path string) vfs.FileInfo                 { return nil }
func (w *fsWrapper) WalkDir(root string, fn vfs.WalkDirFunc) error { return nil }
func (w *fsWrapper) Realpath(path string) string                   { return path }

type fullHost struct{ fs vfs.FS }

func (h *fullHost) FS() vfs.FS                                  { return h.fs }
func (h *fullHost) GetCanonicalFileName(path string) string     { return path }
func (h *fullHost) GetCurrentDirectory() string                 { return "/" }
func (h *fullHost) GetDefaultLibFileName(options any) string    { return "lib.d.ts" }
func (h *fullHost) GetNewLine() string                          { return "\n" }
func (h *fullHost) UseCaseSensitiveFileNames() bool             { return false }
func (h *fullHost) Trace(msg *diagnostics.Message, args ...any) {}
func (h *fullHost) DefaultLibraryPath() string                  { return "/" }
func (h *fullHost) GetResolvedProjectReference(fileName string, path tspath.Path) *tsoptions.ParsedCommandLine {
	return nil
}
func (h *fullHost) GetSourceFile(opts ast.SourceFileParseOptions) *ast.SourceFile {
	pathStr := string(opts.Path)
	content, ok := h.fs.ReadFile(pathStr)
	if !ok {
		return nil
	}
	return parser.ParseSourceFile(opts, content, core.ScriptKindTS)
}

type parseHost struct{ fs vfs.FS }

func (p *parseHost) FS() vfs.FS                          { return p.fs }
func (p *parseHost) GetCurrentDirectory() string         { return "/" }
func (p *parseHost) UseCaseSensitiveFileNames() bool     { return false }
func (p *parseHost) ReadFile(path string) (string, bool) { return p.fs.ReadFile(path) }
func (p *parseHost) FileExists(path string) bool         { return p.fs.FileExists(path) }
func (p *parseHost) ReadDirectory(root string, extensions []string, excludes []string, includes []string, depth *int) []string {
	return []string{}
}

//export TranspileAndCheckTS
func TranspileAndCheckTS(cCode *C.char) *C.char {
	tsCode := C.GoString(cCode)
	wrapper := &fsWrapper{files: make(map[string]string)}
	libContent, _ := os.ReadFile("internal/bundled/libs/lib.d.ts")
	wrapper.files["lib.d.ts"] = string(libContent)
	wrapper.files["embedded.ts"] = tsCode

	host := &fullHost{fs: wrapper}

	// We rely on ParseCommandLine to return the configuration.
	// If 'Options' is inaccessible, the library likely expects us to pass
	// command line args via the slice passed to ParseCommandLine.
	config := tsoptions.ParseCommandLine([]string{
		"embedded.ts",
		"--module", "esnext",
		"--target", "esnext",
		"--moduleDetection", "force",
	}, &parseHost{fs: wrapper})

	prog := compiler.NewProgram(compiler.ProgramOptions{
		Host:   host,
		Config: config,
	})

	if prog == nil {
		return C.CString("Error: Failed to init program")
	}
	prog.GetSourceFile("embedded.ts")

	var sb strings.Builder
	prog.Emit(context.Background(), compiler.EmitOptions{
		WriteFile: func(fileName string, text string, data *compiler.WriteFileData) error {
			sb.WriteString(text)
			return nil
		},
	})

	res := sb.String()
	if res == "" {
		return C.CString("Error: No code emitted")
	}
	return C.CString(res)
}

func main() {}
