// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"testing"
)

func TestReplaceIPWithROData_Spacing(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		addend   int64
		expected string
	}{
		{
			name:     "first operand zero addend",
			input:    "MOVQ (IP), AX",
			addend:   0,
			expected: "MOVQ ·rodata<>(SB), AX",
		},
		{
			name:     "first operand positive addend",
			input:    "VMOVDQU (RIP), Y0",
			addend:   32,
			expected: "VMOVDQU ·rodata<>+32(SB), Y0",
		},
		{
			name:     "second operand zero addend",
			input:    "MOVQ AX, (IP)",
			addend:   0,
			expected: "MOVQ AX, ·rodata<>(SB)",
		},
		{
			name:     "second operand positive addend",
			input:    "VMOVDQU Y0, (RIP)",
			addend:   64,
			expected: "VMOVDQU Y0, ·rodata<>+64(SB)",
		},
		{
			name:     "three operands with rodata first",
			input:    "VPBLENDVB (RIP), X1, X2",
			addend:   16,
			expected: "VPBLENDVB ·rodata<>+16(SB), X1, X2",
		},
		{
			name:     "unrelated instruction untouched",
			input:    "ADDQ AX, BX",
			addend:   0,
			expected: "ADDQ AX, BX",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := replaceIPWithROData(tc.input, tc.addend)
			if got != tc.expected {
				t.Errorf("replaceIPWithROData(%q, %d) = %q, want %q", tc.input, tc.addend, got, tc.expected)
			}
		})
	}
}

func TestReplaceARM64ROData(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		addend   int64
		expected string
	}{
		{
			name:     "ADRP rodata with zero addend",
			input:    "ADRP 0, R0",
			addend:   0,
			expected: "MOVD $·rodata<>(SB), R0",
		},
		{
			name:     "LDR rodata with non-zero addend",
			input:    "LDR 0, R1",
			addend:   32,
			expected: "MOVD $·rodata<>+32(SB), R1",
		},
		{
			name:     "MOVD rodata",
			input:    "MOVD 0, R2",
			addend:   16,
			expected: "MOVD $·rodata<>+16(SB), R2",
		},
		{
			name:     "unrelated instruction",
			input:    "ADD R0, R1, R2",
			addend:   0,
			expected: "ADD R0, R1, R2",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := replaceARM64ROData(tc.input, tc.addend)
			if got != tc.expected {
				t.Errorf("replaceARM64ROData(%q, %d) = %q, want %q", tc.input, tc.addend, got, tc.expected)
			}
		})
	}
}

func TestCleanPlan9Syntax_CMOV(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		// 32-bit destination
		{"CMOVAE AX, EBX", "CMOVLCC AX, EBX"},
		{"CMOVB AX, EBX", "CMOVLCS AX, EBX"},
		{"CMOVE AX, EBX", "CMOVLEQ AX, EBX"},
		{"CMOVNE AX, EBX", "CMOVLNE AX, EBX"},
		{"CMOVG AX, EBX", "CMOVLGT AX, EBX"},
		{"CMOVGE AX, EBX", "CMOVLGE AX, EBX"},
		{"CMOVL AX, EBX", "CMOVLLT AX, EBX"},
		{"CMOVLE AX, EBX", "CMOVLLE AX, EBX"},
		{"CMOVNS AX, EBX", "CMOVLPL AX, EBX"},
		{"CMOVS AX, EBX", "CMOVLMI AX, EBX"},

		// 64-bit destination
		{"CMOVAE AX, R8", "CMOVQCC AX, R8"},
		{"CMOVB AX, R9", "CMOVQCS AX, R9"},
		{"CMOVE AX, R10", "CMOVQEQ AX, R10"},
		{"CMOVNE AX, R11", "CMOVQNE AX, R11"},
		{"CMOVG AX, R12", "CMOVQGT AX, R12"},
		{"CMOVGE AX, R13", "CMOVQGE AX, R13"},
		{"CMOVL AX, R14", "CMOVQLT AX, R14"},
		{"CMOVLE AX, R15", "CMOVQLE AX, R15"},
		{"CMOVNS AX, DI", "CMOVQPL AX, DI"},
		{"CMOVS AX, SI", "CMOVQMI AX, SI"},
	}

	for _, tc := range tests {
		got := cleanPlan9Syntax(tc.input)
		if got != tc.expected {
			t.Errorf("cleanPlan9Syntax(%q) = %q, want %q", tc.input, got, tc.expected)
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
		{"SETE R10", "SETEQ R10B"},
		{"SETE R11", "SETEQ R11B"},
		{"SETE R12", "SETEQ R12B"},
		{"SETE R13", "SETEQ R13B"},
		{"SETE R14", "SETEQ R14B"},
		{"SETE R15", "SETEQ R15B"},
	}

	for _, tc := range tests {
		got := cleanPlan9Syntax(tc.input)
		if got != tc.expected {
			t.Errorf("cleanPlan9Syntax(%q) = %q, want %q", tc.input, got, tc.expected)
		}
	}
}

func TestCleanPlan9Syntax_MnemonicAliases(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"MOVZX AX, BX", "MOVBLZX AX, BX"},
		{"MOVSX AX, BX", "MOVBLSX AX, BX"},
		{"MOVSXD AX, BX", "MOVLQSX AX, BX"},
		{"TZCNT AX, BX", "TZCNTL AX, BX"},
		{"BSF AX, BX", "BSFL AX, BX"},
		{"BSR AX, BX", "BSRL AX, BX"},
		{"LZCNT AX, BX", "LZCNTL AX, BX"},
		{"POPCNT AX, BX", "POPCNTL AX, BX"},
	}

	for _, tc := range tests {
		got := cleanPlan9Syntax(tc.input)
		if got != tc.expected {
			t.Errorf("cleanPlan9Syntax(%q) = %q, want %q", tc.input, got, tc.expected)
		}
	}
}

func TestCleanPlan9Syntax_ByteOpSubstitutions(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"TESTL AL, AL", "TESTB AL, AL"},
		{"CMPL BL, $10", "CMPB BL, $10"},
		{"MOVL CL, DL", "MOVB CL, DL"},
		{"ANDL SIL, $1", "ANDB SIL, $1"},
		{"ORL DIL, $2", "ORB DIL, $2"},
		{"XORL R8B, R8B", "XORB R8B, R8B"},
		{"ADDL R9B, $5", "ADDB R9B, $5"},
		{"SUBL R10B, $3", "SUBB R10B, $3"},
		{"SHRL $1, R11B", "SHRB $1, R11B"},
		{"SHLL $2, R12B", "SHLB $2, R12B"},
		{"SARL $1, R13B", "SARB $1, R13B"},
	}

	for _, tc := range tests {
		got := cleanPlan9Syntax(tc.input)
		if got != tc.expected {
			t.Errorf("cleanPlan9Syntax(%q) = %q, want %q", tc.input, got, tc.expected)
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
		{"ANDQ $-32, SP", "ANDQ $-32, SP"},
	}

	for _, tc := range tests {
		got := cleanPlan9Syntax(tc.input)
		if got != tc.expected {
			t.Errorf("cleanPlan9Syntax(%q) = %q, want %q", tc.input, got, tc.expected)
		}
	}
}

func TestCleanPlan9Syntax_GeneralCleanups(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"JE L_0014(SB)", "JE L_0014"},
		{"CS NOP", "NOP"},
		{"MOVQ %rax, %rbx", "MOVQ AX, BX"},
	}

	for _, tc := range tests {
		got := cleanPlan9Syntax(tc.input)
		if got != tc.expected {
			t.Errorf("cleanPlan9Syntax(%q) = %q, want %q", tc.input, got, tc.expected)
		}
	}
}

func TestHasRexW(t *testing.T) {
	tests := []struct {
		prefix   byte
		expected bool
	}{
		{0x40, false}, // REX without W
		{0x47, false}, // REX without W
		{0x48, true},  // REX.W
		{0x49, true},  // REX.W | B
		{0x4F, true},  // REX.W | R | X | B
		{0x00, false},
		{0xC5, false},
	}

	for _, tc := range tests {
		got := hasRexW(tc.prefix)
		if got != tc.expected {
			t.Errorf("hasRexW(0x%02x) = %v, want %v", tc.prefix, got, tc.expected)
		}
	}
}

func TestIsVZeroUpper(t *testing.T) {
	code := []byte{0x90, 0xC5, 0xF8, 0x77, 0x90}
	if !isVZeroUpper(code, 1) {
		t.Errorf("expected isVZeroUpper at offset 1 to be true")
	}
	if isVZeroUpper(code, 0) {
		t.Errorf("expected isVZeroUpper at offset 0 to be false")
	}
	if isVZeroUpper(code, 3) {
		t.Errorf("expected isVZeroUpper at offset 3 to be false (too short)")
	}
}

func TestDisassembleAMD64_BranchToZero(t *testing.T) {
	// JMP rel8 (-2): EB FE is an infinite jump back to self (offset 0)
	// EB FE -> pc=0, len=2, rel=-2 -> target = 0+2+(-2) = 0
	code := []byte{0xEB, 0xFE}
	res, err := DisassembleAMD64(code, 0, nil)
	if err != nil {
		t.Fatalf("DisassembleAMD64 failed: %v", err)
	}

	if len(res.Insts) == 0 {
		t.Fatalf("expected at least 1 instruction")
	}

	// Verify that target 0 received a clean label L_0000
	firstInst := res.Insts[0]
	if firstInst.Label != "L_0000" {
		t.Errorf("expected first instruction to have label L_0000, got %q", firstInst.Label)
	}
}

func TestDisassembleARM64_BranchToZero(t *testing.T) {
	// B offset: in AArch64, B 0 is opcode 0x14000000 (branch to self at pc=0)
	// Word: 0x00, 0x00, 0x00, 0x14 in LittleEndian
	code := []byte{0x00, 0x00, 0x00, 0x14}
	res, err := DisassembleARM64(code, 0, nil)
	if err != nil {
		t.Fatalf("DisassembleARM64 failed: %v", err)
	}

	if len(res.Insts) == 0 {
		t.Fatalf("expected at least 1 instruction")
	}

	firstInst := res.Insts[0]
	if firstInst.Label != "L_0000" {
		t.Errorf("expected first instruction to have label L_0000, got %q", firstInst.Label)
	}
}
