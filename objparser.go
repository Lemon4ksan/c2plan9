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
	"io"
	"os"
	"sort"
	"strings"

	"github.com/lemon4ksan/c2plan9/arch"
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
type Relocation = arch.Relocation

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
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("object file path cannot be empty")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read object file: %w", err)
	}

	if isELF(data) {
		obj, err := parseELF(data)
		if err != nil {
			return nil, fmt.Errorf("failed to parse ELF object %s: %w", path, err)
		}
		return obj, nil
	}

	if isPE(data) {
		obj, err := parsePE(data)
		if err != nil {
			return nil, fmt.Errorf("failed to parse PE object %s: %w", path, err)
		}
		return obj, nil
	}

	if isMachO(data) {
		obj, err := parseMachO(data)
		if err != nil {
			return nil, fmt.Errorf("failed to parse Mach-O object %s: %w", path, err)
		}
		return obj, nil
	}

	// Fallback trial in case magic bytes detection missed a rare format variant
	if obj, err := parseELF(data); err == nil {
		return obj, nil
	}
	if obj, err := parsePE(data); err == nil {
		return obj, nil
	}
	if obj, err := parseMachO(data); err == nil {
		return obj, nil
	}

	return nil, fmt.Errorf("unsupported object file format: %s", path)
}

func isELF(data []byte) bool {
	return len(data) >= 4 && data[0] == 0x7f && data[1] == 'E' && data[2] == 'L' && data[3] == 'F'
}

func isPE(data []byte) bool {
	return len(data) >= 2 && data[0] == 'M' && data[1] == 'Z'
}

func isMachO(data []byte) bool {
	if len(data) < 4 {
		return false
	}
	magic := binary.BigEndian.Uint32(data[0:4])
	return magic == 0xfeedface || magic == 0xcefaedfe || // 32-bit Mach-O (BE, LE)
		magic == 0xfeedfacf || magic == 0xcffaedfe || // 64-bit Mach-O (BE, LE)
		magic == 0xcafebabe || magic == 0xbebafeca // Universal/Fat binary
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
		secIdx := int(sym.Section)
		// Function or STT_NOTYPE symbols with non-zero size or in .text
		if secIdx >= 0 && secIdx < len(f.Sections) && f.Sections[secIdx].Name == ".text" {
			if sym.Size > 0 {
				// Guard against uint64 overflow: sym.Value + sym.Size
				if sym.Value > ^uint64(0)-sym.Size {
					continue
				}
				end := sym.Value + sym.Size
				if sym.Value < uint64(len(textBytes)) && end <= uint64(len(textBytes)) && end > sym.Value {
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
			secIdx := int(sym.Section)
			if secIdx >= 0 && secIdx < len(f.Sections) && f.Sections[secIdx].Name == ".text" && sym.Name != "" {
				textSymbols = append(textSymbols, sym)
			}
		}
		sort.Slice(textSymbols, func(i, j int) bool {
			return textSymbols[i].Value < textSymbols[j].Value
		})

		for i, sym := range textSymbols {
			start := sym.Value
			if start >= uint64(len(textBytes)) {
				continue
			}
			var end uint64
			if i+1 < len(textSymbols) {
				end = textSymbols[i+1].Value
			} else {
				end = uint64(len(textBytes))
			}
			if end > uint64(len(textBytes)) {
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
			if rErr == nil && len(relaData) >= 24 && len(relaData)%24 == 0 && f.ByteOrder != nil {
				r := bytes.NewReader(relaData)
				for i := 0; i < len(relaData)/24; i++ {
					var rOffset, rInfo uint64
					var rAddend int64
					if err := binary.Read(r, f.ByteOrder, &rOffset); err != nil {
						break
					}
					if err := binary.Read(r, f.ByteOrder, &rInfo); err != nil {
						break
					}
					if err := binary.Read(r, f.ByteOrder, &rAddend); err != nil {
						break
					}

					symIdx := int(rInfo >> 32)
					var symName string
					isRO := false

					if symIdx >= 0 && symIdx < len(syms) {
						s := syms[symIdx]
						symName = s.Name
						sSec := int(s.Section)
						if sSec >= 0 && sSec < len(f.Sections) {
							targetSecName := f.Sections[sSec].Name
							if strings.HasPrefix(targetSecName, ".rodata") {
								isRO = true
							}
						}
					}

					if !isRO && symIdx >= 0 && symIdx < len(f.Sections) {
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
		if start >= uint64(len(textBytes)) {
			continue
		}
		var end uint64
		if i+1 < len(peSymbols) {
			end = uint64(peSymbols[i+1].Value)
		} else {
			end = uint64(len(textBytes))
		}
		if end > uint64(len(textBytes)) {
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
		// Identify the 1-based section index of the __text section
		var textSectIndex uint8
		for idx, sec := range f.Sections {
			if sec.Name == "__text" {
				textSectIndex = uint8(idx + 1)
				break
			}
		}

		// Filter symbols belonging strictly to the __text section
		var textSyms []macho.Symbol
		for _, sym := range f.Symtab.Syms {
			if textSectIndex > 0 && sym.Sect == textSectIndex && sym.Name != "" {
				textSyms = append(textSyms, sym)
			}
		}

		sort.Slice(textSyms, func(i, j int) bool {
			return textSyms[i].Value < textSyms[j].Value
		})

		for i, sym := range textSyms {
			offset := sym.Value
			if offset >= textSec.Addr && textSec.Addr > 0 {
				offset -= textSec.Addr
			}
			if offset >= uint64(len(textBytes)) {
				continue
			}

			var end uint64
			if i+1 < len(textSyms) {
				nextOffset := textSyms[i+1].Value
				if nextOffset >= textSec.Addr && textSec.Addr > 0 {
					nextOffset -= textSec.Addr
				}
				if nextOffset > offset && nextOffset <= uint64(len(textBytes)) {
					end = nextOffset
				} else {
					end = uint64(len(textBytes))
				}
			} else {
				end = uint64(len(textBytes))
			}

			if end > offset && end <= uint64(len(textBytes)) {
				funcSyms = append(funcSyms, Symbol{
					Name:   sym.Name,
					Offset: offset,
					Size:   end - offset,
					Code:   textBytes[offset:end],
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
	if off < 0 {
		return 0, errors.New("negative offset out of bounds")
	}
	if off >= int64(len(r)) {
		return 0, io.EOF
	}
	n = copy(p, r[off:])
	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
}

func bytesReaderAt(b []byte) readerAtBytes {
	return readerAtBytes(b)
}
