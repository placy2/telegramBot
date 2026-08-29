# Modernization runbook

A staged cleanup pass for a repo last touched in 2020. Work top to bottom —
the phases are ordered so that each one leaves the repo in a working state,
and so the thing that actually **unblocks the bot** comes first.

Scope note: this is a ~150-line hobby project across 6 files. Everything
below is deliberately proportional to that — no CI pipelines, no test
frameworks, no container work. The goal is "runs correctly on modern
tooling, no landmines," not "production service."

## Baseline (verified 2026-08-21)

Good news first, so you know what you're *not* fighting:

```
go build ./...   # clean
go vet ./...     # clean
```

The repo still **compiles** fine under Go 1.27 despite `go 1.15` in
`go.mod`. Nothing here is a compile-error rescue mission. The breakage is
all at runtime and in the dependency layer.

---

## Phase 1 — The bot is dead; replace the Reddit data source

**This is the whole ballgame.** Everything else in this runbook is
housekeeping. Right now the Reddit half of the bot cannot work at all.
Verified from this machine on 2026-08-21 (control hosts `example.com` and
`proxy.golang.org` returned 200 on the same run, so this is Reddit
blocking, not a local network problem):

```
GET  https://www.reddit.com/r/soccer/new.json  -> 403   (Reddit's HTML block page)
POST https://www.reddit.com/api/login/{user}   -> 403
```

Two things rotted since 2020:

1. `geddit.NewLoginSession` posts to `/api/login/{username}` — Reddit's
   pre-OAuth login endpoint, dead for years.
2. `Session.SubredditSubmissions` hits the **public** `.json` listing
   endpoint unauthenticated. Reddit's 2023 API lockdown killed that path;
   it 403s regardless of User-Agent (tried both `gedditAgent v1` and a
   browser UA).

### Why not OAuth

The obvious fix is Reddit's OAuth script-app flow, and geddit does ship
an `OAuthSession` pointed at the right endpoints. I probed the token
endpoint with bogus credentials and got a clean `401
{"message": "Unauthorized", "error": 401}` — meaning the endpoint is
alive and parsed the request correctly.

**But that only proves the protocol works, not that you can still get
credentials.** Per your research, personal-use script app registration
is no longer available. I could not independently confirm the current
state of `prefs/apps` (my search turned up only 2023-era material, and
my training data may predate the change), so treat OAuth as unavailable
unless you find otherwise. Even if it were available, it would mean
betting on a library frozen since 2020 plus two more secrets to manage.

### Use the RSS feeds instead

Reddit still serves Atom feeds with no authentication at all. Verified
working on 2026-08-21:

```
GET https://www.reddit.com/r/soccer/new.rss        -> 200  (122 KB, live data)
GET https://old.reddit.com/r/soccer/new.rss        -> 429  (use www)
```

Each `<entry>` carries everything both tasks need:

| Atom field | Example | Used for |
|---|---|---|
| `<title>` | `Kim Min-soo Transfer to Rangers Finalized` | keyword/team matching |
| `<link href>` | `https://www.reddit.com/r/soccer/comments/1vuw4s6/...` | the link to send |
| `<published>` | `2026-08-21T23:28:21+00:00` | time-window filtering |
| `<id>` | `t3_1vuw4s6` | dedupe key |
| `<author><name>` | `/u/rdh2dmd` | optional attribution |

**Know the ceiling before you commit to this** (all measured on
r/soccer, a high-traffic sub):

- `?limit=100` works. `limit=250` still returns 100; `limit=500` returns 0.
  **100 entries is a hard cap.**
- `after=` pagination **does not work** on the RSS endpoint — I tried
  paging with the last entry's `t3_` id and got an empty page back.
- Coverage is therefore activity-dependent: 25 entries (the default)
  spans ~3.2 hours on r/soccer; 100 entries spans ~7.6 hours.

So a single RSS fetch **cannot** cover a 24-hour window on a busy sub.
That's fine for the existing hype task (it currently asks for 15 posts
per sub). For the soccer digest it means one of:

- Accept "the last ~100 posts" as the practical definition of recent,
  which is reasonable for an on-demand `/digest` you trigger manually.
- Run periodically and let a dedupe table accumulate the 24h picture
  across runs — better anyway, and no longer blocked on a hosting story:
  `dispatch/poller` (added after this section was written; see the
  correction below) is exactly that periodic runner, ticking in-process
  for as long as the bot is up. The table it accumulates into is now
  called `HypeClip`, not `RedditPost` (see Phase 3g's update).
- Point at quieter, team-specific subs, where 100 posts can span days.

Note this deletes the pagination design from the soccer plan
(`GetFromSubredditsSince` walking `After`/`FullID`) — that only worked
against the JSON API.

### What this looks like in code

Dropping geddit entirely is the simplification here: you need `net/http`
and `encoding/xml` from the standard library, and nothing else. It also
removes geddit's transitive deps (`go-rate`, `go-querystring`).

Good Go practice in this step: **struct tags drive the XML decoder**, the
same mechanism as the JSON tags you will use for config in Phase 4c.

```go
package reddit

type Entry struct {
    ID        string    `xml:"id"`
    Title     string    `xml:"title"`
    Published time.Time `xml:"published"` // encoding/xml parses RFC3339 into time.Time for you
    Link      struct {
        Href string `xml:"href,attr"`
    } `xml:"link"`
}

type feed struct {
    Entries []Entry `xml:"entry"`
}

func FetchSubreddit(ctx context.Context, sub string, limit int) ([]Entry, error) {
    url := fmt.Sprintf("https://www.reddit.com/r/%s/new.rss?limit=%d", sub, limit)
    req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
    if err != nil {
        return nil, err
    }
    // Reddit wants a descriptive UA; generic ones get throttled harder.
    req.Header.Set("User-Agent", "telegramBot/1.0 (by /u/yourusername)")

    resp, err := http.DefaultClient.Do(req)
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        return nil, fmt.Errorf("reddit rss %s: %s", sub, resp.Status)
    }

    var f feed
    if err := xml.NewDecoder(resp.Body).Decode(&f); err != nil {
        return nil, err
    }
    return f.Entries, nil
}
```

Two things worth internalizing from that snippet:

- `defer resp.Body.Close()` immediately after the error check — the Go
  idiom for guaranteed cleanup. Leaking response bodies leaks connections.
- `http.NewRequestWithContext` rather than `http.Get`, so a hung request
  can be cancelled. Pass `context.Background()` for now; it costs nothing
  to have the seam there.

I got a `429` on `old.reddit.com` during testing, so add a small sleep
between subreddit fetches (a plain `time.Sleep(time.Second)` in the loop
is plenty at this scale — no rate-limiter library needed).

> **Correction (2026-08-29):** `time.Sleep(time.Second)` was wrong by
> about 60x. Measured directly against `www.reddit.com/.../new.rss` with a
> conforming User-Agent: the anonymous budget is **exactly 1 request per
> 60-second window, per IP, shared across all feeds** —
> `x-ratelimit-used: 1` / `x-ratelimit-remaining: 0.0` after a single
> success, confirmed clean over a 10-minute probe with no penalty
> escalation. A 1-second gap between subreddits guarantees 429s on every
> fetch but the first, which is exactly the bug reported when `/hype` was
> first tested end-to-end. The fix wasn't a longer sleep — inline fetching
> at 1 req/min makes a multi-subreddit command take minutes. Instead,
> Reddit fetching moved out of the command path entirely into
> `dispatch/poller`, a background goroutine that ticks at a safe interval
> (default 75s, floored at 60s via `config.PollSeconds`) and persists
> matches for `/hype` to read. See `CLAUDE.md`'s architecture section for
> the resulting package. This closes out the `time.Sleep` line above —
> don't reintroduce a sleep-based pacing loop on a command's hot path.

### Rewiring the call sites

`utils.GetFromSubreddits` currently takes a `*geddit.LoginSession` and
returns `[]*geddit.Submission`. It becomes a loop over
`reddit.FetchSubreddit` returning `[]reddit.Entry`. `tasks/reddit.go`
drops its `geddit.NewLoginSession` block entirely — and with it, the
`REDDIT_USERNAME` / `REDDIT_PASSWORD` env vars. The bot goes from four
required secrets to **one** (`TELEGRAM_KEY`, plus
`TELEGRAM_OWNER_CHATID`).

Finally: `go mod tidy` after removing the geddit import, and update the
README's env var section to match.

> **Honest risk note:** unauthenticated RSS is not a contract Reddit owes
> you — it's a path they have left open, and they have been progressively
> closing such paths since 2023. It works today, verified. If it closes,
> the remaining options are an official (paid/approved) API key or a
> different data source. That risk is worth taking here over betting on a
> six-year-old library plus credentials you may not be able to obtain.

---

## Phase 2 — Dependencies and language version

### 2a. Bump the `go` directive

`go.mod` says `go 1.15`. Bump it to something current:

```
go 1.24
```

(1.27 is what you have installed; 1.24 leaves breathing room. Either is
fine.) Since Go 1.21 this directive is a real minimum-version
requirement with toolchain management attached, not the advisory hint it
was in 1.15.

### 2b. Telegram library: v4 `+incompatible` → v5

`github.com/go-telegram-bot-api/telegram-bot-api v4.6.4+incompatible` is
the pre-modules layout — that `+incompatible` suffix means the library
never adopted Go modules for v4. v5.5.1 is the current release and lives
at a **different module path** (`/v5` suffix).

```bash
go get github.com/go-telegram-bot-api/telegram-bot-api/v5@v5.5.1
```

Update every place `NewBotAPI` gets constructed to the `/v5` import path —
don't hardcode file paths here, they've already moved once this session
(`main.go`, plus wherever the `telegram` package's `Send` function lives).
**Done** — both call sites are on `/v5` now, and `go mod tidy` dropped the
v4 requirement along with its own transitive dependency
(`technoweenie/multipartstreamer`) since nothing imports it anymore. The
API turned out to be nearly identical — `NewUpdate`, `NewMessage`, and
`Send` all kept their signatures. **One real break:**

```go
// v4 — returns (channel, error)
updates, err := bot.GetUpdatesChan(u)

// v5 — returns only the channel (bot.go:431)
updates := bot.GetUpdatesChan(u)
```

Which is convenient, because the current code captures that `err` and
then never checks it. Now it can't.

> Heads up: upstream archived this repo, so v5.5.1 is effectively
> terminal. It works fine and is the right call today. If it ever breaks
> against a Telegram API change, active community forks exist — not
> worth switching preemptively.

### 2c. GORM and SQLite driver

```bash
go get gorm.io/gorm@latest          # 1.20.2 -> 1.31.2
go get gorm.io/driver/sqlite@latest # 1.1.3  -> 1.6.0
```

No API changes you'll notice at this usage level.

### 2d. Tidy

```bash
go mod tidy
```

**Done.** `go.sum` went from ~380 lines of transitive cruft
(`cloud.google.com/go/bigquery`, `pubsub`, GLFW bindings — all pulled in
by `geddit`'s `oauth2` dependency graph, gone since Phase 1) down to 14.
`go.mod` now lists exactly four direct dependencies: telegram-bot-api v5,
gorm, and the sqlite driver.

---

## Phase 3 — Correctness bugs worth fixing while you're in here

These are all real, all small, and all currently latent in the code.

### 3a. Nil dereference in the telegram package's `Send`

```go
bot, err := tgbotapi.NewBotAPI(os.Getenv("TELEGRAM_KEY"))
if err != nil {
    fmt.Println(err.Error())   // <-- logs, then keeps going
}

if bot.Self.UserName == "" {   // <-- bot is nil here. panic.
```

`NewBotAPI` returns `nil, err` on failure (confirmed in the v4 source,
`bot.go:36-54`). Printing an error and continuing is the bug — in Go, an
error return means the other value is not usable. `return` after the log.

### 3b. `dao.Exists` returns `true` on unexpected errors

```go
result := db.First(&post, "perma_link = ?", PermaLink)
if result.Error != nil {
    msg := result.Error.Error()
    if msg == "record not found" { return false }
    fmt.Println(msg)
}
return true   // <-- any other error falls through to "yes, it exists"
```

Two problems. First, string-matching the error text is fragile; use the
sentinel:

```go
if errors.Is(result.Error, gorm.ErrRecordNotFound) {
    return false
}
```

`errors.Is` is the modern idiom (Go 1.13+) and is exactly the kind of
thing that postdates this codebase. Second, on a *real* DB error the
function currently claims the post exists, which silently swallows
headlines. Failing toward "already seen" means you lose posts; decide
deliberately which way you want to fail and comment it.

### 3c. One DB handle instead of one per call

`dao.Init()` opens a fresh `*gorm.DB` (a whole connection pool) on every
single `Exists` and `Create` call, and never closes any of them. Hoist it
to a package-level handle initialized once:

```go
var db *gorm.DB

func Init() { /* assign db once */ }
```

Call `dao.Init()` from `main()`, and have `Exists`/`Create` use the
package handle. `sync.Once` is an option if you want it lazy, but for a
single-process bot an explicit `Init()` at startup is simpler and easier
to reason about.

### 3d. Make dedupe survive restarts

```go
gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
```

In-memory means the `RedditPost` dedupe table is wiped every restart —
so a restart re-sends posts you've already been shown. Point it at a
file:

```go
gorm.Open(sqlite.Open("bot.db"), &gorm.Config{})
```

`.gitignore` already covers `*.db`, so this needs no other changes.

### 3e. Stop hardcoding `bot.Debug = true` — done

`main.go:18` logs every update including full message content,
unconditionally. Gate it: `bot.Debug = os.Getenv("BOT_DEBUG") != ""`.

**Done (2026-08-29).** `main.go` now sets
`bot.Debug = os.Getenv("BOT_DEBUG") != ""`; document the env var in
`README.md`.

### 3f. `/hype` sends an empty acknowledgment

`main.go`'s `"hype"` case calls `tasks.SendHypePlays(ctx, cfg)` but never
sets `msg.Text` first:

```go
case "hype":
    tasks.SendHypePlays(ctx, cfg)   // msg.Text is still ""
```

`bot.Send(msg)` then fires with an empty string every time — Telegram
silently drops it. The task itself runs fine; only the immediate ack is
broken, so this is easy to miss. Give it a line like the other cases:

```go
case "hype":
    msg.Text = "Looking for hype plays..."
    tasks.SendHypePlays(ctx, cfg)
```

**This fix was necessary but not sufficient.** Setting `msg.Text` fixed
the empty-message bug, but `bot.Send(msg)` in `main.go` was still called
*after* the `switch` block — so the ack, now non-empty, still didn't go
out until `SendHypePlays` (a synchronous, multi-second-plus network call)
had fully returned. First end-to-end test (2026-08-29) surfaced this as
"the 'no plays found' message arrived before the 'looking for plays'
message." Real fix: `bot.Send(msg)` moved to fire immediately after the
`switch`, before the command's work runs — see `main.go` and `CLAUDE.md`'s
architecture section. Also see Phase 1's correction above:
`SendHypePlays` no longer does the network fetch itself at all, so this
is doubly moot now — the ack is followed by a database read, not a
multi-subreddit RSS crawl.

### 3g. `store`'s field name lagged behind what it stores

`hype.go` dedupes via `store.Exists(e.ID)` / `store.Create(e.ID)` —
`e.ID` is the RSS entry's `<id>` (e.g. `t3_1vuw4s6`), not a permalink.
That's a fine dedupe key (short, stable), but `store/entities.go`'s field
is still called `PermaLink`, and `store.go`'s query still hardcodes the
column name to match:

```go
result := db.First(&post, "perma_link = ?", PermaLink)
```

Independent of soccer — this is fallout from the Phase 1 switch to RSS,
where "the unique thing about a post" stopped being a URL and became an
entry ID. Rename the field to something like `PostID` in both
`entities.go` and `store.go`, **and** update the query string alongside
it — GORM's default snake_case mapping means `PostID` → `post_id`, so
the hardcoded `"perma_link = ?"` has to change too or the lookup will
silently query a column that doesn't exist under the new field name.

**Done, and superseded (2026-08-29).** `PostID` landed before this pass.
`store` has since moved beyond a dedupe-only table entirely: `RedditPost`
(one `PostID` string) is now `HypeClip` (`PostID` unique-indexed, plus
`Subreddit`, `Title`, `URL`, `Published`, `SentAt`) because the background
poller (Phase 1's correction, above) needs somewhere to hold a match
between finding it and `/hype` delivering it — `Exists`/`Create` became
`SaveClip`/`UnsentClips`/`MarkSent`. See `CLAUDE.md`.

### 3h. Ctrl-C could hang for up to a minute — fixed (2026-08-29)

Not in the original pass — introduced by the poller work's graceful
shutdown, then fixed in the same session. `main.go` added
`signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)` so the poller
goroutine could be told to stop cleanly. That replaced Go's default
SIGINT behavior (instant kill) with "cancel a context, then wait for the
program to notice" — and the thing that has to notice,
`tgbotapi.GetUpdatesChan`'s internal loop, only checks for shutdown
*between* long-poll requests. Each request can block up to `u.Timeout`
(60s) waiting on Telegram, and `NewBotAPI`'s `http.Client` has no
client-side timeout backing it, so the worst case was bounded only by
Telegram actually responding. First Ctrl-C after this shipped looked like
it did nothing; a second one didn't help either, since the context was
already cancelled.

Fix: the shutdown goroutine now force-`os.Exit`s 3 seconds after the
signal if the natural shutdown hasn't landed by then — there's no
buffered work here that needs a longer graceful drain. Verified against a
standalone repro of the same shape (a goroutine that only checks a
shutdown channel between blocking calls): without the fix it hangs for
the full call duration; with it, exits in exactly 3s. If you touch
`main.go`'s shutdown path again, keep the force-exit — this is a known
limitation of the archived `telegram-bot-api` library, not something
fixable by waiting more patiently.

---

## Phase 4 — General abstractions pulled forward from the soccer plan

These were written up in `docs/daily-soccer-digest.md` but aren't
soccer-specific at all — they're cleanup of existing code that the soccer
feature merely *happened* to need. Doing them here means the soccer plan
shrinks to just its genuinely soccer-shaped parts.

### 4a. `utils/reddit_links.go` — old.reddit rewriting

Note this changes shape because of Phase 1. The old geddit
`Submission.Permalink` was a **path** (`/r/soccer/comments/...`), so the
helper concatenated a host onto it. RSS `<link href>` is already a
**full URL** (`https://www.reddit.com/r/soccer/comments/...`), so the
helper becomes a host *rewrite* instead:

```go
// OldRedditLink rewrites a reddit.com URL to the ad-free old.reddit.com host.
// Returns the input unchanged if it isn't a URL we recognize.
func OldRedditLink(link string) string {
    u, err := url.Parse(link)
    if err != nil || !strings.HasSuffix(u.Host, "reddit.com") {
        return link
    }
    u.Host = "old.reddit.com"
    return u.String()
}
```

Using `net/url` rather than `strings.Replace` is the point of the
exercise — it parses the URL properly instead of pattern-matching text,
so a stray `www.reddit.com` inside a query string or path can't be
corrupted by accident.

**The double-slash bug this originally fixed is now fixed for free.**
`tasks/reddit.go:36` currently does
`fmt.Sprintf("https://www.reddit.com/%s", s.Permalink)` against a
permalink that already starts with `/`, producing
`https://www.reddit.com//r/soccer/...` — every link the bot has ever
sent has had it. Once entries come from RSS with a complete `href`, that
`Sprintf` disappears entirely rather than needing a fix.

### 4b. `utils/matching.go` — case-insensitive keyword match

```go
func MatchesAny(haystack string, needles []string) bool {
    lower := strings.ToLower(haystack)
    for _, n := range needles {
        if strings.Contains(lower, strings.ToLower(n)) {
            return true
        }
    }
    return false
}
```

Replaces `tasks/reddit.go:35`:

```go
if strings.Contains(s.Title, "HYPE") || strings.Contains(s.Title, "Hype") || strings.Contains(s.Title, "hype") {
```

which enumerates three casings and still misses `hYpE`. Becomes
`utils.MatchesAny(s.Title, []string{"hype"})`.

### 4c. Un-hardcode the subreddit list

`tasks/reddit.go:27` has its subreddit list baked into the function body.
The soccer plan introduced a `config/` package for exactly this reason —
if you build that package here, in general form, the soccer work
inherits it for free. Keep it minimal: a struct with JSON tags and a
`Load(path string) (*Config, error)`. Don't build a config framework.

### What stays in the soccer plan

For clarity when you circle back to `docs/daily-soccer-digest.md` — after
this runbook, only these remain genuinely soccer-specific:

- `tasks/soccer.go` — team-grouped digest assembly
- The `--digest-once` flag / cron entry point
- Team + alias config shape (built on top of 4c's loader)
- A time-window filter — but a much simpler one than planned (see below)

Steps 1, 2, and part of 4 of that guide are superseded by Phase 4 here.

**Step 3 of that guide is now obsolete.** `GetFromSubredditsSince` was
designed to paginate the JSON API via `After`/`FullID` to reach back 24
hours. RSS has no working pagination and caps at 100 entries (Phase 1),
so that function collapses into a filter over a single fetch:

```go
cutoff := time.Now().Add(-lookback)
for _, e := range entries {
    if e.Published.Before(cutoff) {
        continue
    }
    // ...
}
```

Simpler code, but read the coverage math in Phase 1 before you decide
what `lookback` can honestly promise — on a busy sub, 100 entries is
about 7.6 hours, not 24.

---

## Phase 5 — Light polish (optional)

Proportional to a hobby repo — don't gold-plate:

- **A couple of unit tests.** `utils.MatchesAny` and `utils.RedditLink`
  are pure functions with zero dependencies — ideal first tests, and a
  good excuse to learn Go's table-driven test idiom. `go test ./...`.
  There are currently no tests at all; two good ones beats a coverage
  mandate.

  **Done (2026-08-29).** `dispatch/reddit/reddit_test.go` covers
  `OldLink` (now `reddit.OldLink`, since Phase 4a landed) and
  `MatchesAny` (now `reddit.MatchesAny`, moved from `tasks` so
  `dispatch/poller` can share it too).
- **Remove the committed binary.** `telegramBot` is tracked in git — a
  6-year-old **Linux x86-64** ELF executable, on a repo you now develop
  on darwin/arm64. It cannot even run here.
  ```bash
  git rm --cached telegramBot
  ```

  **Done.** Both the original `telegramBot` (removed in the initial
  modernization refactor commit) and the later locally-built `/bot`
  (removed once `.gitignore` covered it) are gone from git.
  Then add `/bot` and `/telegramBot` to `.gitignore` (the existing rules
  cover `*.exe`/`*.so` but not extensionless Unix binaries).
- **README refresh.** It documents 4 env vars; after Phase 1 you'll be
  down to **2** (`TELEGRAM_KEY`, `TELEGRAM_OWNER_CHATID`) — the Reddit
  credentials go away entirely with RSS. Worth calling out prominently;
  it makes the bot far easier to set up. The "Goal/Updates" log at the
  bottom still ends at 11/2/2020 — either continue it or drop it.
- **`CLAUDE.md`** — update the architecture notes for the config package
  and the data-source change, so future sessions aren't working from a
  stale map.
- **Tasks run synchronously inside the update loop.** `SendHypePlays` now
  does real HTTP fetches plus a `time.Sleep(time.Second)` per subreddit,
  so a `/hype` command blocks the loop for several seconds before the next
  incoming command gets handled. Telegram's update channel just buffers
  it — nothing breaks — but it's sluggish for a single-user bot. Worth a
  `go tasks.SendHypePlays(ctx, cfg)` once you're back in this code; not
  worth its own commit today.

  **Superseded (2026-08-29), not just done.** The concern itself is gone,
  not merely fixed by adding `go`: `SendHypePlays` no longer takes `ctx`
  or does network I/O at all (see Phase 1's correction) — it's a bounded
  SQLite read (`store.UnsentClips`, capped at 20 rows) plus a handful of
  Telegram sends, all fast. The actual blocking work — Reddit fetches —
  now lives in `dispatch/poller`'s own goroutine, off the update loop
  entirely. A `go` wrapper around `SendHypePlays` would add nothing today.

---

## Target file layout

Where you land after all five phases. The guiding principle for Go, and
the one that trips up people arriving from Java/C#/Python: **the package
is the unit of encapsulation, not the file.** Files in the same package
share unexported identifiers freely, so you split into a new package only
when you want a real boundary — not merely to keep files tidy.

```
telegramBot/
├── main.go                       # entry point: bot setup, poller launch, command dispatch
└── dispatch/
    ├── config/
    │   ├── config.go             # Config struct + Load() (with defaults/floors)
    │   └── config.json           # subreddits, keywords, post count, poll interval (hand-edited)
    ├── reddit/
    │   ├── reddit.go             # Entry, FetchSubreddit, RateLimitError, OldLink
    │   ├── match.go              # MatchesAny — exported, shared across packages
    │   └── reddit_test.go
    ├── poller/
    │   └── poller.go             # Run() — the only caller of FetchSubreddit; Snapshot() health
    ├── telegram/
    │   └── telegram.go           # Send()
    ├── store/
    │   ├── store.go              # Init(), SaveClip(), UnsentClips(), MarkSent()
    │   └── entities.go           # HypeClip
    ├── tasks/
    │   ├── hype.go               # SendHypePlays — reads store, no network
    │   └── soccer.go             # placeholder — see daily-soccer-digest.md
    └── docs/
```

This is what's actually on disk as of this pass — a deliberate choice, not
the un-prefixed layout this section originally recommended. The `dispatch/`
wrapper groups every bot-owned package under one recognizable name without
reaching for `internal/` (which the guidance below still argues against for
the reasons given there) — a lightweight middle ground between "everything
at root" and the compiler-enforced privacy `internal/` provides.

**Updated 2026-08-29** to reflect the poller work: `dispatch/poller` was
added (new package, not anticipated when this layout was first written),
`tasks/match.go` moved to `reddit/match.go` as the exported `MatchesAny`,
and `store`'s dedupe-only API became a fuller clip-lifecycle API on a
renamed `HypeClip` type. See `CLAUDE.md`'s architecture section for the
authoritative current description — this section is a snapshot of intent
per-phase and will drift again as soon as `soccer.go` stops being a
placeholder.

### What moved, and why

| Before | Became | Reason |
|---|---|---|
| `utils/telegram.go` | `dispatch/telegram/telegram.go` | Named for what it provides |
| `utils/multisub.go` | `dispatch/reddit/reddit.go` | Reddit domain logic belongs together |
| `utils/reddit_links.go` (planned) | `dispatch/reddit/reddit.go` | Same — it's Reddit knowledge |
| `utils/matching.go` (planned) | `dispatch/reddit/match.go`, exported `MatchesAny` | Landed in `tasks/match.go` unexported first; re-exported and moved once `dispatch/poller` needed it too |
| `dao/` | `dispatch/store/` | `dao` is a Java-ism; Go prefers plain names |
| `tasks/reddit.go` | `dispatch/tasks/hype.go` | Avoids confusion with the `reddit` package |

**`utils/` disappears entirely, and that's the point.** A package named
`utils` (or `common`, `helpers`, `misc`) tells a reader nothing about
what's inside, and it becomes a junk drawer that everything imports.
Every planned occupant had a natural home somewhere more specific.

Note `matchesAny` in particular: at this point in the pass (Phase 4b) it's
a fair call to make it a lowercase helper shared by `hype.go` and
`soccer.go`, both in package `tasks` — no new package, no exported API.
If you were writing this in Java you'd reach for a `StringUtils` class; in
Go, a third file in the same package is the whole answer.

**This didn't survive Phase 1's later correction.** Once Reddit fetching
moved into `dispatch/poller` — a *third* package that also needs to
filter entries by keyword — `tasks`-private no longer covered every
caller. `matchesAny` is now the exported `reddit.MatchesAny`, living in
`dispatch/reddit/match.go`. The lesson still holds as written (don't
reach for a package prematurely) — it's just that "every caller" turned
out to include code that hadn't been designed yet when this section was
written. `dispatch/tasks/match.go` no longer exists.

### Naming: avoid stutter

Package name and identifier are read together at the call site, so don't
repeat yourself:

```go
utils.SendTelegramMessage(msg)   // before
telegram.Send(msg)               // after

utils.OldRedditLink(link)        // before
reddit.OldLink(link)             // after
```

The rule of thumb is that the package name is already half the
documentation — `telegram.Send` is unambiguous, and `telegram.SendTelegramMessage`
just stutters. Same reason the standard library has `http.Get`, not
`http.HTTPGetRequest`.

### Two things you'll see recommended — and can skip

- **`cmd/`** — for a repo that builds one binary, `main.go` at the root
  is correct and simpler. `cmd/` earns its place when you have *multiple*
  binaries (`cmd/bot/`, `cmd/migrate/`) and need somewhere to put each
  `package main`. You have one.
- **`internal/`** — a real feature, not a convention: the compiler
  *refuses* imports of `internal/...` from outside the module. That
  matters if you're publishing a library and want to keep packages
  private. For a personal bot nobody imports, it buys you a guarantee
  against a problem you don't have, at the cost of an extra path segment
  everywhere.

If you ever do publish this, moving everything under `internal/` is a
mechanical change — so there's no cost to deferring the decision.

> You may run across `golang-standards/project-layout`, which prescribes
> `pkg/`, `api/`, `build/`, `configs/` and more. It's widely cargo-culted
> and is not an official standard — the Go team has never endorsed it. It
> would roughly triple the directory count here for zero benefit. Ignore
> it at this size.

---

## Suggested commit sequence

One commit per phase keeps the diff reviewable and gives you clean
rollback points:

```
1. deps: bump go directive, telegram-bot-api v5, gorm; go mod tidy
2. feat: replace geddit/JSON API with unauthenticated Reddit RSS — restores a working bot
3. fix: nil deref, Exists error handling, shared DB handle, file-backed SQLite, empty /hype ack, PermaLink->PostID rename
4. refactor: extract link + matching helpers, un-hardcode subreddits
5. refactor: dissolve utils/ into telegram|reddit|tasks; dao/ -> store/
6. chore: drop committed binary, tests, README/CLAUDE.md
```

Keep commit 5 (the package moves) free of behavior changes — pure
renames and relocations. A diff that only moves code is trivial to
review and trivial to revert; one that moves *and* edits code is
neither. `git mv` where you can, so the history follows the file.

Note this inverts Phase 1 and 2 relative to the doc — doing the dep bump
first means you write the new Reddit client against the libraries you're
actually keeping, rather than migrating it twice. Read the runbook in the
order above, commit in this order.

Commit 2 is the one that matters. Everything before it is cosmetic;
everything after it is polish on a bot that works again.

Run `go build ./... && go vet ./...` between each. Both are clean today,
so any output is something you just introduced.

**This sequence predates the poller work and stops short of it.** As of
2026-08-29 the working tree also carries (uncommitted): the rate-limit
fix and `dispatch/poller` package, the `HypeClip` store rework, the
message-ordering fix, the graceful-shutdown fix (3h above), and the doc
updates across this file, `CLAUDE.md`, `README.md`, and
`daily-soccer-digest.md`. None of that fits cleanly into commits 1–6
above; treat it as its own commit (or a couple, split by
poller-vs-shutdown-fix if you want the history granular) on top of
whatever of 1–6 already landed.
