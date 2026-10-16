// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"github.com/lemon4ksan/c2plan9/arch"
	"github.com/lemon4ksan/c2plan9/arch/amd64"
)

// DisassembleAMD64 disassembles raw x86-64 machine code into Plan 9 assembly instructions.
// It is a backward-compatible facade delegating to the amd64 architecture package.
func DisassembleAMD64(code []byte, baseOffset uint64, relocs []arch.Relocation) (arch.DisassemblyResult, error) {
	return amd64.Disassemble(code, baseOffset, relocs)
}
