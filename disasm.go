// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"github.com/lemon4ksan/c2plan9/arch/amd64"
)

// DisassembleAMD64 disassembles raw x86-64 machine code into Plan 9 assembly instructions.
// It is a backward-compatible facade delegating to the amd64 architecture package.
func DisassembleAMD64(code []byte, baseOffset uint64, relocs []Relocation) (DisassemblyResult, error) {
	return amd64.Disassemble(code, baseOffset, relocs)
}

// replaceIPWithROData rewrites instruction text containing (IP) or (RIP) relative memory operands
// to reference the local static read-only data symbol ·rodata<>(SB).
func replaceIPWithROData(text string, addend int64) string {
	return amd64.ReplaceIPWithROData(text, addend)
}

// cleanPlan9Syntax standardizes register and instruction syntax for Go's AMD64 assembler.
func cleanPlan9Syntax(s string) string {
	return amd64.CleanPlan9Syntax(s)
}

// isVZeroUpper reports whether the machine code at the given offset matches the 3-byte VZEROUPPER sequence.
func isVZeroUpper(code []byte, offset int) bool {
	return amd64.IsVZeroUpper(code, offset)
}

// hasRexW reports whether the given instruction prefix byte is a REX prefix with the REX.W bit set.
func hasRexW(prefix byte) bool {
	return amd64.HasRexW(prefix)
}
