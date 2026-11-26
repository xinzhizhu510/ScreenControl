#!/bin/sh
set -eu
cd "$(dirname "$0")/.."

app="$PWD/build/ScreenControl.app"
version=$(/usr/libexec/PlistBuddy -c 'Print CFBundleShortVersionString' "$app/Contents/Info.plist")
output="$PWD/build/ScreenControl-$version-macos-arm64.dmg"
stage=$(mktemp -d)
trap 'rm -rf "$stage"' EXIT

codesign --verify --deep --strict "$app"
mkdir "$stage/volume"
ditto "$app" "$stage/volume/ScreenControl.app"
ln -s /Applications "$stage/volume/Applications"
hdiutil makehybrid -hfs -hfs-volume-name "ScreenControl $version" \
  -hfs-openfolder "$stage/volume" -o "$stage/ScreenControl.dmg" "$stage/volume"
hdiutil convert "$stage/ScreenControl.dmg" -format UDZO -ov -o "$output"
hdiutil verify "$output"
