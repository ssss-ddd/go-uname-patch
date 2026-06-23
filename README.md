# go-uname-patch

**Fix Go 1.26+ binaries that crash on non‑standard Linux kernel version strings**  
**修复因非标准 Linux 内核版本字符串导致 Go 1.26+ 程序崩溃的工具**

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

---

## English

### Why this tool?

Starting from **Go 1.26**, the runtime strictly parses the output of `uname -r` (or `/proc/sys/kernel/osrelease`).  
If the kernel version string does **not** follow the standard `X.Y.Z[-something]` format (for example `4.9.118_gk6323v100c` with an underscore), the program will abort immediately with:
fatal error: failed to parse kernel version from uname

This often happens on:

- Android TV boxes / set‑top boxes running custom Linux kernels
- Embedded devices with vendor‑specific kernel strings
- Termux environments on devices with non‑standard kernel versions

The Go runtime calls `runtime.throw()` during initialisation, and there is **no way** to work around it without modifying the binary.

### How it works

`patch_kernel` modifies the first 8 bytes of the `runtime.getKernelVersion` function inside an ELF binary, replacing them with:
MOV R0, #0 ; return 0
MOV PC, LR ; return to caller

This effectively makes the function return a default value without ever trying to parse the kernel version string.  
No source code, root access, or recompilation is needed.

The tool locates the function in two ways (automatically tried in order):

1. **By symbol** – if the binary is not stripped, `runtime.getKernelVersion` is found by name.
2. **By configurable byte signature** – a file named `signatures.conf` (placed next to the tool) contains 8‑byte hex signatures of the function's first instructions. The tool searches for these bytes in executable segments. This works even for stripped binaries.

### Usage

Download the pre‑compiled `patch_kernel` binary from [Releases](https://github.com/yourname/go-uname-patch/releases), or build it yourself (see below).

#### Basic patching


./patch_kernel /path/to/program
If you want to keep the original file unchanged:
./patch_kernel /path/to/program ./patched_program
The patched program will run normally, no longer crashing on the kernel version check.

Signature configuration
The tool reads an optional signatures.conf file from the same directory as the patch_kernel binary.

Each non‑empty, non‑comment line (comments start with #) must contain an 8‑byte hex signature.

Example:
# Go 1.26.3 ARMv7
08109ae54e2f4de2
How to obtain a new signature (e.g. when a new Go version is released):

Compile a simple Go program with the target Go version (do not strip it):
echo 'package main; import "fmt"; func main(){ fmt.Println("hi") }' > hi.go
go build -o hi hi.go
Find the virtual address of runtime.getKernelVersion:
readelf -s ./hi | grep getKernelVersion
Convert that virtual address to a file offset (use readelf -l and calculate:
file_offset = segment_offset + (symbol_vaddr - segment_vaddr)).

Extract 8 bytes from that offset:
xxd -s <offset> -l 8 -p ./hi
Append the resulting hex string to signatures.conf.

That’s it – you never need to recompile the tool itself.

Building from source
You need Go 1.22 or 1.23 (which do not have the strict kernel version check) to build the tool.
GOOS=linux GOARCH=arm GOARM=7 go build -o patch_kernel patch_kernel.go
License
MIT – see LICENSE.


## 中文
### 为什么需要这个工具？
从 Go 1.26 开始，运行时严格解析 uname -r（或 /proc/sys/kernel/osrelease）的输出。
如果内核版本字符串不符合 X.Y.Z[-something] 标准格式（例如 4.9.118_gk6323v100c 包含下划线），程序会在启动阶段立刻崩溃，并输出：
fatal error: failed to parse kernel version from uname
这种情况常见于：

安卓电视盒子 / 机顶盒（运行定制 Linux 内核）

嵌入式设备（厂商自定义内核版本字符串）

Termux 环境（特别是内核版本带有非标准字符的设备）

Go 运行时在初始化时会调用 runtime.throw()，无法通过环境变量或配置绕过，只能修改二进制文件。

实现原理
patch_kernel 修改 ELF 二进制文件中 runtime.getKernelVersion 函数的前 8 字节，替换为：
MOV R0, #0    ; 返回值置为 0
MOV PC, LR    ; 立即返回调用者
这使得该函数直接返回默认值，不再尝试解析内核版本字符串。
整个过程不需要源代码、root 权限，也不需要重新编译目标程序。

工具通过两种方式自动定位函数（按顺序尝试）：

通过符号 – 如果程序未被 strip，直接通过名称 runtime.getKernelVersion 找到函数。

通过可配置的字节签名 – 工具同目录下的 signatures.conf 文件存放着函数开头 8 字节的十六进制签名。工具在可执行段中搜索这些字节，即使程序被 strip 也能成功定位。

使用方法
从 Releases 下载预编译的 patch_kernel 二进制，或自行编译（见下文）。

基本修补
./patch_kernel /path/to/program
如果想保留原文件：
./patch_kernel /path/to/program ./patched_program
修补后的程序即可正常运行，不会再因内核版本检查而崩溃。

签名配置
工具会读取与 patch_kernel 二进制同目录下的 signatures.conf 文件（可选）。

每行一个 8 字节十六进制签名，以 # 开头的行为注释。

示例：
# Go 1.26.3 ARMv7
08109ae54e2f4de2
如何获取新签名（例如 Go 发布新版本后）：

用目标 Go 版本编译一个简单的程序（不要 strip）：
echo 'package main; import "fmt"; func main(){ fmt.Println("hi") }' > hi.go
go build -o hi hi.go
找到 runtime.getKernelVersion 的虚拟地址：
readelf -s ./hi | grep getKernelVersion
将虚拟地址转换为文件偏移（使用 readelf -l 计算：
文件偏移 = 段偏移 + (符号地址 - 段虚拟地址)）。

提取 8 字节：
xxd -s <偏移> -l 8 -p ./hi
将得到的十六进制字符串追加到 signatures.conf。

完成以上步骤后，工具便能修补该版本编译的任何程序，无需重新编译工具本身。

从源码编译
需要使用 Go 1.22 或 1.23（这些版本无严格内核版本检查）来编译本工具。
GOOS=linux GOARCH=arm GOARM=7 go build -o patch_kernel patch_kernel.go
许可证
MIT – 详见 LICENSE。

⭐ Star & Contributions
If this tool helped you, please consider giving it a ⭐ on GitHub – it means a lot!
如果这个工具帮助到了你，请给一颗 ⭐，赠人玫瑰，手有余香 🌹

Pull requests and new signatures for additional Go versions are highly welcome!
