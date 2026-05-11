# WOMS Agent Guide / WOMS Agent 指南

## zh-TW

### 專案目標
WOMS 是晶圓訂單管理與排程系統，必須以最終部署型態開發：Go API、Go scheduler worker、原生 HTML/CSS/JS 前端、PostgreSQL、Redis、Apache Kafka、Docker、Kubernetes、Helm、NGINX Ingress、KEDA、GitHub Actions 與 Docker Hub。

### 分支與 Git 規則
- 禁止直接在 `main` 開發。
- 功能分支使用 `feat/xxxx-xxxx` 命名，例如 `feat/woms-foundation`。
- 建立或重整 feature branch 前，先 `git fetch origin` 並確認 `origin/main` 是最新；不要直接從過時的本地 `main` 切分支。這次 `feat/scheduling-calendar-fixes` 一度出現 `1 ahead, 3 behind`，原因是它先從舊的本地 `main` 分出去，等 `origin/main` 後來前進後才需要 rebase 才能正常開 PR。
- PR 標題與 commit message 前綴使用 `feat: xxxx`，不要使用 `[feat] xxxx` 或 `[codex] xxxx`。
- 每次 PR message 必須說明做了什麼事，格式使用 `feat: xxxx` 作為標題，內文用條列列出主要變更，例如 `- xxxx`。
- `main` 必須存在並設定保護；所有功能都應透過 PR 進入 `main`，讓 CI bot 檢查。
- Docker Hub publish 不應在 feature branch push 執行，只能在 `main`、`release/**` 或手動 workflow 執行。
- 每完成一項功能都必須執行可用測試、更新 README、更新 `.gitignore`、`git add`、commit、push。
- 不得提交 secrets、`.env`、本機 volume、build output、cache、IDE 私有設定。

### 文件規則
- 每次討論需求、設計或實作前，先用 `grill-me` skill 依序把所有需要確認的事項問完，逐題收斂共識後再開始做。
- 所有 `.md` 報告與文件都必須有 `zh-TW` 與 `en` 版本。
- `README.md` 預設使用英文，頂部提供語言列可跳到 `README.zh-TW.md`；同時保留 `README.en.md` 作英文版本。
- `AGENTS.md` 在同一檔案中維持雙語內容，作為團隊上下文記憶。
- 一般報告使用成對檔案，例如 `docs/implementation-plan.zh-TW.md` 與 `docs/implementation-plan.en.md`。
- 實作或部署行為改變時，README 必須同步更新。

### 編碼與繁中文案規則
- 所有 source code、SQL migration、Markdown 與前端檔案都使用 UTF-8。不要用系統預設 ANSI/Big5/CP950 重新寫檔。
- 在 Windows PowerShell 中修改含繁中文案的檔案時，避免用會套用預設編碼的寫檔方式；優先使用 `apply_patch`。若必須用 PowerShell 寫檔，明確指定 UTF-8。
- 這次 `db/migrations/001_init.sql` 的 `schedule_allocations.status` constraint 出現 `敺?蝔?` 等亂碼，根因是繁中 UTF-8 內容曾被錯誤 code page 讀寫，造成 mojibake。狀態值應維持為 `待排程`、`已排程`、`生產中`、`已完成`、`需業務處理`。
- 若發現 `敺`、`蝔`、`銝`、`撌`、`�` 等疑似 mojibake 字串，先追查是否為 UTF-8 被 CP950/ANSI 誤解碼，不要把亂碼加入新的 schema、API contract 或文件。

### 開發原則
- 採 TDD。排程、權限、狀態轉換、Redis lock、Kafka job flow、KEDA 驗證腳本都要先有測試或明確驗證案例。
- Go 程式碼必須可 `gofmt`，並通過 `go test ./...`。
- API 必須使用 JWT 與 RBAC；Ingress auth 只作為入口驗證，Go API 必須再次驗證。
- 排程演算法必須 deterministic，避免同樣輸入產生不同結果。
- 任何人工強制介入高優先級訂單都必須留下 audit log。

### 部署與驗證
- Docker image 必須可由 GitHub Actions build 並推到 Docker Hub。
- Kubernetes 部署使用 Helm chart，image tag 必須可由 values 指定。
- KEDA worker autoscaling 主要使用 Kafka lag，CPU trigger 為輔助。
- 完成功能後需依 `docs/verification.zh-TW.md` 與 `docs/verification.en.md` 驗證。

## en

### Project Goal
WOMS is a wafer order management and scheduling system. It must be built in the final deployment shape: Go API, Go scheduler worker, vanilla HTML/CSS/JS frontend, PostgreSQL, Redis, Apache Kafka, Docker, Kubernetes, Helm, NGINX Ingress, KEDA, GitHub Actions, and Docker Hub.

### Branch And Git Rules
- Never develop directly on `main`.
- Feature branches use `feat/xxxx-xxxx`, for example `feat/woms-foundation`.
- Before creating or rebasing a feature branch, run `git fetch origin` and confirm `origin/main` is current; do not branch from a stale local `main`. This `feat/scheduling-calendar-fixes` incident showed up as `1 ahead, 3 behind` because the branch was cut from an old local `main`, and once `origin/main` moved forward the branch had to be rebased before a PR could be opened cleanly.
- PR titles and commit messages use the `feat: xxxx` prefix. Do not use `[feat] xxxx` or `[codex] xxxx`.
- Every PR message must explain what changed: use `feat: xxxx` as the title and list the main changes in bullet points, for example `- xxxx`.
- `main` must exist and be protected; all features should enter `main` through PRs so the CI bot can check them.
- Docker Hub publishing must not run on feature branch pushes. It runs only on `main`, `release/**`, or manual workflow dispatch.
- After every completed feature, run available tests, update README, update `.gitignore`, `git add`, commit, and push.
- Do not commit secrets, `.env`, local volumes, build outputs, caches, or private IDE settings.

### Documentation Rules
- Before discussing requirements, design, or implementation, use the `grill-me` skill to ask every question in sequence and converge on shared understanding before starting work.
- Every `.md` report and document must provide both `zh-TW` and `en` content.
- `README.md` defaults to English and provides a language switcher to `README.zh-TW.md`; keep `README.en.md` as the English version.
- `AGENTS.md` keeps both languages in the same file as shared team memory.
- General reports use paired files, for example `docs/implementation-plan.zh-TW.md` and `docs/implementation-plan.en.md`.
- README must be updated whenever implementation or deployment behavior changes.

### Encoding And zh-TW Copy Rules
- All source code, SQL migrations, Markdown, and frontend files use UTF-8. Do not rewrite files with the system default ANSI/Big5/CP950 encoding.
- On Windows PowerShell, avoid write methods that use implicit default encodings when files contain zh-TW copy; prefer `apply_patch`. If PowerShell must write a file, explicitly choose UTF-8.
- The `schedule_allocations.status` constraint in `db/migrations/001_init.sql` previously showed mojibake such as `敺?蝔?`. The root cause was zh-TW UTF-8 text being read or written through the wrong code page. Status values must remain `待排程`, `已排程`, `生產中`, `已完成`, and `需業務處理`.
- If strings such as `敺`, `蝔`, `銝`, `撌`, or `�` appear unexpectedly, investigate UTF-8 text decoded as CP950/ANSI before adding the text to schema, API contracts, or documentation.

### Development Principles
- Use TDD. Scheduling, authorization, state transitions, Redis locks, Kafka job flow, and KEDA verification scripts need tests or explicit verification scenarios.
- Go code must be `gofmt` compatible and pass `go test ./...`.
- APIs must use JWT and RBAC. Ingress auth is only an entry check; the Go API must validate again.
- Scheduling must be deterministic so the same input produces the same result.
- Any manual force intervention that moves high-priority orders must create an audit log.

### Deployment And Verification
- Docker images must be buildable by GitHub Actions and pushable to Docker Hub.
- Kubernetes deployment uses a Helm chart, and image tags must be configurable through values.
- KEDA worker autoscaling primarily uses Kafka lag, with CPU as a secondary trigger.
- Completed features must be verified with `docs/verification.zh-TW.md` and `docs/verification.en.md`.
