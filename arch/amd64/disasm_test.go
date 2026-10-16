// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package amd64_test

import (
	"testing"

	"github.com/lemon4ksan/c2plan9/arch"
	"github.com/lemon4ksan/c2plan9/arch/amd64"
)

func TestDisassembleAMD64_RET(t *testing.T) {
	backend, err := arch.GetArch("amd64")
	if err != nil {
		t.Fatalf("failed to get amd64 arch: %v", err)
	}

	code := []byte{0xC3} // RET
	res, err := backend.Disassemble(code, 0, nil)
	if err != nil {
		t.Fatalf("Disassemble failed: %v", err)
	}
	if len(res.Insts) != 1 {
		t.Fatalf("expected 1 instruction, got %d", len(res.Insts))
	}
	if !res.Insts[0].IsRet {
		t.Errorf("expected IsRet to be true")
	}
	if res.Insts[0].Text != "RET" {
		t.Errorf("expected text RET, got %s", res.Insts[0].Text)
	}
}

func TestReplaceIPWithROData_Spacing(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		addend   int64
		expected string
	}{
		{
			name:     "single operand",
			input:    "MOVQ 0(IP)",
			addend:   16,
			expected: "MOVQ ·rodata<>+16(SB)",
		},
		{
			name:     "first operand RIP, second register",
			input:    "MOVQ 16(RIP), AX",
			addend:   16,
			expected: "MOVQ ·rodata<>+16(SB), AX",
		},
		{
			name:     "first operand register, second operand RIP (BUG-001 regression)",
			input:    "MOVQ AX, 16(RIP)",
			addend:   16,
			expected: "MOVQ AX, ·rodata<>+16(SB)",
		},
		{
			name:     "negative addend",
			input:    "MOVQ -32(RIP), CX",
			addend:   -32,
			expected: "MOVQ ·rodata<>-32(SB), CX",
		},
		{
			name:     "zero addend",
			input:    "MOVQ 0(RIP), DX",
			addend:   0,
			expected: "MOVQ ·rodata<>(SB), DX",
		},
		{
			name:     "unrelated instruction",
			input:    "ADDQ $32, SP",
			addend:   0,
			expected: "ADDQ $32, SP",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := amd64.ReplaceIPWithROData(tc.input, tc.addend)
			if got != tc.expected {
				t.Errorf("ReplaceIPWithROData(%q, %d) = %q, want %q", tc.input, tc.addend, got, tc.expected)
			}
		})
	}
}

func TestCleanPlan9Syntax_CMOV(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"CMOVAE AX, BX", "CMOVQCC AX, BX"},
		{"CMOVB CX, DX", "CMOVQCS CX, DX"},
		{"CMOVE R8, R9", "CMOVQEQ R8, R9"},
		{"CMOVNE R10, R11", "CMOVQNE R10, R11"},
		{"CMOVG R12, R13", "CMOVQGT R12, R13"},
		{"CMOVGE R14, R15", "CMOVQGE R14, R15"},
		{"CMOVL SI, DI", "CMOVQLT SI, DI"},
		{"CMOVLE BP, SP", "CMOVQLE BP, SP"},
		{"CMOVNS AX, R8", "CMOVQPL AX, R8"},
		{"CMOVS BX, R9", "CMOVQMI BX, R9"},
		{"CMOVE AX, EAX", "CMOVLEQ AX, EAX"},
	}

	for _, tc := range tests {
		got := amd64.CleanPlan9Syntax(tc.input)
		if got != tc.expected {
			t.Errorf("CleanPlan9Syntax(%q) = %q, want %q", tc.input, got, tc.expected)
		}
	}
}

func TestCleanPlan9Syntax_SETcc(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"SETNS AX", "SETPL AL"},
		{"SETS BX", "SETMI BL"},
		{"SETE CX", "SETEQ CL"},
		{"SETAE DX", "SETCC DL"},
		{"SETB SI", "SETCS SIL"},
		{"SETA DI", "SETHI DIL"},
		{"SETBE BP", "SETLS BPL"},
		{"SETL SP", "SETLT SPL"},
		{"SETG R8", "SETGT R8B"},
		{"SETNE R9", "SETNE R9B"},
	}

	for _, tc := range tests {
		got := amd64.CleanPlan9Syntax(tc.input)
		if got != tc.expected {
			t.Errorf("CleanPlan9Syntax(%q) = %q, want %q", tc.input, got, tc.expected)
		}
	}
}

func TestCleanPlan9Syntax_MnemonicAliases(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"MOVZX AL, BX", "MOVBLZX AL, BX"},
		{"MOVSX BL, CX", "MOVBLSX BL, CX"},
		{"MOVSXD EAX, RDX", "MOVLQSX EAX, RDX"},
		{"TZCNT AX, BX", "TZCNTL AX, BX"},
		{"BSF CX, DX", "BSFL CX, DX"},
		{"BSR SI, DI", "BSRL SI, DI"},
		{"LZCNT R8, R9", "LZCNTL R8, R9"},
		{"POPCNT R10, R11", "POPCNTL R10, R11"},
	}

	for _, tc := range tests {
		got := amd64.CleanPlan9Syntax(tc.input)
		if got != tc.expected {
			t.Errorf("CleanPlan9Syntax(%q) = %q, want %q", tc.input, got, tc.expected)
		}
	}
}

func TestCleanPlan9Syntax_StackAlignment(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"ANDQ $-0x20, SP", "ANDQ $-32, SP"},
		{"ANDQ $-0x40, SP", "ANDQ $-64, SP"},
		{"ANDL $-0x10, SP", "ANDL $-16, SP"},
	}

	for _, tc := range tests {
		got := amd64.CleanPlan9Syntax(tc.input)
		if got != tc.expected {
			t.Errorf("CleanPlan9Syntax(%q) = %q, want %q", tc.input, got, tc.expected)
		}
	}
}

func TestIsVZeroUpper(t *testing.T) {
	code := []byte{0x90, 0xC5, 0xF8, 0x77, 0xC3}
	if !amd64.IsVZeroUpper(code, 1) {
		t.Errorf("expected IsVZeroUpper at offset 1 to be true")
	}
	if amd64.IsVZeroUpper(code, 0) {
		t.Errorf("expected IsVZeroUpper at offset 0 to be false")
	}
	if amd64.IsVZeroUpper(code, 3) {
		t.Errorf("expected IsVZeroUpper at offset 3 to be false (too short)")
	}
}
