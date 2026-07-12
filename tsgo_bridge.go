package main

/*
#include <stdlib.h>
*/
import "C"
import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/microsoft/typescript-go/internal/ast"
	"github.com/microsoft/typescript-go/internal/bundled"
	"github.com/microsoft/typescript-go/internal/compiler"
	"github.com/microsoft/typescript-go/internal/core"
	"github.com/microsoft/typescript-go/internal/diagnostics"
	"github.com/microsoft/typescript-go/internal/parser"
	"github.com/microsoft/typescript-go/internal/tsoptions"
	"github.com/microsoft/typescript-go/internal/tspath"
	"github.com/microsoft/typescript-go/internal/vfs"
)

//go:embed types
var typesFS embed.FS

var bundledTypes map[string]string

func init() {
	bundledTypes = make(map[string]string)
	fs.WalkDir(typesFS, "types", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := typesFS.ReadFile(path)
		if err != nil {
			return err
		}
		bundledTypes["/"+path] = string(data)
		return nil
	})
}

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
func (h *fullHost) DefaultLibraryPath() string                  { return bundled.LibPath() }
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

const tsconfigJSON = `{
	"compilerOptions": {
		"target": "ESNext",
		"module": "ESNext",
		"allowJs": true,
		"checkJs": true,
		"removeComments": true,
		"forceConsistentCasingInFileNames": true,
		"strict": true,
		"noUnusedLocals": true,
		"noUnusedParameters": true,
		"noFallthroughCasesInSwitch": true,
		"noImplicitOverride": true,
		"skipDefaultLibCheck": true
	}
}`

const consoleDTS = `declare const console: Console;
export default console;
`

//export transpile
func transpile(cFileName *C.char, cCode *C.char, cDtsCode *C.char, cOutDir *C.char) *C.char {
	fileName := "/" + C.GoString(cFileName)
	tsCode := C.GoString(cCode)

	wrapper := &fsWrapper{files: make(map[string]string)}
	wrapper.files[fileName] = tsCode
	wrapper.files["/console.d.ts"] = consoleDTS

	// inject bundled types/*.d.ts into virtual FS
	for path, content := range bundledTypes {
		wrapper.files[path] = content
	}

	// inject dts into virtual FS if provided
	if cDtsCode != nil {
		wrapper.files["/types.d.ts"] = C.GoString(cDtsCode)
	}

	embeddedFS := bundled.WrapFS(wrapper)

	host := &fullHost{fs: embeddedFS}
	ph := &parseHost{fs: embeddedFS}

	json, diags := tsoptions.ParseConfigFileTextToJson("/tsconfig.json", "/tsconfig.json", tsconfigJSON)
	if len(diags) > 0 {
		return C.CString("Error: Failed to parse tsconfig")
	}

	config := tsoptions.ParseJsonConfigFileContent(
		json,
		ph,
		"/",
		nil,
		"/tsconfig.json",
		nil,
		nil,
		nil,
	)
	config.ParsedConfig.FileNames = append(config.ParsedConfig.FileNames, fileName)

	// add bundled types to file list
	for path := range bundledTypes {
		config.ParsedConfig.FileNames = append(config.ParsedConfig.FileNames, path)
	}

	// add dts to file list if provided
	if cDtsCode != nil {
		config.ParsedConfig.FileNames = append(config.ParsedConfig.FileNames, "/types.d.ts")
	}

	prog := compiler.NewProgram(compiler.ProgramOptions{
		Host:   host,
		Config: config,
	})

	if prog == nil {
		return C.CString("Error: Failed to init program")
	}

	ctx := context.Background()
	sf := prog.GetSourceFile(fileName)
	if sf != nil {
		diags := append(
			prog.GetSyntacticDiagnostics(ctx, sf),
			prog.GetSemanticDiagnostics(ctx, sf)...,
		)
		for _, d := range diags {
			fmt.Printf("[%s] TS%d: %s\n", d.Category().String(), d.Code(), d.String())
		}
	}

	outDir := ""
	if cOutDir != nil {
		outDir = C.GoString(cOutDir)
	}

	if outDir == "" {
		var sb strings.Builder
		prog.Emit(ctx, compiler.EmitOptions{
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

	var emitErr error
	prog.Emit(ctx, compiler.EmitOptions{
		WriteFile: func(outFileName string, text string, data *compiler.WriteFileData) error {
			dest := filepath.Join(outDir, outFileName)
			if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
				emitErr = err
				return err
			}
			if err := os.WriteFile(dest, []byte(text), 0o644); err != nil {
				emitErr = err
				return err
			}
			return nil
		},
	})

	if emitErr != nil {
		return C.CString("Error: " + emitErr.Error())
	}
	return C.CString("")
}

func main() {}
