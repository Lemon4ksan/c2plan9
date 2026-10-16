# GEMINI.md — Development Guidelines & AI Assistant Protocol

This document outlines the architecture, coding standards, build/test commands, and interaction rules for AI assistants and software engineers working in the **c2plan9** repository.

## 1. Project Overview

**`c2plan9`** is a high-performance compiler pipeline and machine code disassembly tool that converts C, C++, and LLVM IR source files into Go-compatible Plan 9 assembly (`.s`) and generates corresponding Go companion stubs (`.go`).

> **Core Engineering Manifesto**:
> _«Zero CGO latency meets bare-metal vectorization — compile C intrinsics directly into Go Plan 9 assembly.»_

> **The "c2plan9 v1" Compatibility & Forever-Frozen Core Manifesto**:
> _"Code generated with **c2plan9 v1.0.0** is guaranteed to remain assemble-able by the standard Go toolchain. The Plan 9 assembly translation grammar and ELF disassembly invariants are permanent."_

### Key Capabilities & Architectural Pillars
- **Portable ELF Object Generation**: Forces `-target x86_64-pc-linux-gnu` or `aarch64-linux-gnu` to bypass OS-specific object discrepancies.
- **Typed Return Frame Tracking**: Emits correct register transfers (`VMOVDQU`, `FMOVQ`, `MOVSS`, etc.) into Go return frames for scalar and vector types (`m128`, `m256`).
- **Stack Alignment Preservation**: Retains dynamic stack alignment (`ANDQ $-32, SP`) with exact frame padding to avoid AVX alignment faults.
- **Dual Architecture**: Full AMD64 (AVX2/AVX-512/FMA/PCLMUL) and ARM64 (NEON/Crypto) translation.

---

## 2. Repository Layout

```text
c2plan9/
├── main.go                           // CLI command entrypoint and argument routing
├── compiler.go                       // Clang / LLVM compiler invocation and WSL bridge
├── objparser.go                      // ELF, Mach-O, and PE object format decoders
├── emitter.go                        // Plan 9 assembly emitter and Go stub generator
├── pipeline.go                       // Modular compilation and translation pipeline engine
├── arch.go                           // Root architecture facade and re-exported types
├── disasm.go                         // Backward-compatible AMD64 facade
├── arm64_disasm.go                   // Backward-compatible ARM64 facade
├── doc.go                            // Package documentation
├── arch/                             // Architecture subsystem
│   ├── arch.go                       // ArchBackend interface, registry, and shared types
│   ├── amd64/                        // AMD64 backend and machine code disassembler
│   │   ├── backend.go
│   │   └── disasm.go
│   └── arm64/                        // ARM64 backend and machine code disassembler
│       ├── backend.go
│       └── disasm.go
└── .agents/                          // AI assistant guidelines and rules
```

---

## 3. Core Architectural Principles & Technical Constraints

1. **Deterministic Translation**:
   - Compiling the same C source code must yield identical, reproducible Plan 9 assembly.
2. **Go ABI Conformance**:
   - Always adhere to Go internal ABI requirements for frame sizing, stack growth, and register clobbering.
3. **No CGO Dependency**:
   - The tool is designed to eliminate CGO at runtime for consuming packages. Emitted code must be standalone Plan 9 assembly.

---

## 4. Code Style & Quality Standards

- **Go Version**: 1.27+.
- **Documentation**: All public symbols and exported types must have clear Godoc documentation.
- **Linters**: Pass `golangci-lint run` with 0 warnings.

---

## 5. Development & Testing Commands

| Command | Purpose |
| :--- | :--- |
| `go test ./...` | Run fast unit tests |
| `go test -race ./...` | Unit tests with race detector |
| `make lint` | Run `golangci-lint` verification |
| `make format` | Format code (`gofmt`, `golangci-lint --fix`) |

---

## 6. Commit & Pull Request Guidelines

### Conventional Commits Format:
```text
<type>(<scope>): <short summary>

<detailed description of changes>
```

Commit Types:
- `feat`: New instruction disassembler or architecture feature
- `fix`: Bug fix in opcode translation or stack alignment
- `impr`: Performance optimization or emitter clean-up
- `docs`: Documentation updates
- `test`: Unit or assembly regression test additions
