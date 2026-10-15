// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
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

	var flags []string
	if *extraFlags != "" {
		flags = strings.Fields(*extraFlags)
	}

	archName := *targetArch
	if archName == "" {
		if strings.Contains(*oFile, "_arm64") || strings.Contains(*cFile, "_arm64") {
			archName = "arm64"
		} else {
			archName = "amd64"
		}
	}

	pipeline, err := NewPipeline(archName)
	if err != nil {
		return err
	}

	cfg := PipelineConfig{
		SourceFile: *cFile,
		OutputFile: *oFile,
		StubFile:   *stubFile,
		PkgName:    *pkgName,
		ClangPath:  *clangPath,
		ExtraFlags: flags,
		TargetArch: archName,
		UseWSL:     *useWSL,
		RetType:    *retType,
	}

	fmt.Printf("[c2plan9] Compiling %s for %s via Clang/LLVM (WSL=%v)...\n", *cFile, archName, *useWSL)
	res, err := pipeline.Execute(cfg)
	if err != nil {
		return err
	}

	fmt.Printf("[c2plan9] Successfully emitted %s (%d bytes)\n", res.OutputFile, len(res.AssemblyBytes))
	if res.StubFile != "" {
		fmt.Printf("[c2plan9] Successfully emitted Go stub %s\n", res.StubFile)
	}

	return nil
}
