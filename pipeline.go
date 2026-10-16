// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/lemon4ksan/c2plan9/arch"
	_ "github.com/lemon4ksan/c2plan9/arch/amd64"
	_ "github.com/lemon4ksan/c2plan9/arch/arm64"
)

// PipelineConfig configures the execution of the c2plan9 translation pipeline.
type PipelineConfig struct {
	SourceFile string
	OutputFile string
	StubFile   string
	PkgName    string
	ClangPath  string
	ExtraFlags []string
	TargetArch string
	UseWSL     bool
	RetType    string
}

// PipelineResult encapsulates the generated artifacts and metadata from a pipeline run.
type PipelineResult struct {
	Arch          string
	OutputFile    string
	AssemblyBytes []byte
	StubFile      string
	StubBytes     []byte
	Symbols       []Symbol
}

// Pipeline coordinates the stages of compiling, parsing, disassembling,
// and emitting Plan 9 assembly and companion Go stubs.
type Pipeline struct {
	backend arch.Backend
}

// NewPipeline creates a new Pipeline for the specified architecture target.
func NewPipeline(targetArch string) (*Pipeline, error) {
	backend, err := arch.Get(targetArch)
	if err != nil {
		return nil, err
	}
	return &Pipeline{backend: backend}, nil
}

// Backend returns the underlying ArchBackend used by the pipeline.
func (p *Pipeline) Backend() arch.Backend {
	return p.backend
}

// NormalizeConfig resolves defaults for unspecified configuration fields.
func (p *Pipeline) NormalizeConfig(cfg *PipelineConfig) error {
	if cfg.SourceFile == "" {
		return errors.New("source file (-c) is required")
	}

	if cfg.TargetArch == "" {
		if strings.Contains(cfg.OutputFile, "_arm64") || strings.Contains(cfg.SourceFile, "_arm64") {
			cfg.TargetArch = "arm64"
		} else {
			cfg.TargetArch = "amd64"
		}
	}

	if cfg.OutputFile == "" {
		base := strings.TrimSuffix(filepath.Base(cfg.SourceFile), filepath.Ext(cfg.SourceFile))
		cfg.OutputFile = filepath.Join(filepath.Dir(cfg.SourceFile), base+"_"+cfg.TargetArch+".s")
	}

	if cfg.PkgName == "" {
		cfg.PkgName = filepath.Base(filepath.Dir(cfg.OutputFile))
		if cfg.PkgName == "." || cfg.PkgName == "/" || cfg.PkgName == "\\" {
			cfg.PkgName = "main"
		}
	}

	return nil
}

// Execute executes the complete translation pipeline: compile, parse, emit assembly, and emit stub.
func (p *Pipeline) Execute(ctx context.Context, cfg PipelineConfig) (*PipelineResult, error) {
	if err := p.NormalizeConfig(&cfg); err != nil {
		return nil, err
	}

	if cfg.TargetArch != p.backend.Name() {
		backend, err := arch.Get(cfg.TargetArch)
		if err != nil {
			return nil, err
		}
		p.backend = backend
	}

	// Step 1: Compile source to object file
	objPath, err := Compile(ctx, CompileOptions{
		ClangPath:  cfg.ClangPath,
		SourceFile: cfg.SourceFile,
		TargetArch: cfg.TargetArch,
		ExtraFlags: cfg.ExtraFlags,
		TargetSysV: true,
		UseWSL:     cfg.UseWSL,
	})
	if err != nil {
		return nil, fmt.Errorf("compilation failed: %w", err)
	}
	defer os.Remove(objPath)

	// Step 2: Parse object file
	obj, err := ParseObject(objPath)
	if err != nil {
		return nil, fmt.Errorf("object parsing failed: %w", err)
	}

	// Step 3: Transform parsed object into assembly and stubs
	res, err := p.TranslateObject(obj, cfg)
	if err != nil {
		return nil, err
	}

	// Step 4: Write output files
	if err := os.WriteFile(cfg.OutputFile, res.AssemblyBytes, 0o644); err != nil {
		return nil, fmt.Errorf("failed to write %s: %w", cfg.OutputFile, err)
	}

	if cfg.StubFile != "" && len(res.StubBytes) > 0 {
		if err := os.WriteFile(cfg.StubFile, res.StubBytes, 0o644); err != nil {
			return nil, fmt.Errorf("failed to write stub %s: %w", cfg.StubFile, err)
		}
	}

	return res, nil
}

// TranslateObject processes an already parsed object file without invoking the compiler.
// This allows testing and modular usage of the pipeline with in-memory or fixture objects.
func (p *Pipeline) TranslateObject(obj *ParsedObject, cfg PipelineConfig) (*PipelineResult, error) {
	paramCount := p.backend.DefaultParamCount()

	sigsMap := make(map[string]FuncSignature)
	if cfg.RetType != "" {
		rt := ReturnType(cfg.RetType)
		hasRet := rt.Kind() != arch.RetNone
		for _, sym := range obj.Symbols {
			if !sym.IsLocal {
				sigsMap[sym.Name] = FuncSignature{
					Name:       sym.Name,
					ParamCount: paramCount,
					HasReturn:  hasRet,
					ReturnType: rt,
				}
			}
		}
	}

	asmBytes, err := EmitPlan9Assembly(cfg.PkgName, obj.Symbols, sigsMap, obj.ROData, obj.Relocations, p.backend.Name())
	if err != nil {
		return nil, fmt.Errorf("assembly generation failed: %w", err)
	}

	var stubBytes []byte
	if cfg.StubFile != "" {
		var sigs []FuncSignature
		for _, sym := range obj.Symbols {
			if !sym.IsLocal {
				if sig, ok := sigsMap[sym.Name]; ok {
					sigs = append(sigs, sig)
				} else {
					sigs = append(sigs, FuncSignature{
						Name:       sym.Name,
						ParamCount: paramCount,
						HasReturn:  true,
					})
				}
			}
		}
		stubBytes = EmitGoStub(cfg.PkgName, sigs, p.backend.Name())
	}

	return &PipelineResult{
		Arch:          p.backend.Name(),
		OutputFile:    cfg.OutputFile,
		AssemblyBytes: asmBytes,
		StubFile:      cfg.StubFile,
		StubBytes:     stubBytes,
		Symbols:       obj.Symbols,
	}, nil
}
