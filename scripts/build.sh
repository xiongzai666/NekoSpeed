#!/usr/bin/env bash
# 交叉编译各平台 NekoSpeed 二进制到 dist/，并构建 Android jniLibs 原生库
set -euo pipefail
cd "$(dirname "$0")/.."

VERSION=$(git describe --tags --always 2>/dev/null || echo "dev")
mkdir -p dist

# 内置中文字体（厂商 ROM 系统字体解析失败时经 -font / NEKOSPEED_FONT_PATH 兜底；SIL OFL 许可）
font_dst="android/app/src/main/assets/fonts/NotoSansCJKsc-Regular.otf"
if [ ! -f "$font_dst" ] || [ "$(wc -c < "$font_dst" 2>/dev/null || echo 0)" -le 5242880 ]; then
  echo "==> 下载内置中文字体 NotoSansCJKsc-Regular.otf (~16MB)"
  mkdir -p "$(dirname "$font_dst")"
  tmp_font="${font_dst}.tmp"
  rm -f "$tmp_font"
  if curl -fsSL -o "$tmp_font" \
    "https://cdn.jsdelivr.net/gh/notofonts/noto-cjk@main/Sans/OTF/SimplifiedChinese/NotoSansCJKsc-Regular.otf" \
    || curl -fsSL -o "$tmp_font" \
    "https://raw.githubusercontent.com/notofonts/noto-cjk/main/Sans/OTF/SimplifiedChinese/NotoSansCJKsc-Regular.otf"; then
    font_size=$(wc -c < "$tmp_font" 2>/dev/null || echo 0)
    if [ "$font_size" -gt 5242880 ]; then
      mv -f "$tmp_font" "$font_dst"
    else
      rm -f "$tmp_font"
      echo "!! 字体文件过小 (${font_size} bytes)：请手动放置到 $font_dst"
    fi
  else
    rm -f "$tmp_font"
    echo "!! 字体下载失败：请手动放置到 $font_dst"
  fi
fi

targets=(
  "windows amd64 .exe"
  "windows arm64 .exe"
  "darwin amd64"
  "darwin arm64"
  "linux amd64"
  "linux arm64"
)

for t in "${targets[@]}"; do
  read -r goos goarch ext <<<"$t"
  out="dist/nekospeed-${goos}-${goarch}${ext}"
  echo "==> $out"
  CGO_ENABLED=0 GOOS=$goos GOARCH=$goarch \
    go build -trimpath -ldflags "-s -w -X nekospeed/internal/runner.Version=${VERSION}" -o "$out" ./cmd/nekospeed
done

host_tag="linux-x86_64"
default_sdk="$HOME/Android/Sdk"
if [ "$(uname -s)" = "Darwin" ]; then
  host_tag="darwin-x86_64"
  default_sdk="$HOME/Library/Android/sdk"
fi
sdk_root="${ANDROID_HOME:-${ANDROID_SDK_ROOT:-$default_sdk}}"

resolve_ndk_bin() {
  local candidate="$1"
  if [ -z "$candidate" ]; then
    return 1
  fi
  if [ -d "$candidate" ] && ls "$candidate"/*-linux-android*-clang >/dev/null 2>&1; then
    echo "$candidate"
    return 0
  fi
  if [ -d "$candidate/toolchains/llvm/prebuilt/$host_tag/bin" ]; then
    echo "$candidate/toolchains/llvm/prebuilt/$host_tag/bin"
    return 0
  fi
  local any_bin
  any_bin=$(ls -d "$candidate"/toolchains/llvm/prebuilt/*/bin 2>/dev/null | sort -V | tail -1 || true)
  if [ -n "$any_bin" ] && [ -d "$any_bin" ]; then
    echo "$any_bin"
    return 0
  fi
  return 1
}

ndk_bin=""
for ndk_var in "${ANDROID_NDK_LATEST_HOME:-}" "${ANDROID_NDK_HOME:-}" "${ANDROID_NDK:-}"; do
  if ndk_bin=$(resolve_ndk_bin "$ndk_var"); then
    break
  fi
done
if [ -z "$ndk_bin" ] && [ -d "$sdk_root/ndk" ]; then
  latest_ndk=$(ls -d "$sdk_root"/ndk/* 2>/dev/null | sort -V | tail -1 || true)
  ndk_bin=$(resolve_ndk_bin "$latest_ndk" || true)
fi

find_ndk_cc() {
  local triple="$1"
  if [ -z "$ndk_bin" ]; then
    return 0
  fi
  for api in 26 21; do
    if [ -e "$ndk_bin/${triple}${api}-clang" ]; then
      echo "$ndk_bin/${triple}${api}-clang"
      return 0
    fi
  done
  ls "$ndk_bin/${triple}"*-clang 2>/dev/null | sort -V | head -1 || true
}

android_abis="${ANDROID_ABIS:-arm64}"
android_abis="${android_abis//,/ }"
page_flags="-Wl,-z,max-page-size=16384 -Wl,-z,common-page-size=16384"

for abi_req in $android_abis; do
  case "$abi_req" in
    arm64|arm64-v8a)
      goarch="arm64"
      abi_dir="arm64-v8a"
      triple="aarch64-linux-android"
      ;;
    amd64|x86_64)
      goarch="amd64"
      abi_dir="x86_64"
      triple="x86_64-linux-android"
      ;;
    *)
      echo "!! 不支持的 Android ABI: $abi_req（支持 arm64 / amd64），跳过"
      continue
      ;;
  esac

  android_out="android/app/src/main/jniLibs/${abi_dir}/libnekospeed.so"
  cc=$(find_ndk_cc "$triple")
  if [ -z "$cc" ]; then
    echo "!! 未找到 NDK clang（${triple}26-clang / ${triple}21-clang），跳过 ${abi_dir} 构建"
    continue
  fi

  echo "==> $android_out (CC=$cc)"
  mkdir -p "$(dirname "$android_out")"
  # 统一走 NDK cgo + 16 KB ELF 页对齐，打入 jniLibs 由系统解压到 nativeLibraryDir 执行（绕过 Android 10+ W^X 限制）
  CGO_ENABLED=1 \
  GOOS=android \
  GOARCH="$goarch" \
  CC="$cc" \
  CGO_LDFLAGS="$page_flags" \
    go build -trimpath \
      -ldflags "-s -w -linkmode=external -extldflags '${page_flags}' -X nekospeed/internal/runner.Version=${VERSION}" \
      -o "$android_out" ./cmd/nekospeed
done

echo "完成，桌面产物在 dist/，Android jniLibs 产物在 android/app/src/main/jniLibs/"
