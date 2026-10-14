// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main_test

import (
	"fmt"
	"strings"

	c2plan9 "github.com/lemon4ksan/c2plan9"
)

func ExampleBuildClangArgs() {
	opts := c2plan9.CompileOptions{
		SourceFile: "kernel.c",
		TargetArch: "amd64",
	}

	args := c2plan9.BuildClangArgs(opts, "kernel.o")

	// Target flag is always forced to ELF triple
	var hasTarget bool
	for i, arg := range args {
		if arg == "-target" && i+1 < len(args) && args[i+1] == "x86_64-pc-linux-gnu" {
			hasTarget = true
			break
		}
	}

	fmt.Printf("Enforces ELF Target: %v\n", hasTarget)
	// Output:
	// Enforces ELF Target: true
}

func ExampleEmitGoStub() {
	sigs := []c2plan9.FuncSignature{
		{
			Name:       "sum_floats",
			GoName:     "SumFloats",
			ParamCount: 2,
			HasReturn:  true,
			ReturnType: c2plan9.TypeFloat32,
		},
		{
			Name:       "xor_vectors",
			GoName:     "XorVectors",
			ParamCount: 2,
			HasReturn:  true,
			ReturnType: c2plan9.TypeVector128,
		},
	}

	stubs := c2plan9.EmitGoStub("vectormath", sigs, "amd64")
	output := string(stubs)

	hasSum := strings.Contains(output, "func SumFloats(arg0 uint64, arg1 uint64) float32")
	hasXor := strings.Contains(output, "func XorVectors(arg0 uint64, arg1 uint64) [16]byte")
	fmt.Printf("Has SumFloats stub: %v\n", hasSum)
	fmt.Printf("Has XorVectors stub: %v\n", hasXor)
	// Output:
	// Has SumFloats stub: true
	// Has XorVectors stub: true
}
