// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package amd64 implements the AMD64 (x86-64) architecture target backend and disassembler for c2plan9.
package amd64

import (
	"github.com/lemon4ksan/c2plan9/arch"
)

// Backend implements arch.ArchBackend for x86-64 / AMD64.
type Backend struct{}

// New returns a new AMD64 backend instance.
func New() *Backend {
	return &Backend{}
}

// Name returns "amd64".
func (b *Backend) Name() string { return "amd64" }

// ParamRegisters returns the System V AMD64 argument registers: DI, SI, DX, CX, R8, R9.
func (b *Backend) ParamRegisters() []string {
	return []string{"DI", "SI", "DX", "CX", "R8", "R9"}
}

// LoadOp returns "MOVQ".
func (b *Backend) LoadOp() string { return "MOVQ" }

// DefaultParamCount returns 6.
func (b *Backend) DefaultParamCount() int { return 6 }

// BuildTag returns "//go:build amd64 && !purego".
func (b *Backend) BuildTag() string { return "//go:build amd64 && !purego" }

// ClangTarget returns "x86_64-pc-linux-gnu".
func (b *Backend) ClangTarget() string { return "x86_64-pc-linux-gnu" }

// ReturnPlacement maps ReturnKind to the corresponding AMD64 return register and store operation.
func (b *Backend) ReturnPlacement(kind arch.ReturnKind) arch.ReturnPlacement {
	switch kind {
	case arch.RetFloat32:
		return arch.ReturnPlacement{Op: "MOVSS", Reg: "X0"}
	case arch.RetFloat64:
		return arch.ReturnPlacement{Op: "MOVSD", Reg: "X0"}
	case arch.RetVector128:
		return arch.ReturnPlacement{Op: "MOVOU", Reg: "X0"}
	case arch.RetVector256:
		return arch.ReturnPlacement{Op: "VMOVDQU", Reg: "Y0"}
	case arch.RetInt32:
		return arch.ReturnPlacement{Op: "MOVL", Reg: "AX"}
	default:
		return arch.ReturnPlacement{Op: "MOVQ", Reg: "AX"}
	}
}

// Disassemble decodes x86-64 machine code.
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
