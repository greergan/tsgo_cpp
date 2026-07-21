# libtsgo
A C and C++ callable static library that wraps the [microsoft/typescript-go](https://github.com/microsoft/typescript-go) compiler, enabling high-performance TypeScript compilation from C or C++.
## Table of Contents
- [Compiler Options](#compiler-options)
- [API](#api)
  - [GoStr helper](#gostr-helper)
  - [fetch\_and\_transpile](#fetch_and_transpile)
  - [build](#build)
- [Type Resolution](#type-resolution)
- [Requirements](#requirements)
- [Build](#build-1)
  - [Environment Variables](#environment-variables)
  - [make build](#make-build)
  - [make test](#make-test)
  - [make test-cpp](#make-test-cpp)
  - [make test-c](#make-test-c)
  - [make package](#make-package)
  - [make install](#make-install)
  - [make clean](#make-clean)
- [Examples](#examples)
  - [fetch\_and\_transpile](#fetch_and_transpile-1)
  - [build](#build-2)
  - [Type Resolution](#type-resolution-1)
## Compiler Options
The following `compilerOptions` are embedded at compile time:
| Option | Value |
|---|---|
| `target` | `ESNext` |
| `module` | `ESNext` |
| `moduleResolution` | `bundler` |
| `allowJs` | `true` |
| `checkJs` | `true` |
| `removeComments` | `false` |
| `forceConsistentCasingInFileNames` | `true` |
| `strict` | `true` |
| `noUnusedLocals` | `true` |
| `noUnusedParameters` | `true` |
| `noFallthroughCasesInSwitch` | `true` |
| `noImplicitOverride` | `true` |
| `skipDefaultLibCheck` | `true` |
[↑ Top](#table-of-contents)
## API
### GoStr helper
A lightweight wrapper for strings returned by the library. Include `tsgo.h` to use it.
```c
#include "tsgo.h"
```
In C++, `GoStr` is an RAII struct — the destructor calls `free()` automatically.
```cpp
GoStr result = fetch_and_transpile(...);
std::cout << result.view() << std::endl;
// freed on scope exit
```
In C, `GoStr` is a plain struct — call `GoStr_free()` manually.
```c
GoStr result;
result.p = fetch_and_transpile(...);
printf("%s\n", result.p ? result.p : "");
GoStr_free(result);
```
### `fetch_and_transpile`
Compiles a single TypeScript file from a URI.
```c
char* fetch_and_transpile(char* cSrcURI);
```
`cSrcURI` may be a local file URI or an HTTP/HTTPS URL:
| Scheme | Example |
|---|---|
| `file://` | `file:///path/to/input.ts` |
| `http://` | `http://example.com/input.ts` |
| `https://` | `https://example.com/input.ts` |
Returns emitted JavaScript. Caller must `free()` the returned string, or use the provided `GoStr` helper above.
### `build`
Compiles all `.ts` files in a source tree. Diagnostics and errors are printed to stderr.
```c
void build(char* srcDir, char* outDir);
```
[↑ Top](#table-of-contents)
## Type Resolution
| Source | Content | When |
|---|---|---|
| `typescript-go` | All `microsoft/typescript-go` compiler libraries | Embedded at compile time |
| `lib/` | TypeScript standard library `.d.ts` files | Embedded at compile time |
| `types/` | User-provided type definitions | Loaded at runtime from working directory |
| `file://` / `http://` / `https://` | Sibling declaration file — `input.ts` resolves `input.d.ts` from the same location | Resolved at runtime |
| `/// <reference path="..." />` | Referenced `.d.ts` files | Resolved recursively, cycle-safe |
[↑ Top](#table-of-contents)
## Requirements
- Go 1.26+
- gcc
- g++ (-std=c++23)
- git
- make
- dpkg / dpkg-architecture (Debian/Ubuntu — required for `make package` and `make install`)
- rpmbuild / rpm (RPM-based systems — required for `make package` and `make install`)
[↑ Top](#table-of-contents)
## Build
```bash
git clone https://github.com/greergan/libtsgo.git
cd libtsgo
make && make test
```
This will:
- Clone `microsoft/typescript-go` at branch `typescript/v7.0.2`
- Build `libtsgo.a` and `libtsgo.h`

### Environment Variables
| Variable | Default | Description |
|---|---|---|
| `TSGO_LIB_DIR` | `lib/` | Overrides the library directory copied into the `typescript-go` build tree. Must exist if set. |

### make build
Fetches `microsoft/typescript-go` at branch `typescript/v7.0.2` if not already present, then builds `libtsgo.a` and `libtsgo.h`. Skips the Go build if all artifacts are up to date.
```bash
make build
```

### make test
Runs both `test-cpp` and `test-c`.
```bash
make test
```

### make test-cpp
Compiles `test.cpp` against `libtsgo.a` using `g++ -std=c++23` with `-lpthread -ldl`, then runs the resulting binary.
```bash
make test-cpp
```

### make test-c
Compiles `test.c` against `libtsgo.a` using `gcc` with `-lpthread -ldl`, then runs the resulting binary.
```bash
make test-c
```

### make package
Builds a package for the current OS and architecture. Requires a git tag on the current commit. Errors on unrecognized OS.
```bash
make package
```
| OS Family | Output |
|---|---|
| Debian / Ubuntu | `dist/libtsgo_<version>_<arch>.deb` |
| RPM-based | `dist/libtsgo-<version>-1.<arch>.rpm` |

### make install
Packages and installs `libtsgo` for the current OS using `dpkg` or `rpm`.
```bash
make install
```

### make clean
Removes `libtsgo.a`, `libtsgo.h`, `test_cpp`, `test_c`, `dist/`, and `.lib_dir_stamp`.
```bash
make clean
```
[↑ Top](#table-of-contents)
## Examples
### fetch\_and\_transpile
#### C
```c
#include "tsgo.h"
#include "libtsgo.h"
GoStr result;
result.p = fetch_and_transpile((char*)"file:///path/to/input.ts");
printf("%s\n", result.p ? result.p : "");
GoStr_free(result);
```
#### C++
```cpp
#include "tsgo.h"
#include "libtsgo.h"
GoStr result = fetch_and_transpile(const_cast<char*>("https:///path/to/input.ts"));
std::cout << result.view() << std::endl;
```
### build
#### C
```c
#include "libtsgo.h"
build((char*)"src", (char*)"dist");
```
#### C++
```cpp
#include "libtsgo.h"
build(const_cast<char*>("src"), const_cast<char*>("dist"));
```
### Type Resolution
#### file.d.ts
```typescript
/// <reference path="console.d.ts" />
/// <reference path="utils.d.ts" />
```
[↑ Top](#table-of-contents)
