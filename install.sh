#!/usr/bin/env bash
# One-time Pylons installer: places a working pylon in ./pylons (or
# the directory you name), with the pylon binary and its own git
# repository.
#
#   curl -fsSL https://raw.githubusercontent.com/EstebanForge/Pylons/main/install.sh | bash
#   curl -fsSL ... | bash -s -- my-kb                  # custom directory
#   curl -fsSL ... | PYLONS_VERSION=v0.2.0 bash        # pinned version
#
# Requires bash, curl or wget, and a sha256 tool. The installer never
# touches anything outside the target directory.
main() {
	set -euo pipefail

	REPO="EstebanForge/Pylons"
	VERSION="${PYLONS_VERSION:-latest}"
	DEST="${1:-pylons}"

	log() { printf 'pylons-install: %s\n' "$*"; }
	die() { printf 'pylons-install: %s\n' "$*" >&2; exit 1; }

	# --- tools, up front ------------------------------------------------
	command -v curl >/dev/null 2>&1 || command -v wget >/dev/null 2>&1 ||
		die "need curl or wget to download"
	command -v sha256sum >/dev/null 2>&1 || command -v shasum >/dev/null 2>&1 ||
		command -v openssl >/dev/null 2>&1 || die "need sha256sum, shasum, or openssl"

	fetch() { # fetch <url> <outfile>
		if command -v curl >/dev/null 2>&1; then
			curl -fsSL --proto '=https' --tlsv1.2 "$1" -o "$2"
		else
			wget -qO "$2" --secure-protocol=TLSv1_2 "$1"
		fi
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
			latest_url="https://github.com/$REPO/releases/latest"
			if command -v curl >/dev/null 2>&1; then
				# Ask for the redirect itself; do not follow it.
				loc="$(curl -fsS --proto '=https' -o /dev/null -w '%{redirect_url}' "$latest_url" || true)"
			else
				loc="$(wget -S --spider --max-redirect=0 "$latest_url" 2>&1 |
					tr -d '\r' | sed -n 's/^ *[Ll]ocation: *//p' | head -n1 || true)"
			fi
			case "$loc" in */releases/tag/*) VERSION="${loc##*/}" ;; *) die "could not resolve the latest release; pin one, e.g. curl ... | PYLONS_VERSION=v0.2.0 bash" ;; esac
			;;
	esac
	# The tag is bare; download URLs want the v-form, so normalize after
	# the case: the redirect-resolved tag must not skip this step.
	case "$VERSION" in
		v*) ;;
		*) VERSION="v$VERSION" ;;
	esac
	# Semantic form only; VERSION lands in download URLs, so constrain it.
	[[ "$VERSION" =~ ^v[0-9]+\.[0-9]+\.[0-9]+([-+][0-9A-Za-z.-]+)?$ ]] ||
		die "invalid version '$VERSION'"
	base="https://github.com/$REPO/releases/download/$VERSION"

	# --- download + verify, before touching the target ------------------
	bin_asset="pylons-$VERSION-$os-$arch.tar.gz"
	kb_asset="pylons-kb-$VERSION.tar.gz"
	work="$(mktemp -d)"
	trap 'rm -rf "$work"' EXIT

	log "downloading $VERSION for $os/$arch"
	fetch "$base/$bin_asset" "$work/$bin_asset"
	fetch "$base/$kb_asset" "$work/$kb_asset"
	fetch "$base/checksums.txt" "$work/checksums.txt"

	for f in "$bin_asset" "$kb_asset"; do
		want="$(awk -v f="$f" '{ sub(/.*\//, "", $2); if ($2 == f) print $1 }' "$work/checksums.txt")"
		got="$(sum "$work/$f")"
		[ -n "$want" ] || die "no checksum found for $f"
		[ "$want" = "$got" ] || die "checksum mismatch for $f (want $want, got $got)"
	done
	log "checksums verified"

	mkdir -p "$work/stage"
	tar -xzf "$work/$bin_asset" -C "$work/stage"
	tar -xzf "$work/$kb_asset" -C "$work/stage"
	chmod +x "$work/stage/bin/pylon"

	# --- target directory (atomic-ish: only extracted content lands) ----
	[ -e "$DEST" ] && die "$DEST already exists; remove it or pass another directory"
	# Rename the whole stage dir: no pre-created target to conflict with,
	# and a failure above this line leaves nothing on disk.
	mv "$work/stage" "$DEST"

	# --- git + gates ----------------------------------------------------
	printf -v q '%q' "$DEST"
	if command -v git >/dev/null 2>&1; then
		if (cd "$DEST" && git init -q -b main 2>/dev/null || { git init -q && git symbolic-ref HEAD refs/heads/main; }); then
			log "initialized git repository in $DEST"
			if (cd "$DEST" && ./bin/pylon install --pylons .); then
				log "gate hooks wired"
			else
				log "hooks not wired; run: cd $q && ./bin/pylon install --pylons ."
			fi
			# Scaffolders commit: an untracked skeleton tracks nothing.
			if (cd "$DEST" && git add -A && git commit -qm "chore: scaffold pylons" 2>/dev/null); then
				log "initial commit created"
			else
				log "no git identity; run: cd $q && git config user.name <name> && git config user.email <email> && git add -A && git commit -m 'chore: scaffold pylons'"
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
	log "  2. ./bin/pylon help"
}
main "$@"
