// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestSeal_ReaderAtBytes_NegativeOffset_PanicPoC verifies that readerAtBytes
// adheres strictly to io.ReaderAt semantics and does not panic on negative offsets.
func TestSeal_ReaderAtBytes_NegativeOffset_PanicPoC(t *testing.T) {
	data := []byte("defensive-seal-invariant-testing")
	r := bytesReaderAt(data)

	// 1. Negative offset must return error, NOT panic with slice bounds out of range
	buf := make([]byte, 8)
	n, err := r.ReadAt(buf, -1)
	if err == nil {
		t.Fatal("expected error on negative offset -1, got nil")
	}
	if n != 0 {
		t.Fatalf("expected 0 bytes read on negative offset, got %d", n)
	}

	n, err = r.ReadAt(buf, -100)
	if err == nil {
		t.Fatal("expected error on negative offset -100, got nil")
	}
	if n != 0 {
		t.Fatalf("expected 0 bytes read on negative offset, got %d", n)
	}

	// 2. Out-of-bounds offset must return io.EOF per io.ReaderAt contract
	n, err = r.ReadAt(buf, int64(len(data)))
	if !errors.Is(err, io.EOF) {
		t.Fatalf("expected io.EOF on offset == len, got %v", err)
	}
	if n != 0 {
		t.Fatalf("expected 0 bytes read on EOF, got %d", n)
	}

	// 3. Partial read at end of buffer must return io.EOF per io.ReaderAt contract
	largeBuf := make([]byte, 32)
	n, err = r.ReadAt(largeBuf, int64(len(data)-4))
	if !errors.Is(err, io.EOF) {
		t.Fatalf("expected io.EOF on partial read, got %v", err)
	}
	if n != 4 {
		t.Fatalf("expected 4 bytes read on partial read, got %d", n)
	}

	// 4. Exact valid read must return nil error
	exactBuf := make([]byte, 4)
	n, err = r.ReadAt(exactBuf, 0)
	if err != nil {
		t.Fatalf("expected nil error on valid read, got %v", err)
	}
	if n != 4 {
		t.Fatalf("expected 4 bytes read, got %d", n)
	}
}

// TestSeal_ELF_IntegerOverflow_PanicPoC tests that an ELF binary with an
// arithmetic overflow (sym.Value + sym.Size wrapping uint64) does not panic.
func TestSeal_ELF_IntegerOverflow_PanicPoC(t *testing.T) {
	elfBytes := buildSyntheticELF(t, ^uint64(0), 2) // sym.Value = max uint64, sym.Size = 2 -> wraps to 1

	tmpDir := t.TempDir()
	elfPath := filepath.Join(tmpDir, "overflow.o")
	if err := os.WriteFile(elfPath, elfBytes, 0o644); err != nil {
		t.Fatalf("failed to write synthetic ELF: %v", err)
	}

	// Must parse cleanly without panic!
	obj, err := ParseObject(elfPath)
	if err != nil {
		t.Fatalf("ParseObject failed on synthetic ELF: %v", err)
	}
	if obj.Arch != "amd64" {
		t.Errorf("expected amd64 arch, got %s", obj.Arch)
	}
	// The overflowing symbol must be rejected/omitted, preventing OOB access
	if len(obj.Symbols) != 0 {
		t.Fatalf("expected 0 valid symbols due to overflow rejection, got %d", len(obj.Symbols))
	}
}

// TestSeal_MachO_OOB_Symbol_PanicPoC tests that a Mach-O binary with an out-of-bounds
// symbol value does not panic or underflow uint64 when computing symbol size.
func TestSeal_MachO_OOB_Symbol_PanicPoC(t *testing.T) {
	machoBytes := buildSyntheticMachO(t, 9999) // sym.Value = 9999 (well past 16-byte __text)

	tmpDir := t.TempDir()
	machoPath := filepath.Join(tmpDir, "oob.o")
	if err := os.WriteFile(machoPath, machoBytes, 0o644); err != nil {
		t.Fatalf("failed to write synthetic Mach-O: %v", err)
	}

	// Must parse cleanly without panic!
	obj, err := ParseObject(machoPath)
	if err != nil {
		t.Fatalf("ParseObject failed on synthetic Mach-O: %v", err)
	}
	if obj.Arch != "amd64" {
		t.Errorf("expected amd64 arch, got %s", obj.Arch)
	}
	// The OOB symbol must be safely omitted rather than causing a panic
	if len(obj.Symbols) != 0 {
		t.Fatalf("expected 0 valid symbols due to OOB rejection, got %d", len(obj.Symbols))
	}
}

// TestSeal_Compiler_InputValidation verifies input sanitization, directory checking,
// architecture allowlisting, and flag validation in compiler.go.
func TestSeal_Compiler_InputValidation(t *testing.T) {
	// 1. Empty source file
	_, err := Compile(CompileOptions{SourceFile: ""})
	if err == nil || !strings.Contains(err.Error(), "source file path cannot be empty") {
		t.Fatalf("expected empty source file error, got: %v", err)
	}

	// 2. Non-existent source file
	_, err = Compile(CompileOptions{SourceFile: "non_existent_seal_file.c"})
	if err == nil || !strings.Contains(err.Error(), "not found or inaccessible") {
		t.Fatalf("expected inaccessible file error, got: %v", err)
	}

	// 3. Directory passed as source file
	tmpDir := t.TempDir()
	_, err = Compile(CompileOptions{SourceFile: tmpDir})
	if err == nil || !strings.Contains(err.Error(), "is a directory") {
		t.Fatalf("expected directory error, got: %v", err)
	}

	// 4. Unsupported target architecture
	dummySrc := filepath.Join(tmpDir, "dummy.c")
	if err := os.WriteFile(dummySrc, []byte("int x;"), 0o644); err != nil {
		t.Fatalf("failed to write dummy source: %v", err)
	}

	_, err = Compile(CompileOptions{
		SourceFile: dummySrc,
		TargetArch: "; evil_injection",
	})
	if err == nil || !strings.Contains(err.Error(), "unsupported target architecture") {
		t.Fatalf("expected architecture error, got: %v", err)
	}

	// 5. Null byte in extra flags
	_, err = Compile(CompileOptions{
		SourceFile: dummySrc,
		TargetArch: "amd64",
		ExtraFlags: []string{"-O3", "-DTEST\x00INJECT"},
	})
	if err == nil || !strings.Contains(err.Error(), "null byte") {
		t.Fatalf("expected null byte flag error, got: %v", err)
	}

	// 6. Context cancellation support in CompileContext
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err = CompileContext(ctx, CompileOptions{
		SourceFile: dummySrc,
		TargetArch: "amd64",
	})
	if err == nil {
		t.Fatal("expected error on cancelled context, got nil")
	}
}

// TestSeal_ToWSLPath verifies path translation edge cases for WSL clang invocations.
func TestSeal_ToWSLPath(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{`C:\project\kernel.c`, `/mnt/c/project/kernel.c`},
		{`c:\test.c`, `/mnt/c/test.c`},
		{`D:\code\main.c`, `/mnt/d/code/main.c`},
		{`C:kernel.c`, `/mnt/c/kernel.c`}, // Drive-relative path invariant fix
		{`C:\`, `/mnt/c`},
		{`/mnt/c/linux/path.c`, `/mnt/c/linux/path.c`},
	}

	for _, tc := range tests {
		got := toWSLPath(tc.input)
		if got != tc.expected {
			t.Errorf("toWSLPath(%q) = %q, want %q", tc.input, got, tc.expected)
		}
	}
}

// TestSeal_ParseObject_ErrorSanitizationAndMagic verifies that ParseObject inspects
// magic bytes and produces descriptive format-specific errors instead of generic swallows.
func TestSeal_ParseObject_ErrorSanitizationAndMagic(t *testing.T) {
	tmpDir := t.TempDir()

	// Empty path
	_, err := ParseObject("")
	if err == nil || !strings.Contains(err.Error(), "empty") {
		t.Fatalf("expected empty path error, got %v", err)
	}

	// Corrupted ELF
	corruptedELF := filepath.Join(tmpDir, "bad.elf")
	_ = os.WriteFile(corruptedELF, []byte("\x7fELF\x02\x01\x01\x00corrupted-payload"), 0o644)
	_, err = ParseObject(corruptedELF)
	if err == nil || !strings.Contains(err.Error(), "failed to parse ELF object") {
		t.Fatalf("expected descriptive ELF error, got %v", err)
	}

	// Corrupted PE
	corruptedPE := filepath.Join(tmpDir, "bad.exe")
	_ = os.WriteFile(corruptedPE, []byte("MZ\x90\x00corrupted-pe-payload"), 0o644)
	_, err = ParseObject(corruptedPE)
	if err == nil || !strings.Contains(err.Error(), "failed to parse PE object") {
		t.Fatalf("expected descriptive PE error, got %v", err)
	}

	// Corrupted Mach-O
	corruptedMachO := filepath.Join(tmpDir, "bad.macho")
	_ = os.WriteFile(corruptedMachO, []byte("\xcf\xfa\xed\xfe\x07\x00\x00\x01corrupted-macho"), 0o644)
	_, err = ParseObject(corruptedMachO)
	if err == nil || !strings.Contains(err.Error(), "failed to parse Mach-O object") {
		t.Fatalf("expected descriptive Mach-O error, got %v", err)
	}
}

// Helper to construct a minimal valid 64-bit ELF object with a test symbol
func buildSyntheticELF(t *testing.T, symVal, symSize uint64) []byte {
	t.Helper()
	var buf bytes.Buffer

	// Section data
	textBytes := []byte{0x90, 0x90, 0x90, 0x90, 0x90, 0x90, 0x90, 0x90, 0x90, 0x90, 0x90, 0x90, 0x90, 0x90, 0x90, 0xC3}
	shstrtab := "\x00.shstrtab\x00.text\x00.strtab\x00.symtab\x00"
	strtab := "\x00vulnerable_func\x00"

	// Symbol table entries (Entry 0: STN_UNDEF, Entry 1: vulnerable_func)
	var symtab bytes.Buffer
	// Entry 0 (all zeros)
	symtab.Write(make([]byte, 24))
	// Entry 1
	_ = binary.Write(&symtab, binary.LittleEndian, uint32(1)) // st_name (offset 1 in strtab)
	symtab.WriteByte(0x12)                                    // st_info (STB_GLOBAL | STT_FUNC)
	symtab.WriteByte(0)                                       // st_other
	_ = binary.Write(&symtab, binary.LittleEndian, uint16(2)) // st_shndx (section 2 is .text)
	_ = binary.Write(&symtab, binary.LittleEndian, symVal)    // st_value
	_ = binary.Write(&symtab, binary.LittleEndian, symSize)   // st_size

	// Calculate layout:
	// 0..64: ELF Header
	// 64..64+len(text): .text
	// next: .shstrtab
	// next: .strtab
	// next: .symtab
	// next: Section Header Table (5 headers * 64 bytes)
	offsetText := uint64(64)
	sizeText := uint64(len(textBytes))

	offsetShstrtab := offsetText + sizeText
	sizeShstrtab := uint64(len(shstrtab))

	offsetStrtab := offsetShstrtab + sizeShstrtab
	sizeStrtab := uint64(len(strtab))

	offsetSymtab := offsetStrtab + sizeStrtab
	sizeSymtab := uint64(symtab.Len())

	offsetSht := offsetSymtab + sizeSymtab
	numSections := uint16(5)

	// ELF Header
	ehdr := make([]byte, 64)
	copy(ehdr[0:4], []byte{0x7F, 'E', 'L', 'F'})
	ehdr[4] = 2                                           // ELFCLASS64
	ehdr[5] = 1                                           // ELFDATA2LSB
	ehdr[6] = 1                                           // EV_CURRENT
	binary.LittleEndian.PutUint16(ehdr[16:], 1)           // ET_REL
	binary.LittleEndian.PutUint16(ehdr[18:], 62)          // EM_X86_64
	binary.LittleEndian.PutUint32(ehdr[20:], 1)           // EV_CURRENT
	binary.LittleEndian.PutUint64(ehdr[40:], offsetSht)   // e_shoff
	binary.LittleEndian.PutUint16(ehdr[52:], 64)          // e_ehsize
	binary.LittleEndian.PutUint16(ehdr[58:], 64)          // e_shentsize
	binary.LittleEndian.PutUint16(ehdr[60:], numSections) // e_shnum
	binary.LittleEndian.PutUint16(ehdr[62:], 1)           // e_shstrndx (.shstrtab is index 1)

	buf.Write(ehdr)
	buf.Write(textBytes)
	buf.WriteString(shstrtab)
	buf.WriteString(strtab)
	buf.Write(symtab.Bytes())

	// Section Headers:
	// Sec 0: SHN_UNDEF
	buf.Write(make([]byte, 64))

	// Sec 1: .shstrtab
	sec1 := make([]byte, 64)
	binary.LittleEndian.PutUint32(sec1[0:], 1)               // sh_name (".shstrtab")
	binary.LittleEndian.PutUint32(sec1[4:], 3)               // SHT_STRTAB
	binary.LittleEndian.PutUint64(sec1[24:], offsetShstrtab) // sh_offset
	binary.LittleEndian.PutUint64(sec1[32:], sizeShstrtab)   // sh_size
	binary.LittleEndian.PutUint64(sec1[56:], 1)              // sh_addralign
	buf.Write(sec1)

	// Sec 2: .text
	sec2 := make([]byte, 64)
	binary.LittleEndian.PutUint32(sec2[0:], 11)          // sh_name (".text")
	binary.LittleEndian.PutUint32(sec2[4:], 1)           // SHT_PROGBITS
	binary.LittleEndian.PutUint64(sec2[8:], 6)           // SHF_ALLOC | SHF_EXECINSTR
	binary.LittleEndian.PutUint64(sec2[24:], offsetText) // sh_offset
	binary.LittleEndian.PutUint64(sec2[32:], sizeText)   // sh_size
	binary.LittleEndian.PutUint64(sec2[56:], 16)         // sh_addralign
	buf.Write(sec2)

	// Sec 3: .strtab
	sec3 := make([]byte, 64)
	binary.LittleEndian.PutUint32(sec3[0:], 17)            // sh_name (".strtab")
	binary.LittleEndian.PutUint32(sec3[4:], 3)             // SHT_STRTAB
	binary.LittleEndian.PutUint64(sec3[24:], offsetStrtab) // sh_offset
	binary.LittleEndian.PutUint64(sec3[32:], sizeStrtab)   // sh_size
	binary.LittleEndian.PutUint64(sec3[56:], 1)            // sh_addralign
	buf.Write(sec3)

	// Sec 4: .symtab
	sec4 := make([]byte, 64)
	binary.LittleEndian.PutUint32(sec4[0:], 25)            // sh_name (".symtab")
	binary.LittleEndian.PutUint32(sec4[4:], 2)             // SHT_SYMTAB
	binary.LittleEndian.PutUint64(sec4[24:], offsetSymtab) // sh_offset
	binary.LittleEndian.PutUint64(sec4[32:], sizeSymtab)   // sh_size
	binary.LittleEndian.PutUint32(sec4[40:], 3)            // sh_link (points to .strtab index 3)
	binary.LittleEndian.PutUint32(sec4[44:], 1)            // sh_info (one local symbol)
	binary.LittleEndian.PutUint64(sec4[56:], 8)            // sh_addralign
	binary.LittleEndian.PutUint64(sec4[48:], 24)           // sh_entsize
	buf.Write(sec4)

	return buf.Bytes()
}

// Helper to construct a minimal valid 64-bit Mach-O object with a test symbol
func buildSyntheticMachO(t *testing.T, symVal uint64) []byte {
	t.Helper()
	var buf bytes.Buffer

	textBytes := []byte{0x90, 0x90, 0x90, 0x90, 0x90, 0x90, 0x90, 0x90, 0x90, 0x90, 0x90, 0x90, 0x90, 0x90, 0x90, 0xC3}
	strtab := "\x00_test_macho_func\x00"

	// Layout:
	// 0..32: Header
	// 32..184: LC_SEGMENT_64 (72 bytes cmd + 80 bytes section) = 152 bytes
	// 184..208: LC_SYMTAB (24 bytes)
	// Total commands size = 176 bytes
	// 208..224: textBytes (16 bytes)
	// 224..240: nlist_64 (16 bytes)
	// 240..258: strtab (18 bytes)

	offsetText := uint32(208)
	offsetSym := uint32(224)
	offsetStr := uint32(240)

	// 1. Mach-O Header (32 bytes)
	hdr := make([]byte, 32)
	binary.LittleEndian.PutUint32(hdr[0:], 0xfeedfacf) // MH_MAGIC_64
	binary.LittleEndian.PutUint32(hdr[4:], 0x01000007) // CPU_TYPE_X86_64
	binary.LittleEndian.PutUint32(hdr[8:], 0x80000003) // CPU_SUBTYPE_LIB64
	binary.LittleEndian.PutUint32(hdr[12:], 1)         // MH_OBJECT
	binary.LittleEndian.PutUint32(hdr[16:], 2)         // ncmds (LC_SEGMENT_64 + LC_SYMTAB)
	binary.LittleEndian.PutUint32(hdr[20:], 176)       // sizeofcmds (152 + 24)
	binary.LittleEndian.PutUint32(hdr[24:], 0)         // flags
	buf.Write(hdr)

	// 2. LC_SEGMENT_64 Command (72 bytes + 80 bytes = 152 bytes)
	segCmd := make([]byte, 72)
	binary.LittleEndian.PutUint32(segCmd[0:], 0x19) // LC_SEGMENT_64
	binary.LittleEndian.PutUint32(segCmd[4:], 152)  // cmdsize
	// segname is empty for MH_OBJECT
	binary.LittleEndian.PutUint64(segCmd[24:], 0)                      // vmaddr
	binary.LittleEndian.PutUint64(segCmd[32:], uint64(len(textBytes))) // vmsize
	binary.LittleEndian.PutUint64(segCmd[40:], uint64(offsetText))     // fileoff
	binary.LittleEndian.PutUint64(segCmd[48:], uint64(len(textBytes))) // filesize
	binary.LittleEndian.PutUint32(segCmd[56:], 7)                      // maxprot
	binary.LittleEndian.PutUint32(segCmd[60:], 7)                      // initprot
	binary.LittleEndian.PutUint32(segCmd[64:], 1)                      // nsects
	buf.Write(segCmd)

	// Section __text (80 bytes)
	sec := make([]byte, 80)
	copy(sec[0:], "__text")
	copy(sec[16:], "__TEXT")
	binary.LittleEndian.PutUint64(sec[32:], 0)                      // addr
	binary.LittleEndian.PutUint64(sec[40:], uint64(len(textBytes))) // size
	binary.LittleEndian.PutUint32(sec[48:], offsetText)             // offset
	binary.LittleEndian.PutUint32(sec[52:], 4)                      // align
	binary.LittleEndian.PutUint32(
		sec[64:],
		0x80000400,
	) // flags (S_ATTR_PURE_INSTRUCTIONS | S_ATTR_SOME_INSTRUCTIONS)
	buf.Write(sec)

	// 3. LC_SYMTAB Command (24 bytes)
	symCmd := make([]byte, 24)
	binary.LittleEndian.PutUint32(symCmd[0:], 2)                    // LC_SYMTAB
	binary.LittleEndian.PutUint32(symCmd[4:], 24)                   // cmdsize
	binary.LittleEndian.PutUint32(symCmd[8:], offsetSym)            // symoff
	binary.LittleEndian.PutUint32(symCmd[12:], 1)                   // nsyms
	binary.LittleEndian.PutUint32(symCmd[16:], offsetStr)           // stroff
	binary.LittleEndian.PutUint32(symCmd[20:], uint32(len(strtab))) // strsize
	buf.Write(symCmd)

	// 4. Section Data
	buf.Write(textBytes)

	// 5. Symbol Table (nlist_64: 16 bytes)
	nlist := make([]byte, 16)
	binary.LittleEndian.PutUint32(nlist[0:], 1)      // n_strx (offset 1 in strtab)
	nlist[4] = 0x0f                                  // n_type (N_SECT | N_EXT)
	nlist[5] = 1                                     // n_sect (section 1: __text)
	binary.LittleEndian.PutUint16(nlist[6:], 0)      // n_desc
	binary.LittleEndian.PutUint64(nlist[8:], symVal) // n_value (OOB or normal)
	buf.Write(nlist)

	// 6. String Table
	buf.WriteString(strtab)

	return buf.Bytes()
}
