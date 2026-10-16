// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package arch_test

import (
	"slices"
	"testing"

	"github.com/lemon4ksan/c2plan9/arch"
)

type dummyArch struct {
	name string
}

func (d *dummyArch) Name() string             { return d.name }
func (d *dummyArch) ParamRegisters() []string { return []string{"R0"} }
func (d *dummyArch) LoadOp() string           { return "LOAD" }
func (d *dummyArch) ReturnPlacement(arch.ReturnKind) arch.ReturnPlacement {
	return arch.ReturnPlacement{Op: "STORE", Reg: "R0"}
}

func (d *dummyArch) Disassemble([]byte, uint64, []arch.Relocation) (arch.DisassemblyResult, error) {
	return arch.DisassemblyResult{}, nil
}
func (d *dummyArch) DefaultParamCount() int { return 1 }
func (d *dummyArch) BuildTag() string       { return "//go:build test" }
func (d *dummyArch) ClangTarget() string    { return "dummy-target" }

func TestRegistry(t *testing.T) {
	dummy := &dummyArch{name: "testarch"}
	arch.RegisterArch(dummy)

	got, err := arch.GetArch("testarch")
	if err != nil {
		t.Fatalf("expected to get testarch, got err: %v", err)
	}
	if got.Name() != "testarch" {
		t.Errorf("got name %q, want %q", got.Name(), "testarch")
	}

	all := arch.RegisteredArchs()
	if !slices.Contains(all, "testarch") {
		t.Errorf("RegisteredArchs() = %v, want it to contain testarch", all)
	}

	_, err = arch.GetArch("nonexistent_arch_xyz")
	if err == nil {
		t.Errorf("expected error for nonexistent arch, got nil")
	}
}

func TestNilRegisterPanic(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("expected RegisterArch(nil) to panic, did not panic")
		}
	}()
	arch.RegisterArch(nil)
}

func TestDisassemblerFunc(t *testing.T) {
	called := false
	fn := arch.DisassemblerFunc(
		func(code []byte, baseOffset uint64, relocs []arch.Relocation) (arch.DisassemblyResult, error) {
			called = true
			return arch.DisassemblyResult{
				Insts: []arch.DisassembledInst{
					{Offset: baseOffset, Length: len(code), Text: "NOP"},
				},
				FrameSize: 16,
			}, nil
		},
	)

	res, err := fn.Disassemble([]byte{0x90}, 0x100, nil)
	if err != nil {
		t.Fatalf("Disassemble failed: %v", err)
	}
	if !called {
		t.Errorf("expected DisassemblerFunc to be called")
	}
	if len(res.Insts) != 1 || res.Insts[0].Text != "NOP" {
		t.Errorf("unexpected disassembly result: %+v", res)
	}
}
