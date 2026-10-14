# Security Policy

## Supported Versions

| Version | Supported |
|---------|-----------|
| latest  | ✅        |

Only the latest release receives security updates and vulnerability patches. We recommend running the most recent version of `c2plan9`.

## Security Model & Code Generation Safety

`c2plan9` executes external toolchains and parses binary ELF/Mach-O/PE object code to generate assembly:

1. **Stack Alignment & Frame Safety:** The emitter guarantees that stack frame allocations and dynamic alignment directives (`ANDQ $-32, SP`) strictly adhere to the Go ABI and prevent stack-smashing or misaligned vector exceptions.
2. **Object Parser Boundaries:** ELF, PE, and Mach-O section header decoders enforce bounds checking on offsets and section sizes to prevent out-of-bounds reads on crafted binaries.
3. **Command Execution Hygiene:** Invocations of Clang validate source paths and compiler flags to prevent command injection.

## Reporting a Vulnerability

**Please do not open a public GitHub issue for security vulnerabilities.**

Report security issues privately via GitHub's built-in
[Security Advisories](https://github.com/lemon4ksan/c2plan9/security/advisories/new)
feature (Security → Advisories → New draft advisory).

Include as much of the following as you can:

- A description of the vulnerability and its potential impact (e.g. malformed assembly generation, stack boundary violation, parser panic, arbitrary execution vector)
- Steps to reproduce (C source snippet or minimal reproduction harness)
- The version of `c2plan9`, Clang version, and Go version
- Target architecture (AMD64 or ARM64) and operating system

We will acknowledge receipt within **72 hours** and aim to release a fix within
**14 days** for confirmed critical issues.
