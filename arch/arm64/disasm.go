// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package arm64

import (
	"encoding/binary"
	"fmt"
	"strings"

	"golang.org/x/arch/arm64/arm64asm"

	"github.com/lemon4ksan/c2plan9/arch"
)

const (
	// instSize defines the fixed 4-byte instruction width of the AArch64 architecture.
	instSize = 4
)

// directOpMap translates AArch64 instruction mnemonics directly to their Plan 9 equivalents.
var directOpMap = map[string]string{
	// Unscaled Loads (LDUR -> MOV)
	"LDURBW":  "MOVBU",
	"LDURB":   "MOVBU",
	"LDURSBW": "MOVB",
	"LDURSB":  "MOVB",
	"LDURHW":  "MOVHU",
	"LDURH":   "MOVHU",
	"LDURSHW": "MOVH",
	"LDURSH":  "MOVH",
	"LDURW":   "MOVW",
	"LDURSW":  "MOVW",
	"LDURD":   "FMOVD",
	"LDURS":   "FMOVS",
	"LDURQ":   "FMOVQ",

	// Unscaled Stores (STUR -> MOV)
	"STURBW": "MOVB",
	"STURB":  "MOVB",
	"STURHW": "MOVH",
	"STURH":  "MOVH",
	"STURW":  "MOVW",
	"STURD":  "FMOVD",
	"STURS":  "FMOVS",
	"STURQ":  "FMOVQ",

	// Across-vector NEON reductions
	"VMAXV": "UMAXV",
	"VMINV": "UMINV",
	"VADDV": "ADDV",
}

// Disassemble disassembles raw AArch64 machine code into Plan 9 ARM64 assembly lines.
// It performs a two-pass decode: Pass 1 discovers branch targets and generates local labels,
// while Pass 2 translates opcodes into Plan 9 ARM64 syntax, detects stack frame allocation
// in prologues, elides frame pointer/link register saves, and rewrites rodata relocations.
func Disassemble(code []byte, baseOffset uint64, relocs []arch.Relocation) (arch.DisassemblyResult, error) {
	var (
		insts       []arch.DisassembledInst
		jumpTargets = make(map[uint64]string)
		offset      = 0
		n           = len(code)
		frameSize   uint64
	)

	// Pass 1: Discover all branch targets and assign clean label names
	for offset+instSize <= n {
		inst, err := arm64asm.Decode(code[offset : offset+instSize])
		if err != nil {
			offset += instSize
			continue
		}

		pc := uint64(offset)
		for _, arg := range inst.Args {
			if pcrel, ok := arg.(arm64asm.PCRel); ok {
				targetOffset := int64(pc) + int64(pcrel)
				if targetOffset >= 0 && targetOffset <= int64(n) {
					target := uint64(targetOffset)
					if _, exists := jumpTargets[target]; !exists {
						jumpTargets[target] = fmt.Sprintf("L_%04x", target)
					}
				}
			}
		}

		offset += instSize
	}

	// SymLookup callback for arm64asm.GoSyntax to resolve local labels
	symLookup := func(addr uint64) (string, uint64) {
		if label, ok := jumpTargets[addr]; ok {
			return label, addr
		}
		return "", 0
	}

	// Pass 2: Generate Plan 9 ARM64 syntax for each instruction
	offset = 0
	for offset+instSize <= n {
		pc := uint64(offset)
		raw := code[offset : offset+instSize]
		inst, err := arm64asm.Decode(raw)
		if err != nil {
			word := binary.LittleEndian.Uint32(raw)
			insts = append(insts, arch.DisassembledInst{
				Offset:   pc,
				Length:   instSize,
				RawBytes: raw,
				Text:     fmt.Sprintf("WORD $0x%08x", word),
				Label:    jumpTargets[pc],
			})
			offset += instSize
			continue
		}

		text := arm64asm.GoSyntax(inst, pc, symLookup, nil)
		text = CleanPlan9Syntax(text)

		// Detect stack frame allocation in prologue (e.g. SUB $N, RSP)
		if (inst.Op == arm64asm.SUB) && (strings.Contains(text, "RSP, RSP") || strings.Contains(text, "SP, SP")) {
			if len(inst.Args) >= 3 {
				if imm, isImm := inst.Args[2].(arm64asm.Imm); isImm && imm.Imm > 0 {
					frameSize = uint64(imm.Imm)
				}
			}
			if jumpTargets[pc] != "" {
				insts = append(insts, arch.DisassembledInst{
					Offset:   pc,
					Length:   instSize,
					RawBytes: raw,
					Text:     "NOP",
					Label:    jumpTargets[pc],
				})
			}
			offset += instSize
			continue
		}

		// Detect stack frame deallocation in epilogue (e.g. ADD $N, RSP)
		if (inst.Op == arm64asm.ADD) && (strings.Contains(text, "RSP, RSP") || strings.Contains(text, "SP, SP")) {
			if jumpTargets[pc] != "" {
				insts = append(insts, arch.DisassembledInst{
					Offset:   pc,
					Length:   instSize,
					RawBytes: raw,
					Text:     "NOP",
					Label:    jumpTargets[pc],
				})
			}
			offset += instSize
			continue
		}

		// Omit frame pointer / link register pushes (STP (R29, R30), -16(RSP)!) in Go functions
		if (inst.Op == arm64asm.STP || inst.Op == arm64asm.LDP) &&
			(strings.Contains(text, "R29") || strings.Contains(text, "R30")) {
			if jumpTargets[pc] != "" {
				insts = append(insts, arch.DisassembledInst{
					Offset:   pc,
					Length:   instSize,
					RawBytes: raw,
					Text:     "NOP",
					Label:    jumpTargets[pc],
				})
			}
			offset += instSize
			continue
		}

		// Check for .rodata relocation on memory access / ADRP
		var matchedReloc *arch.Relocation
		for i := range relocs {
			r := &relocs[i]
			if r.Offset >= pc+baseOffset && r.Offset < pc+baseOffset+instSize {
				matchedReloc = r
				break
			}
		}

		if matchedReloc != nil && (matchedReloc.IsROData || strings.HasPrefix(matchedReloc.SymName, ".rodata")) {
			text = ReplaceROData(text, matchedReloc.Addend)
		}

		isRet := inst.Op == arm64asm.RET

		insts = append(insts, arch.DisassembledInst{
			Offset:   pc,
			Length:   instSize,
			RawBytes: raw,
			Text:     text,
			Label:    jumpTargets[pc],
			IsRet:    isRet,
		})

		offset += instSize
	}

	return arch.DisassemblyResult{
		Insts:     insts,
		FrameSize: frameSize,
	}, nil
}

// CleanPlan9Syntax standardizes register and instruction syntax for Go's ARM64 assembler.
// It normalizes SP to RSP, strips redundant symbol qualifiers from local branches, converts
// unscaled load/store instructions, translates NEON reduction operations, and normalizes barriers.
func CleanPlan9Syntax(s string) string {
	s = strings.TrimSpace(s)

	// Replace SP with RSP
	if strings.Contains(s, " SP,") || strings.Contains(s, ", SP") || strings.Contains(s, "(SP)") {
		s = strings.ReplaceAll(s, " SP,", " RSP,")
		s = strings.ReplaceAll(s, ", SP", ", RSP")
		s = strings.ReplaceAll(s, "(SP)", "(RSP)")
	}

	// Clean RET instruction
	if strings.HasPrefix(s, "RET") {
		return "RET"
	}

	// Strip (SB) from local jump targets (e.g. B L_0014(SB) -> B L_0014)
	if strings.Contains(s, "(SB)") && strings.Contains(s, "L_") {
		s = strings.ReplaceAll(s, "(SB)", "")
	}

	// Translate unscaled loads, stores, vector reductions, and barriers via table lookup
	parts := strings.SplitN(s, " ", 2)
	if len(parts) >= 1 {
		op := parts[0]
		args := ""
		if len(parts) == 2 {
			args = parts[1]
		}

		if targetOp, ok := directOpMap[op]; ok {
			return targetOp + " " + args
		}

		if op == "LDUR" || op == "STUR" {
			return translateUnscaledLDURSTUR(op, args)
		}

		if op == "ISB" && (args == "$15" || args == "$0xf" || args == "15") {
			return "ISB"
		}
	}

	return s
}

// translateUnscaledLDURSTUR converts unscaled 64-bit LDUR/STUR instructions to either SIMD
// vector (FMOVQ) or integer (MOVD) Plan 9 instructions based on operand register types.
func translateUnscaledLDURSTUR(op, args string) string {
	if strings.Contains(args, "F") || strings.Contains(args, "V") || strings.Contains(args, "Q") {
		return "FMOVQ " + args
	}
	return "MOVD " + args
}

// ReplaceROData rewrites ARM64 ADRP, LDR, or MOVD instructions referencing static read-only data
// to load the address of the Plan 9 ·rodata<>(SB) symbol into the destination register.
func ReplaceROData(text string, addend int64) string {
	roTarget := fmt.Sprintf("·rodata<>+%d(SB)", addend)
	if addend == 0 {
		roTarget = "·rodata<>(SB)"
	}

	parts := strings.SplitN(text, " ", 2)
	if len(parts) == 2 {
		op := parts[0]
		if strings.HasPrefix(op, "ADRP") || strings.HasPrefix(op, "LDR") || strings.HasPrefix(op, "MOVD") {
			args := strings.Split(parts[1], ",")
			if len(args) >= 2 {
				return fmt.Sprintf("MOVD $%s, %s", roTarget, strings.TrimSpace(args[len(args)-1]))
			}
		}
	}
	return text
}
