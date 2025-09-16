## quick-pipreqs

A tiny CLI to quickly (re)generate Python `requirements.txt` files using `pipreqs`, across a project tree.

It scans for directories containing `requirements.txt` (within a configurable depth), and for each directory it:

- Backs up the current `requirements.txt` to `requirements.txt.bak`
- Runs `pipreqs .` in that directory
- Reports whether the file changed

If no `requirements.txt` is found under the root, it runs `pipreqs` once at the root.

### Why

- Keep `requirements.txt` in sync with imports without hand-editing
- Batch-update multiple Python subprojects in a mono-repo
- Fast and concurrent, with deterministic directory processing

---

## Installation

Prerequisites:

- Go (module declares `go 1.25.1`)
- Python environment with `pipreqs` available on `PATH` ([How to install...](https://github.com/bndr/pipreqs?tab=readme-ov-file#installation))

Build from source:

```bash
make build  # produces ./quick-pipreqs
```

Optional release build (Linux/amd64 binary + checksum):

```bash
make build-release
```

Or using plain Go:

```bash
go build -v -o quick-pipreqs ./cmd/quick_pipreqs
```

## Usage

```bash
quick-pipreqs [options] <path>
```

Options:

- `--dry-run`           Print actions without executing any changes
- `--version`           Print tool version and exit
- `--max-depth <int>`   Maximum recursion depth (default: 2, 0 = only root)
- `--concurrency <n>`   Max concurrent updates (default: 12, capped at 12)

Behavior:

- Scans `<path>` for `requirements.txt` files up to `--max-depth`
- Processes directories in sorted order for deterministic output
- Requires `pipreqs` in `PATH` (skipped check when `--dry-run` is used)
- Creates/overwrites a backup: `requirements.txt.bak`

Exit codes:

- `0` success (even if nothing changed)
- `1` operational error (e.g., `pipreqs` failed)
- `2` usage/validation error (e.g., missing path, invalid flags)

### Examples

Update a mono-repo root, scanning to depth 2 (default):

```bash
./quick-pipreqs /path/to/repo
```

Preview what would happen without writing files:

```bash
./quick-pipreqs --dry-run /path/to/repo
```

Only check the root directory (no recursion):

```bash
./quick-pipreqs --max-depth 0 /path/to/project
```

Run with limited parallelism:

```bash
./quick-pipreqs --concurrency 4 /path/to/repo
```

Print version:

```bash
./quick-pipreqs --version
```

---

## Development

Common targets:

```bash
make help
make deps
make fmt
make vet
make build
```

### Versioning

Version format is `major.minor.YYYYMMDD`. The date is updated automatically on release builds and when bumping versions.

Helper script:

```bash
# Show current and full version
./scripts/version.sh current

# Bump major or minor (also updates date)
./scripts/version.sh major 2
./scripts/version.sh minor 3

# Build a release (updates date, builds, and writes checksum)
./scripts/version.sh build
```

The values are stored under `version/version.go` and read at runtime by the CLI.

---

## Requirements and notes

- `pipreqs` must be installed and discoverable on `PATH` for non-dry runs
- Backups overwrite any existing `requirements.txt.bak`
- When no `requirements.txt` is found below the root, `pipreqs` is run once at the root

---

## License

See `LICENSE` for details.


