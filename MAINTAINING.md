# Maintaining

This covers the Go-specific parts of looking after the provider, on top of the team's general process for dependency updates and vulnerabilities.

## Vulnerable dependencies

Two things report known vulnerabilities:

- **The `vulncheck` CI job** runs [govulncheck](https://go.dev/doc/tutorial/govulncheck) on every PR, on pushes to `main` and nightly. It fails the build when our code reaches a vulnerable function, in a dependency or in the standard library. The nightly run catches advisories published against code that hasn't changed.
- **Dependabot alerts** cover every module in `go.mod`, direct or indirect, whether or not our code reaches the vulnerable part.

### Fixing a vulnerability

Work through these in order, and stop at the first one that works.

1. **Upgrade the direct dependency.** If the vulnerable module is a direct dependency (no `// indirect` comment in `go.mod`), upgrade it to the fixed release with `go get <module>@<version>`, then run `go mod tidy`. If it's an indirect dependency, run `go mod why -m <module>` to find the direct dependency that pulls it in, and upgrade that if a newer release requires the fixed version.
2. **Raise the indirect module's minimum.** If no release of the direct dependency fixes it yet, upgrade the indirect module itself, then add a comment naming the advisory after `// indirect;`:

   ```bash
   go get example.com/some/module@v1.2.4
   go mod tidy
   ```

   ```
   require example.com/some/module v1.2.4 // indirect; CVE-2026-12345, remove once go-sdk requires the fix
   ```

   Keep the `indirect;` prefix so Go still treats the requirement as indirect. `go mod tidy` keeps the comment. Our `go.mod` requirements apply to customers' builds too, so they get the fixed version. Don't use a `replace` directive for this: Go ignores it outside our module, so customers never see it.

   Once the direct dependency requires the fixed version, remove the comment. The `require` line stays, because `go.mod` lists every module in the build.
3. **Mitigate it.** If there's no fixed version, govulncheck has no way to ignore a finding, but it only fails on code we reach. For a reachable vulnerability, the usual mitigation is to stop calling the vulnerable function. If our code can't reach it, dismiss the Dependabot alert with a comment explaining why it's safe and what would end the exception.
