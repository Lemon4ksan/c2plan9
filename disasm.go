// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"golang.org/x/arch/x86/x86asm"
)

// DisassembledInst represents a single disassembled machine instruction.
type DisassembledInst struct {
	Offset   uint64
	Length   int
	RawBytes []byte
	Text     string
	Label    string // non-empty if a label should be placed before this instruction
	IsRet    bool
}

// DisassemblyResult contains disassembled instructions and detected stack frame size.
type DisassemblyResult struct {
	Insts     []DisassembledInst
	FrameSize uint64
}

// AMD64Disassembler bridges DisassembleAMD64 to the Disassembler interface.
var AMD64Disassembler Disassembler = DisassemblerFunc(DisassembleAMD64)

// DisassembleAMD64 disassembles raw x86-64 machine code into Plan 9 assembly lines.
func DisassembleAMD64(code []byte, baseOffset uint64, relocs []Relocation) (DisassemblyResult, error) {
	var (
		insts       []DisassembledInst
		jumpTargets = make(map[uint64]string)
		offset      = 0
		n           = len(code)
		frameSize   uint64
	)

	// Pass 1: Discover all branch targets and assign clean label names
	for offset < n {
		inst, err := x86asm.Decode(code[offset:], 64)
		if err != nil {
			// Single unrecognized byte
			offset++
			continue
		}

		if isVZeroUpper(code, offset) {
			inst.Len = 3
		}

		pc := uint64(offset)
		for _, arg := range inst.Args {
			if rel, ok := arg.(x86asm.Rel); ok {
				target := uint64(int64(pc) + int64(inst.Len) + int64(rel))
				if target <= uint64(n) && target > 0 {
					if _, exists := jumpTargets[target]; !exists {
						jumpTargets[target] = fmt.Sprintf("L_%04x", target)
					}
				}
			}
		}

		offset += inst.Len
	}

	// SymLookup callback for Plan9Syntax to resolve local labels
	symLookup := func(addr uint64) (string, uint64) {
		if label, ok := jumpTargets[addr]; ok {
			return label, addr
		}
		return "", 0
	}

	// Pass 2: Generate Plan 9 syntax for each instruction
	offset = 0
	for offset < n {
		pc := uint64(offset)
		inst, err := x86asm.Decode(code[offset:], 64)
		if err != nil {
			// Emit as raw byte
			raw := code[offset : offset+1]
			insts = append(insts, DisassembledInst{
				Offset:   pc,
				Length:   1,
				RawBytes: raw,
				Text:     fmt.Sprintf("BYTE $0x%02x", raw[0]),
				Label:    jumpTargets[pc],
			})
			offset++
			continue
		}

		// Fix x86asm decoder bug: VZEROUPPER (C5 F8 77) has no ModR/M byte (length is always 3)
		if isVZeroUpper(code, offset) {
			inst.Len = 3
		}

		raw := code[offset : offset+inst.Len]
		var curSymLookup x86asm.SymLookup
		if isBranchOp(inst.Op) {
			curSymLookup = symLookup
		}
		text := x86asm.GoSyntax(inst, pc, curSymLookup)
		text = cleanPlan9Syntax(text)

		// Check for .rodata relocation on memory access
		var matchedReloc *Relocation
		for i := range relocs {
			r := &relocs[i]
			if r.Offset >= pc+baseOffset && r.Offset < pc+baseOffset+uint64(inst.Len) {
				matchedReloc = r
				break
			}
		}

		if matchedReloc != nil &&
			(matchedReloc.IsROData || strings.HasPrefix(matchedReloc.SymName, ".rodata") || strings.HasPrefix(matchedReloc.SymName, ".LC")) {
			text = replaceIPWithROData(text, matchedReloc.Addend)
		} else if strings.Contains(text, "(IP)") ||
			strings.Contains(text, "(RIP)") {
			for _, arg := range inst.Args {
				if mem, ok := arg.(x86asm.Mem); ok && mem.Base == x86asm.RIP {
					text = replaceIPWithROData(text, int64(mem.Disp))
				}
			}
		}

		if inst.Op == x86asm.BSWAP && strings.HasPrefix(text, "BSWAP") {
			hasRexW := len(raw) >= 3 && (raw[0]&0xF8 == 0x48)
			reg := strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(text, "BSWAPQ"), "BSWAPL"))
			if strings.HasPrefix(text, "BSWAP ") {
				reg = strings.TrimSpace(text[6:])
			}
			if hasRexW {
				text = "BSWAPQ " + reg
			} else {
				text = "BSWAPL " + reg
			}
		}

		// Fix x86asm GoSyntax bug where 8-bit registers (R8B..R15B, AL..DL, SIL, DIL) lose their 'B' and become CMPL/TESTL/MOVL/ANDL
		for _, arg := range inst.Args {
			if reg, ok := arg.(x86asm.Reg); ok {
				regStr := reg.String()
				if is8BitRegStr(regStr) {
					baseReg := strings.TrimSuffix(regStr, "B")
					switch {
					case strings.HasPrefix(text, "CMPL "):
						text = "CMPB " + text[5:]
					case strings.HasPrefix(text, "TESTL "):
						text = "TESTB " + text[6:]
					case strings.HasPrefix(text, "MOVL "):
						text = "MOVB " + text[5:]
					case strings.HasPrefix(text, "ANDL "):
						text = "ANDB " + text[5:]
					case strings.HasPrefix(text, "ADDL "):
						text = "ADDB " + text[5:]
					case strings.HasPrefix(text, "SUBL "):
						text = "SUBB " + text[5:]
					case strings.HasPrefix(text, "ORL "):
						text = "ORB " + text[4:]
					case strings.HasPrefix(text, "XORL "):
						text = "XORB " + text[5:]
					}
					if !strings.Contains(text, regStr) {
						text = strings.ReplaceAll(text, " "+baseReg+",", " "+regStr+",")
						text = strings.ReplaceAll(text, ", "+baseReg, ", "+regStr)
					}
				}
			}
		}

		// Normalize MOVBLZX/MOVBQZX destination register (destination is 32/64-bit, not 8-bit)
		if strings.HasPrefix(text, "MOVBLZX ") || strings.HasPrefix(text, "MOVBQZX ") {
			parts := strings.Split(text, ",")
			if len(parts) == 2 {
				dstPart := strings.TrimSpace(parts[1])
				if is8BitRegStr(dstPart) {
					cleanDst := strings.TrimSuffix(dstPart, "B")
					text = parts[0] + ", " + cleanDst
				}
			}
		}

		// Normalize any possible duplicate 'B' suffixes (e.g. R11BB -> R11B)
		doubleRegs := []string{
			"R8BB",
			"R9BB",
			"R10BB",
			"R11BB",
			"R12BB",
			"R13BB",
			"R14BB",
			"R15BB",
			"ALB",
			"BLB",
			"CLB",
			"DLB",
		}
		for _, dr := range doubleRegs {
			text = strings.ReplaceAll(text, dr, strings.TrimSuffix(dr, "B"))
		}

		// Detect stack frame allocation in prologue (e.g. SUBQ $N, SP)
		if (inst.Op == x86asm.SUB) && (strings.HasPrefix(text, "SUBQ $") || strings.HasPrefix(text, "SUBL $")) &&
			strings.HasSuffix(text, ", SP") {
			if len(inst.Args) >= 2 {
				if imm, isImm := inst.Args[1].(x86asm.Imm); isImm && imm > 0 {
					frameSize = uint64(imm)
				}
			}
			if jumpTargets[pc] != "" {
				insts = append(insts, DisassembledInst{
					Offset:   pc,
					Length:   inst.Len,
					RawBytes: raw,
					Text:     "NOP",
					Label:    jumpTargets[pc],
				})
			}
			offset += inst.Len
			continue
		}

		// Detect stack frame deallocation in epilogue (e.g. ADDQ $N, SP)
		if (inst.Op == x86asm.ADD) && (strings.HasPrefix(text, "ADDQ $") || strings.HasPrefix(text, "ADDL $")) &&
			strings.HasSuffix(text, ", SP") {
			if jumpTargets[pc] != "" {
				insts = append(insts, DisassembledInst{
					Offset:   pc,
					Length:   inst.Len,
					RawBytes: raw,
					Text:     "NOP",
					Label:    jumpTargets[pc],
				})
			}
			offset += inst.Len
			continue
		}

		// Omit SysV callee-saved register PUSH/POP in Go leaf functions
		if (inst.Op == x86asm.PUSH || inst.Op == x86asm.POP) && len(inst.Args) > 0 {
			if _, isReg := inst.Args[0].(x86asm.Reg); isReg {
				if jumpTargets[pc] != "" {
					insts = append(insts, DisassembledInst{
						Offset:   pc,
						Length:   inst.Len,
						RawBytes: raw,
						Text:     "NOP",
						Label:    jumpTargets[pc],
					})
				}
				offset += inst.Len
				continue
			}
		}

		// Stack alignment instructions (e.g. ANDQ $-32, SP) must be preserved in the prologue!
		// If stack alignment is present, ensure frame size accounts for alignment padding.
		if inst.Op == x86asm.AND && (strings.HasPrefix(text, "ANDQ $-") || strings.HasPrefix(text, "ANDL $-")) &&
			strings.HasSuffix(text, ", SP") {
			align := uint64(32)
			numStr := strings.TrimSuffix(strings.TrimPrefix(strings.TrimPrefix(text, "ANDQ $-"), "ANDL $-"), ", SP")
			if val, err := strconv.ParseUint(numStr, 10, 64); err == nil && val > 0 {
				align = val
			} else if len(inst.Args) >= 2 {
				if imm, ok := inst.Args[1].(x86asm.Imm); ok {
					if imm < 0 {
						align = uint64(-imm)
					} else if int32(imm) < 0 {
						align = uint64(-int64(int32(imm)))
					}
				}
			}
			if align < 16 {
				align = 16
			}
			mask := align - 1
			if frameSize < align {
				frameSize = align
			} else {
				frameSize = (frameSize + mask) &^ mask
			}
		}

		insts = append(insts, DisassembledInst{
			Offset:   pc,
			Length:   inst.Len,
			RawBytes: raw,
			Text:     text,
			Label:    jumpTargets[pc],
			IsRet:    inst.Op == x86asm.RET,
		})

		offset += inst.Len
	}

	return DisassemblyResult{
		Insts:     insts,
		FrameSize: frameSize,
	}, nil
}

// cleanPlan9Syntax standardizes register and instruction syntax for Go's assembler.
func cleanPlan9Syntax(s string) string {
	s = strings.TrimSpace(s)

	// Replace known register names if necessary
	replacements := []struct {
		old string
		new string
	}{
		{"%rax", "AX"},
		{"%rbx", "BX"},
		{"%rcx", "CX"},
		{"%rdx", "DX"},
		{"%rsi", "SI"},
		{"%rdi", "DI"},
		{"%rbp", "BP"},
		{"%rsp", "SP"},
		{"%r8", "R8"},
		{"%r9", "R9"},
		{"%r10", "R10"},
		{"%r11", "R11"},
		{"%r12", "R12"},
		{"%r13", "R13"},
		{"%r14", "R14"},
		{"%r15", "R15"},
	}

	for _, r := range replacements {
		s = strings.ReplaceAll(s, r.old, r.new)
	}

	// Strip (SB) from local jump targets (e.g. JE L_0014(SB) -> JE L_0014)
	if strings.Contains(s, "(SB)") && strings.Contains(s, "L_") {
		s = strings.ReplaceAll(s, "(SB)", "")
	}

	// Replace CS NOP / prefix NOP with standard NOP
	if strings.Contains(s, "NOP") {
		s = "NOP"
	}

	// Standardize stack alignment instructions to decimal imm (e.g. ANDQ $-0x20, SP -> ANDQ $-32, SP)
	if (strings.HasPrefix(s, "ANDQ $-0x") || strings.HasPrefix(s, "ANDL $-0x")) && strings.HasSuffix(s, ", SP") {
		var op, hexStr string
		if strings.HasPrefix(s, "ANDQ $-0x") {
			op = "ANDQ"
			hexStr = strings.TrimSuffix(strings.TrimPrefix(s, "ANDQ $-0x"), ", SP")
		} else {
			op = "ANDL"
			hexStr = strings.TrimSuffix(strings.TrimPrefix(s, "ANDL $-0x"), ", SP")
		}
		if val, err := strconv.ParseUint(hexStr, 16, 64); err == nil {
			s = fmt.Sprintf("%s $-%d, SP", op, val)
		}
	}

	// Normalize 8-bit registers for SETcc instructions
	if strings.HasPrefix(s, "SET") {
		parts := strings.SplitN(s, " ", 2)
		if len(parts) == 2 {
			op := parts[0]
			reg := strings.TrimSpace(parts[1])
			switch reg {
			case "AX":
				reg = "AL"
			case "BX":
				reg = "BL"
			case "CX":
				reg = "CL"
			case "DX":
				reg = "DL"
			case "SI":
				reg = "SIL"
			case "DI":
				reg = "DIL"
			case "BP":
				reg = "BPL"
			case "SP":
				reg = "SPL"
			case "R8":
				reg = "R8B"
			case "R9":
				reg = "R9B"
			case "R10":
				reg = "R10B"
			case "R11":
				reg = "R11B"
			case "R12":
				reg = "R12B"
			case "R13":
				reg = "R13B"
			case "R14":
				reg = "R14B"
			case "R15":
				reg = "R15B"
			}
			s = op + " " + reg
		}
	}

	// Fix 8-bit register suffixes/prefixes (e.g. CMPL DL, $160 -> CMPB DL, $160)
	eightBitRegs := []string{
		"AL",
		"BL",
		"CL",
		"DL",
		"SIL",
		"DIL",
		"BPL",
		"SPL",
		"R8B",
		"R9B",
		"R10B",
		"R11B",
		"R12B",
		"R13B",
		"R14B",
		"R15B",
	}
	for _, reg := range eightBitRegs {
		if strings.Contains(s, " "+reg+",") || strings.HasSuffix(s, ", "+reg) || strings.HasSuffix(s, " "+reg) {
			switch {
			case strings.HasPrefix(s, "TESTL "):
				s = "TESTB " + s[6:]
			case strings.HasPrefix(s, "CMPL "):
				s = "CMPB " + s[5:]
			case strings.HasPrefix(s, "MOVL "):
				s = "MOVB " + s[5:]
			case strings.HasPrefix(s, "ANDL "):
				s = "ANDB " + s[5:]
			case strings.HasPrefix(s, "ORL "):
				s = "ORB " + s[4:]
			case strings.HasPrefix(s, "XORL "):
				s = "XORB " + s[5:]
			case strings.HasPrefix(s, "ADDL "):
				s = "ADDB " + s[5:]
			case strings.HasPrefix(s, "SUBL "):
				s = "SUBB " + s[5:]
			case strings.HasPrefix(s, "SHRL "):
				s = "SHRB " + s[5:]
			case strings.HasPrefix(s, "SHLL "):
				s = "SHLB " + s[5:]
			case strings.HasPrefix(s, "SARL "):
				s = "SARB " + s[5:]
			}
		}
	}

	sixtyFourBitRegs := []string{
		"AX",
		"BX",
		"CX",
		"DX",
		"SI",
		"DI",
		"BP",
		"SP",
		"R8",
		"R9",
		"R10",
		"R11",
		"R12",
		"R13",
		"R14",
		"R15",
	}
	is64Dest := func(str string) bool {
		parts := strings.Split(str, ",")
		if len(parts) >= 2 {
			dest := strings.TrimSpace(parts[len(parts)-1])
			if slices.Contains(sixtyFourBitRegs, dest) {
				return true
			}
		}
		return false
	}

	// Mnemonic translations for Go's Plan 9 assembler
	switch {
	case strings.HasPrefix(s, "MOVZX "):
		s = "MOVBLZX " + s[6:]
	case strings.HasPrefix(s, "MOVSX "):
		s = "MOVBLSX " + s[6:]
	case strings.HasPrefix(s, "MOVSXD "):
		s = "MOVLQSX " + s[7:]
	case strings.HasPrefix(s, "CMOVAE "):
		if is64Dest(s) {
			s = "CMOVQCC " + s[7:]
		} else {
			s = "CMOVLCC " + s[7:]
		}
	case strings.HasPrefix(s, "CMOVB "):
		if is64Dest(s) {
			s = "CMOVQCS " + s[6:]
		} else {
			s = "CMOVLCS " + s[6:]
		}
	case strings.HasPrefix(s, "CMOVE "):
		if is64Dest(s) {
			s = "CMOVQEQ " + s[6:]
		} else {
			s = "CMOVLEQ " + s[6:]
		}
	case strings.HasPrefix(s, "CMOVNE "):
		if is64Dest(s) {
			s = "CMOVQNE " + s[7:]
		} else {
			s = "CMOVLNE " + s[7:]
		}
	case strings.HasPrefix(s, "CMOVG "):
		if is64Dest(s) {
			s = "CMOVQGT " + s[6:]
		} else {
			s = "CMOVLGT " + s[6:]
		}
	case strings.HasPrefix(s, "CMOVGE "):
		if is64Dest(s) {
			s = "CMOVQGE " + s[7:]
		} else {
			s = "CMOVLGE " + s[7:]
		}
	case strings.HasPrefix(s, "CMOVL "):
		if is64Dest(s) {
			s = "CMOVQLT " + s[6:]
		} else {
			s = "CMOVLLT " + s[6:]
		}
	case strings.HasPrefix(s, "CMOVLE "):
		if is64Dest(s) {
			s = "CMOVQLE " + s[7:]
		} else {
			s = "CMOVLLE " + s[7:]
		}
	case strings.HasPrefix(s, "CMOVNS "):
		if is64Dest(s) {
			s = "CMOVQPL " + s[7:]
		} else {
			s = "CMOVLPL " + s[7:]
		}
	case strings.HasPrefix(s, "CMOVS "):
		if is64Dest(s) {
			s = "CMOVQMI " + s[6:]
		} else {
			s = "CMOVLMI " + s[6:]
		}
	case strings.HasPrefix(s, "TZCNT "):
		s = "TZCNTL " + s[6:]
	case strings.HasPrefix(s, "BSF "):
		s = "BSFL " + s[4:]
	case strings.HasPrefix(s, "BSR "):
		s = "BSRL " + s[4:]
	case strings.HasPrefix(s, "LZCNT "):
		s = "LZCNTL " + s[6:]
	case strings.HasPrefix(s, "POPCNT "):
		s = "POPCNTL " + s[7:]
	case strings.HasPrefix(s, "SETNS "):
		s = "SETPL " + s[6:]
	case strings.HasPrefix(s, "SETS "):
		s = "SETMI " + s[5:]
	case strings.HasPrefix(s, "SETE "):
		s = "SETEQ " + s[5:]
	case strings.HasPrefix(s, "SETAE "):
		s = "SETCC " + s[6:]
	case strings.HasPrefix(s, "SETB "):
		s = "SETCS " + s[5:]
	case strings.HasPrefix(s, "SETA "):
		s = "SETHI " + s[5:]
	case strings.HasPrefix(s, "SETBE "):
		s = "SETLS " + s[6:]
	case strings.HasPrefix(s, "SETL "):
		s = "SETLT " + s[5:]
	case strings.HasPrefix(s, "SETG "):
		s = "SETGT " + s[5:]
	case strings.HasPrefix(s, "SETNE "):
		s = "SETNE " + s[6:]
	}

	return s
}

func is8BitRegStr(r string) bool {
	switch r {
	case "AL", "BL", "CL", "DL", "AH", "BH", "CH", "DH",
		"SIL", "DIL", "BPL", "SPL",
		"R8B", "R9B", "R10B", "R11B", "R12B", "R13B", "R14B", "R15B":
		return true
	}
	return false
}

func isBranchOp(op x86asm.Op) bool {
	switch op {
	case x86asm.CALL, x86asm.JMP,
		x86asm.JA, x86asm.JAE, x86asm.JB, x86asm.JBE,
		x86asm.JCXZ, x86asm.JECXZ, x86asm.JRCXZ,
		x86asm.JE, x86asm.JG, x86asm.JGE, x86asm.JL, x86asm.JLE,
		x86asm.JNE, x86asm.JNO, x86asm.JNP, x86asm.JNS, x86asm.JO,
		x86asm.JP, x86asm.JS, x86asm.LOOP, x86asm.LOOPE, x86asm.LOOPNE:
		return true
	}
	return false
}

func replaceIPWithROData(text string, addend int64) string {
	var roTarget string
	if addend > 0 {
		roTarget = fmt.Sprintf("·rodata<>+%d(SB)", addend)
	} else if addend < 0 {
		roTarget = fmt.Sprintf("·rodata<>%d(SB)", addend)
	} else {
		roTarget = "·rodata<>(SB)"
	}

	parts := strings.SplitN(text, " ", 2)
	if len(parts) == 2 {
		op := parts[0]
		args := strings.Split(parts[1], ",")
		for i, arg := range args {
			a := strings.TrimSpace(arg)
			if strings.Contains(a, "(IP)") || strings.Contains(a, "(RIP)") {
				args[i] = roTarget
			} else {
				args[i] = a
			}
		}
		return op + " " + strings.Join(args, ", ")
	}
	return text
}

var vzeroupperSeq = []byte{0xC5, 0xF8, 0x77}

func isVZeroUpper(code []byte, offset int) bool {
	if offset+3 <= len(code) {
		return slices.Equal(code[offset:offset+3], vzeroupperSeq)
	}
	return false
}
