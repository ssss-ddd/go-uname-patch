package main

import (
    "bufio"
    "bytes"
    "debug/elf"
    "encoding/binary"
    "encoding/hex"
    "fmt"
    "os"
    "path/filepath"
    "strings"
)

var patchCode = []byte{
    0x00, 0x00, 0xa0, 0xe3, // MOV R0, #0
    0x0e, 0xf0, 0xa0, 0xe1, // MOV PC, LR
}

// 内置备用签名（Go 1.26.3 常见，用于没有配置文件的情况）
var fallbackSigs = []string{
    "08109ae54e2f4de2",
}

func main() {
    if len(os.Args) < 2 || len(os.Args) > 3 {
        printUsage()
        os.Exit(1)
    }

    inPath := os.Args[1]
    outPath := inPath
    if len(os.Args) == 3 {
        outPath = os.Args[2]
    }

    sigs := loadSignatures()

    data, err := os.ReadFile(inPath)
    if err != nil {
        fmt.Fprintf(os.Stderr, "Read error: %v\n", err)
        os.Exit(1)
    }

    ef, err := elf.NewFile(bytes.NewReader(data))
    if err != nil {
        fmt.Fprintf(os.Stderr, "Not a valid ELF: %v\n", err)
        os.Exit(1)
    }
    defer ef.Close()

    if ef.Class != elf.ELFCLASS32 || ef.Machine != elf.EM_ARM || ef.ByteOrder != binary.LittleEndian {
        fmt.Fprintf(os.Stderr, "Only 32-bit ARM little-endian ELF is supported\n")
        os.Exit(1)
    }

    // 1. 优先通过符号定位
    if fileOff, ok := trySymbol(ef, "runtime.getKernelVersion"); ok {
        applyPatch(data, fileOff, outPath, "symbol")
        return
    }

    fmt.Println("Symbol not found, searching by configured signatures...")

    // 2. 尝试所有配置的签名
    for _, sigHex := range sigs {
        sigBytes, err := hex.DecodeString(strings.TrimSpace(sigHex))
        if err != nil || len(sigBytes) != 8 {
            continue
        }
        for _, prog := range ef.Progs {
            if prog.Flags&elf.PF_X == 0 {
                continue
            }
            segData := data[prog.Off : prog.Off+prog.Filesz]
            idx := bytes.Index(segData, sigBytes)
            if idx != -1 {
                funcFileOff := prog.Off + uint64(idx)
                fmt.Printf("Found signature %s at file offset 0x%x\n", sigHex, funcFileOff)
                applyPatch(data, funcFileOff, outPath, "signature")
                return
            }
        }
    }

    fmt.Fprintf(os.Stderr, "No matching signature found.\n")
    fmt.Fprintf(os.Stderr, "Add the correct 8-byte hex to %s\n", sigFilePath())
    fmt.Fprintf(os.Stderr, "Use 'readelf -s' and 'xxd' on an unstripped Go binary to extract it.\n")
    os.Exit(1)
}

// printUsage 显示完整帮助，包括配置文件说明
func printUsage() {
    exe := os.Args[0]
    fmt.Fprintf(os.Stderr, "Patch Go 1.26+ binaries that crash on non-standard kernel version strings.\n\n")
    fmt.Fprintf(os.Stderr, "Usage:\n  %s <input_elf> [output_elf]\n\n", exe)
    fmt.Fprintf(os.Stderr, "If output_elf is omitted, the input file is patched in place.\n\n")
    fmt.Fprintf(os.Stderr, "Signature configuration:\n")
    fmt.Fprintf(os.Stderr, "  The tool looks for a file named 'signatures.conf' in the same directory\n")
    fmt.Fprintf(os.Stderr, "  as the executable (%s).\n", sigFilePath())
    fmt.Fprintf(os.Stderr, "  Each non-empty, non-comment line (comments start with #) must contain\n")
    fmt.Fprintf(os.Stderr, "  an 8-byte hex signature of the 'runtime.getKernelVersion' function.\n\n")
    fmt.Fprintf(os.Stderr, "How to obtain a new signature:\n")
    fmt.Fprintf(os.Stderr, "  1. Compile any simple Go program with the target Go version (do not strip).\n")
    fmt.Fprintf(os.Stderr, "  2. Run: readelf -s ./hi | grep getKernelVersion\n")
    fmt.Fprintf(os.Stderr, "  3. Convert the virtual address to file offset (use readelf -l).\n")
    fmt.Fprintf(os.Stderr, "  4. Extract 8 bytes: xxd -s <offset> -l 8 -p ./hi\n")
    fmt.Fprintf(os.Stderr, "  5. Append the hex string to signatures.conf\n\n")
    fmt.Fprintf(os.Stderr, "Example signatures.conf line:\n  08109ae54e2f4de2\n")
}

// 签名加载：优先从工具同目录下的 signatures.conf 读取，每行一个 hex 串
func loadSignatures() []string {
    path := sigFilePath()
    f, err := os.Open(path)
    if err != nil {
        fmt.Fprintf(os.Stderr, "Config %s not found, using built-in fallback signatures\n", path)
        return fallbackSigs
    }
    defer f.Close()
    var sigs []string
    scanner := bufio.NewScanner(f)
    for scanner.Scan() {
        line := strings.TrimSpace(scanner.Text())
        if line != "" && !strings.HasPrefix(line, "#") {
            sigs = append(sigs, line)
        }
    }
    if len(sigs) == 0 {
        fmt.Fprintf(os.Stderr, "Config %s is empty, using built-in fallback\n", path)
        return fallbackSigs
    }
    return sigs
}

func sigFilePath() string {
    exe, err := os.Executable()
    if err != nil {
        return "signatures.conf"
    }
    return filepath.Join(filepath.Dir(exe), "signatures.conf")
}

func trySymbol(ef *elf.File, name string) (uint64, bool) {
    syms, err := ef.Symbols()
    if err == nil {
        for _, s := range syms {
            if s.Name == name {
                if fileOff, err := vaddrToFileOffset(ef, s.Value); err == nil {
                    return fileOff, true
                }
            }
        }
    }
    syms, err = ef.DynamicSymbols()
    if err == nil {
        for _, s := range syms {
            if s.Name == name {
                if fileOff, err := vaddrToFileOffset(ef, s.Value); err == nil {
                    return fileOff, true
                }
            }
        }
    }
    return 0, false
}

func vaddrToFileOffset(ef *elf.File, vaddr uint64) (uint64, error) {
    for _, prog := range ef.Progs {
        if vaddr >= prog.Vaddr && vaddr < prog.Vaddr+prog.Filesz {
            return prog.Off + (vaddr - prog.Vaddr), nil
        }
    }
    for _, sec := range ef.Sections {
        if vaddr >= sec.Addr && vaddr < sec.Addr+sec.Size {
            return sec.Offset + (vaddr - sec.Addr), nil
        }
    }
    return 0, fmt.Errorf("no mapping")
}

func applyPatch(data []byte, funcOff uint64, outPath string, method string) {
    if funcOff+8 > uint64(len(data)) {
        fmt.Fprintf(os.Stderr, "Patch offset out of range\n")
        os.Exit(1)
    }
    orig := make([]byte, 8)
    copy(orig, data[funcOff:funcOff+8])
    copy(data[funcOff:], patchCode)

    if err := os.WriteFile(outPath, data, 0755); err != nil {
        fmt.Fprintf(os.Stderr, "Write error: %v\n", err)
        os.Exit(1)
    }
    fmt.Printf("Patched via %s at 0x%x:\n  original: % x\n  patched : % x\nSuccessfully wrote -> %s\n",
        method, funcOff, orig, patchCode, outPath)
}
