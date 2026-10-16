// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"github.com/lemon4ksan/c2plan9/arch"
	"github.com/lemon4ksan/c2plan9/arch/amd64"
	"github.com/lemon4ksan/c2plan9/arch/arm64"
)

// Re-export arch types for backward compatibility.
type (
	// ReturnPlacement specifies the assembly instruction and register used to store
	// a function return value back onto the caller's stack frame.
	ReturnPlacement = arch.ReturnPlacement

	// Disassembler decodes machine code into DisassembledInst structures.
	Disassembler = arch.Disassembler

	// DisassemblerFunc is an adapter to allow the use of ordinary functions as Disassemblers.
	DisassemblerFunc = arch.DisassemblerFunc

	// DisassembledInst represents a single disassembled machine instruction with its offset, raw bytes,
	// normalized Plan 9 assembly representation, and associated metadata.
	DisassembledInst = arch.DisassembledInst

	// DisassemblyResult contains the sequence of disassembled instructions and detected stack frame size.
	DisassemblyResult = arch.DisassemblyResult

	// ArchBackend defines the interface for an architecture target in the c2plan9 pipeline.
	ArchBackend = arch.ArchBackend

	// AMD64Backend implements ArchBackend for x86-64 / AMD64.
	AMD64Backend = amd64.Backend

	// ARM64Backend implements ArchBackend for AArch64 / ARM64.
	ARM64Backend = arm64.Backend
)

// Disassembler bridges for AMD64 and ARM64.
var (
	AMD64Disassembler arch.Disassembler = arch.DisassemblerFunc(DisassembleAMD64)
	ARM64Disassembler arch.Disassembler = arch.DisassemblerFunc(DisassembleARM64)
)

// RegisterArch registers an ArchBackend for a given architecture name.
func RegisterArch(backend ArchBackend) {
	arch.RegisterArch(backend)
}

// GetArch retrieves the ArchBackend for the specified architecture name.
// If arch is empty, it defaults to "amd64".
func GetArch(name string) (ArchBackend, error) {
	return arch.GetArch(name)
}

// RegisteredArchs returns a sorted list of registered architecture names.
func RegisteredArchs() []string {
	return arch.RegisteredArchs()
}
