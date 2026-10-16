// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package arm64 implements the ARM64 (AArch64) architecture target backend and disassembler for c2plan9.
package arm64

import (
	"github.com/lemon4ksan/c2plan9/arch"
)

// Backend implements arch.ArchBackend for AArch64 / ARM64.
type Backend struct{}

// New returns a new ARM64 backend instance.
func New() *Backend {
	return &Backend{}
}

// Name returns "arm64".
func (b *Backend) Name() string { return "arm64" }

// ParamRegisters returns the AAPCS64 argument registers: R0-R7.
func (b *Backend) ParamRegisters() []string {
	return []string{"R0", "R1", "R2", "R3", "R4", "R5", "R6", "R7"}
}

// LoadOp returns "MOVD".
func (b *Backend) LoadOp() string { return "MOVD" }

// DefaultParamCount returns 8.
func (b *Backend) DefaultParamCount() int { return 8 }

// BuildTag returns "//go:build arm64 && !purego".
func (b *Backend) BuildTag() string { return "//go:build arm64 && !purego" }

// ClangTarget returns "aarch64-linux-gnu".
func (b *Backend) ClangTarget() string { return "aarch64-linux-gnu" }

// ReturnPlacement maps ReturnKind to the corresponding ARM64 return register and store operation.
func (b *Backend) ReturnPlacement(kind arch.ReturnKind) arch.ReturnPlacement {
	switch kind {
	case arch.RetFloat32:
		return arch.ReturnPlacement{Op: "FMOVS", Reg: "F0"}
	case arch.RetFloat64:
		return arch.ReturnPlacement{Op: "FMOVD", Reg: "F0"}
	case arch.RetVector128, arch.RetVector256:
		return arch.ReturnPlacement{Op: "FMOVQ", Reg: "F0"}
	case arch.RetInt32:
		return arch.ReturnPlacement{Op: "MOVW", Reg: "R0"}
	default:
		return arch.ReturnPlacement{Op: "MOVD", Reg: "R0"}
	}
}

// Disassemble decodes AArch64 machine code.
func (b *Backend) Disassemble(
	code []byte,
	baseOffset uint64,
	relocs []arch.Relocation,
) (arch.DisassemblyResult, error) {
	return Disassemble(code, baseOffset, relocs)
}

func init() {
	arch.Register(New())
}
