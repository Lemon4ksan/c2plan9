// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// CompileOptions represents compilation parameters for Clang.
type CompileOptions struct {
	ClangPath  string
	SourceFile string
	OutputFile string
	TargetArch string // amd64, arm64
	TargetOS   string // linux, windows, darwin (defaults to sysv-compatible)
	ExtraFlags []string
	TargetSysV bool // Force System V ABI (standard for Go AMD64 ASM)
	UseWSL     bool // Compile via WSL clang
}

// Compile compiles a C/LLVM IR source file into a temporary object file using Clang.
func Compile(opts CompileOptions) (string, error) {
	clang := opts.ClangPath
	if clang == "" {
		clang = "clang"
	}

	out := opts.OutputFile
	if out == "" {
		tmpDir := os.TempDir()
		base := strings.TrimSuffix(filepath.Base(opts.SourceFile), filepath.Ext(opts.SourceFile))
		out = filepath.Join(tmpDir, fmt.Sprintf("%s_%s_%d.o", base, opts.TargetArch, os.Getpid()))
	}

	useWSL := opts.UseWSL
	if !useWSL && runtime.GOOS == "windows" && opts.TargetArch == "arm64" {
		// Native Windows clang often lacks aarch64 target headers, check if WSL is requested
		if opts.UseWSL {
			useWSL = true
		}
	}

	if useWSL {
		wslSrc := toWSLPath(opts.SourceFile)
		wslOut := toWSLPath(out)
		target := ClangTarget(opts.TargetArch)
		wslArgs := make([]string, 0, 18+len(opts.ExtraFlags))
		wslArgs = append(wslArgs,
			"clang", "-c", wslSrc, "-o", wslOut,
			"-O3",
			"-ffreestanding",
			"-fno-asynchronous-unwind-tables",
			"-fno-exceptions",
			"-fno-rtti",
			"-fno-stack-protector",
			"-fomit-frame-pointer",
			"-fno-jump-tables",
			"-target", target,
		)
		if opts.TargetArch == "arm64" {
			wslArgs = append(wslArgs, "-march=armv8-a+simd+crypto")
		} else {
			wslArgs = append(wslArgs,
				"-mno-red-zone",
				"-mavx2",
				"-mpclmul",
				"-mfma",
			)
		}
		wslArgs = append(wslArgs, opts.ExtraFlags...)
		cmd := exec.CommandContext(context.Background(), "wsl", wslArgs...)
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			return "", fmt.Errorf("wsl clang compilation failed (%w): %s", err, stderr.String())
		}
		return out, nil
	}

	args := BuildClangArgs(opts, out)

	cmd := exec.CommandContext(context.Background(), clang, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("clang compilation failed (%w): %s\ncommand: %s %s",
			err, stderr.String(), clang, strings.Join(args, " "))
	}

	return out, nil
}

func toWSLPath(winPath string) string {
	cleaned := filepath.Clean(winPath)
	if len(cleaned) >= 2 && cleaned[1] == ':' {
		drive := strings.ToLower(string(cleaned[0]))
		rest := filepath.ToSlash(cleaned[2:])
		return "/mnt/" + drive + rest
	}
	return filepath.ToSlash(cleaned)
}

// ClangTarget returns the standard ELF target triple for the specified architecture.
func ClangTarget(arch string) string {
	switch arch {
	case "arm64":
		return "aarch64-linux-gnu"
	default:
		return "x86_64-pc-linux-gnu"
	}
}

// BuildClangArgs constructs command line arguments for Clang given options,
// enforcing an ELF target on all platforms for consistent relocation generation.
func BuildClangArgs(opts CompileOptions, out string) []string {
	target := ClangTarget(opts.TargetArch)
	args := []string{
		"-c",
		opts.SourceFile,
		"-o", out,
		"-O3",
		"-ffreestanding",
		"-fno-asynchronous-unwind-tables",
		"-fno-exceptions",
		"-fno-rtti",
		"-fno-stack-protector",
		"-fomit-frame-pointer",
		"-fno-jump-tables",
		"-target", target,
	}

	switch opts.TargetArch {
	case "", "amd64":
		args = append(args,
			"-mno-red-zone",
			"-mavx2",
			"-mpclmul",
			"-mfma",
		)
	case "arm64":
		args = append(args,
			"-march=armv8-a+simd+crypto",
		)
	}

	args = append(args, opts.ExtraFlags...)
	return args
}
