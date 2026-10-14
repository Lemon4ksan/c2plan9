// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"debug/elf"
	"debug/macho"
	"debug/pe"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
)

// Symbol represents an extracted function symbol with its byte slice.
type Symbol struct {
	Name    string
	Offset  uint64
	Size    uint64
	Code    []byte
	IsLocal bool
}

// Relocation represents a relocation entry in the code referencing rodata or external symbols.
type Relocation struct {
	Offset   uint64 // byte offset in .text section
	SymName  string // referenced symbol name
	IsROData bool   // true if referencing .rodata section
	Addend   int64  // relocation addend
}

// ParsedObject contains the extracted machine code and symbol metadata.
type ParsedObject struct {
	Arch        string
	Symbols     []Symbol
	TextBytes   []byte
	ROData      []byte
	Relocations []Relocation
}

// ParseObject parses a binary object file (.o) across ELF, PE, or Mach-O formats.
func ParseObject(path string) (*ParsedObject, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read object file: %w", err)
	}

	// Try ELF first (standard output when targeting Linux/SysV)
	if obj, err := parseELF(data); err == nil {
		return obj, nil
	}

	// Try PE (Windows native)
	if obj, err := parsePE(data); err == nil {
		return obj, nil
	}

	// Try Mach-O (macOS native)
	if obj, err := parseMachO(data); err == nil {
		return obj, nil
	}

	return nil, fmt.Errorf("unsupported object file format: %s", path)
}

func parseELF(data []byte) (*ParsedObject, error) {
	f, err := elf.NewFile(bytesReaderAt(data))
	if err != nil {
		return nil, err
	}
	defer f.Close()

	textSec := f.Section(".text")
	if textSec == nil {
		return nil, errors.New("ELF object missing .text section")
	}

	textBytes, err := textSec.Data()
	if err != nil {
		return nil, fmt.Errorf("failed to read ELF .text data: %w", err)
	}

	var rodataBytes []byte
	for _, sec := range f.Sections {
		if strings.HasPrefix(sec.Name, ".rodata") {
			data, dErr := sec.Data()
			if dErr == nil {
				rodataBytes = append(rodataBytes, data...)
			}
		}
	}

	syms, err := f.Symbols()
	if err != nil {
		return nil, fmt.Errorf("failed to read ELF symbols: %w", err)
	}

	var funcSyms []Symbol
	for _, sym := range syms {
		if sym.Section == elf.SHN_UNDEF {
			continue
		}
		// Function or STT_NOTYPE symbols with non-zero size or in .text
		if int(sym.Section) < len(f.Sections) && f.Sections[sym.Section].Name == ".text" {
			if sym.Size > 0 {
				end := sym.Value + sym.Size
				if end <= uint64(len(textBytes)) {
					funcSyms = append(funcSyms, Symbol{
						Name:    sym.Name,
						Offset:  sym.Value,
						Size:    sym.Size,
						Code:    textBytes[sym.Value:end],
						IsLocal: elf.ST_BIND(sym.Info) == elf.STB_LOCAL,
					})
				}
			}
		}
	}

	// If no function sizes were explicitly stored in ELF symbols, deduce by sort offsets
	if len(funcSyms) == 0 {
		var textSymbols []elf.Symbol
		for _, sym := range syms {
			if int(sym.Section) < len(f.Sections) && f.Sections[sym.Section].Name == ".text" && sym.Name != "" {
				textSymbols = append(textSymbols, sym)
			}
		}
		sort.Slice(textSymbols, func(i, j int) bool {
			return textSymbols[i].Value < textSymbols[j].Value
		})

		for i, sym := range textSymbols {
			start := sym.Value
			var end uint64
			if i+1 < len(textSymbols) {
				end = textSymbols[i+1].Value
			} else {
				end = uint64(len(textBytes))
			}
			if end > start && end <= uint64(len(textBytes)) {
				funcSyms = append(funcSyms, Symbol{
					Name:    sym.Name,
					Offset:  start,
					Size:    end - start,
					Code:    textBytes[start:end],
					IsLocal: elf.ST_BIND(sym.Info) == elf.STB_LOCAL,
				})
			}
		}
	}

	var relocs []Relocation
	for _, sec := range f.Sections {
		if sec.Type == elf.SHT_RELA && sec.Info < uint32(len(f.Sections)) && f.Sections[sec.Info].Name == ".text" {
			relaData, rErr := sec.Data()
			if rErr == nil && len(relaData)%24 == 0 {
				r := bytes.NewReader(relaData)
				for i := 0; i < len(relaData)/24; i++ {
					var rOffset, rInfo uint64
					var rAddend int64
					_ = binary.Read(r, f.ByteOrder, &rOffset)
					_ = binary.Read(r, f.ByteOrder, &rInfo)
					_ = binary.Read(r, f.ByteOrder, &rAddend)

					symIdx := int(rInfo >> 32)
					var symName string
					isRO := false

					if symIdx >= 0 && symIdx < len(syms) {
						s := syms[symIdx]
						symName = s.Name
						if int(s.Section) < len(f.Sections) {
							targetSecName := f.Sections[s.Section].Name
							if strings.HasPrefix(targetSecName, ".rodata") {
								isRO = true
							}
						}
					}

					if !isRO && symIdx < len(f.Sections) {
						if strings.HasPrefix(f.Sections[symIdx].Name, ".rodata") {
							isRO = true
						}
					}

					relocs = append(relocs, Relocation{
						Offset:   rOffset,
						SymName:  symName,
						IsROData: isRO,
						Addend:   rAddend,
					})
				}
			}
		}
	}

	arch := "amd64"
	if f.Machine == elf.EM_AARCH64 {
		arch = "arm64"
	}

	return &ParsedObject{
		Arch:        arch,
		Symbols:     funcSyms,
		TextBytes:   textBytes,
		ROData:      rodataBytes,
		Relocations: relocs,
	}, nil
}

func parsePE(data []byte) (*ParsedObject, error) {
	f, err := pe.NewFile(bytesReaderAt(data))
	if err != nil {
		return nil, err
	}
	defer f.Close()

	textSec := f.Section(".text")
	if textSec == nil {
		return nil, errors.New("PE object missing .text section")
	}

	textBytes, err := textSec.Data()
	if err != nil {
		return nil, fmt.Errorf("failed to read PE .text data: %w", err)
	}

	var funcSyms []Symbol
	var peSymbols []*pe.Symbol
	for _, sym := range f.Symbols {
		if sym.SectionNumber > 0 && int(sym.SectionNumber) <= len(f.Sections) {
			if f.Sections[sym.SectionNumber-1].Name == ".text" && sym.Name != "" {
				peSymbols = append(peSymbols, sym)
			}
		}
	}

	sort.Slice(peSymbols, func(i, j int) bool {
		return peSymbols[i].Value < peSymbols[j].Value
	})

	for i, sym := range peSymbols {
		start := uint64(sym.Value)
		var end uint64
		if i+1 < len(peSymbols) {
			end = uint64(peSymbols[i+1].Value)
		} else {
			end = uint64(len(textBytes))
		}

		if end > start && end <= uint64(len(textBytes)) {
			funcSyms = append(funcSyms, Symbol{
				Name:   sym.Name,
				Offset: start,
				Size:   end - start,
				Code:   textBytes[start:end],
			})
		}
	}

	var rodataBytes []byte
	if roSec := f.Section(".rdata"); roSec != nil {
		rodataBytes, _ = roSec.Data()
	} else if roSec := f.Section(".rodata"); roSec != nil {
		rodataBytes, _ = roSec.Data()
	}

	var relocs []Relocation
	for _, rel := range textSec.Relocs {
		symIdx := int(rel.SymbolTableIndex)
		var symName string
		isRO := false
		if symIdx >= 0 && symIdx < len(f.Symbols) {
			s := f.Symbols[symIdx]
			symName = s.Name
			if s.SectionNumber > 0 && int(s.SectionNumber) <= len(f.Sections) {
				secName := f.Sections[s.SectionNumber-1].Name
				if strings.HasPrefix(secName, ".rdata") || strings.HasPrefix(secName, ".rodata") {
					isRO = true
				}
			}
		}
		relocs = append(relocs, Relocation{
			Offset:   uint64(rel.VirtualAddress),
			SymName:  symName,
			IsROData: isRO,
			Addend:   0,
		})
	}

	arch := "amd64"
	if f.Machine == pe.IMAGE_FILE_MACHINE_ARM64 {
		arch = "arm64"
	}

	return &ParsedObject{
		Arch:        arch,
		Symbols:     funcSyms,
		TextBytes:   textBytes,
		ROData:      rodataBytes,
		Relocations: relocs,
	}, nil
}

func parseMachO(data []byte) (*ParsedObject, error) {
	f, err := macho.NewFile(bytesReaderAt(data))
	if err != nil {
		return nil, err
	}
	defer f.Close()

	textSec := f.Section("__text")
	if textSec == nil {
		return nil, errors.New("Mach-O object missing __text section")
	}

	textBytes, err := textSec.Data()
	if err != nil {
		return nil, fmt.Errorf("failed to read Mach-O __text data: %w", err)
	}

	var rodataBytes []byte
	if roSec := f.Section("__rodata"); roSec != nil {
		rodataBytes, _ = roSec.Data()
	} else if roSec := f.Section("__const"); roSec != nil {
		rodataBytes, _ = roSec.Data()
	}

	arch := "amd64"
	if f.Cpu == macho.CpuArm64 {
		arch = "arm64"
	}

	var funcSyms []Symbol
	if f.Symtab != nil {
		for _, sym := range f.Symtab.Syms {
			if sym.Sect > 0 && sym.Name != "" {
				funcSyms = append(funcSyms, Symbol{
					Name:   sym.Name,
					Offset: sym.Value,
					Size:   uint64(len(textBytes)) - sym.Value,
					Code:   textBytes[sym.Value:],
				})
			}
		}
	}

	return &ParsedObject{
		Arch:      arch,
		Symbols:   funcSyms,
		TextBytes: textBytes,
		ROData:    rodataBytes,
	}, nil
}

type readerAtBytes []byte

func (r readerAtBytes) ReadAt(p []byte, off int64) (n int, err error) {
	if off >= int64(len(r)) {
		return 0, errors.New("offset out of bounds")
	}
	n = copy(p, r[off:])
	return n, nil
}

func bytesReaderAt(b []byte) readerAtBytes {
	return readerAtBytes(b)
}
