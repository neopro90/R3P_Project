#!/usr/bin/env bash
set -euo pipefail

export PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
OUTPUT_ARG="${1:-}"

if [[ -z "$OUTPUT_ARG" ]]; then
    echo "Usage: $0 custom-output-directory" >&2
    exit 2
fi

if [[ "$OUTPUT_ARG" = /* ]]; then
    OUTPUT_DIR="$OUTPUT_ARG"
else
    OUTPUT_DIR="$PROJECT_ROOT/build/output/$OUTPUT_ARG"
fi
[[ -d "$OUTPUT_DIR" ]] || {
    echo "Custom output directory not found: $OUTPUT_DIR" >&2
    exit 1
}
OUTPUT_DIR="$(realpath -e -- "$OUTPUT_DIR")"
OUTPUT_ROOT="$(realpath -e -- "$PROJECT_ROOT/build/output")"
case "$OUTPUT_DIR/" in
    "$OUTPUT_ROOT/"*) ;;
    *)
        echo "Custom output must be inside $OUTPUT_ROOT" >&2
        exit 2
        ;;
esac

IMAGE_LIMIT=261619712
KERNEL_LIMIT=4194304
images=("$OUTPUT_DIR"/*factory.bin "$OUTPUT_DIR"/*sysupgrade.bin)
[[ -f "${images[0]}" && -f "${images[1]}" && ${#images[@]} -eq 2 ]] || {
    echo "Expected exactly one factory and one sysupgrade image in $OUTPUT_DIR" >&2
    exit 1
}
factory="${images[0]}"
sysupgrade="${images[1]}"

for image in "$factory" "$sysupgrade"; do
    size="$(stat -c%s "$image")"
    (( size <= IMAGE_LIMIT )) || {
        echo "Image exceeds profile limit: $image ($size bytes)" >&2
        exit 1
    }
done

(cd "$OUTPUT_DIR" && sha256sum --check sha256sums)
factory_type="$(file "$factory")"
sysupgrade_type="$(file "$sysupgrade")"
[[ "$factory_type" == *'u-boot legacy uImage'* && "$factory_type" == *MIPS* ]]
[[ "$sysupgrade_type" == *'POSIX tar archive'* ]]
archive_listing="$(tar -tf "$sysupgrade")"
grep -Fxq 'sysupgrade-xiaomi_mi-router-3-pro/kernel' <<< "$archive_listing"
grep -Fxq 'sysupgrade-xiaomi_mi-router-3-pro/root' <<< "$archive_listing"
kernel_size="$(tar -xOf "$sysupgrade" sysupgrade-xiaomi_mi-router-3-pro/kernel | wc -c)"
(( kernel_size <= KERNEL_LIMIT )) || {
    echo "Kernel exceeds profile limit: $kernel_size bytes" >&2
    exit 1
}

UNSQUASHFS=/usr/bin/unsquashfs
[[ -x "$UNSQUASHFS" ]] || {
    echo "unsquashfs is required to inspect the firmware root filesystem" >&2
    exit 1
}
command -v upx >/dev/null 2>&1 || true
API_EXPECTED="$PROJECT_ROOT/build/artifacts/r3p-api"
[[ -f "$API_EXPECTED" ]] || API_EXPECTED="$PROJECT_ROOT/build/output/r3p-api-mipsle"
[[ -f "$API_EXPECTED" ]] || {
    echo "Expected MIPSLE API binary missing: $API_EXPECTED" >&2
    exit 1
}
rootfs_tmp="$(mktemp "$OUTPUT_DIR/.verify-root.XXXXXX")"
listing_tmp="$(mktemp "$OUTPUT_DIR/.verify-list.XXXXXX")"
trap 'rm -f -- "$rootfs_tmp" "$listing_tmp"' EXIT
tar -xOf "$sysupgrade" sysupgrade-xiaomi_mi-router-3-pro/root > "$rootfs_tmp"
"$UNSQUASHFS" -ll "$rootfs_tmp" > "$listing_tmp"

for path in \
    /usr/bin/r3p-api \
    /www/index.html \
    /etc/init.d/r3p-api \
    /etc/uci-defaults/99-r3p-setup \
    /etc/uci-defaults/98-r3p-aria2 \
    /etc/init.d/r3p-aria2 \
    /etc/config/uhttpd \
    /etc/r3p/version.json
do
    grep -Eq "$path"'$' "$listing_tmp" || {
        echo "Required overlay path missing from image: $path" >&2
        exit 1
    }
done
grep -Eq '/www/assets/[^[:space:]]+\.js$' "$listing_tmp"
grep -Eq '^kmod-usb3 - ' "$OUTPUT_DIR"/*.manifest
grep -Eq '^kmod-usb-storage - ' "$OUTPUT_DIR"/*.manifest
grep -Eq '^kmod-fs-ext4 - ' "$OUTPUT_DIR"/*.manifest
grep -Eq '^block-mount - ' "$OUTPUT_DIR"/*.manifest
grep -Eq '^dropbear - ' "$OUTPUT_DIR"/*.manifest
grep -Eq '^dnsmasq - ' "$OUTPUT_DIR"/*.manifest
grep -Eq '^aria2(-| )' "$OUTPUT_DIR"/*.manifest
grep -Eq '^adblock-fast - ' "$OUTPUT_DIR"/*.manifest
grep -Eq '^ksmbd-server - ' "$OUTPUT_DIR"/*.manifest
grep -Eq '^kmod-fs-ksmbd - ' "$OUTPUT_DIR"/*.manifest
grep -Eq '^uci - ' "$OUTPUT_DIR"/*.manifest
if grep -Eiq '^(luci|rpcd-mod-luci)' "$OUTPUT_DIR"/*.manifest; then
    echo "LuCI package unexpectedly present in custom image manifest" >&2
    exit 1
fi
if grep -Eq '/(root/r3p-api-token|etc/r3p/api-token)$' "$listing_tmp"; then
    echo "A pre-provisioned API token must not be baked into the image" >&2
    exit 1
fi
ROOT_API_TMP="$(mktemp "$OUTPUT_DIR/.verify-api.XXXXXX")"
"$UNSQUASHFS" -cat "$rootfs_tmp" usr/bin/r3p-api > "$ROOT_API_TMP"
if command -v upx >/dev/null 2>&1 && upx -t "$ROOT_API_TMP" >/dev/null 2>&1; then
    upx -d "$ROOT_API_TMP" >/dev/null
fi
cmp -s "$ROOT_API_TMP" "$API_EXPECTED"
rm -f -- "$ROOT_API_TMP"
"$UNSQUASHFS" -cat "$rootfs_tmp" www/index.html | grep -E 'assets/[^"]+\.js' > /dev/null
"$UNSQUASHFS" -cat "$rootfs_tmp" etc/config/uhttpd | grep -E 'option enabled 0' > /dev/null

echo "Custom firmware verification passed."
echo "Factory: $(basename "$factory") ($(stat -c%s "$factory") bytes)"
echo "Sysupgrade: $(basename "$sysupgrade") ($(stat -c%s "$sysupgrade") bytes)"
echo "Kernel payload: $kernel_size bytes (limit $KERNEL_LIMIT)"
