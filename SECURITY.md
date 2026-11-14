# Security Policy

Report vulnerabilities by opening a GitHub issue with minimal reproduction, or by contacting the maintainer through the profile on the repository owner account.

Notes on the security model:

- HTML output is escaped by default using handlebars.js' exact escape map. `{{{triple}}}`, `{{& amp}}` and `Options.NoEscape` bypass it by design.
- Registered helpers run with your program's privileges. Treat helper code as code.
- The parser is fuzz-tested (see `fuzz_test.go`), but this library is early-stage and has not been independently audited.
