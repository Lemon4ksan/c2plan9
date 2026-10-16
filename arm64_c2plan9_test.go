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

	objPath, err := Compile(t.Context(), CompileOptions{
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
