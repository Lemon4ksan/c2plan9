// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package arm64_test

import (
	"testing"

	"github.com/lemon4ksan/c2plan9/arch"
	"github.com/lemon4ksan/c2plan9/arch/arm64"
)

func TestDisassembleARM64_RET(t *testing.T) {
	backend, err := arch.GetArch("arm64")
	if err != nil {
		t.Fatalf("failed to get arm64 arch: %v", err)
	}

	code := []byte{0xc0, 0x03, 0x5f, 0xd6} // RET
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

func TestCleanPlan9ARM64Syntax(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{
			input:    "ADD R0, SP, R0",
			expected: "ADD R0, RSP, R0",
		},
		{
			input:    "SUB R1, R2, SP",
			expected: "SUB R1, R2, RSP",
		},
		{
			input:    "LDR R0, (SP)",
			expected: "LDR R0, (RSP)",
		},
		{
			input:    "RET",
			expected: "RET",
		},
		{
			input:    "B L_0014(SB)",
			expected: "B L_0014",
		},
		{
			input:    "LDURBW [X0, #1], W1",
			expected: "MOVBU [X0, #1], W1",
		},
		{
			input:    "STURBW W1, [X0, #1]",
			expected: "MOVB W1, [X0, #1]",
		},
		{
			input:    "VMAXV V0.4S, S1",
			expected: "UMAXV V0.4S, S1",
		},
		{
			input:    "LDUR [X0, #8], X1",
			expected: "MOVD [X0, #8], X1",
		},
		{
			input:    "LDUR [X0, #16], Q0",
			expected: "FMOVQ [X0, #16], Q0",
		},
		{
			input:    "ISB $15",
			expected: "ISB",
		},
	}

	for _, tc := range tests {
		got := arm64.CleanPlan9Syntax(tc.input)
		if got != tc.expected {
			t.Errorf("CleanPlan9Syntax(%q) = %q, want %q", tc.input, got, tc.expected)
		}
	}
}

func TestReplaceROData(t *testing.T) {
	tests := []struct {
		input    string
		addend   int64
		expected string
	}{
		{
			input:    "ADRP 0, X0",
			addend:   16,
			expected: "MOVD $·rodata<>+16(SB), X0",
		},
		{
			input:    "LDR [X0], X1",
			addend:   0,
			expected: "MOVD $·rodata<>(SB), X1",
		},
	}

	for _, tc := range tests {
		got := arm64.ReplaceROData(tc.input, tc.addend)
		if got != tc.expected {
			t.Errorf("ReplaceROData(%q, %d) = %q, want %q", tc.input, tc.addend, got, tc.expected)
		}
	}
}
