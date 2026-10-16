// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package amd64

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"golang.org/x/arch/x86/x86asm"

	"github.com/lemon4ksan/c2plan9/arch"
)

const (
	// minStackAlign defines the minimum 16-byte stack alignment required by Go's AMD64 ABI.
	minStackAlign = 16

	// defaultAVXAlign defines the 32-byte stack alignment required for 256-bit AVX/AVX2 vector operations.
	defaultAVXAlign = 32

	// vzeroupperLen defines the exact byte length of the VZEROUPPER instruction (0xC5 0xF8 0x77).
	vzeroupperLen = 3

	// rexPrefixMask isolates the high nibble of an instruction prefix to identify REX bytes (0x40..0x4F).
	rexPrefixMask = 0xF0

	// rexPrefixBase is the base byte value identifying an x86-64 REX prefix.
	rexPrefixBase = 0x40

	// rexWBit indicates a 64-bit operand width promotion in an x86-64 instruction.
	rexWBit = 0x08
)

// vzeroupperSeq is the 3-byte VEX-encoded machine instruction for VZEROUPPER (VEX.128.0F.WIG 77).
var vzeroupperSeq = []byte{0xC5, 0xF8, 0x77}

// atntRegisterReplacer normalizes AT&T-style percent-prefixed register names to Plan 9 names.
var atntRegisterReplacer = strings.NewReplacer(
	"%rax", "AX",
	"%rbx", "BX",
	"%rcx", "CX",
	"%rdx", "DX",
	"%rsi", "SI",
	"%rdi", "DI",
	"%rbp", "BP",
	"%rsp", "SP",
	"%r8", "R8",
	"%r9", "R9",
	"%r10", "R10",
	"%r11", "R11",
	"%r12", "R12",
	"%r13", "R13",
	"%r14", "R14",
	"%r15", "R15",
)

// setcc8BitRegs maps general-purpose register names to their 8-bit low byte equivalent for SETcc instructions.
var setcc8BitRegs = map[string]string{
	"AX":  "AL",
	"BX":  "BL",
	"CX":  "CL",
	"DX":  "DL",
	"SI":  "SIL",
	"DI":  "DIL",
	"BP":  "BPL",
	"SP":  "SPL",
	"R8":  "R8B",
	"R9":  "R9B",
	"R10": "R10B",
	"R11": "R11B",
	"R12": "R12B",
	"R13": "R13B",
	"R14": "R14B",
	"R15": "R15B",
}

// eightBitRegs lists all 8-bit registers recognized in AMD64 machine code translations.
var eightBitRegs = []string{
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

// sixtyFourBitRegs lists all 64-bit general-purpose registers recognized in AMD64 destination checks.
var sixtyFourBitRegs = []string{
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

// byteOpSubstitutions translates 32-bit register operations to their 8-bit byte variants when 8-bit operands are detected.
var byteOpSubstitutions = map[string]string{
	"TESTL": "TESTB",
	"CMPL":  "CMPB",
	"MOVL":  "MOVB",
	"ANDL":  "ANDB",
	"ORL":   "ORB",
	"XORL":  "XORB",
	"ADDL":  "ADDB",
	"SUBL":  "SUBB",
	"SHRL":  "SHRB",
	"SHLL":  "SHLB",
	"SARL":  "SARB",
}

// cmovConditions maps x86 conditional move mnemonics to their Plan 9 condition suffixes.
var cmovConditions = map[string]string{
	"CMOVAE": "CC",
	"CMOVB":  "CS",
	"CMOVE":  "EQ",
	"CMOVNE": "NE",
	"CMOVG":  "GT",
	"CMOVGE": "GE",
	"CMOVL":  "LT",
	"CMOVLE": "LE",
	"CMOVNS": "PL",
	"CMOVS":  "MI",
}

// setccMnemonicMap maps x86 SETcc mnemonics to Plan 9 assembler equivalents.
var setccMnemonicMap = map[string]string{
	"SETNS": "SETPL",
	"SETS":  "SETMI",
	"SETE":  "SETEQ",
	"SETAE": "SETCC",
	"SETB":  "SETCS",
	"SETA":  "SETHI",
	"SETBE": "SETLS",
	"SETL":  "SETLT",
	"SETG":  "SETGT",
	"SETNE": "SETNE",
}

// mnemonicAliasesAMD64 maps x86-64 mnemonics to Plan 9 assembler equivalents.
var mnemonicAliasesAMD64 = map[string]string{
	"MOVZX":  "MOVBLZX",
	"MOVSX":  "MOVBLSX",
	"MOVSXD": "MOVLQSX",
	"TZCNT":  "TZCNTL",
	"BSF":    "BSFL",
	"BSR":    "BSRL",
	"LZCNT":  "LZCNTL",
	"POPCNT": "POPCNTL",
}

// doubleRegs lists accidental duplicate 'B' suffixes produced by intermediate register naming translations.
var doubleRegs = []string{
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

// Disassemble disassembles raw x86-64 machine code into Plan 9 assembly instructions.
// It executes a two-pass analysis: Pass 1 discovers branch targets and generates local labels,
// while Pass 2 translates instructions, strips callee-saved frame register saves, preserves
// dynamic stack alignment, and resolves rodata relocations.
func Disassemble(code []byte, baseOffset uint64, relocs []arch.Relocation) (arch.DisassemblyResult, error) {
	var (
		insts       []arch.DisassembledInst
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

		if IsVZeroUpper(code, offset) {
			inst.Len = vzeroupperLen
		}

		pc := uint64(offset)
		for _, arg := range inst.Args {
			if rel, ok := arg.(x86asm.Rel); ok {
				targetOffset := int64(pc) + int64(inst.Len) + int64(rel)
				if targetOffset >= 0 && targetOffset <= int64(n) {
					target := uint64(targetOffset)
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
			insts = append(insts, arch.DisassembledInst{
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
		if IsVZeroUpper(code, offset) {
			inst.Len = vzeroupperLen
		}

		raw := code[offset : offset+inst.Len]
		var curSymLookup x86asm.SymLookup
		if isBranchOp(inst.Op) {
			curSymLookup = symLookup
		}
		text := x86asm.GoSyntax(inst, pc, curSymLookup)
		text = CleanPlan9Syntax(text)

		// Check for .rodata relocation on memory access
		var matchedReloc *arch.Relocation
		for i := range relocs {
			r := &relocs[i]
			if r.Offset >= pc+baseOffset && r.Offset < pc+baseOffset+uint64(inst.Len) {
				matchedReloc = r
				break
			}
		}

		if matchedReloc != nil &&
			(matchedReloc.IsROData || strings.HasPrefix(matchedReloc.SymName, ".rodata") || strings.HasPrefix(matchedReloc.SymName, ".LC")) {
			text = ReplaceIPWithROData(text, matchedReloc.Addend)
		} else if strings.Contains(text, "(IP)") || strings.Contains(text, "(RIP)") {
			for _, arg := range inst.Args {
				if mem, ok := arg.(x86asm.Mem); ok && mem.Base == x86asm.RIP {
					text = ReplaceIPWithROData(text, int64(mem.Disp))
				}
			}
		}

		if inst.Op == x86asm.BSWAP && strings.HasPrefix(text, "BSWAP") {
			hasRex := len(raw) >= 3 && HasRexW(raw[0])
			reg := strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(text, "BSWAPQ"), "BSWAPL"))
			if strings.HasPrefix(text, "BSWAP ") {
				reg = strings.TrimSpace(text[6:])
			}
			if hasRex {
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
					parts := strings.SplitN(text, " ", 2)
					if len(parts) == 2 {
						if bOp, ok := byteOpSubstitutions[parts[0]]; ok {
							text = bOp + " " + parts[1]
						}
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
				insts = append(insts, arch.DisassembledInst{
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
				insts = append(insts, arch.DisassembledInst{
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
					insts = append(insts, arch.DisassembledInst{
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
			align := uint64(defaultAVXAlign)
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
			if align < minStackAlign {
				align = minStackAlign
			}
			mask := align - 1
			if frameSize < align {
				frameSize = align
			} else {
				frameSize = (frameSize + mask) &^ mask
			}
		}

		insts = append(insts, arch.DisassembledInst{
			Offset:   pc,
			Length:   inst.Len,
			RawBytes: raw,
			Text:     text,
			Label:    jumpTargets[pc],
			IsRet:    inst.Op == x86asm.RET,
		})

		offset += inst.Len
	}

	return arch.DisassemblyResult{
		Insts:     insts,
		FrameSize: frameSize,
	}, nil
}

// CleanPlan9Syntax standardizes register and instruction syntax for Go's AMD64 assembler.
// It normalizes AT&T register syntax, strips redundant symbol qualifiers, standardizes NOPs
// and stack alignment directives, repairs 8-bit register naming artifacts, and applies
// table-driven CMOVcc, SETcc, and mnemonic conversions.
func CleanPlan9Syntax(s string) string {
	s = strings.TrimSpace(s)
	s = atntRegisterReplacer.Replace(s)

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
			if b8, ok := setcc8BitRegs[reg]; ok {
				s = op + " " + b8
			}
		}
	}

	// Fix 8-bit register suffixes/prefixes (e.g. CMPL DL, $160 -> CMPB DL, $160)
	for _, reg := range eightBitRegs {
		if strings.Contains(s, " "+reg+",") || strings.HasSuffix(s, ", "+reg) || strings.HasSuffix(s, " "+reg) {
			parts := strings.SplitN(s, " ", 2)
			if len(parts) == 2 {
				if bOp, ok := byteOpSubstitutions[parts[0]]; ok {
					s = bOp + " " + parts[1]
				}
			}
		}
	}

	// Mnemonic translations for Go's Plan 9 assembler
	parts := strings.SplitN(s, " ", 2)
	if len(parts) == 2 {
		op := parts[0]
		args := parts[1]

		if cc, ok := cmovConditions[op]; ok {
			if is64Dest(s) {
				s = "CMOVQ" + cc + " " + args
			} else {
				s = "CMOVL" + cc + " " + args
			}
		} else if repl, ok := setccMnemonicMap[op]; ok {
			s = repl + " " + args
		} else if repl, ok := mnemonicAliasesAMD64[op]; ok {
			s = repl + " " + args
		}
	}

	return s
}

// HasRexW reports whether the given instruction prefix byte is a REX prefix with the 64-bit operand size bit (REX.W) set.
func HasRexW(prefix byte) bool {
	return (prefix&rexPrefixMask) == rexPrefixBase && (prefix&rexWBit) != 0
}

// is64Dest reports whether the destination operand in a comma-separated instruction string is a 64-bit register.
func is64Dest(str string) bool {
	parts := strings.Split(str, ",")
	if len(parts) >= 2 {
		dest := strings.TrimSpace(parts[len(parts)-1])
		return slices.Contains(sixtyFourBitRegs, dest)
	}
	return false
}

// is8BitRegStr reports whether the given register name represents an 8-bit x86 register.
func is8BitRegStr(r string) bool {
	switch r {
	case "AL", "BL", "CL", "DL", "AH", "BH", "CH", "DH",
		"SIL", "DIL", "BPL", "SPL",
		"R8B", "R9B", "R10B", "R11B", "R12B", "R13B", "R14B", "R15B":
		return true
	}
	return false
}

// isBranchOp reports whether the specified x86asm opcode represents a branch or jump instruction.
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

// ReplaceIPWithROData rewrites instruction text containing (IP) or (RIP) relative memory operands
// to reference the local static read-only data symbol ·rodata<>(SB) with the given addend offset,
// maintaining proper operand spacing and comma separation.
func ReplaceIPWithROData(text string, addend int64) string {
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
		rawArgs := strings.Split(parts[1], ",")
		args := make([]string, len(rawArgs))
		for i, arg := range rawArgs {
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

// IsVZeroUpper reports whether the machine code at the given offset matches the 3-byte VZEROUPPER sequence.
func IsVZeroUpper(code []byte, offset int) bool {
	if offset+vzeroupperLen <= len(code) {
		return slices.Equal(code[offset:offset+vzeroupperLen], vzeroupperSeq)
	}
	return false
}
