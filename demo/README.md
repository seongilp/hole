# Hole 데모 Worker

랜딩 페이지용 데모입니다. 방문자가 URL을 입력하면 Cloudflare Browser Rendering(헤드리스 Chrome)으로 페이지를 열어 로드되는 모든 호스트를 수집하고, 그중 Hole 차단 목록에 걸리는 것을 알려줍니다.

- `POST /api/scan` `{ "url": "example.com" }` → 검사 결과
- `GET /api/health`
- 모든 응답은 `{ success, data, error }` 형식입니다.

## 배포

```sh
cd demo
npm install
npx wrangler login      # 최초 1회
npm run deploy          # 차단 목록을 새로 받아 빌드(build:list)한 뒤 배포
```

- KV 네임스페이스(`SCANS`)는 `wrangler.jsonc`에 id가 없으면 첫 배포 때 자동으로 만들어지고 id가 설정 파일에 기록됩니다.
- Rate Limiting 바인딩의 `namespace_id`(`41017`)는 계정 안에서 고유해야 합니다. 겹치면 다른 숫자로 바꾸세요.
- 허용 출처는 `wrangler.jsonc`의 `ALLOWED_ORIGINS`(쉼표 구분)에서 바꿀 수 있습니다.

## 개발

```sh
npm run build:list   # src/blocklist.txt, src/blocklist-meta.json 생성 (git 제외 대상)
npm test             # vitest (Workers 런타임 불필요)
npm run typecheck
npm run dev          # 브라우저 바인딩이 remote: true라 로그인 필요
```

`scripts/parse-hosts.mjs`는 Go 파서(`internal/blocklist/blocklist.go`)와 같은 규칙을 따릅니다. Go 쪽을 바꾸면 함께 바꿔 주세요.

## 무료 플랜 한도

Workers Free의 Browser Rendering은 하루 10 브라우저 분, 동시 브라우저 3개, 새 브라우저 20초에 1개입니다. 검사 한 번에 약 15~25초가 걸려서 하루 약 25~40회 정도 가능합니다. 같은 URL은 24시간 캐시하고 IP당 분당 5회로 제한해 한도를 보호합니다. 한도를 넘으면 503과 안내 문구를 돌려줍니다(UTC 자정에 초기화).
