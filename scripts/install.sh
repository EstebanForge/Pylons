#!/usr/bin/env bash
# One-time Pylons installer: places a working pylon in ./pylons (or
# the directory you name), with the pylon binary and its own git
# repository.
#
#   curl -fsSL https://raw.githubusercontent.com/EstebanForge/Pylons/main/scripts/install.sh | bash
#   curl -fsSL ... | bash -s -- my-kb                  # custom directory
#   curl -fsSL ... | PYLONS_VERSION=0.2.0 bash        # pinned version
#   PYLONS_BASE_URL=https://mirror.example/dl         # asset mirror (optional)
#   PYLONS_BASE_URL=file:///path/to/assets            # offline bundle (air-gapped install)
#   curl -fsSL ... | bash -s -- --bin-only .          # binary only, into an existing directory
#
# Run from a file (bash install.sh) the installer asks for the ledger
# folder name, default pylons. A piped install takes the default or
# the positional argument without a prompt. A fresh install creates
# the ledger with its own remote-less git repository; an install over
# an existing pylon updates it in place and never touches user files.
#
# --bin-only places bin/pylon (with its license files) into an
# existing directory and nothing else: no scaffolding, no git, no
# hooks. For ledgers that predate Pylons.
#
# Requires bash, curl or wget, and a sha256 tool. The installer never
# touches anything outside the target directory.
main() {
	set -euo pipefail

	REPO="EstebanForge/Pylons"
	VERSION="${PYLONS_VERSION:-latest}"
	BIN_ONLY=0
	if [ "${1:-}" = "--bin-only" ]; then
		BIN_ONLY=1
		shift
	fi
	if [ -z "${1:-}" ] && [ "$BIN_ONLY" != 1 ] && [ -t 0 ]; then
		# Ask for the ledger folder name. A piped install (curl | bash)
		# has the script on stdin, so -t 0 is false there and the
		# default applies without a prompt.
		printf 'pylons-install: ledger folder name [pylons]: '
		IFS= read -r answer || answer=""
		DEST="${answer:-pylons}"
	else
		DEST="${1:-pylons}"
	fi
	log() { printf 'pylons-install: %s\n' "$*"; }
	die() { printf 'pylons-install: %s\n' "$*" >&2; exit 1; }

	case "$DEST" in
		*/*|*'>'*) die "ledger folder name must be a plain folder name, got '$DEST'" ;;
	esac

	# --- tools, up front ------------------------------------------------
	command -v curl >/dev/null 2>&1 || command -v wget >/dev/null 2>&1 ||
		die "need curl or wget to download"
	command -v sha256sum >/dev/null 2>&1 || command -v shasum >/dev/null 2>&1 ||
		command -v openssl >/dev/null 2>&1 || die "need sha256sum, shasum, or openssl"

	fetch() { # fetch <url> <outfile>
		case "$1" in
			file://*)
				# A local mirror (file://) serves air-gapped installs
				# and offline bundles; no network transport runs.
				cp "${1#file://}" "$2"
				;;
			*)
				if command -v curl >/dev/null 2>&1; then
					curl -fsSL --proto '=https' --tlsv1.2 "$1" -o "$2"
				else
					wget -qO "$2" --secure-protocol=TLSv1_2 "$1"
				fi
				;;
		esac
	}
	sum() { # sum <file> -> sha256 hex
		if command -v sha256sum >/dev/null 2>&1; then sha256sum "$1" | cut -d' ' -f1
		elif command -v shasum >/dev/null 2>&1; then shasum -a 256 "$1" | cut -d' ' -f1
		else openssl dgst -sha256 -r "$1" | cut -d' ' -f1
		fi
	}

	# --- platform -------------------------------------------------------
	case "$(uname -s)" in
		Linux) os=linux ;;
		Darwin) os=darwin ;;
		*) die "unsupported OS '$(uname -s)'; download a tarball manually from https://github.com/$REPO/releases" ;;
	esac
	case "$(uname -m)" in
		x86_64|amd64) arch=amd64 ;;
		arm64|aarch64) arch=arm64 ;;
		*) die "unsupported architecture '$(uname -m)'; download a tarball manually from https://github.com/$REPO/releases" ;;
	esac

	# --- resolve version ------------------------------------------------
	case "$VERSION" in
		latest)
			case "${PYLONS_BASE_URL:-}" in
				file://*) die "PYLONS_BASE_URL=file:// needs an explicit PYLONS_VERSION" ;;
			esac
			latest_url="https://github.com/$REPO/releases/latest"
			if command -v curl >/dev/null 2>&1; then
				# Ask for the redirect itself; do not follow it.
				loc="$(curl -fsS --proto '=https' -o /dev/null -w '%{redirect_url}' "$latest_url" || true)"
			else
				loc="$(wget -S --spider --max-redirect=0 "$latest_url" 2>&1 |
					tr -d '\r' | sed -n 's/^ *[Ll]ocation: *//p' | head -n1 || true)"
			fi
			case "$loc" in */releases/tag/*) VERSION="${loc##*/}" ;; *) die "could not resolve the latest release; pin one, e.g. curl ... | PYLONS_VERSION=0.2.0 bash" ;; esac
			;;
	esac
	# Semantic form only; VERSION lands in download URLs, so constrain
	# it. Bare semver everywhere: the tag, the URLs, the asset names.
	[[ "$VERSION" =~ ^[0-9]+\.[0-9]+\.[0-9]+([-+][0-9A-Za-z.-]+)?$ ]] ||
		die "invalid version '$VERSION'"
	base="${PYLONS_BASE_URL:-https://github.com/$REPO/releases/download}"
	case "$base" in
		file://*) ;; # a bundle already contains the version directory
		*) base="$base/$VERSION" ;;
	esac

	# --- download + verify, before touching the target ------------------
	bin_asset="pylons-$VERSION-$os-$arch.tar.gz"
	kb_asset="pylons-kb-$VERSION.tar.gz"
	work="$(mktemp -d)"
	trap 'rm -rf "$work"' EXIT

	log "downloading $VERSION for $os/$arch"
	fetch "$base/$bin_asset" "$work/$bin_asset"
	fetch "$base/checksums.txt" "$work/checksums.txt"
	if [ "$BIN_ONLY" != 1 ]; then
		fetch "$base/$kb_asset" "$work/$kb_asset"
	fi

	for f in "$bin_asset" $([ "$BIN_ONLY" != 1 ] && printf '%s' "$kb_asset"); do
		want="$(awk -v f="$f" '{ sub(/.*\//, "", $2); if ($2 == f) print $1 }' "$work/checksums.txt")"
		got="$(sum "$work/$f")"
		[ -n "$want" ] || die "no checksum found for $f"
		[ "$want" = "$got" ] || die "checksum mismatch for $f (want $want, got $got)"
	done
	log "checksums verified"

	if [ "$BIN_ONLY" = 1 ]; then
		# Binary-only mode: an existing ledger gets the engine and its
		# license files under bin/ and nothing else.
		[ -d "$DEST" ] || die "--bin-only needs an existing directory; $DEST does not exist"
		# The tarball carries a bin/ prefix; strip it so everything
		# lands flat in the target's bin/.
		mkdir -p "$DEST/bin"
		tar -xzf "$work/$bin_asset" -C "$DEST/bin" --strip-components=1
		chmod +x "$DEST/bin/pylon"
		printf -v q '%q' "$DEST"
		log "binary installed at $DEST/bin/pylon"
		log "next steps:"
		log "  1. write manifests/pylons.yaml in $q (see the README)"
		log "  2. $DEST/bin/pylon install --pylons $DEST"
		log "  3. consider adding bin/ to .gitignore"
		log "done"
		return 0
	fi

	mkdir -p "$work/stage"
	tar -xzf "$work/$bin_asset" -C "$work/stage"
	tar -xzf "$work/$kb_asset" -C "$work/stage"
	chmod +x "$work/stage/bin/pylon"

	# --- target: fresh install or in-place update -----------------------
	printf -v q '%q' "$DEST"
	printf -v q2 '%q' "$DEST"
	if [ -d "$DEST" ] && [ -f "$DEST/manifests/pylons.yaml" ]; then
		# An installed pylon: hand the staged release to the NEW binary.
		# self-update never touches user files; it three-ways every
		# product file, backs up what it replaces under _pylons-backup,
		# and leaves your edits as .pylons-new copies for review.
		log "existing pylon at $DEST; updating in place"
		if "$work/stage/bin/pylon" self-update --from "$work/stage" --pylons "$DEST"; then
			log "update applied; review with: cd $q2 && git status"
		else
			die "self-update reported drift or failed; inspect $DEST (.pylons-new copies), resolve or accept each one, then rerun this installer"
		fi
		log "done"
		return 0
	fi
	# An existing repository inside the target is never touched: a
	# folder holding a repo but no pylons.yaml is someone's work, not
	# a fresh install.
	[ -e "$DEST/.git" ] && die "$DEST already holds a git repository and does not look like a pylon (no manifests/pylons.yaml); refusing to touch it"
	[ -e "$DEST" ] && die "$DEST already exists and does not look like a pylon (no manifests/pylons.yaml); remove it or pass another directory"
	# Rename the whole stage dir: no pre-created target to conflict with,
	# and a failure above this line leaves nothing on disk.
	mv "$work/stage" "$DEST"

	# --- git + gates ----------------------------------------------------
	# The ledger tracks its own work: a fresh, remote-less repository
	# inside the ledger folder, never a remote and never a reuse of an
	# existing repo.
	if command -v git >/dev/null 2>&1; then
		if (cd "$DEST" && git init -q -b main 2>/dev/null || { git init -q && git symbolic-ref HEAD refs/heads/main; }); then
			log "initialized a fresh git repository in $DEST (no remote)"
			if (cd "$DEST" && ./bin/pylon install --pylons .); then
				log "gate hooks wired"
			else
				log "hooks not wired; run: cd $q && ./bin/pylon install --pylons ."
			fi
			# Scaffolders commit: an untracked skeleton tracks nothing.
			if commit_err="$(cd "$DEST" && git add -A && git commit -qm "chore: scaffold pylons" 2>&1)"; then
				log "initial commit created"
			else
				log "initial commit failed: $commit_err"
				log "if git identity is missing: cd $q && git config user.name <name> && git config user.email <email> && git add -A && git commit -m 'chore: scaffold pylons'"
			fi
		else
			log "git init failed; run it later: cd $q && git init && ./bin/pylon install --pylons ."
		fi
	else
		log "git not found; run: cd $q && git init && ./bin/pylon install --pylons ."
	fi

	log "installed $VERSION in $DEST/"
	log "next steps:"
	log "  1. cd $q and register your code repositories in manifests/pylons.yaml"
	log "  2. ./bin/pylon --help"
}
# Sourced callers (tests, mirrors) may pre-define the fetch surface;
# a direct run keeps the https-only transport.
if [ -z "${PYLONS_SOURCED:-}" ]; then
	main "$@"
fi
