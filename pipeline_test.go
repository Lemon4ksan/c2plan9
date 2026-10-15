// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"strings"
	"testing"
)

func TestPipelineNormalization(t *testing.T) {
	p, err := NewPipeline("amd64")
	if err != nil {
		t.Fatalf("failed to create pipeline: %v", err)
	}

	t.Run("Missing Source File", func(t *testing.T) {
		cfg := PipelineConfig{}
		err := p.NormalizeConfig(&cfg)
		if err == nil {
			t.Fatal("expected error for empty source file, got nil")
		}
	})

	t.Run("Default AMD64 Output and Package", func(t *testing.T) {
		cfg := PipelineConfig{
			SourceFile: "kernels/sha256.c",
		}
		if err := p.NormalizeConfig(&cfg); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.TargetArch != "amd64" {
			t.Errorf("expected amd64 target, got %s", cfg.TargetArch)
		}
		if !strings.HasSuffix(cfg.OutputFile, "sha256_amd64.s") {
			t.Errorf("expected sha256_amd64.s output file, got %s", cfg.OutputFile)
		}
		if cfg.PkgName != "kernels" {
			t.Errorf("expected kernels package, got %s", cfg.PkgName)
		}
	})

	t.Run("Deduce ARM64 From Source Name", func(t *testing.T) {
		cfg := PipelineConfig{
			SourceFile: "kernels/crypto_arm64.c",
		}
		if err := p.NormalizeConfig(&cfg); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.TargetArch != "arm64" {
			t.Errorf("expected arm64 target, got %s", cfg.TargetArch)
		}
		if !strings.HasSuffix(cfg.OutputFile, "crypto_arm64_arm64.s") {
			t.Errorf("expected arm64 suffix, got %s", cfg.OutputFile)
		}
	})
}

func TestPipelineTranslateObjectAMD64(t *testing.T) {
	p, err := NewPipeline("amd64")
	if err != nil {
		t.Fatalf("failed to create pipeline: %v", err)
	}

	obj := &ParsedObject{
		Arch: "amd64",
		Symbols: []Symbol{
			{
				Name: "kernel_dot_product",
				Code: []byte{
					0xc3, // RET
				},
			},
		},
	}

	cfg := PipelineConfig{
		SourceFile: "kernel.c",
		OutputFile: "kernel_amd64.s",
		StubFile:   "kernel_amd64.go",
		PkgName:    "mathkernel",
		TargetArch: "amd64",
		RetType:    "float64",
	}

	res, err := p.TranslateObject(obj, cfg)
	if err != nil {
		t.Fatalf("TranslateObject failed: %v", err)
	}

	asm := string(res.AssemblyBytes)
	if !strings.Contains(asm, "//go:build amd64 && !purego") {
		t.Errorf("assembly missing amd64 build tag:\n%s", asm)
	}
	if !strings.Contains(asm, "TEXT ·kernel_dot_product(SB)") {
		t.Errorf("assembly missing TEXT declaration:\n%s", asm)
	}
	if !strings.Contains(asm, "MOVSD X0, ret+48(FP)") {
		t.Errorf("assembly missing float64 MOVSD return placement:\n%s", asm)
	}

	stub := string(res.StubBytes)
	if !strings.Contains(stub, "func kernel_dot_product") || !strings.Contains(stub, "float64") {
		t.Errorf("stub missing kernel_dot_product signature:\n%s", stub)
	}
}

func TestPipelineTranslateObjectARM64(t *testing.T) {
	p, err := NewPipeline("arm64")
	if err != nil {
		t.Fatalf("failed to create pipeline: %v", err)
	}

	obj := &ParsedObject{
		Arch: "arm64",
		Symbols: []Symbol{
			{
				Name: "neon_matrix_mult",
				Code: []byte{
					0xc0, 0x03, 0x5f, 0xd6, // RET
				},
			},
		},
	}

	cfg := PipelineConfig{
		SourceFile: "neon.c",
		OutputFile: "neon_arm64.s",
		StubFile:   "neon_arm64.go",
		PkgName:    "neonmath",
		TargetArch: "arm64",
		RetType:    "m128",
	}

	res, err := p.TranslateObject(obj, cfg)
	if err != nil {
		t.Fatalf("TranslateObject failed: %v", err)
	}

	asm := string(res.AssemblyBytes)
	if !strings.Contains(asm, "//go:build arm64 && !purego") {
		t.Errorf("assembly missing arm64 build tag:\n%s", asm)
	}
	if !strings.Contains(asm, "TEXT ·neon_matrix_mult(SB)") {
		t.Errorf("assembly missing TEXT declaration:\n%s", asm)
	}
	if !strings.Contains(asm, "MOVD arg0+0(FP), R0") {
		t.Errorf("assembly missing MOVD param loading:\n%s", asm)
	}
	if !strings.Contains(asm, "FMOVQ F0, ret+64(FP)") {
		t.Errorf("assembly missing FMOVQ return placement:\n%s", asm)
	}

	stub := string(res.StubBytes)
	if !strings.Contains(stub, "func neon_matrix_mult") || !strings.Contains(stub, "[16]byte") {
		t.Errorf("stub missing neon_matrix_mult signature:\n%s", stub)
	}
}
