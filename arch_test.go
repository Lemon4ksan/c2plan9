// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"slices"
	"strings"
	"testing"
)

func TestArchitectureRegistry(t *testing.T) {
	t.Run("GetArch Default", func(t *testing.T) {
		backend, err := GetArch("")
		if err != nil {
			t.Fatalf("expected default arch, got error: %v", err)
		}
		if backend.Name() != "amd64" {
			t.Errorf("expected default arch amd64, got %s", backend.Name())
		}
	})

	t.Run("GetArch AMD64", func(t *testing.T) {
		backend, err := GetArch("amd64")
		if err != nil {
			t.Fatalf("expected amd64 backend, got error: %v", err)
		}
		if backend.Name() != "amd64" {
			t.Errorf("expected amd64, got %s", backend.Name())
		}
		if backend.DefaultParamCount() != 6 {
			t.Errorf("expected 6 default params, got %d", backend.DefaultParamCount())
		}
		if backend.LoadOp() != "MOVQ" {
			t.Errorf("expected MOVQ, got %s", backend.LoadOp())
		}
		if backend.ClangTarget() != "x86_64-pc-linux-gnu" {
			t.Errorf("expected x86_64-pc-linux-gnu, got %s", backend.ClangTarget())
		}
		regs := backend.ParamRegisters()
		expectedRegs := []string{"DI", "SI", "DX", "CX", "R8", "R9"}
		if !slices.Equal(regs, expectedRegs) {
			t.Errorf("expected regs %v, got %v", expectedRegs, regs)
		}
	})

	t.Run("GetArch ARM64", func(t *testing.T) {
		backend, err := GetArch("arm64")
		if err != nil {
			t.Fatalf("expected arm64 backend, got error: %v", err)
		}
		if backend.Name() != "arm64" {
			t.Errorf("expected arm64, got %s", backend.Name())
		}
		if backend.DefaultParamCount() != 8 {
			t.Errorf("expected 8 default params, got %d", backend.DefaultParamCount())
		}
		if backend.LoadOp() != "MOVD" {
			t.Errorf("expected MOVD, got %s", backend.LoadOp())
		}
		if backend.ClangTarget() != "aarch64-linux-gnu" {
			t.Errorf("expected aarch64-linux-gnu, got %s", backend.ClangTarget())
		}
		regs := backend.ParamRegisters()
		expectedRegs := []string{"R0", "R1", "R2", "R3", "R4", "R5", "R6", "R7"}
		if !slices.Equal(regs, expectedRegs) {
			t.Errorf("expected regs %v, got %v", expectedRegs, regs)
		}
	})

	t.Run("GetArch Unsupported", func(t *testing.T) {
		_, err := GetArch("mips64")
		if err == nil {
			t.Fatal("expected error for unsupported arch, got nil")
		}
	})

	t.Run("RegisteredArchs", func(t *testing.T) {
		archs := RegisteredArchs()
		if !slices.Contains(archs, "amd64") || !slices.Contains(archs, "arm64") {
			t.Errorf("missing core architectures in RegisteredArchs: %v", archs)
		}
	})
}

func TestReturnPlacement(t *testing.T) {
	amd64, err := GetArch("amd64")
	if err != nil {
		t.Fatal(err)
	}

	arm64, err := GetArch("arm64")
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		kind    ReturnKind
		wantAMD ReturnPlacement
		wantARM ReturnPlacement
	}{
		{RetFloat32, ReturnPlacement{Op: "MOVSS", Reg: "X0"}, ReturnPlacement{Op: "FMOVS", Reg: "F0"}},
		{RetFloat64, ReturnPlacement{Op: "MOVSD", Reg: "X0"}, ReturnPlacement{Op: "FMOVD", Reg: "F0"}},
		{RetVector128, ReturnPlacement{Op: "MOVOU", Reg: "X0"}, ReturnPlacement{Op: "FMOVQ", Reg: "F0"}},
		{RetVector256, ReturnPlacement{Op: "VMOVDQU", Reg: "Y0"}, ReturnPlacement{Op: "FMOVQ", Reg: "F0"}},
		{RetInt32, ReturnPlacement{Op: "MOVL", Reg: "AX"}, ReturnPlacement{Op: "MOVW", Reg: "R0"}},
		{RetInt64, ReturnPlacement{Op: "MOVQ", Reg: "AX"}, ReturnPlacement{Op: "MOVD", Reg: "R0"}},
	}

	for _, tc := range tests {
		gotAMD := amd64.ReturnPlacement(tc.kind)
		if gotAMD != tc.wantAMD {
			t.Errorf("amd64 ReturnPlacement(%d) = %+v, want %+v", tc.kind, gotAMD, tc.wantAMD)
		}

		gotARM := arm64.ReturnPlacement(tc.kind)
		if gotARM != tc.wantARM {
			t.Errorf("arm64 ReturnPlacement(%d) = %+v, want %+v", tc.kind, gotARM, tc.wantARM)
		}
	}
}

type mockCustomArch struct{}

func (m *mockCustomArch) Name() string             { return "custom64" }
func (m *mockCustomArch) ParamRegisters() []string { return []string{"A0", "A1"} }
func (m *mockCustomArch) LoadOp() string           { return "LD" }
func (m *mockCustomArch) DefaultParamCount() int   { return 2 }
func (m *mockCustomArch) BuildTag() string         { return "//go:build custom64 && !purego" }
func (m *mockCustomArch) ClangTarget() string      { return "custom64-linux-gnu" }
func (m *mockCustomArch) ReturnPlacement(ReturnKind) ReturnPlacement {
	return ReturnPlacement{Op: "SD", Reg: "A0"}
}

func (m *mockCustomArch) Disassemble([]byte, uint64, []Relocation) (DisassemblyResult, error) {
	return DisassemblyResult{
		Insts: []DisassembledInst{
			{Offset: 0, Length: 4, Text: "RET", IsRet: true},
		},
		FrameSize: 0,
	}, nil
}

func TestCustomArchRegistration(t *testing.T) {
	mock := &mockCustomArch{}
	RegisterArch(mock)

	backend, err := GetArch("custom64")
	if err != nil {
		t.Fatalf("failed to retrieve registered custom arch: %v", err)
	}
	if backend.Name() != "custom64" {
		t.Fatalf("expected custom64, got %s", backend.Name())
	}

	syms := []Symbol{
		{
			Name: "custom_func",
			Code: []byte{0x00, 0x00, 0x00, 0x00},
		},
	}

	sigs := map[string]FuncSignature{
		"custom_func": {
			Name:       "custom_func",
			GoName:     "CustomFunc",
			ParamCount: 2,
			HasReturn:  true,
		},
	}

	asmBytes, err := EmitPlan9Assembly("testpkg", syms, sigs, nil, nil, "custom64")
	if err != nil {
		t.Fatalf("EmitPlan9Assembly failed with custom arch: %v", err)
	}

	asm := string(asmBytes)
	if !strings.Contains(asm, "//go:build custom64 && !purego") {
		t.Errorf("expected build tag for custom arch, got:\n%s", asm)
	}
	if !strings.Contains(asm, "LD arg0+0(FP), A0") {
		t.Errorf("expected LD A0 param load, got:\n%s", asm)
	}
	if !strings.Contains(asm, "SD A0, ret+16(FP)") {
		t.Errorf("expected SD return placement, got:\n%s", asm)
	}
}

func TestDisassemblyBridges(t *testing.T) {
	// Test AMD64 RET (0xC3)
	amdRes, err := AMD64Disassembler.Disassemble([]byte{0xC3}, 0, nil)
	if err != nil {
		t.Fatalf("AMD64Disassembler failed: %v", err)
	}
	if len(amdRes.Insts) == 0 || !amdRes.Insts[0].IsRet {
		t.Errorf("expected AMD64 RET instruction, got: %+v", amdRes.Insts)
	}

	// Test ARM64 RET (0xd65f03c0)
	armRes, err := ARM64Disassembler.Disassemble([]byte{0xc0, 0x03, 0x5f, 0xd6}, 0, nil)
	if err != nil {
		t.Fatalf("ARM64Disassembler failed: %v", err)
	}
	if len(armRes.Insts) == 0 || !armRes.Insts[0].IsRet {
		t.Errorf("expected ARM64 RET instruction, got: %+v", armRes.Insts)
	}
}

func TestReplaceIPWithROData(t *testing.T) {
	tests := []struct {
		input    string
		addend   int64
		expected string
	}{
		{
			input:    "MOVQ 16(IP), AX",
			addend:   16,
			expected: "MOVQ ·rodata<>+16(SB), AX",
		},
		{
			input:    "MOVQ AX, 16(IP)",
			addend:   16,
			expected: "MOVQ AX, ·rodata<>+16(SB)",
		},
		{
			input:    "MOVQ 0(RIP), DX",
			addend:   0,
			expected: "MOVQ ·rodata<>(SB), DX",
		},
		{
			input:    "MOVQ -32(RIP), CX",
			addend:   -32,
			expected: "MOVQ ·rodata<>-32(SB), CX",
		},
		{
			input:    "NOP",
			addend:   0,
			expected: "NOP",
		},
	}

	for _, tc := range tests {
		got := replaceIPWithROData(tc.input, tc.addend)
		if got != tc.expected {
			t.Errorf("replaceIPWithROData(%q, %d) = %q, want %q", tc.input, tc.addend, got, tc.expected)
		}
	}
}
