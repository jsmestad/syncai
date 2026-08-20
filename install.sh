#!/bin/sh

set -eu

os=$(uname -s | tr '[:upper:]' '[:lower:]')
case "$os" in
	darwin | linux) ;;
	*)
		echo "Unsupported operating system: $os" >&2
		exit 1
		;;
esac

arch=$(uname -m)
case "$arch" in
	x86_64 | amd64) arch=amd64 ;;
	arm64 | aarch64) arch=arm64 ;;
	*)
		echo "Unsupported architecture: $arch" >&2
		exit 1
		;;
esac

install_dir=${SYNCAI_INSTALL_DIR:-"$HOME/.local/bin"}
temporary_dir=$(mktemp -d)
mkdir -p "$install_dir"
temporary_binary=$(mktemp "$install_dir/.syncai-install.XXXXXX")
trap 'rm -rf "$temporary_dir"; rm -f "$temporary_binary"' EXIT HUP INT TERM

if ! command -v ssh-keygen >/dev/null 2>&1; then
	echo 'OpenSSH 8.2 or newer is required to verify the SyncAI release' >&2
	exit 1
fi

latest_release=$(curl --proto '=https' --proto-redir '=https' -fsSL -o /dev/null -w '%{url_effective}' https://github.com/jsmestad/syncai/releases/latest)
tag=${latest_release%/}
tag=${tag##*/}
case "$tag" in
	v[0-9]*) ;;
	*)
		echo "Invalid latest release tag: $tag" >&2
		exit 1
		;;
esac

archive_name="syncai_${os}_${arch}.tar.gz"
release_base="https://github.com/jsmestad/syncai/releases/download/$tag"
curl --proto '=https' --proto-redir '=https' -fsSL "$release_base/$archive_name" -o "$temporary_dir/syncai.tar.gz"
curl --proto '=https' --proto-redir '=https' -fsSL "$release_base/checksums.txt" -o "$temporary_dir/checksums.txt"
curl --proto '=https' --proto-redir '=https' -fsSL "$release_base/checksums.txt.sshsig" -o "$temporary_dir/checksums.txt.sshsig"
printf '%s\n' 'syncai namespaces="syncai-release" ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIK4GBhMSHYZp/IqFIEjriR5a0Zc7bvhy+2oJqgJ2wLjA' > "$temporary_dir/allowed_signers"
{ printf '%s\n' "$tag"; cat "$temporary_dir/checksums.txt"; } > "$temporary_dir/signed-checksums.txt"
if ! ssh-keygen -Y verify -f "$temporary_dir/allowed_signers" -I syncai -n syncai-release -s "$temporary_dir/checksums.txt.sshsig" < "$temporary_dir/signed-checksums.txt" >/dev/null; then
	echo 'Release checksum signature is invalid' >&2
	exit 1
fi

expected_hash=$(awk -v name="$archive_name" '$2 == name || $2 == "*" name { print tolower($1); exit }' "$temporary_dir/checksums.txt")
if command -v sha256sum >/dev/null 2>&1; then
	actual_hash=$(sha256sum "$temporary_dir/syncai.tar.gz" | awk '{ print $1 }')
elif command -v shasum >/dev/null 2>&1; then
	actual_hash=$(shasum -a 256 "$temporary_dir/syncai.tar.gz" | awk '{ print $1 }')
else
	echo 'sha256sum or shasum is required to verify the SyncAI archive' >&2
	exit 1
fi
if [ -z "$expected_hash" ] || [ "$actual_hash" != "$expected_hash" ]; then
	echo "Checksum verification failed for $archive_name" >&2
	exit 1
fi

tar -xzf "$temporary_dir/syncai.tar.gz" -C "$temporary_dir" syncai
if [ ! -f "$temporary_dir/syncai" ] || [ -L "$temporary_dir/syncai" ]; then
	echo 'Release archive does not contain a regular syncai executable' >&2
	exit 1
fi
install -m 0755 "$temporary_dir/syncai" "$temporary_binary"
mv -f "$temporary_binary" "$install_dir/syncai"

printf 'Installed SyncAI to %s\n' "$install_dir/syncai"
