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
	// inject cache
	dtsCacheMu.RLock()
	for path, content := range dtsCache {
		wrapper.files[path] = content
	}
	dtsCacheMu.RUnlock()
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
		fmt.Fprintln(os.Stderr, "Error: Failed to init program")
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

type gitApiKind int

const (
	gitApiNone gitApiKind = iota
	gitApiGitea
	gitApiGithub
)

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

type parsedGitURI struct {
	kind    gitApiKind
	host    string
	owner   string
	repo    string
	branch  string
	dirPath string
}

func dirPathOf(parts []string) string {
	dir := filepath.Dir(strings.Join(parts, "/"))
	if dir == "." {
		return ""
	}
	return dir
}

func parseGitURI(uri string) (parsedGitURI, bool) {
	rest := uri
	rest = strings.TrimPrefix(rest, "https://")
	rest = strings.TrimPrefix(rest, "http://")

	slash := strings.Index(rest, "/")
	if slash < 0 {
		return parsedGitURI{}, false
	}
	host := rest[:slash]
	parts := strings.Split(rest[slash+1:], "/")

	switch gitApiKindForHost(host) {
	case gitApiGithub:
		if len(parts) < 4 {
			return parsedGitURI{}, false
		}
		return parsedGitURI{kind: gitApiGithub, host: host, owner: parts[0], repo: parts[1], branch: parts[2], dirPath: dirPathOf(parts[3:])}, true
	case gitApiGitea:
		if len(parts) < 5 || parts[2] != "raw" || parts[3] != "branch" {
			return parsedGitURI{}, false
		}
		return parsedGitURI{kind: gitApiGitea, host: host, owner: parts[0], repo: parts[1], branch: parts[4], dirPath: dirPathOf(parts[5:])}, true
	default:
		return parsedGitURI{}, false
	}
}

func treeApiURL(pg parsedGitURI) string {
	switch pg.kind {
	case gitApiGithub:
		return fmt.Sprintf("https://api.github.com/repos/%s/%s/git/trees/%s?recursive=1", pg.owner, pg.repo, pg.branch)
	case gitApiGitea:
		return fmt.Sprintf("http://%s/api/v1/repos/%s/%s/git/trees/%s?recursive=1", pg.host, pg.owner, pg.repo, pg.branch)
	default:
		return ""
	}
}

func rawFileURL(pg parsedGitURI, repoPath string) string {
	switch pg.kind {
	case gitApiGithub:
		return fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/%s/%s", pg.owner, pg.repo, pg.branch, repoPath)
	case gitApiGitea:
		return fmt.Sprintf("http://%s/%s/%s/raw/branch/%s/%s", pg.host, pg.owner, pg.repo, pg.branch, repoPath)
	default:
		return ""
	}
}

// siblingURI replaces the last path segment of base with name.
// Works uniformly for file://, http://, and https:// URIs.
func siblingURI(base, name string) string {
	idx := strings.LastIndex(base, "/")
	if idx < 0 {
		return name
	}
	return base[:idx+1] + name
}

// resolveReferencedDts parses content for /// <reference path="..." /> directives,
// fetches each referenced .d.ts sibling using the same URI scheme as dtsURI,
// caches it, and recurses. Cycle-safe via cache check.
// Returns false if any referenced file cannot be fetched.
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

		// cycle/duplicate guard
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
				fmt.Fprintf(os.Stderr, "Reference Error: cannot read %s: %s\n", refURI, err.Error())
				return false
			}
		} else {
			res, err := http.Get(refURI)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Reference Error: cannot fetch %s: %s\n", refURI, err.Error())
				return false
			}
			if res.StatusCode != http.StatusOK {
				res.Body.Close()
				fmt.Fprintf(os.Stderr, "Reference Error: %s returned %d\n", refURI, res.StatusCode)
				return false
			}
			data, err = io.ReadAll(res.Body)
			res.Body.Close()
			if err != nil {
				fmt.Fprintf(os.Stderr, "Reference Error: cannot read body %s: %s\n", refURI, err.Error())
				return false
			}
		}

		cacheDts(basename, data)
		if !resolveReferencedDts(refURI, data) {
			return false
		}
	}
	return true
}

func fetchGitDts(uri string) {
	pg, ok := parseGitURI(uri)
	if !ok {
		return
	}

	resp, err := http.Get(treeApiURL(pg))
	if err != nil {
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return
	}

	var result struct {
		Tree []struct {
			Path string `json:"path"`
		} `json:"tree"`
	}
	if json.NewDecoder(resp.Body).Decode(&result) != nil {
		return
	}

	for _, item := range result.Tree {
		if !strings.HasSuffix(item.Path, ".d.ts") || dirPathOf(strings.Split(item.Path, "/")) != pg.dirPath {
			continue
		}

		res, err := http.Get(rawFileURL(pg, item.Path))
		if err != nil {
			continue
		}
		data, err := io.ReadAll(res.Body)
		res.Body.Close()
		if err != nil {
			continue
		}

		basename := filepath.Base(item.Path)
		cacheDts(basename, data)
		resolveReferencedDts(rawFileURL(pg, item.Path), data)
	}
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
			fmt.Fprintf(os.Stderr, "File Error: %s\n", err.Error())
			return C.CString("")
		}
		fileName = "/" + filepath.Base(filePath)

		dtsPath := strings.TrimSuffix(filePath, ".ts") + ".d.ts"
		if content, err := os.ReadFile(dtsPath); err == nil {
			basename := filepath.Base(dtsPath)
			cacheDts(basename, content)
			if !resolveReferencedDts("file://"+dtsPath, content) {
				return C.CString("")
			}
		}
		if !resolveReferencedDts(uri, data) {
			return C.CString("")
		}
	} else {
		resp, err := http.Get(uri)
		if err != nil {
			fmt.Fprintf(os.Stderr, "HTTP Error: %s\n", err.Error())
			return C.CString("")
		}
		data, err = io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Read Error: %s\n", err.Error())
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
				}
				res.Body.Close()
			}
		}

		fetchGitDts(uri)
	}

	// resolve any /// <reference path="..." /> in the source itself
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

	prog := compiler.NewProgram(compiler.ProgramOptions{Host: host, Config: config})
	if prog == nil {
		fmt.Fprintln(os.Stderr, "Error: Failed to init program")
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
