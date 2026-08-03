# Crash report JSON

Use JSON output when scripts need to inspect stored crash reports:

```bash
grut report --list --json
```

The command prints an array of summaries. Use `--limit N` to encode only the newest N reports, or `--limit 0` to keep the default unlimited output. Use `grut report --show <id>` to print one full report.
