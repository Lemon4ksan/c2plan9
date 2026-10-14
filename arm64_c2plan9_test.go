// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const sampleARM64NEONKernel = `
#include <stdint.h>
#include <stddef.h>

#if defined(__ARM_NEON) || defined(__aarch64__)
#include <arm_neon.h>

void fast_neon_xor_block(uint8_t* dst, const uint8_t* src1, const uint8_t* src2, size_t n) {
    size_t i = 0;
    for (; i + 16 <= n; i += 16) {
        uint8x16_t v1 = vld1q_u8(src1 + i);
        uint8x16_t v2 = vld1q_u8(src2 + i);
        uint8x16_t vout = veorq_u8(v1, v2);
        vst1q_u8(dst + i, vout);
    }
    for (; i < n; i++) {
        dst[i] = src1[i] ^ src2[i];
    }
}
#endif
`

func TestC2Plan9ARM64EndToEnd(t *testing.T) {
	tmpDir := t.TempDir()
	cFile := filepath.Join(tmpDir, "neon_kernel.c")
	if err := os.WriteFile(cFile, []byte(sampleARM64NEONKernel), 0o644); err != nil {
		t.Fatalf("failed to write C kernel: %v", err)
	}

	objPath, err := Compile(CompileOptions{
		ClangPath:  "clang",
		SourceFile: cFile,
		TargetArch: "arm64",
	})
	if err != nil {
		t.Skipf("clang aarch64 target not available or failed: %v", err)
	}
	defer os.Remove(objPath)

	obj, err := ParseObject(objPath)
	if err != nil {
		t.Fatalf("failed to parse arm64 object: %v", err)
	}

	if obj.Arch != "arm64" {
		t.Errorf("expected object arch arm64, got %s", obj.Arch)
	}

	if len(obj.Symbols) == 0 {
		t.Fatalf("expected symbols in arm64 object file, got 0")
	}

	sigs := map[string]FuncSignature{
		"fast_neon_xor_block": {
			Name:       "fast_neon_xor_block",
			GoName:     "FastNeonXorBlock",
			ParamCount: 4,
			HasReturn:  false,
		},
	}

	asmBytes, err := EmitPlan9Assembly("neon", obj.Symbols, sigs, obj.ROData, obj.Relocations, "arm64")
	if err != nil {
		t.Fatalf("failed to emit Plan 9 ARM64 assembly: %v", err)
	}

	asmStr := string(asmBytes)
	t.Logf("Generated Plan 9 ARM64 Assembly:\n%s", asmStr)

	// Verify ARM64 Plan 9 directives
	if !strings.Contains(asmStr, "//go:build arm64 && !purego") {
		t.Errorf("assembly missing arm64 build tag, got:\n%s", asmStr)
	}

	if !strings.Contains(asmStr, "TEXT ·FastNeonXorBlock(SB)") {
		t.Errorf("assembly missing TEXT declaration, got:\n%s", asmStr)
	}

	if !strings.Contains(asmStr, "MOVD arg0+0(FP), R0") {
		t.Errorf("assembly missing ARM64 register loading for R0, got:\n%s", asmStr)
	}

	if !strings.Contains(asmStr, "MOVD arg1+8(FP), R1") {
		t.Errorf("assembly missing ARM64 register loading for R1, got:\n%s", asmStr)
	}

	if !strings.Contains(asmStr, "RET") {
		t.Errorf("assembly missing RET instruction, got:\n%s", asmStr)
	}
}

func TestCleanPlan9ARM64Syntax(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"LDURBW -1(R11), R13", "MOVBU -1(R11), R13"},
		{"LDURB -1(R11), R13", "MOVBU -1(R11), R13"},
		{"LDURSBW -1(R11), R13", "MOVB -1(R11), R13"},
		{"LDURSB -1(R11), R13", "MOVB -1(R11), R13"},
		{"LDURHW -2(R11), R13", "MOVHU -2(R11), R13"},
		{"LDURH -2(R11), R13", "MOVHU -2(R11), R13"},
		{"LDURSHW -2(R11), R13", "MOVH -2(R11), R13"},
		{"LDURSH -2(R11), R13", "MOVH -2(R11), R13"},
		{"LDURW -4(R11), R13", "MOVW -4(R11), R13"},
		{"LDURSW -4(R11), R13", "MOVW -4(R11), R13"},
		{"LDUR -8(R11), R13", "MOVD -8(R11), R13"},
		{"LDUR -16(R11), F0", "FMOVQ -16(R11), F0"},
		{"STURBW R12, -1(R10)", "MOVB R12, -1(R10)"},
		{"STURB R12, -1(R10)", "MOVB R12, -1(R10)"},
		{"STURHW R12, -2(R10)", "MOVH R12, -2(R10)"},
		{"STURH R12, -2(R10)", "MOVH R12, -2(R10)"},
		{"STURW R12, -4(R10)", "MOVW R12, -4(R10)"},
		{"STUR R12, -8(R10)", "MOVD R12, -8(R10)"},
		{"STUR F0, -16(R10)", "FMOVQ F0, -16(R10)"},
		{"VMAXV V0.B16, B1", "UMAXV V0.B16, B1"},
		{"VMINV V0.B16, B1", "UMINV V0.B16, B1"},
		{"VADDV V0.B16, B1", "ADDV V0.B16, B1"},
		{"ISB $15", "ISB"},
		{"B L_0014(SB)", "B L_0014"},
		{"ADD $16, SP, SP", "ADD $16, RSP, RSP"},
	}

	for _, tc := range tests {
		got := cleanPlan9ARM64Syntax(tc.input)
		if got != tc.expected {
			t.Errorf("cleanPlan9ARM64Syntax(%q) = %q, want %q", tc.input, got, tc.expected)
		}
	}
}
