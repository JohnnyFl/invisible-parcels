# Невидимі посилки — план реалізації

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Прототип, що звіряє клієнтський трекінг НП зі сканами відділень і показує оператору «невидимі» посилки до того, як їх втратять.

**Architecture:** Go-емулятор тримає «світ НП» у пам'яті. Він віддає клієнтські статуси у форматі НП v2.0 (pull) і надсилає скани вебхуком (push). Go-backend зберігає події в Postgres (журнал тільки доповнюється), а проблеми рахує чистою функцією від подій і віртуального часу. React-фронтенд показує екрани оператора, старшої зміни, клієнта й симулятора; оновлення приходять через SSE.

**Tech Stack:** Go 1.27, Echo v5.4, pgx v5, PostgreSQL 18.6, React 19.3, Vite 8.3, TypeScript 7.0, TanStack Router 1.170, TanStack Query 5.104, Tailwind 4.3, Caddy 2, air (через `go tool`).

**Spec:** [docs/spec.md](../../spec.md) (технічна), [docs/problem-solution.md](../../problem-solution.md) (бізнес). Виконавець читає обидва.

## Global Constraints

- Один Go-модуль `novapost` у корені; пакети `contract/`, `backend/...`, `emulator/...`.
- Залежності Go: лише `github.com/labstack/echo/v5`, `github.com/jackc/pgx/v5`, tool `github.com/air-verse/air`. Без ORM і без бібліотек для сесій чи rate limit.
- Залежності web: `react`, `react-dom`, `@tanstack/react-router`, `@tanstack/router-plugin`, `@tanstack/react-query`, `tailwindcss`, `@tailwindcss/vite`, `@vitejs/plugin-react`, `typescript`. Більше нічого.
- ID локацій: `branch-kyiv-12`, `branch-lviv-5`, `branch-kharkiv-3`, `hub-kyiv`, `hub-lviv`, `hub-kharkiv`.
- ТТН: `fmt.Sprintf("59%012d", n)` (14 цифр). Валідація: `^59\d{12}$`.
- Пороги: `OverdueAfter = 2h`, `StuckAfter = 12h`, `EscalateOrange = 6h`, `EscalateYellow = 24h` (лише в `backend/problems/rules.go`).
- Час у backend і emulator береться **тільки** з `contract.Clock.Now(time.Now())`, ніде не з `time.Now()` напряму (крім `received_at`).
- Рівні в JSON — рядки: `"red" | "orange" | "yellow" | "grey" | "green"`.
- Інтерфейс українською. Фірмовий червоний `#DA291C` лише для шапки й основних кнопок. Текст `#202124`, вторинний `#5F6368`, фон `#F8F9FA`. Статус завжди має іконку + текст. Логотипа НП немає, у шапці «Концепт для Нової пошти».
- Дати у форматі НП (`ScheduledDeliveryDate`) — у часовому поясі `Europe/Kyiv`; `contract` імпортує `time/tzdata` (distroless без tzdata).
- Порти: backend `8080`, emulator `8090`, web dev `5173`.
- Тести з БД беруть `TEST_DATABASE_URL` (типово окрема база `novapost_test`, створюється автоматично) і запускаються з `go test -p 1 ./...`.
- Кожен коміт закінчується рядком `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`.

## Review Focus

1. **Емулятор недоступний**: poller пише в лог і лишає останній знімок `np_status`, процес не падає; `/api/sim/*`, що проксюють у емулятор, повертають 502. Тест у Task 7.
2. **Підроблена або прострочена cookie, чужа роль**: змінений підпис чи `expiresAt` у минулому → 401; оператор на `/api/senior/*` чи `/api/sim/*` → 403. Тест у Task 8.
3. **Перевищено ліміт пошуку**: 31-й скан за годину з того ж відділення → 429, спроба все одно в `audit_log`. Тест у Task 9.
4. **Клієнтський трекінг**: неправильний телефон → лише статус без `details`; невідома ТТН → 404. Тест у Task 9.
5. **Некоректна ТТН** (не `^59\d{12}$`) у `scan` і `track` → 400, ліміт не витрачається. Тест у Task 9.

---

## Структура файлів

```
go.mod                         # module novapost; tool github.com/air-verse/air
Makefile
docker-compose.yml
README.md
contract/
  contract.go                  # конверт НП, події, StatusCode, локації
  sign.go                      # HMAC
  clock.go                     # віртуальний годинник
  contract_test.go
emulator/
  cmd/main.go
  .air.toml
  Dockerfile
  world/world.go               # стан світу, статус клієнта, фізика
  world/world_test.go
  scenarios/scenarios.go       # сценарії 1–5 + фонові посилки
  scenarios/scenarios_test.go
  push/push.go                 # відправка подій з джитером
  api/api.go                   # /v2.0/json/, /admin/*
  api/api_test.go
backend/
  cmd/main.go
  .air.toml
  Dockerfile
  config/config.go
  store/store.go               # підключення, міграції, reset
  store/migrations/001_init.sql
  store/events.go              # InsertEvents, читання подій
  store/queries.go             # вибірки для екранів
  store/testdb_test.go         # helper для тестів з БД
  store/events_test.go
  problems/rules.go
  problems/classify.go
  problems/classify_test.go
  sim/sim.go                   # годинник + режим, синхронізація з емулятором
  sim/sim_test.go
  emuclient/emuclient.go       # HTTP-клієнт емулятора
  poller/poller.go
  poller/poller_test.go
  auth/auth.go                 # cookie, middleware, login/logout
  auth/auth_test.go
  api/server.go                # збирання роутів Echo
  api/hub.go                   # SSE hub
  api/ingest.go
  api/operator.go
  api/senior.go
  api/track.go
  api/simapi.go
  api/api_test.go
web/
  package.json, vite.config.ts, tsconfig.json, index.html
  Caddyfile, Dockerfile
  src/main.tsx
  src/index.css                # Tailwind + бренд-токени
  src/lib/api.ts               # fetch + типи відповідей
  src/lib/useInvalidateStream.ts
  src/components/Header.tsx
  src/components/LevelBadge.tsx
  src/components/ParcelTable.tsx
  src/routes/__root.tsx, index.tsx, login.tsx, track.tsx, operator.tsx, senior.tsx, sim.tsx
```

---

### Task 1: Репозиторій і пакет `contract`

**Files:**
- Create: `go.mod`, `.gitignore`, `Makefile` (поки цілі `db`, `test`), `docker-compose.yml` (поки лише `postgres`), `contract/contract.go`, `contract/sign.go`, `contract/clock.go`
- Test: `contract/contract_test.go`

**Interfaces:**
- Produces:
  - `type Envelope[T any] struct { APIKey string \`json:"apiKey,omitempty"\`; ModelName string \`json:"modelName"\`; CalledMethod string \`json:"calledMethod"\`; MethodProperties T \`json:"methodProperties"\` }`
  - `type Response[T any] struct { Success bool; Data []T; Errors, Warnings, Info []string }` (JSON-теги `success`, `data`, `errors`, `warnings`, `info`; порожні масиви серіалізуються як `[]`, не `null`)
  - `type TrackingRequest struct { Documents []Document \`json:"Documents"\` }`, `type Document struct { DocumentNumber, Phone string }`
  - `type TrackingStatus struct { Number, StatusCode, Status, ScheduledDeliveryDate, WarehouseRecipient, CityRecipient string }`
  - `const NPDateLayout = "02-01-2006 15:04:05"`
  - `type Event struct { EventID string \`json:"EventId"\`; Kind string; TTN string; TripID string \`json:"TripId"\`; LocationID string \`json:"LocationId"\`; OccurredAt time.Time; Source string; Payload json.RawMessage }`
  - `type EventBatch struct { Events []Event }` — `methodProperties` конверта `ScanEvent.push`
  - `type RouteLeg struct { Seq int; FromID, ToID, TripID string; PlannedDeparture, PlannedArrival time.Time }`
  - `type CreatedPayload struct { SenderPhone, RecipientPhone, OriginID, DestinationID string; RouteLegs []RouteLeg }`
  - `type IncidentPayload struct { Cause string }`, `type RedirectPayload struct { NextLocationID string }`, `type ReturnPayload struct { Reason, Comment string }`
  - Константи `Kind*`: `KindCreated="created"`, `KindAccepted="accepted"`, `KindLoaded="loaded"`, `KindTripDeparted="trip_departed"`, `KindTripArrived="trip_arrived"`, `KindArrivedHub="arrived_hub"`, `KindSorted="sorted"`, `KindUnloaded="unloaded"`, `KindArrivedBranch="arrived_branch"`, `KindIssued="issued"`, `KindRedirected="redirected"`, `KindReturnInitiated="return_initiated"`, `KindDamaged="damaged"`, `KindIncident="incident"`, `KindOperatorScan="operator_scan"`, `KindHold="hold"`
  - Константи `Status*`: `StatusCreated="1"`, `StatusAccepted="4"`, `StatusInTransit="5"`, `StatusInRecipientCity="6"`, `StatusArrived="7"`, `StatusIssued="9"`, `StatusRedirected="104"`, `StatusReturn="106"`; `func StatusText(code string) string` (українські тексти, як у spec §4.1)
  - `type Location struct { ID, Kind, City, Name string }`; `var Locations []Location` (6 локацій з Global Constraints, `Kind` = `"branch"` або `"hub"`); `func LocationByID(id string) (Location, bool)`
  - `func Sign(body []byte, secret string) string` (hex HMAC-SHA256), `func Verify(body []byte, secret, sig string) bool` (`hmac.Equal`)
  - `type Clock struct { RealBase, VirtualBase time.Time; Speed float64 }`; `func (c Clock) Now(real time.Time) time.Time` = `VirtualBase + (real − RealBase) × Speed`; `func (c Clock) Rebase(real time.Time, speed float64) Clock` (фіксує поточний віртуальний час як нову базу, міняє швидкість)
  - `var ScenarioStart = time.Date(2026, 10, 17, 8, 0, 0, 0, time.UTC)` — початок віртуального часу всіх сценаріїв (і backend, і emulator беруть звідси)
  - `func TTN(n int) string`, `var TTNPattern = regexp.MustCompile(\`^59\d{12}$\`)`

- [ ] **Step 1: `git init`, `go mod init novapost` (go 1.27), `go get -tool github.com/air-verse/air@latest`, `.gitignore` (`node_modules/`, `web/dist/`, `tmp/`, `.env`)**

- [ ] **Step 2: Написати тести**

```go
func TestSignVerify(t *testing.T) {
	body := []byte(`{"a":1}`)
	sig := Sign(body, "s")
	if !Verify(body, "s", sig) { t.Fatal("valid sig rejected") }
	if Verify(body, "other", sig) || Verify([]byte(`{"a":2}`), "s", sig) { t.Fatal("bad sig accepted") }
}

func TestClockNowAndRebase(t *testing.T) {
	r0 := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	v0 := time.Date(2026, 10, 17, 8, 0, 0, 0, time.UTC)
	c := Clock{RealBase: r0, VirtualBase: v0, Speed: 60}
	if got := c.Now(r0.Add(time.Minute)); !got.Equal(v0.Add(time.Hour)) { t.Fatalf("got %v", got) }
	p := c.Rebase(r0.Add(time.Minute), 0) // пауза
	if got := p.Now(r0.Add(time.Hour)); !got.Equal(v0.Add(time.Hour)) { t.Fatalf("paused clock moved: %v", got) }
}

func TestResponseEmptyArrays(t *testing.T) {
	b, _ := json.Marshal(Response[TrackingStatus]{Success: true})
	if !strings.Contains(string(b), `"errors":[]`) || !strings.Contains(string(b), `"data":[]`) { t.Fatal(string(b)) }
}

func TestTTN(t *testing.T) {
	if TTN(12) != "59000000000012" || !TTNPattern.MatchString(TTN(12)) { t.Fatal(TTN(12)) }
}
```

- [ ] **Step 3: `go test ./contract/` → FAIL (не компілюється)**

- [ ] **Step 4: Реалізувати файли `contract/` за Interfaces.** Для `Response` написати `MarshalJSON`, який замінює nil-зрізи на порожні.

- [ ] **Step 5: `docker-compose.yml`: сервіс `postgres` (`postgres:18.6`, user/pass/db `novapost`, порт 5432, volume `pgdata`). `Makefile`: `db: docker compose up -d postgres`, `test: db` → `go test -p 1 ./...`**

- [ ] **Step 6: `go test ./contract/` → PASS**

- [ ] **Step 7: Commit** `feat: contract package (NP envelope, events, HMAC, clock)`

---

### Task 2: Світ емулятора

**Files:**
- Create: `emulator/world/world.go`
- Test: `emulator/world/world_test.go`

**Interfaces:**
- Consumes: `contract.Event`, `contract.CreatedPayload`, `contract.Status*`, `contract.TrackingStatus`, `contract.NPDateLayout`, `contract.LocationByID`
- Produces:
  - `type Fact struct { Event contract.Event; Hidden bool }`. `Hidden` = подія фізично відбулась, але не повідомлена (пропущений скан). Подія, якої фізично не було, просто відсутня у фактах.
  - `type World struct` (з `sync.Mutex`); `func New() *World`
  - `func (w *World) Load(facts []Fact)` — замінює світ
  - `func (w *World) Due(now time.Time) []contract.Event` — повертає ще не віддані не-`Hidden` події з `OccurredAt <= now`, позначає їх відданими, сортує за `OccurredAt`
  - `func (w *World) Track(ttn string, now time.Time) (contract.TrackingStatus, bool)` — `false`, якщо `created` ще не настав або ТТН невідома
  - `func (w *World) Physical(locationID string, now time.Time) []string` — ТТН, що фізично в локації
  - `func (w *World) Apply(e contract.Event)` — додає подію оператора як не-`Hidden` факт, одразу віддану (повторно не пушиться)

Правила `Track` (беремо лише факти з `OccurredAt <= now`; рейсові події завжди не-`Hidden`; посилкові враховуються, лише якщо не `Hidden`). Пріоритет зверху вниз:
- `issued` → 9; `return_initiated` → 106; `redirected` → 104;
- `unloaded`/`arrived_branch` у `DestinationID` → 7;
- `trip_arrived` для `TripID` останнього плеча → 6, `ScheduledDeliveryDate` = `PlannedArrival` останнього плеча у форматі `NPDateLayout` у `Europe/Kyiv`;
- `trip_departed` для рейсу будь-якого плеча → 5;
- `accepted` → 4; `created` → 1.

`WarehouseRecipient`/`CityRecipient` береться з `LocationByID(DestinationID)`.

Правило `Physical`: остання за `OccurredAt` посилкова подія з усіх фактів (разом з `Hidden`), що має локацію (`accepted`, `arrived_hub`, `sorted`, `unloaded`, `arrived_branch`, `operator_scan`). Якщо її `LocationID == locationID` і після неї немає `loaded`/`issued`/`redirected`/`return_initiated`, посилка тут.

- [ ] **Step 1: Написати тест кейсу з документа**

```go
func TestHiddenUnload_ClientSeesForecast_ParcelPhysicallyAtBranch(t *testing.T) {
	t0 := time.Date(2026, 10, 17, 8, 0, 0, 0, time.UTC)
	ttn := contract.TTN(1)
	legs := []contract.RouteLeg{{Seq: 1, FromID: "hub-kyiv", ToID: "branch-kyiv-12", TripID: "trip-1",
		PlannedDeparture: t0.Add(24 * time.Hour), PlannedArrival: t0.Add(28 * time.Hour)}} // план: завтра 15:00 за Києвом
	created, _ := json.Marshal(contract.CreatedPayload{OriginID: "hub-kyiv", DestinationID: "branch-kyiv-12", RouteLegs: legs})
	w := New()
	w.Load([]Fact{
		{Event: contract.Event{EventID: "e1", Kind: contract.KindCreated, TTN: ttn, OccurredAt: t0, Payload: created}},
		{Event: contract.Event{EventID: "e2", Kind: contract.KindTripDeparted, TripID: "trip-1", OccurredAt: t0.Add(1 * time.Hour)}},
		{Event: contract.Event{EventID: "e3", Kind: contract.KindTripArrived, TripID: "trip-1", LocationID: "branch-kyiv-12", OccurredAt: t0.Add(4 * time.Hour)}},
		{Event: contract.Event{EventID: "e4", Kind: contract.KindUnloaded, TTN: ttn, TripID: "trip-1", LocationID: "branch-kyiv-12", OccurredAt: t0.Add(4 * time.Hour)}, Hidden: true},
	})
	now := t0.Add(6 * time.Hour)
	st, ok := w.Track(ttn, now)
	if !ok || st.StatusCode != contract.StatusInRecipientCity || st.ScheduledDeliveryDate != "18-10-2026 15:00:00" { t.Fatalf("%+v", st) }
	if got := w.Physical("branch-kyiv-12", now); !slices.Contains(got, ttn) { t.Fatalf("physical: %v", got) }
	for _, e := range w.Due(now) { if e.EventID == "e4" { t.Fatal("hidden event pushed") } }

	w.Apply(contract.Event{EventID: "op1", Kind: contract.KindArrivedBranch, TTN: ttn, LocationID: "branch-kyiv-12", OccurredAt: now, Source: "operator"})
	if st, _ := w.Track(ttn, now); st.StatusCode != contract.StatusArrived { t.Fatalf("after accept: %+v", st) }
}

func TestDue_ReturnsEachEventOnce(t *testing.T) { /* Load 2 non-hidden facts; Due(now) → 2; Due(now) again → 0 */ }
func TestTrack_UnknownOrNotYetCreated(t *testing.T) { /* Track(unknown) → false; Track(ttn, before created) → false */ }
```

- [ ] **Step 2: `go test ./emulator/world/` → FAIL**
- [ ] **Step 3: Реалізувати `world.go` за правилами вище**
- [ ] **Step 4: `go test ./emulator/world/` → PASS**
- [ ] **Step 5: Commit** `feat(emulator): world model with hidden scans and client status`

---

### Task 3: Сценарії, пушер і HTTP емулятора

**Files:**
- Create: `emulator/scenarios/scenarios.go`, `emulator/push/push.go`, `emulator/api/api.go`, `emulator/cmd/main.go`, `emulator/.air.toml`
- Test: `emulator/scenarios/scenarios_test.go`, `emulator/api/api_test.go`
- Modify: `Makefile` (ціль `emulator: cd emulator && go tool air`)

**Interfaces:**
- Consumes: `world.*`, `contract.*`
- Produces:
  - `const FocusTTN = "59000000000001"` у `scenarios`; `var Start = contract.ScenarioStart`
  - `func Build(id int) ([]world.Fact, error)` — id 1..5, інакше помилка. Кожен сценарій = 25 фонових посилок (`TTN(100..124)`, маршрути між трьома містами через СЦ, усі події без збоїв, рейси групують по 5 посилок) + фокус-посилка зі збоєм за таблицею spec §12. `EventID` детерміновані (`fmt.Sprintf("s%d-e%d", id, i)`), щоб повторний запуск не створював нових ідентифікаторів.
  - `type Pusher struct { URL, Secret string; Client *http.Client }`; `func (p *Pusher) Send(ctx context.Context, events []contract.Event) error` — один POST `{URL}/api/ingest/events` з конвертом `ScanEvent.push` і `X-Signature`; не-2xx → помилка
  - Цикл у `main`: кожні 200 мс `due := world.Due(clock.Now(time.Now()))`, перемішати (`rand.Shuffle`), ~20% відкласти на наступний тік (події не по порядку), решту відправити. Помилка відправки → події повертаються в чергу.
  - `func NewServer(w *world.World, clk *ClockHolder) *echo.Echo` в `api`, де `type ClockHolder struct { mu; c contract.Clock }` з `Get() contract.Clock`, `Set(contract.Clock)`
  - Роути: `POST /v2.0/json/` (лише `TrackingDocument.getStatusDocuments`, інакше `success:false` з `errors:["Method not supported"]`; >100 документів → `success:false`; невідома ТТН → елемент з `StatusCode:"3"`, `Status:"Номер не знайдено"`), `POST /admin/clock {virtualNow, speed}`, `POST /admin/reset`, `POST /admin/scenarios/:id`, `GET /admin/physical?location=` → `{"ttns":[...]}`, `POST /admin/operator-event` (`contract.Event` → `world.Apply`)
  - Env: `BACKEND_URL` (типово `http://localhost:8080`), `INGEST_SECRET` (`dev-ingest-secret`), `PORT` (`8090`)

- [ ] **Step 1: Тести сценаріїв**

```go
func TestBuild_AllScenariosHaveFocusParcelAndBackground(t *testing.T) {
	for id := 1; id <= 5; id++ {
		facts, err := Build(id)
		if err != nil { t.Fatal(err) }
		created := 0
		for _, f := range facts { if f.Event.Kind == contract.KindCreated { created++ } }
		if created < 26 { t.Fatalf("scenario %d: %d parcels", id, created) }
	}
	if _, err := Build(6); err == nil { t.Fatal("expected error") }
}

func TestScenario2_FocusInRecipientCityButPhysicallyAtBranch(t *testing.T) {
	facts, _ := Build(2); w := world.New(); w.Load(facts)
	now := Start.Add(48 * time.Hour)
	st, _ := w.Track(FocusTTN, now)
	if st.StatusCode != contract.StatusInRecipientCity { t.Fatal(st) }
	if !slices.Contains(w.Physical("branch-kyiv-12", now), FocusTTN) { t.Fatal("not on shelf") }
}

func TestScenario1_FocusLeftAtHub(t *testing.T) {
	facts, _ := Build(1); w := world.New(); w.Load(facts)
	if !slices.Contains(w.Physical("hub-kyiv", Start.Add(48*time.Hour)), FocusTTN) { t.Fatal("expected at hub") }
}

func TestScenario3_FocusAtWrongBranch(t *testing.T) {
	facts, _ := Build(3); w := world.New(); w.Load(facts)
	if !slices.Contains(w.Physical("branch-lviv-5", Start.Add(48*time.Hour)), FocusTTN) { t.Fatal("expected in Lviv") }
}
```

Сценарій 4: фокус-посилка має `created` з `Hidden: true` (backend про неї не знає), фізично лежить у `branch-kharkiv-3`. Сценарій 5: подія `incident` на рейсі `hub-kyiv → branch-kyiv-12`, усі посилки цього рейсу без `unloaded`.

- [ ] **Step 2: Тест HTTP**

```go
func TestGetStatusDocuments(t *testing.T) {
	w := world.New(); facts, _ := scenarios.Build(2); w.Load(facts)
	clk := &ClockHolder{}; clk.Set(contract.Clock{VirtualBase: scenarios.Start.Add(48 * time.Hour)})
	srv := httptest.NewServer(NewServer(w, clk)); defer srv.Close()
	body := `{"apiKey":"demo","modelName":"TrackingDocument","calledMethod":"getStatusDocuments","methodProperties":{"Documents":[{"DocumentNumber":"59000000000001","Phone":""},{"DocumentNumber":"59999999999999","Phone":""}]}}`
	resp, _ := http.Post(srv.URL+"/v2.0/json/", "application/json", strings.NewReader(body))
	var r contract.Response[contract.TrackingStatus]; json.NewDecoder(resp.Body).Decode(&r)
	if !r.Success || r.Data[0].StatusCode != "6" || r.Data[1].StatusCode != "3" { t.Fatalf("%+v", r) }
}

func TestPusherSignsBody(t *testing.T) { /* httptest server: contract.Verify(body, "s", r.Header.Get("X-Signature")) == true і modelName == "ScanEvent" */ }
```

- [ ] **Step 3: `go test ./emulator/...` → FAIL**
- [ ] **Step 4: Реалізувати `scenarios`, `push`, `api`, `cmd/main.go`, `.air.toml` (`cmd = "go build -o ./tmp/emulator ./cmd"`, `bin = "./tmp/emulator"`)**
- [ ] **Step 5: `go test ./emulator/...` → PASS; `make emulator`, `curl -s localhost:8090/v2.0/json/ -d '<body з тесту>'` → JSON зі `"success":true`**
- [ ] **Step 6: Commit** `feat(emulator): scenarios 1-5, NP v2.0 tracking endpoint, signed event push`

---

### Task 4: Сховище і прийом подій

**Files:**
- Create: `backend/config/config.go`, `backend/store/store.go`, `backend/store/migrations/001_init.sql`, `backend/store/events.go`, `backend/store/testdb_test.go`, `backend/api/server.go`, `backend/api/ingest.go`, `backend/cmd/main.go`, `backend/.air.toml`
- Test: `backend/store/events_test.go`, `backend/api/api_test.go`
- Modify: `Makefile` (`backend: cd backend && go tool air`)

**Interfaces:**
- Consumes: `contract.*`
- Produces:
  - `type Config struct { DatabaseURL, EmulatorURL, IngestSecret, AppSecret, AdminPassword, SeniorPassword, OperatorPassword string; NPPollInterval time.Duration; LookupLimitPerHour int; Port string }`; `func Load() Config` (типові значення з spec §13; якщо паролі типові, `log.Println` з попередженням)
  - `type Store struct { Pool *pgxpool.Pool }`; `func Open(ctx context.Context, url string) (*Store, error)` — підключення, міграції з `embed.FS` (таблиця `schema_migrations(name text pk)`), upsert `contract.Locations` у `locations`
  - `func (s *Store) Reset(ctx context.Context) error` — `TRUNCATE events, parcels, route_legs, np_status, audit_log`
  - `func (s *Store) InsertEvents(ctx context.Context, evs []contract.Event, receivedAt time.Time) (inserted int, err error)` — в одній транзакції: `INSERT … ON CONFLICT (event_id) DO NOTHING`; для нових `created` також upsert `parcels` і `route_legs` з payload
  - `func (s *Store) ParcelEvents(ctx context.Context, ttn string) ([]contract.Event, error)` — події посилки + події її рейсів (`trip_id` з `route_legs`) + `incident` на цих рейсах чи локаціях, відсортовані за `occurred_at`
  - `func (s *Store) Parcel(ctx context.Context, ttn string) (*contract.CreatedPayload, []contract.RouteLeg, error)` — `nil`, якщо немає
  - Схема — spec §11 дослівно, плюс `schema_migrations`
  - `func NewServer(d Deps) *echo.Echo` в `api`, де `type Deps struct { Cfg config.Config; Store *store.Store; Sim *sim.Sim; Emu *emuclient.Client; Hub *Hub }` (поля, яких ще немає, додаються в Tasks 5–10)
  - `POST /api/ingest/events`: читає сире тіло, `contract.Verify` (невірно → 401), декодує `Envelope[EventBatch]`, `InsertEvents`, `Hub.Publish("problems","incoming","track")`, відповідає `{"success":true,"inserted":N}`
  - `GET /api/locations` → `contract.Locations`
  - Helper тестів: `func testStore(t *testing.T) *Store` (Open з `TEST_DATABASE_URL` + Reset); та сама функція в `api` (`api_test.go`)

- [ ] **Step 1: Тести прийому**

```go
func TestInsertEvents_IdempotentAndOrderIndependent(t *testing.T) {
	s := testStore(t); ctx := context.Background()
	evs := scenario2Events() // helper: created + trip_departed + trip_arrived для TTN(1), як у Task 2
	n, _ := s.InsertEvents(ctx, evs, time.Now())
	again, _ := s.InsertEvents(ctx, evs, time.Now())
	if n != len(evs) || again != 0 { t.Fatalf("n=%d again=%d", n, again) }
	got1, _ := s.ParcelEvents(ctx, contract.TTN(1))

	s.Reset(ctx)
	rev := slices.Clone(evs); slices.Reverse(rev)
	s.InsertEvents(ctx, rev, time.Now())
	got2, _ := s.ParcelEvents(ctx, contract.TTN(1))
	if !reflect.DeepEqual(ids(got1), ids(got2)) { t.Fatal("order-dependent state") }
	if p, legs, _ := s.Parcel(ctx, contract.TTN(1)); p == nil || len(legs) != 1 { t.Fatal("parcel not materialized") }
}
```

```go
// backend/api/api_test.go
func TestIngest_RejectsBadSignature(t *testing.T) {
	e := NewServer(testDeps(t))
	req := httptest.NewRequest("POST", "/api/ingest/events", strings.NewReader(`{}`))
	req.Header.Set("X-Signature", "deadbeef"); rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != 401 { t.Fatal(rec.Code) }
}
```

Примітка: якщо `created` приходить пізніше за інші події тієї ж посилки (не по порядку), `ParcelEvents` все одно знаходить їх за `ttn`, а рейсові — за `route_legs`, щойно ті з'являться. Тест вище це покриває.

- [ ] **Step 2: `make db && go test -p 1 ./backend/...` → FAIL**
- [ ] **Step 3: Реалізувати**
- [ ] **Step 4: `go test -p 1 ./backend/...` → PASS**
- [ ] **Step 5: Commit** `feat(backend): postgres store, migrations, signed idempotent event ingest`

---

### Task 5: Віртуальний годинник і режим

**Files:**
- Create: `backend/sim/sim.go`, `backend/emuclient/emuclient.go`, `backend/api/simapi.go`
- Test: `backend/sim/sim_test.go`

**Interfaces:**
- Consumes: `contract.Clock`, `store.Store`
- Produces:
  - `type Mode string`; `const ModeAsIs Mode = "as_is"`, `ModeWithSolution Mode = "with_solution"`
  - `type Sim struct` (mutex; `clock contract.Clock`; `mode Mode`; `store *store.Store`; `emu *emuclient.Client`)
  - `func New(ctx context.Context, st *store.Store, emu *emuclient.Client) (*Sim, error)` — читає `app_state` (`clock`, `mode`). Типово: `VirtualBase = contract.ScenarioStart`, `Speed = 0`, `ModeWithSolution`.
  - `func (s *Sim) Now() time.Time`, `func (s *Sim) Clock() contract.Clock`, `func (s *Sim) Mode() Mode`
  - `func (s *Sim) SetSpeed(ctx context.Context, speed float64) error`, `func (s *Sim) Jump(ctx context.Context, virtual time.Time) error`, `func (s *Sim) SetMode(ctx context.Context, m Mode) error` — зберігають у `app_state`; зміни годинника дублюються в `emu.SetClock(virtualNow, speed)`
  - `type Client struct { URL string; HTTP *http.Client }` в `emuclient`: `Track(ctx, ttns []string) ([]contract.TrackingStatus, error)` (пачками по 100), `SetClock(ctx, virtualNow time.Time, speed float64) error`, `Reset(ctx) error`, `LoadScenario(ctx, id int) error`, `Physical(ctx, location string) ([]string, error)`, `OperatorEvent(ctx, e contract.Event) error`
  - Роути (поки без авторизації, роль `admin` додає Task 8): `GET /api/sim/clock` → `{"virtualNow": RFC3339, "speed": n, "mode": "..."}`, `POST /api/sim/clock {speed?, jumpTo?}`, `POST /api/sim/mode {mode}`, `POST /api/sim/reset`, `POST /api/sim/scenarios/:id` (store.Reset → emu.Reset → emu.LoadScenario → Jump(`contract.ScenarioStart`) → SetSpeed(60)). Помилка емулятора → 502 `{"error":"emulator unavailable"}`.

- [ ] **Step 1: Тест**

```go
func TestSim_PersistsClockAndMode(t *testing.T) {
	st := testStore(t); emu := &emuclient.Client{URL: fakeEmulator(t).URL} // httptest, що приймає /admin/clock
	s, _ := New(ctx, st, emu)
	s.Jump(ctx, contract.ScenarioStart.Add(5*time.Hour)); s.SetMode(ctx, ModeAsIs)
	s2, _ := New(ctx, st, emu)
	if !s2.Now().Equal(contract.ScenarioStart.Add(5*time.Hour)) || s2.Mode() != ModeAsIs { t.Fatal("not persisted") }
}
```

- [ ] **Step 2: FAIL → реалізувати → PASS (`go test -p 1 ./backend/sim/`)**
- [ ] **Step 3: Commit** `feat(backend): virtual clock and mode synced to emulator`

---

### Task 6: Класифікація проблемних посилок

**Files:**
- Create: `backend/problems/rules.go`, `backend/problems/classify.go`
- Test: `backend/problems/classify_test.go`

**Interfaces:**
- Consumes: `contract.Event`, `contract.CreatedPayload`, `contract.RouteLeg`
- Produces:
  - `type Severity int`; `const ( Green Severity = iota; Grey; Yellow; Orange; Red )`; `func (s Severity) String() string` → `"green"|"grey"|"yellow"|"orange"|"red"`; `MarshalJSON` через `String`
  - `type Kind string`; константи `Unknown="unknown"`, `Misrouted="misrouted"`, `RiskyReturn="risky_return"`, `BrokenChain="broken_chain"`, `Overdue="overdue"`, `Stuck="stuck"`, `Incident="incident"`
  - `type Problem struct { Kind Kind \`json:"kind"\`; Severity Severity \`json:"severity"\`; Since time.Time \`json:"since"\` }`
  - `type Input struct { TTN string; Parcel *contract.CreatedPayload; Legs []contract.RouteLeg; Events []contract.Event; NPScheduled *time.Time; Now time.Time }`. `Parcel == nil` означає, що ТТН невідома системі. `Events` відсортовані за `OccurredAt`.
  - `func Classify(in Input) []Problem`
  - `func Level(ps []Problem) Severity` — максимум або `Green`

Правила — spec §6 дослівно. Уточнення:
- ETA = `*NPScheduled`, інакше `PlannedArrival` останнього плеча.
- «Прибула» = `unloaded` або `arrived_branch` у `DestinationID`. Посилка з `issued` → жодних проблем. Після «прибула» `broken_chain`, `overdue` і `stuck` не застосовуються.
- `unknown`: є `operator_scan`, і `Parcel == nil`, і немає пізнішої `hold`. `hold` без `Parcel` теж лишається `unknown` (🔴): посилка «на ідентифікації» досі ризикова.
- `misrouted`: остання подія з локацією (`unloaded`, `arrived_branch`, `operator_scan`) має `LocationID`, який є відділенням (`Kind=="branch"`) і не входить у `ToID`/`FromID` жодного плеча, і немає пізнішої `redirected`.
- `broken_chain`: для плеча з `TripID` є `trip_arrived` цього рейсу, а `unloaded`/`arrived_branch` у `ToID` плеча немає.
- `risky_return`: `return_initiated`, коли на момент події посилка мала проблему рівня ≥ Orange або була `hold`. Рахувати так: `Classify` з `Now = OccurredAt` події повернення по подіях до неї. Рекурсія глибиною 1, тож ок.
- Ескалація: `Since` лишається початком умови, а `Severity` підвищується, коли `Now − Since > EscalateOrange` (🟠→🔴) або `> EscalateYellow` (🟡→🟠).
- `incident` пригнічує `stuck` і `overdue`.

- [ ] **Step 1: Табличний тест**

```go
func TestClassify(t *testing.T) {
	t0 := time.Date(2026, 10, 17, 8, 0, 0, 0, time.UTC)
	leg := contract.RouteLeg{Seq: 1, FromID: "hub-kyiv", ToID: "branch-kyiv-12", TripID: "trip-1", PlannedArrival: t0.Add(31 * time.Hour)}
	p := &contract.CreatedPayload{OriginID: "hub-kyiv", DestinationID: "branch-kyiv-12", RouteLegs: []contract.RouteLeg{leg}}
	ev := func(kind string, at time.Duration, mod ...func(*contract.Event)) contract.Event { /* TTN(1), OccurredAt t0+at; mod змінює LocationID/TripID */ }
	tripArrived := ev(contract.KindTripArrived, 4*time.Hour, trip("trip-1"), loc("branch-kyiv-12"))

	cases := []struct {
		name   string
		in     Input
		want   Severity
		kinds  []Kind
	}{
		{"normal in transit", Input{Parcel: p, Legs: p.RouteLegs, Events: []contract.Event{ev(contract.KindCreated, 0)}, Now: t0.Add(2 * time.Hour)}, Green, nil},
		{"broken chain fresh", Input{Parcel: p, Legs: p.RouteLegs, Events: []contract.Event{ev(contract.KindCreated, 0), tripArrived}, Now: t0.Add(5 * time.Hour)}, Orange, []Kind{BrokenChain}},
		{"broken chain escalated after 6h", Input{Parcel: p, Legs: p.RouteLegs, Events: []contract.Event{ev(contract.KindCreated, 0), tripArrived}, Now: t0.Add(10*time.Hour + time.Minute)}, Red, []Kind{BrokenChain}},
		{"overdue", Input{Parcel: p, Legs: p.RouteLegs, Events: []contract.Event{ev(contract.KindCreated, 0)}, Now: t0.Add(33*time.Hour + time.Minute)}, Orange, []Kind{Overdue, Stuck}},
		{"stuck", Input{Parcel: p, Legs: p.RouteLegs, Events: []contract.Event{ev(contract.KindCreated, 0)}, Now: t0.Add(12*time.Hour + time.Minute)}, Yellow, []Kind{Stuck}},
		{"stuck escalated to orange after 24h", /* Now = t0+36h+1m, NPScheduled далеко в майбутньому, щоб не було overdue */ Input{}, Orange, []Kind{Stuck}},
		{"incident suppresses stuck", Input{Parcel: p, Legs: p.RouteLegs, Events: []contract.Event{ev(contract.KindCreated, 0), ev(contract.KindIncident, time.Hour, trip("trip-1"))}, Now: t0.Add(20 * time.Hour)}, Grey, []Kind{Incident}},
		{"unknown scan", Input{TTN: contract.TTN(9), Events: []contract.Event{ev(contract.KindOperatorScan, time.Hour, loc("branch-kharkiv-3"))}, Now: t0.Add(2 * time.Hour)}, Red, []Kind{Unknown}},
		{"misrouted", Input{Parcel: p, Legs: p.RouteLegs, Events: []contract.Event{ev(contract.KindCreated, 0), ev(contract.KindUnloaded, 4*time.Hour, loc("branch-lviv-5"))}, Now: t0.Add(5 * time.Hour)}, Red, []Kind{Misrouted}},
		{"risky return after broken chain", Input{Parcel: p, Legs: p.RouteLegs, Events: []contract.Event{ev(contract.KindCreated, 0), tripArrived, ev(contract.KindReturnInitiated, 5*time.Hour)}, Now: t0.Add(6 * time.Hour)}, Red, []Kind{RiskyReturn, BrokenChain}},
		{"arrived clears everything", Input{Parcel: p, Legs: p.RouteLegs, Events: []contract.Event{ev(contract.KindCreated, 0), tripArrived, ev(contract.KindUnloaded, 4*time.Hour, loc("branch-kyiv-12"), trip("trip-1"))}, Now: t0.Add(40 * time.Hour)}, Green, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Classify(c.in)
			if Level(got) != c.want { t.Fatalf("level %v, problems %+v", Level(got), got) }
			// kinds: множинна рівність (порядок не важливий)
		})
	}
}
```

У рядку «stuck escalated» виконавець заповнює `Input` за коментарем (той самий `p`, `NPScheduled = t0+100h`).

- [ ] **Step 2: `go test ./backend/problems/` → FAIL**
- [ ] **Step 3: Реалізувати**
- [ ] **Step 4: PASS**
- [ ] **Step 5: Commit** `feat(backend): pure problem classification with escalation`

---

### Task 7: Poller клієнтських статусів

**Files:**
- Create: `backend/poller/poller.go`
- Modify: `backend/store/queries.go` (створити), `backend/cmd/main.go`
- Test: `backend/poller/poller_test.go`

**Interfaces:**
- Consumes: `emuclient.Client.Track`, `store.Store`, `api.Hub.Publish`
- Produces:
  - `func (s *Store) UnissuedTTNs(ctx) ([]string, error)` — ТТН з `parcels`, для яких немає події `issued`
  - `func (s *Store) SaveNPStatus(ctx, sts []contract.TrackingStatus, fetchedAt time.Time) error` — upsert у `np_status`; `ScheduledDeliveryDate` парситься за `contract.NPDateLayout` у `Europe/Kyiv`, порожнє значення → NULL
  - `func (s *Store) NPStatus(ctx, ttn string) (code, text string, scheduled *time.Time, ok bool, err error)`
  - `func Run(ctx context.Context, st *store.Store, emu *emuclient.Client, every time.Duration, onChange func())` — тікер; помилка емулятора → `log.Printf` і наступний тік, знімок не чіпається; після успішного збереження викликає `onChange`

- [ ] **Step 1: Тест (Review Focus #1)**

```go
func TestPoller_KeepsSnapshotWhenEmulatorDown(t *testing.T) {
	st := testStore(t); insertParcel(t, st, contract.TTN(1))
	up := true
	emu := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !up { w.WriteHeader(500); return }
		json.NewEncoder(w).Encode(contract.Response[contract.TrackingStatus]{Success: true, Data: []contract.TrackingStatus{{Number: contract.TTN(1), StatusCode: "6", ScheduledDeliveryDate: "18-10-2026 15:00:00"}}})
	}))
	ctx, cancel := context.WithCancel(context.Background())
	go Run(ctx, st, &emuclient.Client{URL: emu.URL}, 10*time.Millisecond, func() {})
	waitFor(t, func() bool { c, _, _, ok, _ := st.NPStatus(ctx, contract.TTN(1)); return ok && c == "6" })
	up = false; time.Sleep(50 * time.Millisecond); cancel()
	if c, _, _, ok, _ := st.NPStatus(context.Background(), contract.TTN(1)); !ok || c != "6" { t.Fatal("snapshot lost") }
}
```

- [ ] **Step 2: FAIL → реалізувати → PASS. У `main` запустити `go poller.Run(ctx, …, cfg.NPPollInterval, func(){ hub.Publish("track","problems") })`**
- [ ] **Step 3: Commit** `feat(backend): NP tracking poller resilient to emulator outages`

---

### Task 8: Авторизація

**Files:**
- Create: `backend/auth/auth.go`
- Modify: `backend/api/server.go` (групи роутів з middleware)
- Test: `backend/auth/auth_test.go`

**Interfaces:**
- Consumes: `config.Config`
- Produces:
  - `type Session struct { Role string \`json:"role"\`; BranchID string \`json:"branchId,omitempty"\`; ExpiresAt time.Time \`json:"-"\` }`; ролі `"admin"`, `"senior"`, `"operator"`
  - `func Encode(s Session, secret string) string` — `base64url(role|branch|unixExpires) + "." + hex(HMAC)`
  - `func Decode(v, secret string, now time.Time) (Session, error)` — помилка при невірному підписі, форматі чи простроченні
  - `func Middleware(secret string, roles ...string) echo.MiddlewareFunc` — немає або невалідна cookie `session` → 401 `{"error":"unauthorized"}`; роль не в списку → 403; кладе `Session` у `c.Set("session", s)`
  - `func FromContext(c *echo.Context) Session`
  - `func Handlers(cfg config.Config) (login, logout, me echo.HandlerFunc)`: `POST /api/login {role, branchId?, password}` — для `operator` `branchId` обов'язковий і має бути відділенням з `contract.Locations`; невірний пароль → 401; cookie `HttpOnly`, `SameSite=Lax`, `Secure`, якщо `X-Forwarded-Proto == "https"`, термін 12 год (реального часу). `POST /api/logout` стирає cookie. `GET /api/me` → `Session` або 401.
  - У `server.go`: `/api/operator/*` → `Middleware(secret,"operator")`, `/api/senior/*` → `"senior"`, `/api/sim/*` → `"admin"`, `/api/stream` → усі три ролі. `/api/track/*`, `/api/locations`, `/api/login` — публічні. `/api/ingest/events` — лише HMAC.

- [ ] **Step 1: Тести (Review Focus #2)**

```go
func TestDecode_RejectsTamperedAndExpired(t *testing.T) {
	now := time.Now()
	v := Encode(Session{Role: "operator", BranchID: "branch-kyiv-12", ExpiresAt: now.Add(time.Hour)}, "k")
	if s, err := Decode(v, "k", now); err != nil || s.BranchID != "branch-kyiv-12" { t.Fatal(err) }
	if _, err := Decode(v, "other", now); err == nil { t.Fatal("wrong secret accepted") }
	forged := Encode(Session{Role: "admin", ExpiresAt: now.Add(time.Hour)}, "x")
	if _, err := Decode(strings.Split(forged, ".")[0]+"."+strings.Split(v, ".")[1], "k", now); err == nil { t.Fatal("forged payload accepted") }
	if _, err := Decode(v, "k", now.Add(2*time.Hour)); err == nil { t.Fatal("expired accepted") }
}

func TestMiddleware_RoleChecks(t *testing.T) {
	// echo з роутом GET /x під Middleware("k","senior"):
	// без cookie → 401; cookie operator → 403; cookie senior → 200
}

func TestLogin(t *testing.T) {
	// невірний пароль → 401; operator без branchId → 400; operator + branch-kyiv-12 + "operator" → 200 і Set-Cookie session=…
}
```

- [ ] **Step 2: FAIL → реалізувати → PASS (`go test ./backend/auth/`)**
- [ ] **Step 3: Commit** `feat(backend): signed-cookie auth with hardcoded role passwords`

---

### Task 9: API оператора, старшої зміни, клієнта і симулятора

**Files:**
- Create: `backend/api/operator.go`, `backend/api/senior.go`, `backend/api/track.go`
- Modify: `backend/store/queries.go`, `backend/api/simapi.go`, `backend/api/server.go`
- Test: `backend/api/api_test.go`

**Interfaces:**
- Consumes: `store.*`, `problems.Classify/Level`, `sim.Sim`, `emuclient.Client`, `auth.FromContext`, `Hub.Publish`
- Produces (store):
  - `func (s *Store) Inputs(ctx, ttns []string, now time.Time) ([]problems.Input, error)` — збирає `Input` для кожної ТТН (Parcel, Legs, Events, NPScheduled). Для ТТН без `parcels` — лише події з цим `ttn`.
  - `func (s *Store) BranchTTNs(ctx, branchID string) ([]string, error)` — невидані посилки, у яких `destination_id = branch`, або плече з `to_id = branch`, або остання подія з локацією в цьому відділенні, плюс ТТН з `operator_scan` у цьому відділенні без рядка в `parcels`
  - `func (s *Store) AllActiveTTNs(ctx) ([]string, error)` — усе невидане + невідомі скани
  - `func (s *Store) Incoming(ctx, branchID string, mode sim.Mode) ([]IncomingRow, error)` — `with_solution`: плечі з `to_id = branch` без `unloaded`/`arrived_branch` у branch; `as_is`: лише ті з них, де є `loaded` на `trip_id` цього плеча
  - `type IncomingRow struct { TTN string \`json:"ttn"\`; FromID string \`json:"from"\`; TripID string \`json:"tripId"\`; ETA time.Time \`json:"eta"\` }`
  - `func (s *Store) Audit(ctx, role, branchID, action, ttn, result string) error`
- Produces (HTTP; рядок проблеми `ProblemRow{ttn, level, problems[], destination, location}`):
  - `GET /api/operator/problems` → `[]ProblemRow` для відділення з сесії, лише рівень ≠ green, сортування: red > orange > yellow > grey, потім `since`. У режимі `as_is` → `[]`.
  - `GET /api/operator/incoming` → `[]IncomingRow`
  - `GET /api/operator/physical` → `emu.Physical(branch)`, 502 при помилці
  - `POST /api/operator/scan {ttn}`:
    - невалідна ТТН → 400 без витрати ліміту;
    - ліміт `LookupLimitPerHour` на відділення (ковзне вікно в пам'яті, `map[branch][]time.Time` під mutex, час — реальний), перевищення → 429 + `Audit(…,"scan","rate_limited")`;
    - записати подію `operator_scan` (`source:"operator"`, `EventID` = random hex) через `InsertEvents` і `emu.OperatorEvent`;
    - `with_solution`: `Parcel(ttn)` → `nil` → `{"outcome":"hold"}`; `DestinationID == branch` → `{"outcome":"accept"}`; інакше `{"outcome":"redirect","nextLocation": <ToID плеча, наступного після поточної локації, або hub міста відділення, якщо поточна локація не в маршруті>}`. Ні телефонів, ні кінцевої адреси у відповіді немає;
    - `as_is`: відомо і для цього відділення → `accept`, інакше `{"outcome":"not_found"}`;
    - `Audit(…,"scan",outcome)`.
  - `POST /api/operator/parcels/:ttn/:action`, `action ∈ accept|redirect|hold|return`: пише подію (`arrived_branch` / `redirected` з `RedirectPayload` / `hold` / `return_initiated` з `ReturnPayload`) у store і в емулятор. Для `return` у `with_solution` `reason ∈ damaged|refused|other` обов'язковий, а для `other` — ще й `comment` (інакше 400); в `as_is` причина не потрібна. `Audit`, `Hub.Publish("problems","incoming","track")`.
  - `GET /api/senior/problems` → `[]ProblemRow` з `AllActiveTTNs`, рівень ≠ green
  - `GET /api/track/:ttn?phone=` (публічний): невалідна ТТН → 400; немає в `np_status` → 404; інакше `{"ttn","statusCode","status","eta","isForecast": statusCode=="6" || statusCode=="5"}`, а `details: {"city","warehouse"}` додається, лише коли `phone` збігається з `recipient_phone` чи `sender_phone`
  - `GET /api/sim/split/:ttn` → `{"client": <як track з details>, "operator": {"visibleInIncoming": bool, "level": "...", "problems": [...]}}` для відділення призначення в поточному режимі
  - `GET /api/sim/physical?location=` → проксі `emu.Physical`

- [ ] **Step 1: Інтеграційний тест кейсу з документа (spec §16.2)**

```go
func TestCase_HiddenUnload_BecomesOrangeThenRed(t *testing.T) {
	d := testDeps(t) // store + sim (з fake емулятором) + hub
	e := NewServer(d)
	ingest(t, e, d.Cfg.IngestSecret, scenario2Events()) // created + trip_departed + trip_arrived(t0+4h), без unloaded
	d.Sim.Jump(ctx, t0.Add(5*time.Hour))
	rows := getProblems(t, e, operatorCookie(d, "branch-kyiv-12"))
	if len(rows) != 1 || rows[0].Level != "orange" || rows[0].Problems[0].Kind != "broken_chain" { t.Fatalf("%+v", rows) }
	d.Sim.Jump(ctx, t0.Add(10*time.Hour+time.Minute))
	if rows := getProblems(t, e, operatorCookie(d, "branch-kyiv-12")); rows[0].Level != "red" { t.Fatalf("%+v", rows) }

	if out := scan(t, e, operatorCookie(d, "branch-kyiv-12"), contract.TTN(1)); out.Outcome != "accept" { t.Fatal(out) }
	act(t, e, operatorCookie(d, "branch-kyiv-12"), contract.TTN(1), "accept", nil)
	if rows := getProblems(t, e, operatorCookie(d, "branch-kyiv-12")); len(rows) != 0 { t.Fatalf("not cleared: %+v", rows) }
}
```

- [ ] **Step 2: Тести Review Focus #3, #4, #5**

```go
func TestScan_RateLimited(t *testing.T) {
	// LookupLimitPerHour = 30; 30 сканів валідних ТТН → 200; 31-й → 429; у audit_log є рядок result="rate_limited"
}
func TestScan_InvalidTTN(t *testing.T) {
	// "123" → 400; потім 30 валідних сканів усі 200 (ліміт не витрачено)
}
func TestTrack_PhoneGatesDetails(t *testing.T) {
	// SaveNPStatus для TTN(1); phone неправильний → 200 без "details"; правильний recipient_phone → є "details"; TTN(9) → 404; "abc" → 400
}
func TestReturn_RequiresReasonWithSolution(t *testing.T) {
	// with_solution: return без reason → 400; reason=other без comment → 400; reason=damaged → 200
	// після return_initiated для посилки з broken_chain → senior/problems містить risky_return
}
```

- [ ] **Step 3: FAIL → реалізувати → PASS (`go test -p 1 ./backend/...`)**
- [ ] **Step 4: Commit** `feat(backend): operator, senior, tracking and simulator APIs`

---

### Task 10: SSE

**Files:**
- Create: `backend/api/hub.go`
- Modify: `backend/api/server.go`, `backend/cmd/main.go`
- Test: `backend/api/api_test.go`

**Interfaces:**
- Produces:
  - `type Hub struct`; `func NewHub() *Hub`; `func (h *Hub) Publish(keys ...string)` — неблокуюче розсилання всім підписникам (буфер каналу 8, повний канал → подію пропускаємо); `func (h *Hub) Subscribe() (<-chan []string, func())`
  - `GET /api/stream`: заголовки `Content-Type: text/event-stream`, `Cache-Control: no-cache`, `X-Accel-Buffering: no`; кожне повідомлення: `data: {"type":"invalidate","keys":[...]}\n\n` + `Flush()`; коментар `: ping` кожні 15 с; завершення за `c.Request().Context().Done()`
  - У `main`: поки `sim.Clock().Speed > 0`, кожні 5 с реального часу `hub.Publish("problems","incoming","track","clock")`. Зміни годинника й режиму в `simapi` → `Publish("clock","problems","incoming")`.

- [ ] **Step 1: Тест**

```go
func TestStream_DeliversInvalidate(t *testing.T) {
	d := testDeps(t); srv := httptest.NewServer(NewServer(d)); defer srv.Close()
	req, _ := http.NewRequest("GET", srv.URL+"/api/stream", nil); req.AddCookie(adminCookie(d))
	resp, _ := http.DefaultClient.Do(req)
	go func() { time.Sleep(50 * time.Millisecond); d.Hub.Publish("problems") }()
	line := readDataLine(t, resp.Body) // перший рядок, що починається з "data: "
	if !strings.Contains(line, `"keys":["problems"]`) { t.Fatal(line) }
}
```

- [ ] **Step 2: FAIL → реалізувати → PASS**
- [ ] **Step 3: Ручна перевірка: `make db`, `make emulator`, `make backend`; залогінитись як admin через curl з cookie-jar; `POST /api/sim/scenarios/2`; `curl -N -b jar localhost:8080/api/stream` → кожні ~5 с з'являються `data: {"type":"invalidate",…}`**
- [ ] **Step 4: Commit** `feat(backend): SSE invalidation stream`

---

### Task 11: Каркас фронтенду, вхід, шапка

**Files:**
- Create: `web/package.json`, `web/vite.config.ts`, `web/tsconfig.json`, `web/index.html`, `web/src/main.tsx`, `web/src/index.css`, `web/src/lib/api.ts`, `web/src/lib/useInvalidateStream.ts`, `web/src/components/Header.tsx`, `web/src/components/LevelBadge.tsx`, `web/src/routes/__root.tsx`, `web/src/routes/index.tsx`, `web/src/routes/login.tsx`
- Modify: `Makefile` (`web: cd web && npm run dev`)

**Interfaces:**
- Consumes: HTTP API з Tasks 4–10
- Produces:
  - `api.ts`: `async function api<T>(path: string, init?: RequestInit): Promise<T>` (`credentials: "same-origin"`, JSON; 401 → `throw new UnauthorizedError()`); типи `Level = "red"|"orange"|"yellow"|"grey"|"green"`, `Me`, `ProblemRow`, `IncomingRow`, `ScanResult`, `Track`, `Split`, `ClockState`, що дзеркалять JSON з Tasks 5 і 9
  - `useInvalidateStream(enabled: boolean): void` — `new EventSource("/api/stream")`, на кожне повідомлення `queryClient.invalidateQueries({ queryKey: [key] })` для кожного ключа
  - `<LevelBadge level={Level} label?: string />` — колір + іконка + текст: red `⛔ Діяти зараз`, orange `⚠️ Перевірити`, yellow `👁 Спостерігати`, grey `⚡ Інцидент`, green `✓ За планом`. Мітки проблем: `unknown` «Невідома», `misrouted` «Не за адресою», `risky_return` «Ризикове повернення», `broken_chain` «Розірваний ланцюг», `overdue` «Прострочена», `stuck` «Застигла», `incident` «Інцидент».
  - `<Header />` — смуга `#DA291C`, текст «Концепт для Нової пошти», роль і відділення з `/api/me`, кнопка «Вийти»
  - `index.css`: `@import "tailwindcss";` і `@theme { --color-brand: #DA291C; --color-ink: #202124; --color-ink-2: #5F6368; --color-bg: #F8F9FA; --color-lvl-red: …; --color-lvl-orange: …; --color-lvl-yellow: …; --color-lvl-grey: …; --color-lvl-green: … }`. Червоний рівня має відрізнятися від бренду (наприклад, `#B3261E` на світлому фоні `#FDECEA`).
  - Маршрут `/` перенаправляє за роллю: operator → `/operator`, senior → `/senior`, admin → `/sim`, без сесії → `/login`
  - `/login`: вибір ролі (Оператор / Старша зміна / Адмін); для оператора — select відділень з `/api/locations` (`kind === "branch"`); пароль; помилка «Невірний пароль». Під формою посилання «Відстежити посилку» → `/track`.
  - `vite.config.ts`: плагіни `tanstackRouter({ target: "react", autoCodeSplitting: true })`, `react()`, `tailwindcss()`; `server.proxy["/api"] = { target: "http://localhost:8080", changeOrigin: true }`

- [ ] **Step 1: `npm create vite@latest web -- --template react-ts`, далі встановити залежності з Global Constraints (останні версії)**
- [ ] **Step 2: Реалізувати файли за Interfaces**
- [ ] **Step 3: `npm run build` без помилок TypeScript**
- [ ] **Step 4: Ручна перевірка: `make backend` + `make web`, відкрити `http://localhost:5173` → редірект на `/login`; вхід оператором Київ №12 з паролем `operator` → `/operator` (поки заглушка), у шапці «Оператор · Київ, Відділення №12»; «Вийти» → `/login`; невірний пароль → повідомлення**
- [ ] **Step 5: Commit** `feat(web): scaffold, login, branded header, SSE invalidation`

---

### Task 12: Екрани оператора і старшої зміни

**Files:**
- Create: `web/src/components/ParcelTable.tsx`, `web/src/routes/operator.tsx`, `web/src/routes/senior.tsx`

**Interfaces:**
- Consumes: `api`, `LevelBadge`, `useInvalidateStream`, ендпоінти `/api/operator/*`, `/api/senior/problems`
- Produces:
  - `<ParcelTable rows={ProblemRow[]} actions?: (row) => ReactNode />` — колонки: рівень (`LevelBadge`), ТТН, проблеми (мітки), відколи (відносний час від віртуального «зараз» з `ClockState`), поточна локація
  - `/operator`, три блоки зверху вниз:
    1. **Проблемні посилки** (`queryKey: ["problems"]`). У режимі «як зараз» блок прихований.
    2. **Очікувані надходження** (`["incoming"]`): ТТН, звідки, рейс, ETA.
    3. **Склад (демо)** (`["physical"]`, теж інвалідується по `"problems"`): список фізичних ТТН. Біля кожної, якої немає в надходженнях і яку вже не прийнято, кнопка «Сканувати» → `POST /api/operator/scan` → модалка з результатом:
       - `accept` — «Посилка для вашого відділення» + кнопка «Прийняти»;
       - `redirect` — «Посилка для іншого відділення. Наступна точка: {назва}» + «Переадресувати»;
       - `hold` — «Не знайдено в системі» + «На ідентифікацію» і другорядна кнопка «Повернути…» (форма причини: Пошкоджена / Відмова отримувача / Інше + поле тексту);
       - `not_found` («як зараз») — «Посилку не знайдено» + лише «Повернути».
       429 → «Перевищено ліміт пошуків, зверніться до старшої зміни».
  - `/senior`: `ParcelTable` з `/api/senior/problems` + фільтр-чипи за рівнем; рядки з `risky_return` зверху

- [ ] **Step 1: Реалізувати**
- [ ] **Step 2: `npm run build` без помилок**
- [ ] **Step 3: Ручна перевірка (backend + emulator + web): адмін запускає сценарій 2 через `curl`, оператор Київ №12 бачить фокус-посилку 🟠 «Розірваний ланцюг», через кілька хвилин віртуального часу 🔴; у «Складі» вона є → «Сканувати» → «Прийняти» → зникає з проблемних без перезавантаження сторінки (SSE)**
- [ ] **Step 4: Commit** `feat(web): operator and senior screens`

---

### Task 13: Трекінг клієнта і симулятор

**Files:**
- Create: `web/src/routes/track.tsx`, `web/src/routes/sim.tsx`

**Interfaces:**
- Consumes: `/api/track/:ttn`, `/api/sim/*`
- Produces:
  - `/track`: поля «Номер ТТН» і «Телефон (необов'язково)», результат: статус, ETA з бейджем **«прогноз»**, коли `isForecast`; `details`, якщо є; 404 → «Посилку не знайдено». Пошук оновлюється по ключу `"track"`.
  - `/sim`:
    - **Годинник**: віртуальна дата й час (тікає локально між оновленнями за `virtualNow + elapsed × speed`), кнопки «Пауза», «×60», «×600», «+1 год», «+6 год»;
    - **Режим**: перемикач «Як зараз / З рішенням»;
    - **Сценарії**: 5 карток з назвами з spec §12 і кнопкою «Запустити»;
    - **Розділений вигляд** для `FocusTTN = "59000000000001"` (поле дозволяє змінити ТТН): зліва «Що бачить клієнт» (той самий компонент, що й на `/track`, з `details`), справа «Що бачить оператор відділення призначення»: «У списку надходжень: так / ні», `LevelBadge`, проблеми. `queryKey: ["split", ttn]`, інвалідується по `"problems"` і `"track"`.

- [ ] **Step 1: Реалізувати**
- [ ] **Step 2: `npm run build` без помилок**
- [ ] **Step 3: Ручна перевірка: на `/sim` запустити сценарій 2 у режимі «Як зараз» → зліва «У місті одержувача, орієнтовно 18.10 15:00 (прогноз)», справа «У списку надходжень: ні», без проблем; перемкнути «З рішенням» → справа з'являється 🟠/🔴. Повторити для сценаріїв 1, 3, 4, 5 і звірити з колонкою «Що має побачити оператор» у spec §12**
- [ ] **Step 4: Commit** `feat(web): client tracking and simulator with split view`

---

### Task 14: Docker, Caddy, README

**Files:**
- Create: `backend/Dockerfile`, `emulator/Dockerfile`, `web/Dockerfile`, `web/Caddyfile`, `README.md`
- Modify: `docker-compose.yml`, `Makefile` (`up: docker compose up --build`)

**Interfaces:**
- Produces:
  - Go Dockerfile (context — корінь репозиторію): `golang:1.27` build → `gcr.io/distroless/static` (або `alpine`), `CGO_ENABLED=0`
  - `web/Dockerfile`: `node:24` → `npm ci && npm run build` → `caddy:2` з `/srv` = `dist`, Caddyfile з spec §14 дослівно
  - `docker-compose.yml`: `postgres`, `backend` (env з адресами `postgres:5432`, `emulator:8090`), `emulator` (`BACKEND_URL=http://backend:8080`), `web` (порти `443:443`, `80:80`, volume `caddy_data` для локального CA); `depends_on` з healthcheck Postgres (`pg_isready`)
  - `README.md` українською: що це (2 речення + посилання на `docs/problem-solution.md`), розробка (`make db/backend/emulator/web`), повний запуск (`make up`, `https://localhost`, `caddy trust` / mkcert), паролі за замовчуванням, як пройти демо (сценарій 2 крок за кроком), `make test`

- [ ] **Step 1: Реалізувати**
- [ ] **Step 2: `make up`; `curl -skI --http2 https://localhost/api/locations` → `HTTP/2 200`; `https://localhost` у браузері відкриває `/login`; повний прогін сценарію 2 як у Task 13, Step 3**
- [ ] **Step 3: `make test` → усі тести PASS**
- [ ] **Step 4: Commit** `chore: docker compose with Caddy (HTTPS, HTTP/2), README`
