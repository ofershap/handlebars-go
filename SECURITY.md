# Security Policy

Report vulnerabilities by opening a GitHub issue with minimal reproduction, or by contacting the maintainer through the profile on the repository owner account.

Notes on the security model:

- Ordinary `{{...}}` interpolation is HTML-escaped using handlebars.js' exact escape map. `{{{triple}}}`, `{{& amp}}`, `SafeString` and `Options.NoEscape` bypass it by design. HTML escaping protects HTML text output only; it is not URL, JS or CSS context sanitization - validate values for those contexts yourself.
- Registered helpers run with your program's privileges. Treat helper code as code.
- The parser is fuzz-tested (see `fuzz_test.go`), but this library is early-stage and has not been independently audited.
