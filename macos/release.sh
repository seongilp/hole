#!/bin/bash
# 릴리스 파이프라인: 빌드 → 서명 → DMG 생성/서명 → (가능하면) 공증 → sha256 출력
# 공증 프로파일이 없으면 먼저 한 번만 (APPLE_ID, TEAM_ID, APP_PASSWORD는 ~/.env):
#   source ~/.env
#   xcrun notarytool store-credentials Hole-notary \
#     --apple-id "$APPLE_ID" --team-id "$TEAM_ID" --password "$APP_PASSWORD"
set -euo pipefail
cd "$(dirname "$0")"

APP_NAME="Hole"
APP_BUNDLE="${APP_NAME}.app"
VERSION=$(defaults read "$(pwd)/Resources/Info.plist" CFBundleShortVersionString)
DMG="${APP_NAME}-${VERSION}.dmg"
NOTARY_PROFILE="${APP_NAME}-notary"

./build.sh

echo "==> DMG 생성: ${DMG}"
STAGING=$(mktemp -d)
cp -R "${APP_BUNDLE}" "${STAGING}/"
ln -s /Applications "${STAGING}/Applications"
rm -f "${DMG}"
hdiutil create -volname "${APP_NAME}" -srcfolder "${STAGING}" -ov -format UDZO "${DMG}" -quiet
rm -rf "${STAGING}"

DEV_ID=$(security find-identity -v -p codesigning | grep -o '"Developer ID Application: [^"]*"' | head -1 | tr -d '"' || true)
if [ -n "${DEV_ID}" ]; then
    echo "==> DMG 서명"
    codesign --force --timestamp --sign "${DEV_ID}" "${DMG}"
fi

if xcrun notarytool history --keychain-profile "${NOTARY_PROFILE}" >/dev/null 2>&1; then
    echo "==> 공증 제출 (완료까지 대기)"
    xcrun notarytool submit "${DMG}" --keychain-profile "${NOTARY_PROFILE}" --wait
    echo "==> 스테이플"
    xcrun stapler staple "${DMG}"
else
    echo "==> 공증 스킵: 키체인 프로파일 '${NOTARY_PROFILE}' 없음"
fi

echo "==> 검증"
codesign --verify --verbose=2 "${DMG}" 2>&1 || true
spctl --assess --type open --context context:primary-signature -v "${DMG}" 2>&1 || true

echo ""
shasum -a 256 "${DMG}"
echo "완료: $(pwd)/${DMG}"
