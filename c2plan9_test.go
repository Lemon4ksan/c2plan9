// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/lemon4ksan/c2plan9/arch"
)

const sampleCKernel = `
#include <stdint.h>
#include <stddef.h>

uint64_t fast_add_accumulate(const uint64_t* a, const uint64_t* b, size_t n) {
    uint64_t sum = 0;
    for (size_t i = 0; i < n; i++) {
        sum += a[i] ^ b[i];
    }
    return sum;
}
`

func TestC2Plan9EndToEnd(t *testing.T) {
	tmpDir := t.TempDir()
	cFile := filepath.Join(tmpDir, "kernel.c")
	if err := os.WriteFile(cFile, []byte(sampleCKernel), 0o644); err != nil {
		t.Fatalf("failed to write C kernel: %v", err)
	}

	objPath, err := Compile(t.Context(), CompileOptions{
		ClangPath:  "clang",
		SourceFile: cFile,
		TargetArch: "amd64",
		TargetSysV: true,
	})
	if err != nil {
		t.Skipf("clang not available or failed: %v", err)
	}
	defer os.Remove(objPath)

	obj, err := ParseObject(objPath)
	if err != nil {
		t.Fatalf("failed to parse object: %v", err)
	}

	if len(obj.Symbols) == 0 {
		t.Fatalf("expected symbols in object file, got 0")
	}

	found := false
	for _, sym := range obj.Symbols {
		if sym.Name == "fast_add_accumulate" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("symbol fast_add_accumulate not found in parsed symbols")
	}

	sigs := map[string]FuncSignature{
		"fast_add_accumulate": {
			Name:       "fast_add_accumulate",
			GoName:     "FastAddAccumulate",
			ParamCount: 3,
			HasReturn:  true,
		},
	}

	asmBytes, err := EmitPlan9Assembly("testkernel", obj.Symbols, sigs, obj.ROData, obj.Relocations, "amd64")
	if err != nil {
		t.Fatalf("failed to emit Plan 9 assembly: %v", err)
	}

	asmStr := string(asmBytes)
	if !strings.Contains(asmStr, "TEXT ·FastAddAccumulate(SB), NOSPLIT") {
		t.Errorf("assembly missing TEXT declaration, got:\n%s", asmStr)
	}

	if !strings.Contains(asmStr, "RET") {
		t.Errorf("assembly missing RET instruction, got:\n%s", asmStr)
	}

	t.Logf("Generated Plan 9 Assembly:\n%s", asmStr)

	// Subtest: Verify that go test can build and run the generated Plan 9 assembly
	sFile := filepath.Join(tmpDir, "kernel_amd64.s")
	if err := os.WriteFile(sFile, asmBytes, 0o644); err != nil {
		t.Fatalf("failed to write assembly file: %v", err)
	}

	stubCode := `//go:build amd64 && !purego

package testkernel

import "unsafe"

//go:noescape
func FastAddAccumulate(a, b unsafe.Pointer, n uint64) uint64
`
	if err := os.WriteFile(filepath.Join(tmpDir, "kernel_amd64.go"), []byte(stubCode), 0o644); err != nil {
		t.Fatalf("failed to write stub: %v", err)
	}

	testCode := `//go:build amd64 && !purego

package testkernel

import (
	"testing"
	"unsafe"
)

func TestKernelExecution(t *testing.T) {
	a := []uint64{1, 2, 3, 4, 5, 6, 7, 8}
	b := []uint64{10, 20, 30, 40, 50, 60, 70, 80}

	var expected uint64
	for i := range a {
		expected += a[i] ^ b[i]
	}

	res := FastAddAccumulate(unsafe.Pointer(&a[0]), unsafe.Pointer(&b[0]), uint64(len(a)))
	if res != expected {
		t.Fatalf("expected %d, got %d", expected, res)
	}
}
`
	if err := os.WriteFile(filepath.Join(tmpDir, "kernel_test.go"), []byte(testCode), 0o644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	goMod := `module testkernel

go 1.25
`
	if err := os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte(goMod), 0o644); err != nil {
		t.Fatalf("failed to write go.mod: %v", err)
	}

	_ = os.Remove(cFile)

	var cmd *exec.Cmd
	if runtime.GOARCH == "amd64" {
		cmd = exec.Command("go", "test", "-v", ".")
	} else {
		cmd = exec.Command("go", "build", "-o", os.DevNull, ".")
		cmd.Env = append(os.Environ(), "GOARCH=amd64")
	}
	cmd.Dir = tmpDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go test failed on generated Plan 9 assembly (%v):\n%s", err, string(out))
	}
	t.Logf("go test on generated kernel passed:\n%s", string(out))
}

type mockCustomArch struct{}

func (m *mockCustomArch) Name() string             { return "custom64" }
func (m *mockCustomArch) ParamRegisters() []string { return []string{"A0", "A1"} }
func (m *mockCustomArch) LoadOp() string           { return "LD" }
func (m *mockCustomArch) DefaultParamCount() int   { return 2 }
func (m *mockCustomArch) BuildTag() string         { return "//go:build custom64 && !purego" }
func (m *mockCustomArch) ClangTarget() string      { return "custom64-linux-gnu" }
func (m *mockCustomArch) ReturnPlacement(arch.ReturnKind) arch.ReturnPlacement {
	return arch.ReturnPlacement{Op: "SD", Reg: "A0"}
}

func (m *mockCustomArch) Disassemble([]byte, uint64, []arch.Relocation) (arch.DisassemblyResult, error) {
	return arch.DisassemblyResult{
		Insts: []arch.DisassembledInst{
			{Offset: 0, Length: 4, Text: "RET", IsRet: true},
		},
		FrameSize: 0,
	}, nil
}

func TestCustomArchRegistration(t *testing.T) {
	mock := &mockCustomArch{}
	arch.Register(mock)

	backend, err := arch.Get("custom64")
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
