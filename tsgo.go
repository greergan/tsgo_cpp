package main

/*
#include <stdlib.h>
*/
import "C"
import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
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

const tsconfigJSON = `{"compilerOptions": {"target": "ESNext", "module": "ESNext", "lib": ["ESNext", "DOM"], "moduleResolution": "bundler", "strict": true}}`

func makeWrapper() *fsWrapper {
	wrapper := &fsWrapper{files: make(map[string]string)}
	for path, content := range bundledTypes {
		wrapper.files[path] = content
	}
	return wrapper
}

func fetchGithubTypes(owner, repo, branch, path string, dtsBuilder *strings.Builder, wrapper *fsWrapper) {
	apiURL := fmt.Sprintf("http://forgejo/api/v1/repos/%s/%s/git/trees/%s?recursive=1", owner, repo, branch)
	resp, err := http.Get(apiURL)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	var result struct {
		Tree []struct {
			Path string `json:"path"`
		} `json:"tree"`
	}
	json.NewDecoder(resp.Body).Decode(&result)
	for _, item := range result.Tree {
		if strings.HasPrefix(item.Path, path+"/") && strings.HasSuffix(item.Path, ".d.ts") {
			rawURL := fmt.Sprintf("http://forgejo/raw/branch/%s/%s", branch, item.Path)
			if res, err := http.Get(rawURL); err == nil {
				data, _ := io.ReadAll(res.Body)
				res.Body.Close()
				dtsBuilder.Write(data)
				dtsBuilder.WriteString("\n")
				wrapper.files["/types/"+filepath.Base(item.Path)] = string(data)
			}
		}
	}
}

//export fetch_and_transpile
func fetch_and_transpile(cSrcURI *C.char) *C.char {
	uri := C.GoString(cSrcURI)
	wrapper := makeWrapper()
	var dtsBuilder strings.Builder
	var fileName string
	var data []byte

	if strings.HasPrefix(uri, "http://forgejo/") {
		parts := strings.Split(strings.TrimPrefix(uri, "http://forgejo/"), "/")
		if len(parts) >= 6 {
			owner, repo, branch := parts[0], parts[1], parts[4]
			dirPath := strings.Join(parts[5:len(parts)-1], "/")
			resp, _ := http.Get(uri)
			if resp != nil {
				data, _ = io.ReadAll(resp.Body)
				resp.Body.Close()
			}
			fetchGithubTypes(owner, repo, branch, dirPath, &dtsBuilder, wrapper)
			fileName = "/" + parts[len(parts)-1]
		}
	} else {
		filePath := strings.TrimPrefix(uri, "file://")
		data, _ = os.ReadFile(filePath)
		fileName = "/" + filepath.Base(filePath)
	}

	wrapper.files[fileName] = string(data)
	dtsCode := dtsBuilder.String()
	if dtsCode != "" {
		wrapper.files["/types/types.d.ts"] = dtsCode
	}

	embeddedFS := bundled.WrapFS(wrapper)
	host := &fullHost{fs: embeddedFS}
	ph := &parseHost{fs: embeddedFS}

	json, _ := tsoptions.ParseConfigFileTextToJson("/tsconfig.json", "/tsconfig.json", tsconfigJSON)
	config := tsoptions.ParseJsonConfigFileContent(json, ph, "/", nil, "/tsconfig.json", nil, nil, nil)
	config.ParsedConfig.FileNames = []string{fileName}
	if dtsCode != "" {
		config.ParsedConfig.FileNames = append(config.ParsedConfig.FileNames, "/types/types.d.ts")
	}

	prog := compiler.NewProgram(compiler.ProgramOptions{Host: host, Config: config})
	if prog == nil {
		return C.CString("")
	}

	var sb strings.Builder
	prog.Emit(context.Background(), compiler.EmitOptions{WriteFile: func(fn, text string, d *compiler.WriteFileData) error { sb.WriteString(text); return nil }})
	return C.CString(sb.String())
}

func main() {}
