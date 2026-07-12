<a href="https://codeberg.org/greergan/SlimTS">
  <img src="https://raw.githubusercontent.com/greergan/SlimTS/master/assets/slimts_logo.png" width="75" alt="SlimTS Logo">
</a>

# libslim-tsgo

A C-callable static library that wraps the [microsoft/typescript-go](https://github.com/microsoft/typescript-go) compiler, enabling TypeScript support for [SlimTS](https://codeberg.org/greergan/SlimTS)

## Table of Contents

- [Typescript Definition Files](#typescript-definition-files)
- [Compiler Options](#compiler-options)
- [API](#api)
  - [GoStr RAII helper](#gostr-raii-helper)
  - [transpile](#transpile)
  - [build](#build)
- [Examples](#examples)
  - [transpile](#transpile-1)
  - [build](#build-1)
- [Requirements](#requirements)
- [Build](#build-2)
- [Install](#install)

## Typescript Definition Files

| Directory | Purpose | When |
|---|---|---|
| `typescript-go` | All `microsoft/typescript-go` compiler libraries | Embedded at compile time into the static archive |
| `lib/` | Slim TypeScript standard library `.d.ts` files | Embedded at compile time into the static archive |
| `types/` | User-provided type definitions | Loaded at runtime from the working directory |

[↑ Top](#table-of-contents)

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

### GoStr RAII helper

```cpp
struct GoStr {
    char* p;
    GoStr(char* p) : p(p) {}
    ~GoStr() { free(p); }
    std::string_view view() const { return p; }
};
```

### `transpile`

Compiles a single TypeScript file in-memory.

```c
GoStr result(transpile(
    char* fileName,   // virtual file name e.g. "input.ts"
    char* tsCode,     // TypeScript source
    char* dtsCode,    // optional .d.ts declarations, or NULL
    char* outDir      // output directory, or NULL for in-memory result
));
```

Returns emitted JavaScript.  
Caller must `free()` the returned string, or use the provided `GoStr` helper above.

### `build`

Compiles all `.ts` files in a source tree. Diagnostics and errors are printed to stdout.

```c
void build(
    char* srcDir,   // source directory e.g. "src"
    char* outDir    // output directory e.g. "dist"
);
```

[↑ Top](#table-of-contents)

## Examples

### transpile

#### In-memory result

```cpp
#include <libslim_tsgo.h>

std::string ts = "const x: number = 42;\nconsole.log(x);\n";

GoStr result(transpile(
    const_cast<char*>("input.ts"),
    const_cast<char*>(ts.c_str()),
    nullptr,
    nullptr
));

std::cout << result.view() << std::endl;
```

#### Emit to disk

```cpp
#include <libslim_tsgo.h>

std::string ts = "const x: number = 42;\nconsole.log(x);\n";

transpile(
    const_cast<char*>("input.ts"),
    const_cast<char*>(ts.c_str()),
    nullptr,
    const_cast<char*>("dist")
);
```

#### With .d.ts declarations

```cpp
#include <libslim_tsgo.h>

std::string dts = "declare function add(a: number, b: number): number;\n";
std::string ts  = "const result = add(1, 2);\nconsole.log(result);\n";

GoStr result(transpile(
    const_cast<char*>("input.ts"),
    const_cast<char*>(ts.c_str()),
    const_cast<char*>(dts.c_str()),
    nullptr
));

std::cout << result.view() << std::endl;
```

### build

```cpp
#include <libslim_tsgo.h>

build(
    const_cast<char*>("src"),
    const_cast<char*>("dist")
);
```

[↑ Top](#table-of-contents)

## Requirements

- Go 1.26+
- g++
- dpkg-deb
- git
- make

[↑ Top](#table-of-contents)

## Build

```bash
git clone https://codeberg.org/greergan/slim-typescript-go-lib.git
cd slim-typescript-go-lib
make deb
```

This will:
- Clone `microsoft/typescript-go` at branch `typescript/v7.0.2`
- Build `libslim_tsgo.a` and `libslim_tsgo.h`
- Package them into `dist/libslim-tsgo_7.0.2_amd64.deb`

[↑ Top](#table-of-contents)

## Install

```bash
sudo dpkg -i dist/libslim-tsgo_7.0.2_amd64.deb
```

Installs:
- `/usr/lib/libslim_tsgo.a`
- `/usr/include/libslim_tsgo.h`

[↑ Top](#table-of-contents)
