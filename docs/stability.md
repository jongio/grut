# Stability and Versioning

grut is pre-1.0. This document says what that means today, what will be
guaranteed at 1.0, and what has to be true before 1.0 is worth tagging.

## Today (0.x)

Anything can change in a minor release. In practice the CLI surface has been
additive for several releases, but nothing here is promised yet, so pin a
version if you depend on grut in automation:

```bash
curl -fsSL https://raw.githubusercontent.com/jongio/grut/main/install.sh | sh -s -- v0.8.0
```

## What 1.0 will cover

At 1.0 the following become part of the compatibility surface. Breaking any of
them requires a major version bump.

### Command and flag names
The subcommands (`clean`, `completion`, `config`, `doctor`, `ext`, `keys`,
`mcp`, `report`, `run`, `status`, `theme`, `update`, `version`) and their
flags. Flags may be added; existing ones keep their meaning.

### Machine-readable output
Ten commands emit structured output behind `--json`, fourteen flags in total
once subcommands are counted. Once 1.0 lands, the shape of that output is a
contract: fields may be added, but existing fields will not be removed,
renamed, or change type.

### Exit codes
`--check` on `clean`, `doctor`, and `status` is an exit-code gate intended for
CI. Zero means the checked condition holds, non-zero means it does not. Scripts
depending on that distinction will keep working.

### Configuration keys
The TOML keys documented in [configuration.md](configuration.md). Keys may be
added. Removing or repurposing one is breaking.

Unknown keys are ignored rather than rejected, so a config written for a newer
grut still loads on an older one, minus the unrecognized settings.

### Extension API
The `extension.toml` manifest schema, the permission model, and the Lua and
WASM host functions in [extensions.md](extensions.md). Third-party extensions
are the surface most expensive to break, since the author is not us.

### Default keybindings
Default bindings will not be reassigned to different actions in a minor
release. Adding a binding to a previously unbound key is not breaking.

## What 1.0 will not cover

- **Go packages under `internal/`.** Go forbids importing these from outside
  the module, so they are not an API. They can be restructured at any time.
- **TUI rendering.** Panel layout, colours, spacing, and glyphs are
  presentation, not contract. Themes exist for callers who care.
- **Log and audit line formats**, unless a specific format is documented as
  machine-readable.
- **Prerelease and dev builds.**

## Before tagging 1.0

Freezing a surface is only meaningful if the surface is written down. Current
gaps:

- [ ] **Document every `--json` shape.** Two of the ten commands are documented
      today ([report-json.md](report-json.md),
      [version-json.md](version-json.md)). The rest are contracts by accident
      rather than by description.
- [ ] **Document the `--check` exit codes**, including what non-zero means for
      each command.
- [ ] **Audit config keys for dead settings.** A key that is declared but
      unread becomes a permanent obligation at 1.0. `socket_auth` was one of
      these and has been removed; the rest of the schema deserves the same
      pass.
- [ ] **Version the extension manifest.** Third-party extensions need a way to
      declare which schema they target.
- [ ] **State the supported platform and Go version floor.**

## Deprecation

After 1.0, anything on the covered surface that is going away gets deprecated
before removal: it keeps working for at least one minor release, warns when
used, and the release notes name the replacement.