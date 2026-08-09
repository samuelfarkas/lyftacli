#!/bin/sh

set -eu

repository="samuelfarkas/lyftacli"
install_dir="${LYFTA_INSTALL_DIR:-$HOME/.local/bin}"

if ! command -v curl >/dev/null 2>&1; then
  echo "curl is required" >&2
  exit 1
fi

case "$(uname -s)" in
  Darwin) os=darwin ;;
  Linux) os=linux ;;
  *)
    echo "Unsupported operating system: $(uname -s)" >&2
    exit 1
    ;;
esac

case "$(uname -m)" in
  arm64 | aarch64) arch=arm64 ;;
  x86_64 | amd64) arch=amd64 ;;
  *)
    echo "Unsupported architecture: $(uname -m)" >&2
    exit 1
    ;;
esac

if [ -n "${LYFTA_VERSION:-}" ]; then
  tag="v${LYFTA_VERSION#v}"
else
  tag=$(curl --proto '=https' --proto-redir '=https' -fLsS -o /dev/null -w '%{url_effective}' \
    "https://github.com/$repository/releases/latest")
  tag=${tag##*/}
fi

case "$tag" in
  v[0-9]*.[0-9]*.[0-9]*) ;;
  *)
    echo "Could not resolve a valid lyfta release version" >&2
    exit 1
    ;;
esac

version=${tag#v}
archive="lyfta_${version}_${os}_${arch}.tar.gz"
base_url="https://github.com/$repository/releases/download/$tag"
temporary_dir=$(mktemp -d)
binary_temp=
trap 'rm -rf "$temporary_dir"; [ -z "$binary_temp" ] || rm -f "$binary_temp"' EXIT HUP INT TERM

curl --proto '=https' --proto-redir '=https' -fLsS "$base_url/$archive" -o "$temporary_dir/$archive"
curl --proto '=https' --proto-redir '=https' -fLsS "$base_url/checksums.txt" -o "$temporary_dir/checksums.txt"

expected=$(awk -v archive="$archive" '$2 == archive { print $1 }' "$temporary_dir/checksums.txt")
if [ -z "$expected" ]; then
  echo "No checksum found for $archive" >&2
  exit 1
fi

if command -v sha256sum >/dev/null 2>&1; then
  actual=$(sha256sum "$temporary_dir/$archive" | awk '{ print $1 }')
elif command -v shasum >/dev/null 2>&1; then
  actual=$(shasum -a 256 "$temporary_dir/$archive" | awk '{ print $1 }')
else
  echo "sha256sum or shasum is required" >&2
  exit 1
fi

if [ "$actual" != "$expected" ]; then
  echo "Checksum verification failed for $archive" >&2
  exit 1
fi

tar -xzf "$temporary_dir/$archive" -C "$temporary_dir"
install -d "$install_dir"
binary_temp="$install_dir/.lyfta.$$"
install -m 0755 "$temporary_dir/${archive%.tar.gz}/lyfta" "$binary_temp"
mv -f "$binary_temp" "$install_dir/lyfta"

echo "Installed lyfta $version to $install_dir/lyfta"
case ":$PATH:" in
  *":$install_dir:"*) ;;
  *) echo "Add $install_dir to PATH to run lyfta." ;;
esac
