#!/bin/bash
set -euo pipefail

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

print_color() {
	local color=$1
	local message=$2
	echo -e "${color}${message}${NC}"
}

read_version() {
	if [ ! -f "version/version.go" ]; then
		print_color "$RED" "version/version.go not found!"
		exit 1
	fi
	local major minor patch
	major=$(grep '^\s*Major\s*=\s*' version/version.go | sed -E 's/.*Major\s*=\s*([0-9]+).*/\1/')
	minor=$(grep '^\s*Minor\s*=\s*' version/version.go | sed -E 's/.*Minor\s*=\s*([0-9]+).*/\1/')
	patch=$(grep '^\s*PatchDate\s*=\s*' version/version.go | sed -E 's/.*PatchDate\s*=\s*"([0-9]+)".*/\1/')
	echo "${major}.${minor}.${patch}"
}

generate_version() {
	read_version
}

show_version() {
	local base_version full_version
	base_version=$(read_version)
	full_version=$(generate_version)
	print_color "$BLUE" "Base version: $base_version"
	print_color "$GREEN" "Full version: $full_version"
}

print_full_version() {
	generate_version
}

update_major() {
	local new_major=$1
	if [ -z "$new_major" ]; then
		print_color "$RED" "Major version number required"
		print_color "$YELLOW" "Usage: $0 major <number>"
		exit 1
	fi
	local today=$(date +%Y%m%d)
	sed -i "s/^\(\s*Major\s*=\s*\).*/\1${new_major}/" version/version.go
	sed -i "s/^\(\s*PatchDate\s*=\s*\)\"[0-9]*\"/\1\"${today}\"/" version/version.go
	print_color "$GREEN" "Updated major version to: $new_major"
	show_version
}

update_minor() {
	local new_minor=$1
	if [ -z "$new_minor" ]; then
		print_color "$RED" "Minor version number required"
		print_color "$YELLOW" "Usage: $0 minor <number>"
		exit 1
	fi
	local today=$(date +%Y%m%d)
	sed -i "s/^\(\s*Minor\s*=\s*\).*/\1${new_minor}/" version/version.go
	sed -i "s/^\(\s*PatchDate\s*=\s*\)\"[0-9]*\"/\1\"${today}\"/" version/version.go
	print_color "$GREEN" "Updated minor version to: $new_minor"
	show_version
}

build_release() {
	local today=$(date +%Y%m%d)
	local major minor
	major=$(grep '^\s*Major\s*=\s*' version/version.go | sed -E 's/.*Major\s*=\s*([0-9]+).*/\1/')
	minor=$(grep '^\s*Minor\s*=\s*' version/version.go | sed -E 's/.*Minor\s*=\s*([0-9]+).*/\1/')
	sed -i "s/^\(\s*PatchDate\s*=\s*\)\"[0-9]*\"/\1\"${today}\"/" version/version.go
	local version=$(generate_version)
	local binary_name="quick-pipreqs-${version}-linux-amd64"
	print_color "$BLUE" "Building release binary: $binary_name"
	GOOS=linux GOARCH=amd64 go build -v -o "$binary_name" ./cmd/quick_pipreqs
	sha256sum "$binary_name" > "${binary_name}.sha256"
	print_color "$GREEN" "Built: $binary_name"
	print_color "$GREEN" "Checksum: ${binary_name}.sha256"
	ls -lh "$binary_name"
	cat "${binary_name}.sha256"
}

case "${1:-help}" in
	"current"|"version")
		show_version
		;;
	"print-full")
		print_full_version
		;;
	"major")
		update_major "$2"
		;;
	"minor")
		update_minor "$2"
		;;
	"build")
		build_release
		;;
	"help"|*)
		print_color "$BLUE" "quick-pipreqs Version Management"
		echo
		print_color "$YELLOW" "Usage: $0 <command> [options]"
		echo
		print_color "$GREEN" "Commands:"
		print_color "$YELLOW" "  current, version    Show current version"
		print_color "$YELLOW" "  major <number>      Update major version"
		print_color "$YELLOW" "  minor <number>      Update minor version"
		print_color "$YELLOW" "  build               Build release binary with current date"
		echo
		print_color "$GREEN" "Version format: major.minor.YYYYMMDD"
		;;
esac


