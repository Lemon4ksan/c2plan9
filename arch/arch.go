// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package arch provides core architecture abstractions, registries, and machine-code
// translation interfaces for the c2plan9 compiler pipeline.
package arch

import (
	"fmt"
	"slices"
	"sync"
)

// ReturnKind classifies a function return type into its machine register and ABI category.
type ReturnKind int

const (
	// RetNone indicates a void function that does not return a value.
	RetNone ReturnKind = iota
	// RetInt64 indicates a 64-bit integer, pointer, or size_t return value.
	RetInt64
	// RetInt32 indicates a 32-bit integer return value.
	RetInt32
	// RetFloat32 indicates a single-precision IEEE 754 float return value.
	RetFloat32
	// RetFloat64 indicates a double-precision IEEE 754 float return value.
	RetFloat64
	// RetVector128 indicates a 128-bit SIMD vector return value (e.g. __m128, [16]byte).
	RetVector128
	// RetVector256 indicates a 256-bit SIMD vector return value (e.g. __m256, [32]byte).
	RetVector256
)

// ReturnPlacement specifies the assembly instruction and register used to store
// a function return value back onto the caller's stack frame.
type ReturnPlacement struct {
	Op  string // Assembly instruction mnemonic (e.g. "MOVQ", "MOVSS", "FMOVD")
	Reg string // Source register name (e.g. "AX", "X0", "R0", "F0")
}

// Relocation represents a relocation entry in the code referencing rodata or external symbols.
type Relocation struct {
	Offset   uint64 // Byte offset in .text section
	SymName  string // Referenced symbol name
	IsROData bool   // True if referencing .rodata section
	Addend   int64  // Relocation addend
}

// DisassembledInst represents a single disassembled machine instruction with its offset, raw bytes,
// normalized Plan 9 assembly representation, and associated metadata.
type DisassembledInst struct {
	Offset   uint64 // Byte offset of instruction within the function symbol
	Length   int    // Instruction length in bytes
	RawBytes []byte // Raw machine code bytes
	Text     string // Formatted Plan 9 assembly instruction text
	Label    string // Optional local jump label (e.g. "L_0010") preceding this instruction
	IsRet    bool   // True if the instruction is a subroutine return
}

// DisassemblyResult contains the sequence of disassembled instructions and detected stack frame size.
type DisassemblyResult struct {
	Insts     []DisassembledInst // Sequence of disassembled instructions
	FrameSize uint64             // Minimum required stack frame size in bytes
}

// Disassembler decodes machine code into DisassemblyResult structures.
type Disassembler interface {
	Disassemble(code []byte, baseOffset uint64, relocs []Relocation) (DisassemblyResult, error)
}

// DisassemblerFunc is an adapter to allow the use of ordinary functions as Disassemblers.
type DisassemblerFunc func(code []byte, baseOffset uint64, relocs []Relocation) (DisassemblyResult, error)

// Disassemble calls f(code, baseOffset, relocs).
func (f DisassemblerFunc) Disassemble(code []byte, baseOffset uint64, relocs []Relocation) (DisassemblyResult, error) {
	return f(code, baseOffset, relocs)
}

// Backend defines the interface for an architecture target in the c2plan9 pipeline.
// Implementations encapsulate architecture-specific calling conventions, parameter registers,
// instruction disassembly, and Plan 9 assembly generation details.
type Backend interface {
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
	registry = make(map[string]Backend)
	mu       sync.RWMutex
)

// Register registers an ArchBackend for a given architecture name.
func Register(backend Backend) {
	mu.Lock()
	defer mu.Unlock()
	if backend == nil {
		panic("cannot register nil ArchBackend")
	}
	registry[backend.Name()] = backend
}

// Get retrieves the ArchBackend for the specified architecture name.
// If arch is empty, it defaults to "amd64".
func Get(name string) (Backend, error) {
	if name == "" {
		name = "amd64"
	}
	mu.RLock()
	backend, ok := registry[name]
	mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("unsupported architecture: %q", name)
	}
	return backend, nil
}

// All returns a sorted list of registered architecture names.
func All() []string {
	mu.RLock()
	defer mu.RUnlock()
	names := make([]string, 0, len(registry))
	for k := range registry {
		names = append(names, k)
	}
	slices.Sort(names)
	return names
}
