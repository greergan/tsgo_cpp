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

// transpile_core builds the virtual fs, compiles, and emits JS for fileName/source
// plus any accumulated .d.ts content in dtsBuilder. Shared by both exported entry points.
func transpile_core(fileName string, source string, dtsBuilder *strings.Builder, wrapper *fsWrapper) string {
	wrapper.files[fileName] = source
	dtsCode := dtsBuilder.String()
	if dtsCode != "" {
		wrapper.files["/types/types.d.ts"] = dtsCode
	}

	embeddedFS := bundled.WrapFS(wrapper)
	host := &fullHost{fs: embeddedFS}
	ph := &parseHost{fs: embeddedFS}

	jsonCfg, _ := tsoptions.ParseConfigFileTextToJson("/tsconfig.json", "/tsconfig.json", tsconfigJSON)
	config := tsoptions.ParseJsonConfigFileContent(jsonCfg, ph, "/", nil, "/tsconfig.json", nil, nil, nil)
	config.ParsedConfig.FileNames = []string{fileName}
	if dtsCode != "" {
		config.ParsedConfig.FileNames = append(config.ParsedConfig.FileNames, "/types/types.d.ts")
	}

	prog := compiler.NewProgram(compiler.ProgramOptions{Host: host, Config: config})
	if prog == nil {
		return ""
	}

	var sb strings.Builder
	prog.Emit(context.Background(), compiler.EmitOptions{WriteFile: func(fn, text string, d *compiler.WriteFileData) error {
		sb.WriteString(text)
		return nil
	}})
	return sb.String()
}

// gitApiKind identifies remote tree/listing API dialect for a git-compat host.
type gitApiKind int

const (
	gitApiNone gitApiKind = iota
	gitApiGitea
	gitApiGithub
)

// gitApiKindForHost maps a known git-compat host to its API dialect.
// Extend this table to support additional forges.
func gitApiKindForHost(host string) gitApiKind {
	switch host {
	case "raw.githubusercontent.com":
		return gitApiGithub
	case "forgejo", "codeberg.org":
		return gitApiGitea
	default:
		return gitApiNone
	}
}

// parsedGitURI holds the pieces needed to hit a tree API and construct raw URLs.
type parsedGitURI struct {
	kind    gitApiKind
	host    string
	owner   string
	repo    string
	branch  string
	dirPath string // repo-relative directory containing the source file
}

// parseGitURI extracts {host, owner, repo, branch} from either uri shape:
//
//	gitea-style:  scheme://HOST/OWNER/REPO/raw/branch/BRANCH/PATH...
//	github-style: scheme://raw.githubusercontent.com/OWNER/REPO/BRANCH/PATH...
func parseGitURI(uri string) (parsedGitURI, bool) {
	rest := uri
	rest = strings.TrimPrefix(rest, "https://")
	rest = strings.TrimPrefix(rest, "http://")

	slash := strings.Index(rest, "/")
	if slash < 0 {
		return parsedGitURI{}, false
	}
	host := rest[:slash]
	path := rest[slash+1:]
	parts := strings.Split(path, "/")

	kind := gitApiKindForHost(host)
	switch kind {
	case gitApiGithub:
		// OWNER/REPO/BRANCH/PATH...
		if len(parts) < 4 {
			return parsedGitURI{}, false
		}
		dirPath := filepath.Dir(strings.Join(parts[3:], "/"))
		if dirPath == "." {
			dirPath = ""
		}
		return parsedGitURI{kind: kind, host: host, owner: parts[0], repo: parts[1], branch: parts[2], dirPath: dirPath}, true
	case gitApiGitea:
		// OWNER/REPO/raw/branch/BRANCH/PATH...
		if len(parts) < 5 || parts[2] != "raw" || parts[3] != "branch" {
			return parsedGitURI{}, false
		}
		dirPath := filepath.Dir(strings.Join(parts[5:], "/"))
		if dirPath == "." {
			dirPath = ""
		}
		return parsedGitURI{kind: kind, host: host, owner: parts[0], repo: parts[1], branch: parts[4], dirPath: dirPath}, true
	default:
		return parsedGitURI{}, false
	}
}

// fetchAllDts walks the repo tree via the appropriate API and fetches every *.d.ts file
// found in the same directory as the source file (pg.dirPath), appending contents to dtsBuilder and storing each under /types/<basename> in wrapper.
// Best-effort: any individual failure is skipped, never fatal.
func fetchAllDts(pg parsedGitURI, dtsBuilder *strings.Builder, wrapper *fsWrapper) {
	var apiURL string
	switch pg.kind {
	case gitApiGithub:
		apiURL = fmt.Sprintf("https://api.github.com/repos/%s/%s/git/trees/%s?recursive=1", pg.owner, pg.repo, pg.branch)
	case gitApiGitea:
		apiURL = fmt.Sprintf("http://%s/api/v1/repos/%s/%s/git/trees/%s?recursive=1", pg.host, pg.owner, pg.repo, pg.branch)
	default:
		return
	}

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
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return
	}

	for _, item := range result.Tree {
		if !strings.HasSuffix(item.Path, ".d.ts") {
			continue
		}
		itemDir := filepath.Dir(item.Path)
		if itemDir == "." {
			itemDir = ""
		}
		if itemDir != pg.dirPath {
			continue
		}

		var rawURL string
		switch pg.kind {
		case gitApiGithub:
			rawURL = fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/%s/%s", pg.owner, pg.repo, pg.branch, item.Path)
		case gitApiGitea:
			rawURL = fmt.Sprintf("http://%s/%s/%s/raw/branch/%s/%s", pg.host, pg.owner, pg.repo, pg.branch, item.Path)
		}

		res, err := http.Get(rawURL)
		if err != nil {
			continue
		}
		data, err := io.ReadAll(res.Body)
		res.Body.Close()
		if err != nil {
			continue
		}

		dtsBuilder.Write(data)
		dtsBuilder.WriteString("\n")
		wrapper.files["/types/"+filepath.Base(item.Path)] = string(data)
	}
}

// fetch_and_transpile handles file:// and plain http(s):// sources.
// For file://, sibling *.d.ts files in the same directory are globbed and included.
// For http(s)://, only the single source file is fetched -- no repo-wide type discovery.
//
//export fetch_and_transpile
func fetch_and_transpile(cSrcURI *C.char) *C.char {
	uri := C.GoString(cSrcURI)
	wrapper := makeWrapper()
	var dtsBuilder strings.Builder
	var fileName string
	var data []byte

	if strings.HasPrefix(uri, "file://") {
		filePath := strings.TrimPrefix(uri, "file://")
		data, _ = os.ReadFile(filePath)
		fileName = "/" + filepath.Base(filePath)

		dir := filepath.Dir(filePath)
		matches, _ := filepath.Glob(filepath.Join(dir, "*.d.ts"))
		for _, m := range matches {
			content, err := os.ReadFile(m)
			if err != nil {
				continue
			}
			dtsBuilder.Write(content)
			dtsBuilder.WriteString("\n")
			wrapper.files["/types/"+filepath.Base(m)] = string(content)
		}
	} else {
		resp, err := http.Get(uri)
		if err == nil {
			data, _ = io.ReadAll(resp.Body)
			resp.Body.Close()
		}
		fileName = "/" + filepath.Base(uri)
	}

	result := transpile_core(fileName, string(data), &dtsBuilder, wrapper)
	return C.CString(result)
}

// fetch_git_and_transpile handles git-compat hosts (github raw, forgejo, codeberg.org).
// It fetches the source file, then discovers and fetches every *.d.ts in the same
// repo directory as the source file via the host's tree/listing API.
//
//export fetch_git_and_transpile
func fetch_git_and_transpile(cSrcURI *C.char) *C.char {
	uri := C.GoString(cSrcURI)
	wrapper := makeWrapper()
	var dtsBuilder strings.Builder
	var data []byte

	resp, err := http.Get(uri)
	if err == nil {
		data, _ = io.ReadAll(resp.Body)
		resp.Body.Close()
	}
	fileName := "/" + filepath.Base(uri)

	if pg, ok := parseGitURI(uri); ok {
		fetchAllDts(pg, &dtsBuilder, wrapper)
	}

	result := transpile_core(fileName, string(data), &dtsBuilder, wrapper)
	return C.CString(result)
}

func main() {}
