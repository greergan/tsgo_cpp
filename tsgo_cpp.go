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

//go:embed lib
var libFS embed.FS

var bundledTypes map[string]string

func init() {
	bundledTypes = make(map[string]string)
	fs.WalkDir(libFS, "lib", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := libFS.ReadFile(path)
		if err != nil {
			return err
		}
		bundledTypes["/"+path] = string(data)
		return nil
	})
}

// --- FS and Host implementations (Keep as before) ---
type fsWrapper struct{ files map[string]string }

func (w *fsWrapper) UseCaseSensitiveFileNames() bool { return false }
func (w *fsWrapper) FileExists(path string) bool     { _, ok := w.files[path]; return ok }
func (w *fsWrapper) ReadFile(path string) (string, bool) {
	s, ok := w.files[path]
	return s, ok
}
func (w *fsWrapper) WriteFile(path string, data string) error      { w.files[path] = data; return nil }
func (w *fsWrapper) AppendFile(path string, data string) error     { w.files[path] += data; return nil }
func (w *fsWrapper) Remove(path string) error                      { delete(w.files, path); return nil }
func (w *fsWrapper) Chtimes(path string, a, m time.Time) error     { return nil }
func (w *fsWrapper) Stat(path string) vfs.FileInfo                 { return nil }
func (w *fsWrapper) WalkDir(root string, fn vfs.WalkDirFunc) error { return nil }
func (w *fsWrapper) Realpath(path string) string                   { return path }

func (w *fsWrapper) DirectoryExists(path string) bool {
	prefix := strings.TrimRight(path, "/") + "/"
	for k := range w.files {
		if strings.HasPrefix(k, prefix) {
			return true
		}
	}
	return false
}

func (w *fsWrapper) GetAccessibleEntries(path string) vfs.Entries {
	prefix := strings.TrimRight(path, "/") + "/"
	seen := make(map[string]bool)
	var entries vfs.Entries
	for k := range w.files {
		if !strings.HasPrefix(k, prefix) {
			continue
		}
		rest := k[len(prefix):]
		parts := strings.SplitN(rest, "/", 2)
		name := parts[0]
		if seen[name] {
			continue
		}
		seen[name] = true
		if len(parts) == 2 {
			// it's a directory entry
			entries.Directories = append(entries.Directories, name)
		} else {
			// it's a file entry
			entries.Files = append(entries.Files, name)
		}
	}
	return entries
}

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
		"moduleResolution": "bundler",
		"allowJs": true,
		"checkJs": true,
		"removeComments": false,
		"forceConsistentCasingInFileNames": true,
		"strict": true,
		"noUnusedLocals": true,
		"noUnusedParameters": true,
		"noFallthroughCasesInSwitch": true,
		"noImplicitOverride": true,
		"skipDefaultLibCheck": true
	}
}`

func makeWrapper() *fsWrapper {
	wrapper := &fsWrapper{files: make(map[string]string)}

	// inject bundled lib/*.d.ts into virtual FS
	for path, content := range bundledTypes {
		wrapper.files[path] = content
	}

	// inject runtime types/ directory into virtual FS
	filepath.WalkDir("types", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		wrapper.files["/"+path] = string(data)
		return nil
	})

	return wrapper
}

func makeConfig(ph *parseHost, fileNames []string) *tsoptions.ParsedCommandLine {
	json, _ := tsoptions.ParseConfigFileTextToJson("/tsconfig.json", "/tsconfig.json", tsconfigJSON)
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
	config.ParsedConfig.FileNames = append(config.ParsedConfig.FileNames, fileNames...)
	return config
}

//export transpile
func transpile(cFileName *C.char, cCode *C.char, cDtsCode *C.char, cOutDir *C.char) *C.char {
	fileName := "/" + C.GoString(cFileName)
	tsCode := C.GoString(cCode)

	wrapper := makeWrapper()
	wrapper.files[fileName] = tsCode

	// inject dts into virtual FS if provided
	if cDtsCode != nil {
		wrapper.files["/types.d.ts"] = C.GoString(cDtsCode)
	}

	embeddedFS := bundled.WrapFS(wrapper)
	host := &fullHost{fs: embeddedFS}
	ph := &parseHost{fs: embeddedFS}

	fileNames := []string{fileName}

	// add bundled lib types to file list
	for path := range bundledTypes {
		fileNames = append(fileNames, path)
	}

	// add runtime types to file list
	filepath.WalkDir("types", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		fileNames = append(fileNames, "/"+path)
		return nil
	})

	// add dts to file list if provided
	if cDtsCode != nil {
		fileNames = append(fileNames, "/types.d.ts")
	}

	json, diags := tsoptions.ParseConfigFileTextToJson("/tsconfig.json", "/tsconfig.json", tsconfigJSON)
	if len(diags) > 0 {
		fmt.Fprintln(os.Stderr, "Error: Failed to parse tsconfig")
		return C.CString("")
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
	config.ParsedConfig.FileNames = append(config.ParsedConfig.FileNames, fileNames...)

	prog := compiler.NewProgram(compiler.ProgramOptions{
		Host:   host,
		Config: config,
	})

	if prog == nil {
		fmt.Fprintln(os.Stderr, "Error: Failed to init program")
		return C.CString("")
	}

	ctx := context.Background()
	sf := prog.GetSourceFile(fileName)
	if sf != nil {
		diags := append(
			prog.GetSyntacticDiagnostics(ctx, sf),
			prog.GetSemanticDiagnostics(ctx, sf)...,
		)
		for _, d := range diags {
			fmt.Fprintf(os.Stderr, "[%s] TS%d: %s\n", d.Category().String(), d.Code(), d.String())
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
		return C.CString(sb.String())
	}

	prog.Emit(ctx, compiler.EmitOptions{
		WriteFile: func(outFileName string, text string, data *compiler.WriteFileData) error {
			dest := filepath.Join(outDir, outFileName)
			if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %s\n", err.Error())
				return err
			}
			if err := os.WriteFile(dest, []byte(text), 0o644); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %s\n", err.Error())
				return err
			}
			return nil
		},
	})

	return C.CString("")
}

//export build
func build(cSrcDir *C.char, cOutDir *C.char) {
	srcDir := C.GoString(cSrcDir)
	outDir := C.GoString(cOutDir)

	wrapper := makeWrapper()
	var fileNames []string

	// add bundled lib types to file list
	for path := range bundledTypes {
		fileNames = append(fileNames, path)
	}

	// add runtime types to file list
	filepath.WalkDir("types", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		fileNames = append(fileNames, "/"+path)
		return nil
	})

	// walk src directory, inject all .ts files into virtual FS
	filepath.WalkDir(srcDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		if !strings.HasSuffix(path, ".ts") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %s\n", err.Error())
			return err
		}
		vPath := "/" + path
		wrapper.files[vPath] = string(data)
		fileNames = append(fileNames, vPath)
		return nil
	})

	embeddedFS := bundled.WrapFS(wrapper)
	host := &fullHost{fs: embeddedFS}
	ph := &parseHost{fs: embeddedFS}

	config := makeConfig(ph, fileNames)

	prog := compiler.NewProgram(compiler.ProgramOptions{
		Host:   host,
		Config: config,
	})

	if prog == nil {
		fmt.Fprintln(os.Stderr, "Error: Failed to init program")
		return
	}

	ctx := context.Background()

	// print all diagnostics
	for _, sf := range prog.GetSourceFiles() {
		diags := append(
			prog.GetSyntacticDiagnostics(ctx, sf),
			prog.GetSemanticDiagnostics(ctx, sf)...,
		)
		for _, d := range diags {
			fmt.Fprintf(os.Stderr, "[%s] TS%d: %s\n", d.Category().String(), d.Code(), d.String())
		}
	}

	prog.Emit(ctx, compiler.EmitOptions{
		WriteFile: func(outFileName string, text string, data *compiler.WriteFileData) error {
			// strip leading "/" and srcDir prefix from virtual path
			rel := strings.TrimPrefix(outFileName, "/"+srcDir+"/")
			dest := filepath.Join(outDir, rel)
			if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %s\n", err.Error())
				return err
			}
			if err := os.WriteFile(dest, []byte(text), 0o644); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %s\n", err.Error())
				return err
			}
			return nil
		},
	})
}

func main() {}
