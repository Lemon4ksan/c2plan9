// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"encoding/binary"
	"fmt"
	"strings"

	"golang.org/x/arch/arm64/arm64asm"
)

// DisassembleARM64 disassembles raw AArch64 machine code into Plan 9 ARM64 assembly lines.
func DisassembleARM64(code []byte, baseOffset uint64, relocs []Relocation) (DisassemblyResult, error) {
	var (
		insts       []DisassembledInst
		jumpTargets = make(map[uint64]string)
		offset      = 0
		n           = len(code)
		frameSize   uint64
	)

	// Pass 1: Discover all branch targets and assign clean label names
	for offset+4 <= n {
		inst, err := arm64asm.Decode(code[offset : offset+4])
		if err != nil {
			offset += 4
			continue
		}

		pc := uint64(offset)
		for _, arg := range inst.Args {
			if pcrel, ok := arg.(arm64asm.PCRel); ok {
				target := uint64(int64(pc) + int64(pcrel))
				if target <= uint64(n) && target > 0 {
					if _, exists := jumpTargets[target]; !exists {
						jumpTargets[target] = fmt.Sprintf("L_%04x", target)
					}
				}
			}
		}

		offset += 4
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
	for offset+4 <= n {
		pc := uint64(offset)
		raw := code[offset : offset+4]
		inst, err := arm64asm.Decode(raw)
		if err != nil {
			word := binary.LittleEndian.Uint32(raw)
			insts = append(insts, DisassembledInst{
				Offset:   pc,
				Length:   4,
				RawBytes: raw,
				Text:     fmt.Sprintf("WORD $0x%08x", word),
				Label:    jumpTargets[pc],
			})
			offset += 4
			continue
		}

		text := arm64asm.GoSyntax(inst, pc, symLookup, nil)
		text = cleanPlan9ARM64Syntax(text)

		// Detect stack frame allocation in prologue (e.g. SUB $N, RSP)
		if (inst.Op == arm64asm.SUB) && (strings.Contains(text, "RSP, RSP") || strings.Contains(text, "SP, SP")) {
			if len(inst.Args) >= 3 {
				if imm, isImm := inst.Args[2].(arm64asm.Imm); isImm && imm.Imm > 0 {
					frameSize = uint64(imm.Imm)
				}
			}
			if jumpTargets[pc] != "" {
				insts = append(insts, DisassembledInst{
					Offset:   pc,
					Length:   4,
					RawBytes: raw,
					Text:     "NOP",
					Label:    jumpTargets[pc],
				})
			}
			offset += 4
			continue
		}

		// Detect stack frame deallocation in epilogue (e.g. ADD $N, RSP)
		if (inst.Op == arm64asm.ADD) && (strings.Contains(text, "RSP, RSP") || strings.Contains(text, "SP, SP")) {
			if jumpTargets[pc] != "" {
				insts = append(insts, DisassembledInst{
					Offset:   pc,
					Length:   4,
					RawBytes: raw,
					Text:     "NOP",
					Label:    jumpTargets[pc],
				})
			}
			offset += 4
			continue
		}

		// Omit frame pointer / link register pushes (STP (R29, R30), -16(RSP)!) in Go functions
		if (inst.Op == arm64asm.STP || inst.Op == arm64asm.LDP) &&
			(strings.Contains(text, "R29") || strings.Contains(text, "R30")) {
			if jumpTargets[pc] != "" {
				insts = append(insts, DisassembledInst{
					Offset:   pc,
					Length:   4,
					RawBytes: raw,
					Text:     "NOP",
					Label:    jumpTargets[pc],
				})
			}
			offset += 4
			continue
		}

		// Check for .rodata relocation on memory access / ADRP
		var matchedReloc *Relocation
		for i := range relocs {
			r := &relocs[i]
			if r.Offset >= pc+baseOffset && r.Offset < pc+baseOffset+4 {
				matchedReloc = r
				break
			}
		}

		if matchedReloc != nil && (matchedReloc.IsROData || strings.HasPrefix(matchedReloc.SymName, ".rodata")) {
			text = replaceARM64ROData(text, matchedReloc.Addend)
		}

		isRet := inst.Op == arm64asm.RET

		insts = append(insts, DisassembledInst{
			Offset:   pc,
			Length:   4,
			RawBytes: raw,
			Text:     text,
			Label:    jumpTargets[pc],
			IsRet:    isRet,
		})

		offset += 4
	}

	return DisassemblyResult{
		Insts:     insts,
		FrameSize: frameSize,
	}, nil
}

// cleanPlan9ARM64Syntax standardizes register and instruction syntax for Go's ARM64 assembler.
func cleanPlan9ARM64Syntax(s string) string {
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

	// Translate unscaled loads, stores, vector reductions, and barriers
	parts := strings.SplitN(s, " ", 2)
	if len(parts) >= 1 {
		op := parts[0]
		args := ""

		if len(parts) == 2 {
			args = parts[1]
		}

		switch op {
		// Unscaled Loads (LDUR -> MOV)
		case "LDURBW", "LDURB":
			return "MOVBU " + args
		case "LDURSBW", "LDURSB":
			return "MOVB " + args
		case "LDURHW", "LDURH":
			return "MOVHU " + args
		case "LDURSHW", "LDURSH":
			return "MOVH " + args
		case "LDURW", "LDURSW":
			return "MOVW " + args
		case "LDUR":
			if strings.Contains(args, "F") || strings.Contains(args, "V") || strings.Contains(args, "Q") {
				return "FMOVQ " + args
			}

			return "MOVD " + args
		case "LDURD":
			return "FMOVD " + args
		case "LDURS":
			return "FMOVS " + args
		case "LDURQ":
			return "FMOVQ " + args

		// Unscaled Stores (STUR -> MOV)
		case "STURBW", "STURB":
			return "MOVB " + args
		case "STURHW", "STURH":
			return "MOVH " + args
		case "STURW":
			return "MOVW " + args
		case "STUR":
			if strings.Contains(args, "F") || strings.Contains(args, "V") || strings.Contains(args, "Q") {
				return "FMOVQ " + args
			}

			return "MOVD " + args
		case "STURD":
			return "FMOVD " + args
		case "STURS":
			return "FMOVS " + args
		case "STURQ":
			return "FMOVQ " + args

		// Vector reductions / NEON across-vector operations
		case "VMAXV":
			return "UMAXV " + args
		case "VMINV":
			return "UMINV " + args
		case "VADDV":
			return "ADDV " + args

		// Barrier cleanups
		case "ISB":
			if args == "$15" || args == "$0xf" || args == "15" {
				return "ISB"
			}
		}
	}

	return s
}

func replaceARM64ROData(text string, addend int64) string {
	roTarget := fmt.Sprintf("·rodata<>+%d(SB)", addend)
	if addend == 0 {
		roTarget = "·rodata<>(SB)"
	}

	parts := strings.Split(text, " ")
	if len(parts) >= 2 {
		op := parts[0]
		if strings.HasPrefix(op, "ADRP") || strings.HasPrefix(op, "LDR") || strings.HasPrefix(op, "MOVD") {
			argsStr := strings.Join(parts[1:], " ")
			args := strings.Split(argsStr, ",")
			if len(args) >= 2 {
				return fmt.Sprintf("MOVD $%s, %s", roTarget, strings.TrimSpace(args[len(args)-1]))
			}
		}
	}
	return text
}
