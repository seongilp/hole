# Hole

Pi-hole 스타일의 네트워크 광고 차단 DNS 서버(Go)와 macOS 메뉴바 앱.

광고/트래커 도메인 질의에는 `0.0.0.0` / `::`를 돌려주고, 나머지는 업스트림 DNS로 전달하고 캐시합니다.

## 구성

```
cmd/hole/            서버 진입점 (DNS + 대시보드/API)
internal/blocklist   hosts / 도메인 / ||adblock^ 형식 파싱, 서브도메인 매칭
internal/resolver    DNS 핸들러: 차단 → null 응답, 그 외 → 업스트림(폴백, TCP 재시도)
internal/cache       TTL 기반 응답 캐시
internal/lists       차단 목록 다운로드(URL/파일)
internal/rules       사용자 허용/차단 규칙 (JSON, atomic write)
internal/engine      목록 + 규칙 → matcher 교체, 주기적 갱신
internal/stats       쿼리 통계, 최근 쿼리 로그
internal/api         JSON API + 내장 웹 대시보드
macos/               메뉴바 앱 (AppKit, 서버 바이너리 내장)
```

## 서버 실행

```bash
go build -o bin/hole ./cmd/hole
./bin/hole                         # DNS :53, 대시보드 http://127.0.0.1:4653
./bin/hole -dns 127.0.0.1:5353     # 테스트용 포트
dig @127.0.0.1 ad.doubleclick.net  # → 0.0.0.0
```

| 플래그 | 기본값 | 설명 |
|---|---|---|
| `-dns` | `:53` | DNS 수신 주소 (UDP+TCP) |
| `-http` | `127.0.0.1:4653` | 대시보드/API 주소 |
| `-upstream` | `1.1.1.1:53,8.8.8.8:53` | 업스트림 DNS, 순서대로 시도 |
| `-lists` | StevenBlack hosts | 차단 목록 URL/파일 (쉼표 구분) |
| `-data` | `data` | 사용자 규칙 저장 디렉토리 |
| `-refresh` | `24h` | 목록 갱신 주기 |

`HOLE_API_TOKEN` 환경변수를 지정하면 변경 API(POST/DELETE)에 `Authorization: Bearer <token>`이 필요합니다. 대시보드를 LAN에 열 때(`-http 0.0.0.0:4653`)는 꼭 지정하세요.

## 메뉴바 앱

```bash
cd macos && ./build.sh && open "$PWD/Hole.app"
```

- 메뉴바 아이콘 옆에 차단 비율을 표시합니다. 차단이 꺼지면 아이콘이 바뀌고, 서버에 연결되지 않으면 경고 아이콘이 뜹니다.
- 차단 켜기/끄기(⌘B), 5분·30분·1시간 일시 정지, 목록 갱신(⌘R), 대시보드(⌘D), 로그인 시 실행.
- 서버가 떠 있지 않으면 내장 `hole-server`를 직접 실행하고, 앱 종료 시 함께 정리합니다.
  데이터: `~/Library/Application Support/Hole`, 로그: `~/Library/Logs/Hole/hole.log`
- 설정 변경: `defaults write com.zihado.Hole httpAddr 127.0.0.1:4653` (`dnsAddr`, `apiToken`, `autoStartServer`)

## 실제로 광고를 막으려면

이 Mac만 막으려면: 시스템 설정 → 네트워크 → (Wi-Fi) 세부사항 → DNS에 `127.0.0.1`을 추가합니다.

네트워크 전체를 막으려면: 공유기 DHCP 설정에서 DNS 서버를 이 기기의 LAN IP로 지정합니다.

브라우저의 "보안 DNS(DoH)"가 켜져 있으면 이 서버를 우회하므로 꺼야 합니다.

## 테스트

```bash
go test -race -cover ./...
```
