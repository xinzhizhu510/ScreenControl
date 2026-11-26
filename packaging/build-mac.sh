#!/bin/sh
set -eu
cd "$(dirname "$0")/.."

app="$PWD/build/ScreenControl.app"
mkdir -p "$app/Contents/MacOS" "$app/Contents/Resources" "$PWD/build/clang-cache"
cp packaging/Info.plist "$app/Contents/Info.plist"
if [ -n "${SCREENCONTROL_VERSION:-}" ]; then
  /usr/libexec/PlistBuddy -c "Set :CFBundleShortVersionString $SCREENCONTROL_VERSION" "$app/Contents/Info.plist"
fi
cp packaging/ScreenControl.icns "$app/Contents/Resources/ScreenControl.icns"
npm --prefix frontend ci
npm --prefix frontend run build
make -C third_party/m1ddc CFLAGS="-Wall -Werror -Wextra -fmodules -fmodules-cache-path=$PWD/build/clang-cache"
CGO_ENABLED=1 GOARCH=arm64 go build -trimpath -tags desktop,production -o "$app/Contents/MacOS/ScreenControl" .
cp third_party/m1ddc/m1ddc "$app/Contents/MacOS/m1ddc"
cp third_party/m1ddc/LICENSE "$app/Contents/Resources/m1ddc-LICENSE.txt"
codesign --force --sign - "$app/Contents/MacOS/m1ddc"
codesign --force --sign - "$app"
codesign --verify --deep --strict "$app"
