#!/bin/bash
# Hole 메뉴바 앱 릴리스 빌드: Go 서버 + Swift 앱을 .app 번들로 조립한다.
set -euo pipefail
cd "$(dirname "$0")"

APP_NAME="Hole"
APP_BUNDLE="${APP_NAME}.app"

echo "==> go build (hole 서버)"
mkdir -p .build
(cd .. && CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o macos/.build/hole-server ./cmd/hole)

echo "==> swift build (release)"
swift build -c release

echo "==> ${APP_BUNDLE} 조립"
rm -rf "${APP_BUNDLE}"
mkdir -p "${APP_BUNDLE}/Contents/MacOS" "${APP_BUNDLE}/Contents/Resources"
cp ".build/release/${APP_NAME}" "${APP_BUNDLE}/Contents/MacOS/${APP_NAME}"
cp ".build/hole-server" "${APP_BUNDLE}/Contents/MacOS/hole-server"
cp "Resources/Info.plist" "${APP_BUNDLE}/Contents/Info.plist"
cp "Resources/AppIcon.icns" "${APP_BUNDLE}/Contents/Resources/AppIcon.icns"

echo "==> 코드 서명"
# Developer ID 인증서가 있으면 정식 서명(hardened runtime + timestamp), 없으면 ad-hoc.
# 내장 바이너리를 먼저 서명한 뒤 앱 번들을 서명한다 (--deep 사용 안 함).
DEV_ID=$(security find-identity -v -p codesigning | grep -o '"Developer ID Application: [^"]*"' | head -1 | tr -d '"' || true)
if [ -n "${DEV_ID}" ]; then
    SIGN=(--force --options runtime --timestamp --sign "${DEV_ID}")
    echo "    서명: ${DEV_ID}"
else
    SIGN=(--force --sign -)
    echo "    서명: ad-hoc (Developer ID 없음)"
fi
codesign "${SIGN[@]}" "${APP_BUNDLE}/Contents/MacOS/hole-server"
codesign "${SIGN[@]}" "${APP_BUNDLE}"

echo "완료: $(pwd)/${APP_BUNDLE}"
