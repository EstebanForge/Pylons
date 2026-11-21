#!/usr/bin/env bash
# Local release: gate, cross-build optimized binaries, pack the pylon
# skeleton, checksum, publish. Mirrors pi-couch/scripts/release.sh.
# Usage: scripts/release.sh [tag]  (tag must match cli/internal/version)
set -euo pipefail
cd "$(dirname "$0")/.."

version="$(sed -n 's/^const Version = "\(.*\)"$/\1/p' cli/internal/version/version.go)"
tag="${1:-$version}"
case "$tag" in
	v*) bare="${tag#v}" ;;
	*) bare="$tag" ;;
esac
if [ "$bare" != "$version" ]; then
	echo "tag $tag does not match cli/internal/version/version.go ($version)" >&2
	exit 1
fi
# Everything is bare semver: the tag, the asset names, the download
# URLs. A v-form argument is accepted and forced to bare.
tag="$bare"

# The skeleton is built from HEAD and the tag points at a remote
# commit, so only a pushed, clean tree may release.
[ -z "$(git status --porcelain)" ] || { echo "working tree not clean; commit or stash first" >&2; exit 1; }
git fetch -q origin
head_sha="$(git rev-parse HEAD)"
origin_sha="$(git rev-parse origin/main)"
[ "$head_sha" = "$origin_sha" ] || { echo "HEAD ($head_sha) is not origin/main ($origin_sha); push first" >&2; exit 1; }

# Gates: format, vet, tests, lint, junk. AppleDouble stubs inside a
# tarball are a known hazard on this tree. bsdtar must not embed them.
[ -z "$(gofmt -l cli)" ] || { echo "gofmt needed:"; gofmt -l cli; exit 1; }
(cd cli && go vet ./... && go test ./... -count=1)
go build -C cli -o ../bin/pylon ./cmd/pylon
./bin/pylon lint >/dev/null
junk="$(find . -name '._*' -not -path './.git/*' -print | head -n1)"
[ -z "$junk" ] || { echo "AppleDouble junk present: $junk; sweep before releasing" >&2; exit 1; }
export COPYFILE_DISABLE=1

# Optimized cross-build: CGO off, trimpath, strip symbol and DWARF
# tables (-s -w). No UPX: AV false positives hurt adoption.
targets="linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64"

rm -rf dist
mkdir -p dist
for pair in $targets; do
	os=${pair%/*}
	arch=${pair#*/}
	name="pylon-$os-$arch"
	echo "== build $name"
	CGO_ENABLED=0 GOOS=$os GOARCH=$arch go build -C cli -trimpath -ldflags "-s -w" -o "../dist/pylon.tmp" ./cmd/pylon
	mkdir -p "dist/$name/bin"
	mv dist/pylon.tmp "dist/$name/bin/pylon"
	if [ "$os" = windows ]; then
		mv "dist/$name/bin/pylon" "dist/$name/bin/pylon.exe"
	fi
	cp LICENSE NOTICE bin/THIRD_PARTY_NOTICES "dist/$name/"
	tar -czf "dist/pylons-$tag-$os-$arch.tar.gz" -C "dist/$name" bin LICENSE NOTICE THIRD_PARTY_NOTICES
	rm -rf "dist/$name"
done

# Smoke test the host platform's artifact: correct version, runs.
case "$(uname -s):$(uname -m)" in
	Linux:x86_64|Linux:amd64) host=linux-amd64 ;;
	Linux:arm64|Linux:aarch64) host=linux-arm64 ;;
	Darwin:x86_64|Darwin:amd64) host=darwin-amd64 ;;
	Darwin:arm64) host=darwin-arm64 ;;
	*) host="" ;;
esac
if [ -n "$host" ]; then
	smoke="$(mktemp -d)"
	tar -xzf "dist/pylons-$tag-$host.tar.gz" -C "$smoke"
	got="$("$smoke/bin/pylon" --version)"
	[ "$got" = "pylon $version" ] || { echo "smoke test failed: '$got'" >&2; exit 1; }
	rm -rf "$smoke"
	echo "== smoke ok ($host)"
fi

# Skeleton from tracked content only; git archive cannot pick up
# junk. Tooling dirs stay out: hooks ship inside the binary, cli/ and
# scripts/ are not part of an installed pylon.
#
# The product manifest is generated, not tracked: it lists exactly
# what self-update may replace, so the updater can three-way every
# product file against user edits.
product_manifest="$(mktemp)"
./bin/pylon manifest product --release-version "$tag" --pylons . > "$product_manifest" \
	|| { echo "product manifest build failed; sweep before releasing" >&2; rm -f "$product_manifest"; exit 1; }

stage="$(mktemp -d)"
git archive --format=tar HEAD \
	. ':(exclude)cli' ':(exclude).githooks' ':(exclude).github' ':(exclude)scripts' ':(exclude)dist' \
	| tar -x -C "$stage"
cp "$product_manifest" "$stage/manifests/product.json"
COPYFILE_DISABLE=1 tar -cz -C "$stage" -f "dist/pylons-kb-$tag.tar.gz" .
rm -rf "$stage" "$product_manifest"

(cd dist && { sha256sum pylons-* 2>/dev/null || shasum -a 256 pylons-*; } > checksums.txt)

# Draft first: a partial upload must never become the public latest.
if gh release view "$tag" >/dev/null 2>&1; then
	echo "release $tag exists; uploading assets"
	gh release upload "$tag" dist/pylons-* dist/checksums.txt --clobber
else
	gh release create "$tag" --draft --target "$head_sha" --title "$tag" --generate-notes
	gh release upload "$tag" dist/pylons-* dist/checksums.txt
	gh release edit "$tag" --draft=false
fi

echo "release $tag published"
