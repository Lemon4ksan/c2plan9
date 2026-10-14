## Description

Provide a concise summary of the change. Link any related issues below.

Fixes # (issue)

## Type of Change

- [ ] Bug fix (non-breaking change which fixes an opcode or alignment issue)
- [ ] New feature (e.g. new instruction disassembler or target architecture)
- [ ] Performance improvement
- [ ] Refactoring (no functional changes)
- [ ] Documentation update

## Checklist

- [ ] I have run `make lint` — no new linter warnings (0 issues reported)
- [ ] I have run `make test` or `make race` — all unit tests pass cleanly
- [ ] Generated Plan 9 assembly is bit-for-bit deterministic
- [ ] Go ABI stack frame sizing and register clobbering invariants are preserved
- [ ] Commits follow Conventional Commits formatting (`feat:`, `fix:`, `perf:`, `docs:`)
- [ ] Public CLI flags and stub generation APIs are documented in `README.md` / `README_RU.md`

## How has this been tested?

Describe how you verified the changes work:

```bash
go test -race ./...
```
