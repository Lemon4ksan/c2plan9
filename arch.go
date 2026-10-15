// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"sort"
	"sync"
)

// ReturnPlacement specifies the assembly instruction and register used to store
// a function return value back onto the caller's stack frame.
type ReturnPlacement struct {
	Op  string
	Reg string
}

// Disassembler decodes machine code into DisassembledInst structures.
type Disassembler interface {
	Disassemble(code []byte, baseOffset uint64, relocs []Relocation) (DisassemblyResult, error)
}

// DisassemblerFunc is an adapter to allow the use of ordinary functions as Disassemblers.
type DisassemblerFunc func(code []byte, baseOffset uint64, relocs []Relocation) (DisassemblyResult, error)

// Disassemble calls f(code, baseOffset, relocs).
func (f DisassemblerFunc) Disassemble(code []byte, baseOffset uint64, relocs []Relocation) (DisassemblyResult, error) {
	return f(code, baseOffset, relocs)
}

// ArchBackend defines the interface for an architecture target in the c2plan9 pipeline.
// Implementations encapsulate architecture-specific calling conventions, parameter registers,
// instruction disassembly, and Plan 9 assembly generation details.
type ArchBackend interface {
	// Name returns the architecture identifier (e.g. "amd64", "arm64").
	Name() string

	// ParamRegisters returns the ordered sequence of register names used for argument passing.
	ParamRegisters() []string

	// LoadOp returns the assembly operation used to load a parameter word from the stack.
	LoadOp() string

	// ReturnPlacement returns the assembly store operation and register name for the given ReturnKind.
	ReturnPlacement(kind ReturnKind) ReturnPlacement

	// Disassemble decodes machine code bytes into DisassembledInst list and computes stack frame size.
	Disassemble(code []byte, baseOffset uint64, relocs []Relocation) (DisassemblyResult, error)

	// DefaultParamCount returns the default number of parameter registers.
	DefaultParamCount() int

	// BuildTag returns the Go build constraint tag (e.g. "//go:build amd64 && !purego").
	BuildTag() string

	// ClangTarget returns the standard ELF target triple for the architecture.
	ClangTarget() string
}

var (
	archRegistry = make(map[string]ArchBackend)
	archMu       sync.RWMutex
)

// RegisterArch registers an ArchBackend for a given architecture name.
func RegisterArch(backend ArchBackend) {
	archMu.Lock()
	defer archMu.Unlock()
	if backend == nil {
		panic("cannot register nil ArchBackend")
	}
	archRegistry[backend.Name()] = backend
}

// GetArch retrieves the ArchBackend for the specified architecture name.
// If arch is empty, it defaults to "amd64".
func GetArch(name string) (ArchBackend, error) {
	if name == "" {
		name = "amd64"
	}
	archMu.RLock()
	backend, ok := archRegistry[name]
	archMu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("unsupported architecture: %q", name)
	}
	return backend, nil
}

// RegisteredArchs returns a sorted list of registered architecture names.
func RegisteredArchs() []string {
	archMu.RLock()
	defer archMu.RUnlock()
	names := make([]string, 0, len(archRegistry))
	for k := range archRegistry {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}

// AMD64Backend implements ArchBackend for x86-64 / AMD64.
type AMD64Backend struct{}

// Name returns "amd64".
func (b *AMD64Backend) Name() string { return "amd64" }

// ParamRegisters returns the System V AMD64 argument registers: DI, SI, DX, CX, R8, R9.
func (b *AMD64Backend) ParamRegisters() []string {
	return []string{"DI", "SI", "DX", "CX", "R8", "R9"}
}

// LoadOp returns "MOVQ".
func (b *AMD64Backend) LoadOp() string { return "MOVQ" }

// DefaultParamCount returns 6.
func (b *AMD64Backend) DefaultParamCount() int { return 6 }

// BuildTag returns "//go:build amd64 && !purego".
func (b *AMD64Backend) BuildTag() string { return "//go:build amd64 && !purego" }

// ClangTarget returns "x86_64-pc-linux-gnu".
func (b *AMD64Backend) ClangTarget() string { return "x86_64-pc-linux-gnu" }

// ReturnPlacement maps ReturnKind to the corresponding AMD64 return register and store operation.
func (b *AMD64Backend) ReturnPlacement(kind ReturnKind) ReturnPlacement {
	switch kind {
	case RetFloat32:
		return ReturnPlacement{Op: "MOVSS", Reg: "X0"}
	case RetFloat64:
		return ReturnPlacement{Op: "MOVSD", Reg: "X0"}
	case RetVector128:
		return ReturnPlacement{Op: "MOVOU", Reg: "X0"}
	case RetVector256:
		return ReturnPlacement{Op: "VMOVDQU", Reg: "Y0"}
	case RetInt32:
		return ReturnPlacement{Op: "MOVL", Reg: "AX"}
	default:
		return ReturnPlacement{Op: "MOVQ", Reg: "AX"}
	}
}

// Disassemble decodes x86-64 machine code.
func (b *AMD64Backend) Disassemble(code []byte, baseOffset uint64, relocs []Relocation) (DisassemblyResult, error) {
	return DisassembleAMD64(code, baseOffset, relocs)
}

// ARM64Backend implements ArchBackend for AArch64 / ARM64.
type ARM64Backend struct{}

// Name returns "arm64".
func (b *ARM64Backend) Name() string { return "arm64" }

// ParamRegisters returns the AAPCS64 argument registers: R0-R7.
func (b *ARM64Backend) ParamRegisters() []string {
	return []string{"R0", "R1", "R2", "R3", "R4", "R5", "R6", "R7"}
}

// LoadOp returns "MOVD".
func (b *ARM64Backend) LoadOp() string { return "MOVD" }

// DefaultParamCount returns 8.
func (b *ARM64Backend) DefaultParamCount() int { return 8 }

// BuildTag returns "//go:build arm64 && !purego".
func (b *ARM64Backend) BuildTag() string { return "//go:build arm64 && !purego" }

// ClangTarget returns "aarch64-linux-gnu".
func (b *ARM64Backend) ClangTarget() string { return "aarch64-linux-gnu" }

// ReturnPlacement maps ReturnKind to the corresponding ARM64 return register and store operation.
func (b *ARM64Backend) ReturnPlacement(kind ReturnKind) ReturnPlacement {
	switch kind {
	case RetFloat32:
		return ReturnPlacement{Op: "FMOVS", Reg: "F0"}
	case RetFloat64:
		return ReturnPlacement{Op: "FMOVD", Reg: "F0"}
	case RetVector128, RetVector256:
		return ReturnPlacement{Op: "FMOVQ", Reg: "F0"}
	case RetInt32:
		return ReturnPlacement{Op: "MOVW", Reg: "R0"}
	default:
		return ReturnPlacement{Op: "MOVD", Reg: "R0"}
	}
}

// Disassemble decodes AArch64 machine code.
func (b *ARM64Backend) Disassemble(code []byte, baseOffset uint64, relocs []Relocation) (DisassemblyResult, error) {
	return DisassembleARM64(code, baseOffset, relocs)
}

func init() {
	RegisterArch(&AMD64Backend{})
	RegisterArch(&ARM64Backend{})
}
