# libslim-tsgo

A C-callable static library that wraps the [microsoft/typescript-go](https://github.com/microsoft/typescript-go) compiler, enabling TypeScript compilation from C++ applications.

## Requirements

- Go 1.26+
- g++
- dpkg-deb
- git

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

## Install

```bash
sudo dpkg -i dist/libslim-tsgo_7.0.2_amd64.deb
```

Installs:
- `/usr/lib/libslim_tsgo.a`
- `/usr/include/libslim_tsgo.h`

## Usage

```cpp
#include <libslim_tsgo.h>

// link with: g++ main.cpp -lslim_tsgo -lpthread -ldl
```

TypeScript standard library definition files from the `lib/` directory are embedded at compile time.

TypeScript type definitions placed in the `types/` directory are automatically loaded at runtime.

## API

### `transpile`

Compiles a single TypeScript file in-memory.

```c
char* transpile(
    char* fileName,   // virtual file name e.g. "input.ts"
    char* tsCode,     // TypeScript source
    char* dtsCode,    // optional .d.ts declarations, or NULL
    char* outDir      // output directory, or NULL for in-memory result
);
```

Returns emitted JavaScript, or an error string prefixed with `Error:`.  
Caller must `free()` the returned string.

### `build`

Compiles all `.ts` files in a source directory.

```c
char* build(
    char* srcDir,   // source directory e.g. "src"
    char* outDir    // output directory e.g. "dist"
);
```

Returns empty string on success, or an error string prefixed with `Error:`.  
Caller must `free()` the returned string.

## License

MIT
