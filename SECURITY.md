# Security Policy

## Supported Versions

| Version | Supported          |
| ------- | ------------------ |
| Latest  | ✓ Yes              |
| Older   | ✗ No               |

Only the latest released version of grut receives security updates. Please ensure you are running the most recent version before reporting a vulnerability.

## Extension Trust Model

grut supports three extension runtimes, each with a different privilege level:

| Runtime | Isolation | Limits |
| ------- | --------- | ------ |
| **Lua** | Sandboxed gopher-lua VM. The `os`, `io`, `debug`, and filesystem modules are removed; only safe standard library modules are available. | Execution timeout enforced by the VM. |
| **WASM** | Sandboxed wazero instance. Extensions run in a WebAssembly virtual machine with no direct host access. | 64 MiB memory cap and 30-second execution timeout. |
| **MCP** | **Unsandboxed** OS subprocess. The child process inherits the user's full operating-system permissions (filesystem, network, IPC, etc.). | 30-minute subprocess lifetime limit. Environment variables are filtered to a safe allowlist to limit secret leakage. |

The MCP subprocess model follows the same trust contract as VS Code extensions: **users must trust every extension they install**, because the extension code runs with the same privileges as the user's own account.

A permission declaration system already exists (`CheckPermission()` in `internal/extension/permissions.go`) and manifests can declare required permissions (`file_read`, `network`, `process`, etc.), but runtime enforcement is **not yet wired in**. A future release will gate subprocess capabilities behind the declared permissions so that, for example, an MCP extension without `network` permission cannot open outbound connections.

**Recommendations for users:**

- Only install extensions from sources you trust.
- Review an extension's `manifest.json` permissions before installation.
- Keep grut updated so you receive future permission-enforcement improvements.

## Reporting a Vulnerability

If you discover a security vulnerability in grut, please report it through one of the following channels:

- **GitHub Private Vulnerability Reporting** (preferred): Use the [private security advisory](https://github.com/jongio/grut/security/advisories/new) feature at <https://github.com/jongio/grut>.
- **GitHub Issues**: Open an issue with the `security` label at <https://github.com/jongio/grut/issues>.

Please include:

- A clear description of the vulnerability
- Steps to reproduce the issue
- The potential impact or severity
- Any suggested fixes, if applicable

## Response Timeline

- **Acknowledgment**: Within **48 hours** of receipt.
- **Resolution**: Depends on severity. Critical and high-severity issues are prioritized for the next patch release. Lower-severity issues are addressed in the regular release cycle.

## Responsible Disclosure

We kindly ask that you:

- Allow us reasonable time to investigate and address the issue before public disclosure.
- Avoid exploiting the vulnerability beyond what is necessary to demonstrate it.
- Do not access or modify other users' data.

We are committed to working with security researchers and will credit reporters in the release notes (unless anonymity is requested).

Thank you for helping keep grut and its users safe.
