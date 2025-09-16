# quick-pipreqs

A CLI tool to quickly regenerate Python `requirements.txt` files using `pipreqs` across a project tree.

## Installation

```bash
go install github.com/bevelwork/quick_pipreqs@latest
```

**Prerequisites:** Python environment with `pipreqs` installed ([install pipreqs](https://github.com/bndr/pipreqs#installation))

## Usage

```bash
quick-pipreqs [options] <path>
```

### Options

- `--dry-run` - Preview changes without executing
- `--max-depth <int>` - Maximum recursion depth (default: 2)
- `--concurrency <n>` - Max concurrent updates (default: 12)
- `--version` - Show version

### Examples

```bash
# Update requirements.txt files in a project
quick-pipreqs /path/to/project

# Preview changes without writing files
quick-pipreqs --dry-run /path/to/project

# Only check root directory
quick-pipreqs --max-depth 0 /path/to/project
```

## How it works

- Scans for directories containing `requirements.txt` files
- Backs up existing files to `requirements.txt.bak`
- Runs `pipreqs` in each directory to regenerate requirements
- Processes directories concurrently for speed

## License

Apache 2.0.


