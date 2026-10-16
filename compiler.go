// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"context"
	"errors"
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
func Compile(ctx context.Context, opts CompileOptions) (string, error) {
	if strings.TrimSpace(opts.SourceFile) == "" {
		return "", errors.New("source file path cannot be empty")
	}

	stat, err := os.Stat(opts.SourceFile)
	if err != nil {
		return "", fmt.Errorf("source file %q not found or inaccessible: %w", opts.SourceFile, err)
	}
	if stat.IsDir() {
		return "", fmt.Errorf("source file %q is a directory, not a source file", opts.SourceFile)
	}

	arch := opts.TargetArch
	if arch == "" {
		arch = "amd64"
	}
	if arch != "amd64" && arch != "arm64" {
		return "", fmt.Errorf("unsupported target architecture: %q (supported: amd64, arm64)", opts.TargetArch)
	}
	opts.TargetArch = arch

	sanitizedFlags := make([]string, 0, len(opts.ExtraFlags))
	for _, flag := range opts.ExtraFlags {
		if strings.ContainsRune(flag, 0) {
			return "", fmt.Errorf("invalid compiler flag containing null byte: %q", flag)
		}
		sanitizedFlags = append(sanitizedFlags, flag)
	}
	opts.ExtraFlags = sanitizedFlags

	out := opts.OutputFile
	if out == "" {
		tmpDir := os.TempDir()
		base := strings.TrimSuffix(filepath.Base(opts.SourceFile), filepath.Ext(opts.SourceFile))
		base = filepath.Base(base)
		out = filepath.Join(tmpDir, fmt.Sprintf("%s_%s_%d.o", base, arch, os.Getpid()))
	} else {
		parent := filepath.Dir(out)
		if parent != "" && parent != "." {
			if err := os.MkdirAll(parent, 0o755); err != nil {
				return "", fmt.Errorf("failed to create output directory %q: %w", parent, err)
			}
		}
	}

	if opts.UseWSL {
		wslSrc := toWSLPath(opts.SourceFile)
		wslOut := toWSLPath(out)
		target := ClangTarget(arch)
		wslClang := opts.ClangPath
		if wslClang == "" {
			wslClang = "clang"
		} else if runtime.GOOS == "windows" && filepath.IsAbs(wslClang) {
			wslClang = toWSLPath(wslClang)
		}
		wslArgs := make([]string, 0, 18+len(opts.ExtraFlags))
		wslArgs = append(wslArgs,
			wslClang, "-c", wslSrc, "-o", wslOut,
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
		if arch == "arm64" {
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
		cmd := exec.CommandContext(ctx, "wsl", wslArgs...)
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			return "", fmt.Errorf("wsl clang compilation failed (%w): %s", err, stderr.String())
		}
		return out, nil
	}

	clang := opts.ClangPath
	if clang == "" {
		clang = "clang"
	}

	args := BuildClangArgs(opts, out)

	cmd := exec.CommandContext(ctx, clang, args...)
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
		driveLetter := cleaned[0]
		if (driveLetter >= 'a' && driveLetter <= 'z') || (driveLetter >= 'A' && driveLetter <= 'Z') {
			drive := strings.ToLower(string(driveLetter))
			rest := filepath.ToSlash(cleaned[2:])
			if !strings.HasPrefix(rest, "/") {
				rest = "/" + rest
			}
			if rest == "/" {
				return "/mnt/" + drive
			}
			return "/mnt/" + drive + rest
		}
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
