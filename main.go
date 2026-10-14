// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "[c2plan9] Error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	var (
		cFile      = flag.String("c", "", "Path to input C / C++ / LLVM IR source file")
		oFile      = flag.String("o", "", "Path to output Plan 9 assembly (.s) file")
		stubFile   = flag.String("stub", "", "Path to output Go companion stub (.go) file (optional)")
		pkgName    = flag.String("pkg", "", "Target Go package name (defaults to output dir name)")
		clangPath  = flag.String("clang", "clang", "Path to Clang executable")
		extraFlags = flag.String("flags", "", "Extra flags to pass to Clang (comma or space separated)")
		targetArch = flag.String("arch", "", "Target architecture: amd64 (default) or arm64")
		useWSL     = flag.Bool("wsl", false, "Compile via Clang inside WSL")
		retType    = flag.String("ret", "", "Return type: float32, float64, m128, m256, int, int32, or void")
	)

	flag.Parse()

	if *cFile == "" {
		flag.Usage()
		return errors.New("-c source file flag is required")
	}

	if *targetArch == "" {
		if strings.Contains(*oFile, "_arm64") || strings.Contains(*cFile, "_arm64") {
			*targetArch = "arm64"
		} else {
			*targetArch = "amd64"
		}
	}

	if *oFile == "" {
		base := strings.TrimSuffix(filepath.Base(*cFile), filepath.Ext(*cFile))
		*oFile = filepath.Join(filepath.Dir(*cFile), base+"_"+*targetArch+".s")
	}

	if *pkgName == "" {
		*pkgName = filepath.Base(filepath.Dir(*oFile))
		if *pkgName == "." || *pkgName == "/" || *pkgName == "\\" {
			*pkgName = "main"
		}
	}

	var flags []string
	if *extraFlags != "" {
		flags = strings.Fields(*extraFlags)
	}

	fmt.Printf("[c2plan9] Compiling %s for %s via Clang/LLVM (WSL=%v)...\n", *cFile, *targetArch, *useWSL)
	objPath, err := Compile(CompileOptions{
		ClangPath:  *clangPath,
		SourceFile: *cFile,
		TargetArch: *targetArch,
		ExtraFlags: flags,
		TargetSysV: true,
		UseWSL:     *useWSL,
	})
	if err != nil {
		return fmt.Errorf("compilation failed: %w", err)
	}
	defer os.Remove(objPath)

	fmt.Printf("[c2plan9] Parsing object file %s...\n", objPath)
	obj, err := ParseObject(objPath)
	if err != nil {
		return fmt.Errorf("object parsing failed: %w", err)
	}

	paramCount := 6
	if *targetArch == "arm64" {
		paramCount = 8
	}

	sigsMap := make(map[string]FuncSignature)
	if *retType != "" {
		rt := ReturnType(*retType)
		hasRet := rt.Kind() != RetNone
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

	fmt.Printf("[c2plan9] Disassembling %d symbol(s) into Plan 9 %s assembly...\n", len(obj.Symbols), *targetArch)
	asmBytes, err := EmitPlan9Assembly(*pkgName, obj.Symbols, sigsMap, obj.ROData, obj.Relocations, *targetArch)
	if err != nil {
		return fmt.Errorf("assembly generation failed: %w", err)
	}

	if err := os.WriteFile(*oFile, asmBytes, 0o644); err != nil {
		return fmt.Errorf("failed to write %s: %w", *oFile, err)
	}
	fmt.Printf("[c2plan9] Successfully emitted %s (%d bytes)\n", *oFile, len(asmBytes))

	if *stubFile != "" {
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
		stubBytes := EmitGoStub(*pkgName, sigs, *targetArch)
		if err := os.WriteFile(*stubFile, stubBytes, 0o644); err != nil {
			return fmt.Errorf("failed to write stub %s: %w", *stubFile, err)
		}
		fmt.Printf("[c2plan9] Successfully emitted Go stub %s\n", *stubFile)
	}

	return nil
}
