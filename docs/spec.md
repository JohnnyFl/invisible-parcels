# Технічна специфікація

Бізнес-контекст: [problem-solution.md](problem-solution.md).

## 1. Компоненти

```
┌──────────────┐  push: події (HMAC)   ┌──────────────┐      ┌──────────┐
│  emulator    │ ────────────────────► │   backend    │ ◄──► │ postgres │
│ (світ НП,    │ ◄──────────────────── │ (Go, Echo)   │      └──────────┘
│  in-memory)  │  pull: /v2.0/json/    │              │
│              │ ◄──────────────────── │              │
└──────────────┘  admin: годинник,     └──────┬───────┘
                  сценарії                    │ /api + SSE
                                       ┌──────┴───────┐
                                       │ web (Caddy + │ ◄── браузер (HTTPS, HTTP/2)
                                       │ React build) │
                                       └──────────────┘
```

| Сервіс | Стек | Порт (локально) |
|---|---|---|
| `backend` | Go 1.27, Echo v5.4, pgx v5 | 8080 |
| `emulator` | Go 1.27, Echo v5.4, стан у пам'яті | 8090 |
| `web` | React 19.3, Vite 8.3, TypeScript 7.0, TanStack Router 1.170, TanStack Query 5.104, Tailwind 4.3; у Docker — Caddy 2 | 5173 (dev) / 443 (Docker) |
| `postgres` | PostgreSQL 18.6 | 5432 |

## 2. Репозиторій

```
nova_post/
  go.mod                 # один Go-модуль `novapost` на backend, emulator і contract
  contract/              # спільні типи: події, конверт НП v2.0, StatusCode, HMAC
  backend/
    cmd/main.go
    store/migrations/*.sql  # вбудовуються (embed) і виконуються при старті
    ...
  emulator/
    cmd/main.go
    scenarios/           # сценарії 1–6 як Go-код
    ...
  web/                   # Vite-проєкт, Caddyfile, Dockerfile
  docs/
  docker-compose.yml
  Makefile
```

Чому один `go.mod`: контракт подій — єдине джерело правди для обох сервісів, а `air` підключено як `tool` в одному місці.

## 3. Доменна модель

**Локація** (`locations`): відділення (`branch`) або сортувальний центр (`hub`). Seed: Київ №12, Львів №5, Харків №3, СЦ Київ, СЦ Львів, СЦ Харків.

**Посилка** (`parcels`): `ttn` (14 цифр, `59…`), телефони відправника й отримувача, відділення відправлення й призначення.

**Рейс** (`trip_id`, `vehicle_id`): авто між двома локаціями. Посилки прив'язуються до рейсу планом маршруту.

**План маршруту** (`route_legs`): впорядковані плечі посилки `from → to`, кожне з `trip_id`, плановим відправленням і прибуттям. План приходить разом із подією `created`.

**Подія** (`events`): журнал, який тільки доповнюється. Стан посилки й рівень проблеми **не зберігаються**, а обчислюються з подій, плану, знімка клієнтського статусу та поточного часу.

## 4. Контракт емулятора (наш «НП»)

### 4.1. Клієнтський шар — копія НП v2.0 (pull)

`POST {EMULATOR_URL}/v2.0/json/`

```json
{
  "apiKey": "demo",
  "modelName": "TrackingDocument",
  "calledMethod": "getStatusDocuments",
  "methodProperties": { "Documents": [{ "DocumentNumber": "59000000000012", "Phone": "" }] }
}
```

```json
{
  "success": true,
  "data": [{
    "Number": "59000000000012",
    "StatusCode": "6",
    "Status": "У місті одержувача, орієнтовна доставка до ВІДДІЛЕННЯ №12",
    "ScheduledDeliveryDate": "19-10-2026 15:00:00",
    "WarehouseRecipient": "Відділення №12",
    "CityRecipient": "Київ"
  }],
  "errors": [], "warnings": [], "info": []
}
```

До 100 ТТН за запит. Backend опитує всі невидані посилки кожні `NP_POLL_INTERVAL` (типово 2 с реального часу) і зберігає знімок у `np_status`.

**Як емулятор рахує клієнтський статус** (це і є модель гіпотези): за **рухом рейсу** з плану маршруту, а не за сканами посилки. Рейс прибув у місто призначення → `6` з `ScheduledDeliveryDate` за розкладом. `7` лише після скану `unloaded`/`arrived_branch` у відділенні призначення. Якщо скан вивантаження пропущено, клієнт бачить `6` і прогноз, а посилка вже на складі. Це кейс із документа.

Відповідність подій і кодів (звірити з офіційною документацією):

| Стан у світі емулятора | StatusCode |
|---|---|
| ТТН створено | 1 |
| прийнято у відділенні відправника | 4 |
| рейс з посилкою в дорозі / на СЦ | 5 |
| рейс прибув у місто одержувача | 6 |
| вивантажено / прибула у відділення | 7 |
| видана | 9 |
| переадресована | 104 |
| ініційовано повернення | 106 |
| інцидент / пошкодження | останній код без змін (публічного коду немає) |

### 4.2. Шар сканів — пропонований контракт (push)

`POST {BACKEND_URL}/api/ingest/events`, заголовок `X-Signature: hex(HMAC-SHA256(body, INGEST_SECRET))`.

```json
{
  "modelName": "ScanEvent",
  "calledMethod": "push",
  "methodProperties": {
    "Events": [{
      "EventId": "0f8c…",
      "Kind": "unloaded",
      "TTN": "59000000000012",
      "TripId": "",
      "LocationId": "branch-kyiv-12",
      "OccurredAt": "2026-10-18T14:05:00Z",
      "Source": "np",
      "Payload": {}
    }]
  }
}
```

Типи (`Kind`):

| Kind | Рівень | Payload |
|---|---|---|
| `created` | посилка | телефони, відділення, `RouteLegs[]` |
| `accepted` | посилка | |
| `loaded` | посилка | `TripId` |
| `trip_departed` | рейс | `TripId` |
| `trip_arrived` | рейс | `TripId` |
| `arrived_hub`, `sorted` | посилка | |
| `unloaded` | посилка | `TripId` |
| `arrived_branch` | посилка | |
| `issued` | посилка | |
| `redirected` | посилка | `NextLocationId` |
| `return_initiated` | посилка | `Reason`, `Comment` |
| `damaged` | посилка | |
| `incident` | рейс або локація | `TripId` або `LocationId`, `Cause` |
| `operator_scan` | посилка | (лише `source = "operator"`) скан у відділенні; ТТН може бути невідомою |
| `hold` | посилка | (лише `source = "operator"`) «На ідентифікації» |
| `check_requested` | посилка | (`source = "senior"`) доручення відділенню `LocationId` знайти посилку; `Note` |
| `check_found` / `check_not_found` | посилка | (`source = "operator"`) відповідь відділення на доручення |
| `return_confirmed` | посилка | (`source = "senior"`) повернення обґрунтоване; `Comment`. Знімає `risky_return` |

`check_*` і `return_confirmed` — службові події: вони не передаються в емулятор, а класифікатор не вважає їх рухом посилки (не скидають «Застигла»).

Правила прийому:
- **Ідемпотентність**: `event_id` є первинним ключем, дубль ігнорується (`ON CONFLICT DO NOTHING`), відповідь 200.
- **Порядок**: стан обчислюється за `occurred_at`, а не за `received_at`. Емулятор навмисно додає затримку 0–N хвилин і перемішує частину подій.
- Невірний підпис → 401. Невідомий `Kind` зберігається (тип — рядок), класифікатор його ігнорує.
- Дії оператора (прийняти, переадресувати, повернути) пишуться в той самий журнал із `source = "operator"`.

### 4.3. Admin API емулятора (тільки для демо)

| Метод | Що робить |
|---|---|
| `POST /admin/clock` | `{ virtualNow, speed }`. Встановлює годинник (його надсилає backend) |
| `POST /admin/reset` | очищає світ |
| `POST /admin/scenarios/{id}` | завантажує сценарій 1–6 |
| `POST /admin/parcels/random` | `{count}` → посилки з випадковою долею (`normal`, `not_loaded`, `missed_scans`, `misrouted`, `unknown`, `incident`; звичайних ≈ 4 з 9), що стартують від поточного віртуального часу. ТТН і ID подій беруться з лічильника, який стартує від реального часу, тож не повторюються після перезапуску |
| `GET /admin/state` | `{ scenario }` — який сценарій завантажено (0 після перезапуску). Backend кожні 2 с звіряє його зі своїм і після перезапуску емулятора відновлює сценарій, годинник і дії операторів |
| `GET /admin/physical?location=` | які посилки **фізично** лежать у локації (панель «Склад» у демо) |
| `POST /admin/operator-event` | дія оператора (`contract.Event`, `source = "operator"`) потрапляє у світ емулятора, і клієнтський статус оновлюється (наприклад, після «Прийняти» → `7`) |

## 5. Віртуальний годинник

Єдине джерело — backend. Стан: `realBase`, `virtualBase`, `speed` (0 = пауза, 1, 60, 600). `now = virtualBase + (real − realBase) × speed`. Кожна зміна дублюється в емулятор через `POST /admin/clock`, тож обидва сервіси рахують однаковий час за однією формулою. Усі пороги, ETA й ескалації рахуються від `clock.Now()`. Перемотка до події = встановити `virtualBase` на час наступної запланованої події.

## 6. Класифікація проблемних посилок

Чиста функція `Classify(parcel, legs, events, npStatus, now) → []Problem`, без БД і без часу зсередини. Тестується таблицею.

Константи (`backend/problems/rules.go`):

```go
OverdueAfter      = 2 * time.Hour   // після ETA
StuckAfter        = 12 * time.Hour  // без подій
UnloadGrace       = 1 * time.Hour   // після прибуття рейсу, перш ніж відсутній скан вивантаження стане проблемою
EscalateOrange    = 6 * time.Hour   // 🟠 → 🔴
EscalateYellow    = 24 * time.Hour  // 🟡 → 🟠
```

| Проблема | Умова | Початок (для ескалації) | Рівень |
|---|---|---|---|
| `unknown` | оператор відсканував ТТН, і її не знайдено | час скану | 🔴 |
| `misrouted` | остання фізична локація — відділення, якого немає в плані маршруту | час події | 🔴 |
| `risky_return` | `return_initiated` для посилки з активною 🔴/🟠 проблемою або в статусі «на ідентифікації» | час події | 🔴 |
| `broken_chain` | `trip_arrived` у локацію X, посилка в маніфесті цього рейсу, а `unloaded` у X немає довше за `UnloadGrace` (скани приходять із затримкою й не по порядку, а вивантаження триває) | час прибуття рейсу + `UnloadGrace` | 🟠 |
| `overdue` | `now > ETA + OverdueAfter` і немає `arrived_branch`/`unloaded` у відділенні призначення | `ETA + OverdueAfter` | 🟠 |
| `stuck` | не видана, остання подія посилки або її рейсу старша за `StuckAfter` | остання подія + `StuckAfter` | 🟡 |
| `incident` | інцидент на рейсі посилки або в її поточній локації | час інциденту | ⚪ |

- ETA = `ScheduledDeliveryDate` зі знімка `np_status`, якщо він є, інакше планове прибуття останнього плеча.
- Ескалація: 🟠, що триває довше `EscalateOrange`, стає 🔴; 🟡 довше `EscalateYellow` стає 🟠.
- `incident` пригнічує `stuck` і `overdue` (відома причина). На `unknown`, `misrouted` і `risky_return` він не впливає.
- Рівень посилки = найвищий серед її проблем. Проблема зникає сама, щойно умова перестає виконуватись.

<!-- ponytail: класифікація на кожен запит по всіх невиданих посилках відділення; для демо (сотні посилок) достатньо, при тисячах — матеріалізувати рівень при прийомі події -->

## 7. Режими «як зараз» / «з рішенням»

Глобальний перемикач (стан у backend, змінює адмін).

| | Як зараз | З рішенням |
|---|---|---|
| Список надходжень оператора | лише посилки з подією `loaded` на рейс у це відділення | усі посилки з планом маршруту в це відділення |
| Проблемні посилки | приховано | головний екран |
| Скан невідомої посилки | «не знайдено» → лише кнопка «Повернути» | глобальний пошук → автоматичний результат |
| Повернення | без причини й контролю | причина обов'язкова, видно старшій зміні |

## 8. Пошук невідомої посилки (Q11)

`POST /api/operator/scan { ttn }`:
- доступний ролі `operator`, відділення береться з cookie;
- відповідь мінімальна: `{ outcome: "accept" | "redirect" | "hold", nextLocation? }`, без телефонів, імен і кінцевої адреси;
- ліміт `LOOKUP_LIMIT_PER_HOUR` (типово 30) на відділення, у пам'яті; перевищення → 429;
- кожен виклик пишеться в `audit_log`.
- «Тільки відскановані»: у демо скан виконується з панелі «Склад», без поля для ручного вводу. У продакшені це забезпечує сканер.

<!-- ponytail: rate limit у пам'яті процесу, скидається при рестарті; для кількох інстансів — у Postgres або Redis -->

Наступні дії: `POST /api/operator/parcels/{ttn}/accept | redirect | hold | return`. Для `return` обов'язкове поле `reason ∈ {damaged, refused, other}`, для `other` також `comment`.

## 9. Авторизація

- Своя форма входу. Ролі: `admin` (симулятор), `senior` (усі відділення), `operator` (одне відділення, вибирається на формі).
- Паролі з env: `ADMIN_PASSWORD`, `SENIOR_PASSWORD`, `OPERATOR_PASSWORD` (типові значення для dev + попередження в лозі). Порівняння через `subtle.ConstantTimeCompare`.
- Cookie `session = base64(role|branch|expiresAt) + "." + hex(HMAC-SHA256(…, APP_SECRET))`, `HttpOnly`, `SameSite=Lax`, `Secure` за HTTPS. Таблиці сесій немає.
- Клієнт без пароля: `GET /api/track/{ttn}?phone=`. Без телефону віддається лише статус і ETA (з позначкою «прогноз»), з правильним телефоном — повні деталі.

## 10. API backend

| Метод і шлях | Роль | Призначення |
|---|---|---|
| `POST /api/login`, `POST /api/logout`, `GET /api/me` | — | сесія |
| `GET /api/locations` | — | список відділень для форми входу |
| `POST /api/ingest/events` | HMAC | прийом подій від емулятора |
| `GET /api/track/{ttn}?phone=` | публічний | клієнтський трекінг |
| `GET /api/operator/incoming` | operator | очікувані надходження |
| `GET /api/operator/problems` | operator | проблемні посилки відділення |
| `POST /api/operator/scan` | operator | пошук невідомої посилки |
| `POST /api/operator/parcels/{ttn}/{action}` | operator | accept / redirect / hold / return |
| `GET /api/senior/problems` | senior | проблемні посилки всіх відділень, ризикові повернення |
| `POST /api/senior/parcels/{ttn}/check` | senior | `{branchId, note?}`: доручити перевірку відділенню |
| `POST /api/senior/parcels/{ttn}/confirm-return` | senior | `{comment}` (обов'язково): підтвердити ризикове повернення |
| `GET /api/operator/tasks` | operator | відкриті доручення для відділення |
| `POST /api/operator/tasks/{ttn}/answer` | operator | `{found}`: відповідь на доручення; без відкритого доручення 404 |
| `GET /api/senior/trace/{ttn}` | senior | «Де обірвався слід»: маршрут за планом поруч із фактичними сканами (§12а) |
| `GET /api/operator/trace/{ttn}` | operator | те саме, але лише для посилок, які відділення вже бачить (адресовані йому, у маршруті через нього, востаннє бачені в ньому); інакше 404; кожен запит в `audit_log` |
| `POST /api/sim/accept/{ttn}` | admin | для туру «Показати кейс»: оператор приймає посилку у відділенні призначення |
| `POST /api/sim/parcels/random` | admin | `{count: 1..20}`: додає випадкові посилки до поточного світу (без скидання), запускає час ×60, якщо він стоїть. Відповідь: `[{ttn, kind, lookAt}]` |
| `GET /api/stream` | будь-яка | SSE |
| `GET/POST /api/sim/clock` | admin | годинник |
| `POST /api/sim/mode` | admin | `as_is` / `with_solution` |
| `POST /api/sim/reset`, `POST /api/sim/scenarios/{id}` | admin | запуск сценарію (проксі в емулятор + очищення БД) |
| `GET /api/sim/split/{ttn}` | admin | клієнтський і операторський вигляд в один момент |
| `GET /api/sim/physical?location=` | operator, admin | панель «Склад» (проксі в емулятор, лише демо) |

SSE: сервер надсилає лише `{"type":"invalidate","keys":["problems","incoming",…]}` після кожної прийнятої події, дії оператора чи зміни годинника, а також раз на 5 с реального часу, поки годинник іде. Клієнт викликає `queryClient.invalidateQueries` і перезапитує дані. Самі дані через SSE не передаються.

## 11. Схема БД

```sql
locations   (id text pk, kind text, city text, name text)
parcels     (ttn text pk, sender_phone text, recipient_phone text,
             origin_id text, destination_id text, created_at timestamptz)
route_legs  (ttn text, seq int, from_id text, to_id text, trip_id text,
             planned_departure timestamptz, planned_arrival timestamptz,
             primary key (ttn, seq))
events      (event_id text pk, kind text, ttn text null, trip_id text null,
             location_id text null, occurred_at timestamptz, received_at timestamptz,
             source text, payload jsonb)
np_status   (ttn text pk, status_code text, status_text text,
             scheduled_delivery timestamptz null, fetched_at timestamptz)
audit_log   (id bigserial pk, at timestamptz, role text, branch_id text,
             action text, ttn text, result text)
app_state   (key text pk, value jsonb)  -- годинник, режим
```

Індекси: `events(ttn, occurred_at)`, `events(trip_id)`, `route_legs(to_id)`, `route_legs(trip_id)`.

`parcels` і `route_legs` наповнюються з події `created`. Окремого API для створення посилок немає.

## 12. Сценарії емулятора

Кожен сценарій — Go-функція, що будує світ: посилки, рейси, плани, розклад подій у віртуальному часі та «злам». Тло в кожному сценарії — 20–30 нормальних посилок, щоб проблемна не була єдиною в списку.

| # | Злам | Що має побачити оператор «з рішенням» |
|---|---|---|
| 1 | фокус-посилку **не завантажили**: фізично лишилась на СЦ Київ, рейс поїхав без неї (подій `loaded`/`unloaded` немає) | «як зараз»: її немає в надходженнях; «з рішенням»: в очікуваних, після прибуття рейсу — 🟠 `broken_chain`; у «Складі» СЦ видно посилку |
| 2 | рейс прибув у Київ №12, `unloaded` для фокус-посилки не відправлено, вона фізично на складі | 🟠 `broken_chain` → 🔴 через 6 год; у «Складі» видно посилку, скан → «Прийняти» |
| 3 | посилку вивантажено у Львів №5 замість Київ №12 | 🔴 `misrouted` у Львові; скан → «Переадресувати», наступна точка — СЦ Львів |
| 4 | невідома посилка у Харкові №3, оператор у режимі «як зараз» повертає її | «як зараз»: посилка зникає; «з рішенням»: 🔴 `risky_return` у старшої зміни |
| 5 | `incident` на рейсі СЦ Київ → Київ №12 | усі посилки рейсу ⚪, не 🟡 |
| 6 | `incident` на локації СЦ Київ (удар по терміналу) до прибуття рейсу; посилки обробляють вручну: у 1–3 пропущено скани на СЦ, у 1–2 ще й завантаження й вивантаження на останньому плечі | 1–2 🔴 `broken_chain`, лежать на полиці Київ №12 без реєстрації; 3–4 дійшли; «Де обірвався слід» показує, на якому плечі обірвався ланцюг |

## 12а. «Де обірвався слід»

Чиста функція `TraceParcel(Input) → Trace` (поруч із `Classify`). Для кожного плеча маршруту — фактичні моменти завантаження, виїзду рейсу, прибуття рейсу, вивантаження і стан:

| Стан | Умова |
|---|---|
| `done` | є `unloaded`/`arrived_branch` у пункті призначення плеча |
| `incident` | інцидент на рейсі плеча або в його початковій чи кінцевій точці |
| `broken` | рейс прибув, а вивантаження немає; або рейс виїхав без підтвердженого завантаження |
| `in_transit` | посилку завантажено, рейс виїхав, ще не прибув |
| `pending` | плече ще не почалось |

Перше `broken` плече дає підказку людською мовою, де шукати (наприклад: «Рейс прибув у Сортувальний центр Київ, але вивантаження посилки не підтверджено. Перевірте полиці й зону розвантаження»). Також повертається останній підтверджений скан посилки.

## 13. Конфігурація

| Змінна | Типове значення (dev) |
|---|---|
| `DATABASE_URL` | `postgres://novapost:novapost@localhost:5432/novapost?sslmode=disable` |
| `EMULATOR_URL` | `http://localhost:8090` |
| `BACKEND_URL` (для емулятора) | `http://localhost:8080` |
| `INGEST_SECRET` | `dev-ingest-secret` |
| `APP_SECRET` | `dev-app-secret` |
| `ADMIN_PASSWORD` / `SENIOR_PASSWORD` / `OPERATOR_PASSWORD` | `admin` / `senior` / `operator` |
| `NP_POLL_INTERVAL` | `2s` |
| `LOOKUP_LIMIT_PER_HOUR` | `30` |

У `docker-compose.yml` адреси перевизначаються на `postgres:5432`, `emulator:8090`, `backend:8080`.

## 14. Запуск

**Розробка** (кожна ціль у своєму терміналі):

```
make db         # docker compose up -d postgres
make backend    # cd backend && go tool air
make emulator   # cd emulator && go tool air
make web        # cd web && npm run dev  (Vite, HMR, проксі /api → :8080)
```

**Повний запуск:** `make up` (`docker compose up --build`), далі `https://localhost`. Щоб браузер не попереджав про сертифікат, один раз виконати `caddy trust` або підкласти сертифікат з `mkcert`.

Caddyfile:
```
localhost {
  tls internal
  handle /api/* { reverse_proxy backend:8080 { flush_interval -1 } }
  handle { root * /srv; try_files {path} /index.html; file_server }
}
```

**Тести:** `make test` (`go test ./...`).

## 15. Фронтенд

Маршрути (TanStack Router, file-based):

| Шлях | Роль | Вміст |
|---|---|---|
| `/login` | — | роль → відділення (для оператора) → пароль |
| `/track` | публічний | ТТН + телефон, статус, ETA з позначкою «прогноз» |
| `/operator` | operator | проблемні посилки (за кольорами), очікувані надходження, панель «Склад» (демо), скан |
| `/senior` | senior | проблемні посилки всіх відділень, ризикові повернення |
| `/sim` | admin | годинник, сценарії, перемикач режиму, розділений вигляд «клієнт \| оператор» |

Дизайн:
- фірмовий червоний НП `#DA291C` лише для шапки, основних кнопок і акцентів; текст `#202124`, вторинний текст `#5F6368`, фон `#F8F9FA`;
- статуси мають власні відтінки, не плутаються з фірмовим червоним і **завжди** мають іконку + текстову мітку;
- логотипа НП немає, у шапці підпис «Концепт для Нової пошти»;
- інтерфейс українською.

## 16. Тести

1. **Класифікація**: табличний тест `Classify`, по рядку на кожну проблему, ескалацію, пригнічення інцидентом і «норму».
2. **Кейс із документа** (інтеграційний, з реальним Postgres): події сценарію 2 → `POST /api/ingest/events` → у `GET /api/operator/problems` посилка 🟠, після зсуву годинника на 6 год — 🔴.
3. **Прийом подій**: дубль `event_id` не змінює стан; події, надіслані у зворотному порядку, дають той самий стан; невірний HMAC → 401.

Фронтенд поки без тестів.
