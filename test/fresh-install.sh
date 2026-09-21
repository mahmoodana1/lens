#!/usr/bin/env bash
# Prove lens works on a machine that has never seen it.
#
# Every check runs in a throwaway container with an empty HOME, so what is
# being tested is the install itself and not the state this machine happens to
# have accumulated. Your own ~/.claude, your hooks, your capture logs and the
# lens you are running are never touched: the repo is mounted read-only and
# nothing is written outside the container.
#
#   ./test/fresh-install.sh            everything
#   ./test/fresh-install.sh --local    no docker: a throwaway HOME on this machine
#   ./test/fresh-install.sh --release  also check the published release assets
#   ./test/fresh-install.sh --online   let the source build download its deps
#   ./test/fresh-install.sh --keep     keep the built artifacts for poking at
#
# Scenarios:
#   source   ./install.sh on a machine with Go        (the from-source path)
#   binary   a prebuilt binary on a machine with none (the release path)
#   musl     the same binary on Alpine                (proves it is static)

set -uo pipefail

repo="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
release=no online=no keep=no local=no
for arg in "$@"; do
  case "$arg" in
    --local)   local=yes ;;
    --release) release=yes ;;
    --online)  online=yes ;;
    --keep)    keep=yes ;;
    -h|--help) sed -n '2,20p' "${BASH_SOURCE[0]}" | sed 's/^# \?//'; exit 0 ;;
    *) echo "fresh-install: unknown option $arg" >&2; exit 2 ;;
  esac
done

say()  { printf '\n\033[1;34m══ %s\033[0m\n' "$1"; }
note() { printf '   %s\n' "$1"; }

if [[ "$local" == no ]]; then
  command -v docker >/dev/null || {
    echo "fresh-install: needs docker to make a genuinely clean machine." >&2
    echo "               Or run ./test/fresh-install.sh --local to test a throwaway" >&2
    echo "               HOME on this machine instead." >&2
    exit 2
  }
  docker info >/dev/null 2>&1 || {
    echo "fresh-install: docker is installed but its daemon is not running." >&2
    echo "               Start it with:  sudo systemctl start docker" >&2
    echo "               Or run ./test/fresh-install.sh --local to test a throwaway" >&2
    echo "               HOME on this machine instead." >&2
    exit 2
  }
fi

art="$(mktemp -d)"
cleanup() {
  if [[ "$keep" == yes ]]; then note "artifacts kept in $art"; else rm -rf "$art"; fi
}
trap cleanup EXIT

# ── the artifact a friend would actually download ────────────────────────────
# Built with the same flags as .github/workflows/release.yml, so what the
# release path is tested with is what the release publishes.
say "building the release artifact"
version="$(sed -n 's/^var version = "\(.*\)"$/\1/p' "$repo/main.go")"
version="${version:-0.0.0-test}"
( cd "$repo" && CGO_ENABLED=0 go build -trimpath \
    -ldflags "-s -w -X main.version=$version" -o "$art/lens" . ) || exit 1
printf '%s' "$version" >"$art/version"
note "lens $version, $(du -h "$art/lens" | cut -f1)"

# Static or it does not travel: a cgo build would be bound to the glibc of
# whatever built it, and the release promises one binary per platform.
if file "$art/lens" | grep -q "statically linked"; then
  note "statically linked"
else
  printf '   \033[31mFAIL\033[0m the binary is not statically linked; it will not run everywhere\n'
  echo "$(file "$art/lens")"
  exit 1
fi

# ── without docker: a throwaway HOME, here ──────────────────────────────────
# Weaker than a container and worth having: it covers the install, the hooks,
# the capture and the panel, but not "a machine with no Go" or another libc.
# Your own HOME is not involved — a temporary one is made and wiped.
if [[ "$local" == yes ]]; then
  say "no docker: testing in a throwaway HOME"
  note "this machine has Go and tmux, so it cannot prove those are unnecessary"
  fresh_failed=()
  for scenario in source binary; do
    say "scenario: $scenario (local)"
    home="$(mktemp -d /tmp/lens-fresh-XXXXXX)"
    if env -i \
         PATH="$PATH" TERM="${TERM:-xterm}" LENS_TEST_HOME="$home" \
         LENS_TEST_REPO="$repo" LENS_TEST_ART="$art" \
         GOMODCACHE="$(go env GOMODCACHE)" GOCACHE="$(go env GOCACHE)" \
         bash "$repo/test/fresh-install-checks.sh" "$scenario"; then
      :
    else
      fresh_failed+=("$scenario")
    fi
    chmod -R u+w "$home" 2>/dev/null
    rm -rf "$home"
  done
  if [[ ${#fresh_failed[@]} -eq 0 ]]; then
    printf '\n\033[1;32m══ a fresh HOME works: every check passed\033[0m\n'
    printf '   For the real thing (no Go, no glibc, published assets):\n'
    printf '     sudo systemctl start docker && ./test/fresh-install.sh --release\n'
    exit 0
  fi
  printf '\n\033[1;31m══ these scenarios failed: %s\033[0m\n' "${fresh_failed[*]}"
  exit 1
fi

# ── images ──────────────────────────────────────────────────────────────────
say "building the clean machines"
docker build -q -t lens-fresh:source - >/dev/null <<'DOCKER' || exit 1
FROM golang:1.25
RUN apt-get update && apt-get install -y --no-install-recommends tmux python3 \
    && rm -rf /var/lib/apt/lists/* \
    && mkdir -p /home/tester && chmod 1777 /home/tester
DOCKER
docker build -q -t lens-fresh:binary - >/dev/null <<'DOCKER' || exit 1
FROM debian:stable-slim
RUN apt-get update && apt-get install -y --no-install-recommends tmux python3 \
    && rm -rf /var/lib/apt/lists/* \
    && mkdir -p /home/tester && chmod 1777 /home/tester
DOCKER
note "source: golang:1.25 + tmux      (has a Go toolchain)"
note "binary: debian:stable-slim + tmux (no Go at all)"

mounts=(-v "$repo:/repo:ro" -v "$art:/artifacts:ro" --user "$(id -u):$(id -g)")
goenv=()
if [[ "$online" == no ]]; then
  # Build from this machine's module cache, read-only, so the test does not
  # depend on the network. --online drops this and downloads like a friend would.
  modcache="$(cd "$repo" && go env GOMODCACHE)"
  if [[ -d "$modcache" ]]; then
    mounts+=(-v "$modcache:/gomod:ro")
    goenv=(-e GOMODCACHE=/gomod -e GOFLAGS=-mod=mod)
  fi
fi

failed=()

run_scenario() {
  local name=$1 image=$2; shift 2
  say "scenario: $name"
  if docker run --rm "${mounts[@]}" "$@" "$image" \
       bash /repo/test/fresh-install-checks.sh "$name"; then
    :
  else
    failed+=("$name")
  fi
}

run_scenario source lens-fresh:source "${goenv[@]}"
run_scenario binary lens-fresh:binary

# ── musl: the binary on a machine with no glibc at all ──────────────────────
say "scenario: musl (alpine, no glibc)"
if docker run --rm -v "$art:/artifacts:ro" alpine:3 \
     sh -c '/artifacts/lens --version && /artifacts/lens doctor >/dev/null 2>&1; [ $? -le 1 ]'; then
  note "it runs where there is no glibc"
else
  printf '   \033[31mFAIL\033[0m the binary does not run on musl\n'
  failed+=(musl)
fi

# ── the published release, as a friend would fetch it ───────────────────────
if [[ "$release" == yes ]]; then
  say "the published release"
  tag="$(gh release view --repo mahmoodana1/lens --json tagName -q .tagName 2>/dev/null)"
  if [[ -z "$tag" ]]; then
    printf '   \033[31mFAIL\033[0m no published release to check\n'; failed+=(release)
  else
    note "latest is $tag"
    dl="$art/dl"; mkdir -p "$dl"
    if gh release download "$tag" --repo mahmoodana1/lens --dir "$dl" --clobber 2>/dev/null; then
      if (cd "$dl" && sha256sum -c SHA256SUMS >/dev/null 2>&1); then
        note "every asset matches SHA256SUMS"
      else
        printf '   \033[31mFAIL\033[0m the checksums do not match the assets\n'; failed+=(release)
      fi
      for a in lens_linux_amd64 lens_linux_arm64 lens_darwin_amd64 lens_darwin_arm64; do
        [[ -f "$dl/$a" ]] && note "$a present" || { printf '   \033[31mFAIL\033[0m %s is missing from the release\n' "$a"; failed+=(release); }
      done
      # The one asset this machine can run: check the URL in the README works
      # and that what comes back is the version the tag claims.
      chmod +x "$dl/lens_linux_amd64" 2>/dev/null
      got=$(docker run --rm -v "$dl:/dl:ro" debian:stable-slim /dl/lens_linux_amd64 --version 2>&1)
      if [[ "$got" == *"${tag#v}"* ]]; then
        note "the downloaded binary reports $got"
      else
        printf '   \033[31mFAIL\033[0m the release binary says %q, not %s\n' "$got" "${tag#v}"; failed+=(release)
      fi
    else
      printf '   \033[31mFAIL\033[0m could not download the release assets\n'; failed+=(release)
    fi
  fi
fi

# ── the tally ───────────────────────────────────────────────────────────────
if [[ ${#failed[@]} -eq 0 ]]; then
  printf '\n\033[1;32m══ a fresh install works: every scenario passed\033[0m\n'
  exit 0
fi
printf '\n\033[1;31m══ these scenarios failed: %s\033[0m\n' "${failed[*]}"
exit 1
