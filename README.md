<div align="center">

# c2plan9

### The C & LLVM IR to Go Plan 9 Assembly Transpiler Pipeline

_«Zero CGO latency meets bare-metal vectorization — compile C intrinsics directly into Go Plan 9 assembly»_

[![Go Version](https://img.shields.io/badge/go-1.27%2B-007d9c?logo=go&logoColor=white&style=flat-square)](https://go.dev/)
[![Go Reference](https://img.shields.io/badge/godoc-reference-007d9c?style=flat-square)](https://pkg.go.dev/github.com/lemon4ksan/c2plan9)
[![License](https://img.shields.io/badge/license-BSD--3--Clause-blue?style=flat-square)](LICENSE)
[![Target](https://img.shields.io/badge/targets-AMD64%20%7C%20ARM64-orange?style=flat-square)](#targets--capabilities)
[![Ecosystem](https://img.shields.io/badge/ecosystem-foundation-blueviolet?style=flat-square)](https://github.com/lemon4ksan/foundation)

**c2plan9** is a high-performance compiler pipeline and machine code disassembly tool that converts C, C++, and LLVM IR source files into Go-compatible Plan 9 assembly (`.s`) and generates corresponding Go companion stubs (`.go`).

#### English • [Русский](README_RU.md)

</div>

## Why c2plan9?

High-throughput systems programming in Go often requires SIMD vectorization (AVX2, AVX-512, NEON, GFNI). However:
1. **CGO Call Latency**: Invoking C libraries via CGO introduces 40–150 ns call overhead, destroying performance benefits for micro-kernels.
2. **Error-Prone Manual Plan 9 Assembly**: Writing Plan 9 assembly by hand for complex vector intrinsics is tedious, brittle, and difficult to verify.
3. **`c2plan9` Solution**: Write clean, optimized C intrinsics, and let `c2plan9` compile them via Clang/LLVM into portable ELF object files, disassemble them into Go Plan 9 assembly, and emit Go function stubs.

## ⚡ Key Capabilities

- **Dual-Architecture Support**:
  - **AMD64**: AVX2, AVX-512, FMA, PCLMULQDQ, AES-NI, BMI1/2.
  - **ARM64**: NEON vectorization, Crypto extensions (AES, SHA1, SHA2, PMULL).
- **Portable ELF Object Generation**: Forces `-target x86_64-pc-linux-gnu` or `aarch64-linux-gnu` across all host operating systems (Windows, Linux, macOS), completely bypassing platform-specific object format discrepancies (PE relocations, Mach-O symbol mangling).
- **Typed Return Frame Tracking**: Full support for scalar returns (`int`, `int32`, `float32`, `float64`) and 128/256-bit SIMD vector returns (`m128`, `m256`), emitting exact register moves into Go's return frame.
- **Stack Alignment Preservation**: Preserves dynamic stack alignment directives (e.g. `ANDQ $-32, SP`) and calculates frame sizes with alignment padding so AVX aligned spills (`VMOVDQA`) never fault.
- **Companion Stub Generation**: Emits strongly-typed Go companion `.go` files with `//go:noescape` annotations.
- **WSL Integration**: Native Windows support with optional `--wsl` flag for compiling via Linux Clang inside WSL.

## ⚠️ Architectural Constraints & Design Rules

`c2plan9` is specifically engineered for **computational SIMD microkernels**, not as a general-purpose C compiler replacement. When authoring C source kernels, adhere to the following architectural invariants:

1. **Leaf Functions Only**:
   - C kernels must be pure leaf functions. They cannot call external functions (`call foo`), use standard library routines (`printf`, `memcpy`, `malloc`), or trigger system calls.
   - All external memory must be passed directly via pointer arguments from Go.
2. **Parameter Limits**:
   - **AMD64**: Maximum 6 integer/pointer arguments (passed via System V registers: `RDI`, `RSI`, `RDX`, `RCX`, `R8`, `R9`).
   - **ARM64**: Maximum 8 integer/pointer arguments (passed via AAPCS64 registers: `R0`–`R7`).
   - *Note*: A Go slice occupies 3 parameter slots (`ptr`, `len`, `cap`). For kernels requiring numerous buffers, pass structured pointer wrappers.
3. **Pointers for Vector Buffers**:
   - SIMD vectors should be passed by pointer (`const uint8_t* in`, `const __m256i* data`), not by value in parameter lists. Return vectors by value (`-ret=m256` or `-ret=m128`) are fully supported.
4. **No Jump Tables**:
   - Dense `switch` statements must avoid jump tables. `c2plan9` automatically passes `-fno-jump-tables` to Clang, forcing binary search branching (`CMP`/`JE`) which cleanly relocates into Plan 9.
5. **Freestanding Environment**:
   - Compiles with `-ffreestanding`, `-fno-stack-protector`, and `-fomit-frame-pointer`. No C runtime (CRT) initialization exists.

## 📦 Installation

```bash
go install github.com/lemon4ksan/c2plan9@latest
```

## 🛠️ Usage

### CLI Flags

```text
Usage of c2plan9:
  -c string
        Path to input C / C++ / LLVM IR source file (required)
  -o string
        Path to output Plan 9 assembly (.s) file (optional)
  -stub string
        Path to output Go companion stub (.go) file (optional)
  -pkg string
        Target Go package name (defaults to output directory name)
  -arch string
        Target architecture: amd64 (default) or arm64
  -clang string
        Path to Clang executable (default "clang")
  -flags string
        Extra flags to pass to Clang
  -ret string
        Return type: float32, float64, m128, m256, int, int32, or void
  -wsl
        Compile via Clang inside WSL
```

### Go Generate Example

In your Go package:

```go
//go:generate c2plan9 -c ../csrc/fastscan.c -o fastscan_amd64.s -stub fastscan_amd64.go -pkg simd
```

Run generation:
```bash
go generate ./...
```

## 🧪 Testing

```bash
make test        # Fast unit tests
make race        # Race detector validation
make lint        # Run golangci-lint
make format      # Format code and enforce license headers
```

## 📄 License

This project is licensed under the **BSD 3-Clause License**. See the [LICENSE](LICENSE) file for details.
