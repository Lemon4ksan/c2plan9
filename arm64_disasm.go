// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"github.com/lemon4ksan/c2plan9/arch/arm64"
)

// DisassembleARM64 disassembles raw AArch64 machine code into Plan 9 ARM64 assembly lines.
// It is a backward-compatible facade delegating to the arm64 architecture package.
func DisassembleARM64(code []byte, baseOffset uint64, relocs []Relocation) (DisassemblyResult, error) {
	return arm64.Disassemble(code, baseOffset, relocs)
}

// cleanPlan9ARM64Syntax standardizes register and instruction syntax for Go's ARM64 assembler.
func cleanPlan9ARM64Syntax(s string) string {
	return arm64.CleanPlan9Syntax(s)
}

// replaceARM64ROData rewrites ARM64 ADRP, LDR, or MOVD instructions referencing static read-only data.
func replaceARM64ROData(text string, addend int64) string {
	return arm64.ReplaceROData(text, addend)
}
