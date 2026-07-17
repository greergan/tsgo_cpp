package main

/*
#include <stdlib.h>
*/
import "C"
import (
	"context"
	"embed"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
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

var (
	dtsCacheMu sync.RWMutex
	dtsCache   = make(map[string]string)
)

func cacheDts(basename string, content []byte) {
	dtsCacheMu.Lock()
	dtsCache["/types/"+basename] = string(content)
	dtsCacheMu.Unlock()
}

func init() {
	bundledTypes = make(map[string]string)
	fs.WalkDir(libFS, "lib", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, _ := libFS.ReadFile(path)
		bundledTypes["/"+path] = string(data)
		return nil
	})
}

type fsWrapper struct{ files map[string]string }

func (w *fsWrapper) UseCaseSensitiveFileNames() bool               { return false }
func (w *fsWrapper) FileExists(path string) bool                   { _, ok := w.files[path]; return ok }
func (w *fsWrapper) ReadFile(path string) (string, bool)           { s, ok := w.files[path]; return s, ok }
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
	var entries vfs.Entries
	seen := make(map[string]bool)
	for k := range w.files {
		if strings.HasPrefix(k, prefix) {
			rest := k[len(prefix):]
			parts := strings.SplitN(rest, "/", 2)
			name := parts[0]
			if !seen[name] {
				seen[name] = true
				if len(parts) == 2 {
					entries.Directories = append(entries.Directories, name)
				} else {
					entries.Files = append(entries.Files, name)
				}
			}
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
	content, ok := h.fs.ReadFile(string(opts.Path))
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

const buildTsconfigJSON = `{
	"compilerOptions": {
		"target": "ESNext",
		"module": "ESNext",
		"lib": ["ESNext", "DOM"],
		"moduleResolution": "bundler",
		"allowJs": true,
		"checkJs": true,
		"removeComments": false,
		"forceConsistentCasingInFileNames": true,
		"strict": true,
		"incremental": true,
		"noUnusedLocals": true,
		"noUnusedParameters": true,
		"noFallthroughCasesInSwitch": true,
		"noImplicitOverride": true,
		"skipDefaultLibCheck": true
	}
}`

const fetchTranspileTsconfigJSON = `{
	"compilerOptions": {
		"target": "ESNext",
		"module": "ESNext",
		"lib": ["ESNext", "DOM"],
		"moduleResolution": "bundler",
		"allowJs": true,
		"checkJs": true,
		"removeComments": false,
		"forceConsistentCasingInFileNames": true,
		"strict": true,
		"incremental": true,
		"noUnusedLocals": true,
		"noUnusedParameters": true,
		"noFallthroughCasesInSwitch": true,
		"noImplicitOverride": true,
		"skipDefaultLibCheck": true
	}
}`

func makeWrapper() *fsWrapper {
	wrapper := &fsWrapper{files: make(map[string]string)}
	for path, content := range bundledTypes {
		wrapper.files[path] = content
	}
	dtsCacheMu.RLock()
	for path, content := range dtsCache {
		wrapper.files[path] = content
	}
	dtsCacheMu.RUnlock()
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
	jsonCfg, _ := tsoptions.ParseConfigFileTextToJson("/tsconfig.json", "/tsconfig.json", buildTsconfigJSON)
	config := tsoptions.ParseJsonConfigFileContent(jsonCfg, ph, "/", nil, "/tsconfig.json", nil, nil, nil)
	config.ParsedConfig.FileNames = fileNames
	return config
}

func transpile_core(fileName string, source string, wrapper *fsWrapper) string {
	wrapper.files[fileName] = source

	dtsCacheMu.RLock()
	var dtsFileNames []string
	for path := range dtsCache {
		dtsFileNames = append(dtsFileNames, path)
	}
	dtsCacheMu.RUnlock()
	embeddedFS := bundled.WrapFS(wrapper)
	host := &fullHost{fs: embeddedFS}
	ph := &parseHost{fs: embeddedFS}

	jsonCfg, _ := tsoptions.ParseConfigFileTextToJson("/tsconfig.json", "/tsconfig.json", fetchTranspileTsconfigJSON)
	config := tsoptions.ParseJsonConfigFileContent(jsonCfg, ph, "/", nil, "/tsconfig.json", nil, nil, nil)
	config.ParsedConfig.FileNames = append([]string{fileName}, dtsFileNames...)

	prog := compiler.NewProgram(compiler.ProgramOptions{Host: host, Config: config})
	if prog == nil {
		fmt.Fprintln(os.Stderr, "transpile_core: failed to create program")
		return ""
	}

	ctx := context.Background()
	if sf := prog.GetSourceFile(fileName); sf != nil {
		diags := append(prog.GetSyntacticDiagnostics(ctx, sf), prog.GetSemanticDiagnostics(ctx, sf)...)
		for _, d := range diags {
			fmt.Fprintf(os.Stderr, "[%s] TS%d: %s\n", d.Category().String(), d.Code(), d.String())
		}
	}

	var sb strings.Builder
	prog.Emit(ctx, compiler.EmitOptions{WriteFile: func(fn, text string, d *compiler.WriteFileData) error {
		sb.WriteString(text)
		return nil
	}})
	return sb.String()
}

func siblingURI(base, name string) string {
	idx := strings.LastIndex(base, "/")
	if idx < 0 {
		return name
	}
	return base[:idx+1] + name
}

func resolveReferencedDts(dtsURI string, content []byte) bool {
	for _, line := range strings.Split(string(content), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, "//") {
			break
		}
		if !strings.HasPrefix(line, "///") {
			continue
		}
		start := strings.Index(line, `path="`)
		if start < 0 {
			continue
		}
		start += len(`path="`)
		end := strings.Index(line[start:], `"`)
		if end < 0 {
			continue
		}
		refName := line[start : start+end]
		if refName == "" || !strings.HasSuffix(refName, ".d.ts") {
			continue
		}

		basename := filepath.Base(refName)

		dtsCacheMu.RLock()
		_, already := dtsCache["/types/"+basename]
		dtsCacheMu.RUnlock()
		if already {
			continue
		}

		refURI := siblingURI(dtsURI, refName)

		var data []byte
		if strings.HasPrefix(refURI, "file://") {
			filePath := strings.TrimPrefix(refURI, "file://")
			var err error
			data, err = os.ReadFile(filePath)
			if err != nil {
				fmt.Fprintf(os.Stderr, "resolveReferencedDts: cannot read %s: %s\n", refURI, err.Error())
				return false
			}
		} else if strings.HasPrefix(refURI, "http://") || strings.HasPrefix(refURI, "https://") {
			res, err := http.Get(refURI)
			if err != nil {
				fmt.Fprintf(os.Stderr, "resolveReferencedDts: cannot fetch %s: %s\n", refURI, err.Error())
				return false
			}
			if res.StatusCode != http.StatusOK {
				res.Body.Close()
				fmt.Fprintf(os.Stderr, "resolveReferencedDts: %s returned %d\n", refURI, res.StatusCode)
				return false
			}
			data, err = io.ReadAll(res.Body)
			res.Body.Close()
			if err != nil {
				fmt.Fprintf(os.Stderr, "resolveReferencedDts: cannot read body of %s: %s\n", refURI, err.Error())
				return false
			}
		} else {
			fmt.Fprintf(os.Stderr, "resolveReferencedDts: unsupported scheme in %s\n", refURI)
			return false
		}

		cacheDts(basename, data)
		if !resolveReferencedDts(refURI, data) {
			return false
		}
	}
	return true
}

//export fetch_and_transpile
func fetch_and_transpile(cSrcURI *C.char) *C.char {
	uri := C.GoString(cSrcURI)
	var fileName string
	var data []byte

	if strings.HasPrefix(uri, "file://") {
		filePath := strings.TrimPrefix(uri, "file://")
		var err error
		data, err = os.ReadFile(filePath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "fetch_and_transpile: cannot read file %s: %s\n", filePath, err.Error())
			return C.CString("")
		}
		fileName = "/" + filepath.Base(filePath)

		if strings.HasSuffix(uri, ".ts") {
			dtsPath := strings.TrimSuffix(filePath, ".ts") + ".d.ts"
			if content, err := os.ReadFile(dtsPath); err == nil {
				cacheDts(filepath.Base(dtsPath), content)
				if !resolveReferencedDts("file://"+dtsPath, content) {
					return C.CString("")
				}
			} else {
				fmt.Fprintf(os.Stderr, "fetch_and_transpile: no .d.ts found for %s (checked %s)\n", filePath, dtsPath)
			}
		}
	} else if strings.HasPrefix(uri, "http://") || strings.HasPrefix(uri, "https://") {
		resp, err := http.Get(uri)
		if err != nil {
			fmt.Fprintf(os.Stderr, "fetch_and_transpile: HTTP GET failed for %s: %s\n", uri, err.Error())
			return C.CString("")
		}
		data, err = io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			fmt.Fprintf(os.Stderr, "fetch_and_transpile: cannot read HTTP body for %s: %s\n", uri, err.Error())
			return C.CString("")
		}
		fileName = "/" + filepath.Base(uri)

		if strings.HasSuffix(uri, ".ts") {
			dtsURL := strings.TrimSuffix(uri, ".ts") + ".d.ts"
			if res, err := http.Get(dtsURL); err == nil {
				if res.StatusCode == http.StatusOK {
					if content, err := io.ReadAll(res.Body); err == nil {
						cacheDts(filepath.Base(dtsURL), content)
						if !resolveReferencedDts(dtsURL, content) {
							res.Body.Close()
							return C.CString("")
						}
					}
				} else {
					fmt.Fprintf(os.Stderr, "fetch_and_transpile: no .d.ts found for %s (checked %s)\n", uri, dtsURL)
				}
				res.Body.Close()
			}
		}
	} else {
		fmt.Fprintf(os.Stderr, "fetch_and_transpile: unsupported URI scheme in %s\n", uri)
		return C.CString("")
	}

	if !resolveReferencedDts(uri, data) {
		return C.CString("")
	}

	wrapper := makeWrapper()
	result := transpile_core(fileName, string(data), wrapper)
	return C.CString(result)
}

//export build
func build(cSrcDir *C.char, cOutDir *C.char) {
	srcDir := C.GoString(cSrcDir)
	outDir := C.GoString(cOutDir)

	wrapper := makeWrapper()
	var fileNames []string

	filepath.WalkDir(srcDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		if !strings.HasSuffix(path, ".ts") || strings.HasSuffix(path, ".d.ts") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "build: cannot read source file %s: %s\n", path, err.Error())
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

	prog := compiler.NewProgram(compiler.ProgramOptions{Host: host, Config: config})
	if prog == nil {
		fmt.Fprintln(os.Stderr, "build: failed to create program")
		return
	}

	ctx := context.Background()

	for _, sf := range prog.GetSourceFiles() {
		diags := append(prog.GetSyntacticDiagnostics(ctx, sf), prog.GetSemanticDiagnostics(ctx, sf)...)
		for _, d := range diags {
			fmt.Fprintf(os.Stderr, "[%s] TS%d: %s\n", d.Category().String(), d.Code(), d.String())
		}
	}

	prog.Emit(ctx, compiler.EmitOptions{
		WriteFile: func(outFileName string, text string, data *compiler.WriteFileData) error {
			rel := strings.TrimPrefix(outFileName, "/"+srcDir+"/")
			dest := filepath.Join(outDir, rel)
			if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
				fmt.Fprintf(os.Stderr, "build: cannot create output directory %s: %s\n", filepath.Dir(dest), err.Error())
				return err
			}
			if err := os.WriteFile(dest, []byte(text), 0o644); err != nil {
				fmt.Fprintf(os.Stderr, "build: cannot write output file %s: %s\n", dest, err.Error())
				return err
			}
			return nil
		},
	})
}

func main() {}
