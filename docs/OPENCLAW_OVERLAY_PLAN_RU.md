# OpenClaw Multi-User Overlay — План реализации

> Документ описывает реализацию административной утилиты `openclaw-multi`,
> которая поверх стандартного OpenClaw (ветка `main`) автоматизирует
> мульти-пользовательскую установку на VPS с разделением по Linux-юзерам,
> автоматическим управлением туннелями (Cloudflare Tunnel + Tailscale) и
> hardening-стандартом adversarial-grade изоляции.
>
> Контекст и обоснование решений — в
> [OPENCLAW_OVERVIEW_RU.md](OPENCLAW_OVERVIEW_RU.md) и
> [OPENCLAW_QA_RU.md](OPENCLAW_QA_RU.md).
> Этот документ — **план разработки**, не сама реализация.

---

## 1. Цели и не-цели

### 1.1. Цели

- **Полностью автоматизированная установка OpenClaw на VPS** с нуля: один
  скрипт, никакого ручного редактирования файлов.
- **Разделение по Linux-юзерам** с adversarial-grade изоляцией (стандартный
  UNIX hardening: UID/GID, FS permissions, systemd cgroups, hidepid,
  ptrace_scope, отдельные `--user` юниты).
- **Автоматическое управление сетью**:
  - **Tailscale** — для admin-доступа к самой VPS (SSH, доступ к
    overlay-API на 127.0.0.1, мониторинг). Юзеру Tailscale не нужен.
  - **Cloudflare Tunnel** — публичный HTTPS для всего, что доступно
    юзерам и внешним сервисам: персональный Control UI каждого юзера,
    OAuth callbacks, webhook URL'ы плагинов и каналов.
  - UFW остаётся `default deny incoming`, никаких ручных правок.
- **Управление жизненным циклом пользователей**: add / remove /
  activate / deactivate / backup / restore / quota.
- **Полное удаление с опциями** — overlay умеет красиво уйти, не оставив
  мусора, с гранулярными опциями по системным сервисам и пользовательским
  данным.
- **Tenant-scoped OpenClaw runtime**: OpenClaw CLI и Node.js живут внутри
  каждого managed Linux user через `nvm`; system-space overlay не зависит от
  глобального OpenClaw CLI.
- **Node.js 24**: tenant runtime ставит Node.js `24` через `nvm`.
- **OpenClaw updates из `pdasilem/openclaw:latest`**: OpenClaw CLI ставится и
  обновляется из этого источника. Команда обновления задается настройкой
  overlay.
- **Non-interactive onboarding**: OpenClaw onboarding внутри tenant запускается
  только через `openclaw onboard --non-interactive`.
- **TUI-интерфейс** для администратора: одно входное меню, удобная
  навигация в SSH-сессии без X-сервера.
- **Terminal tab в админке**: на каждом экране overlay после приветствия/логина
  внизу есть вкладка терминала, чтобы админ запускал команды без выхода из
  панели.

### 1.2. Не-цели

- **Не альтернатива OpenClaw**, не форк, не патч кора. Все патчи в
  upstream — отдельной задачей.
- **Не подменяет публичный flow OpenClaw**. Юзеры подключают каналы
  (Telegram, Slack, MS Teams, Discord, WhatsApp), модельных провайдеров,
  плагины из ClawHub и любые другие фичи через **стандартный** OpenClaw
  (CLI, Control UI, бот). Overlay только публикует нужные сетевые
  endpoint'ы под капотом, не вмешиваясь в сами flow.
- **Не Docker и не VM-перевиртуализация**. Только нативный `systemctl
--user` + UNIX-юзеры.
- **Не панель управления для конечных юзеров**. TUI overlay — только
  инструмент **админа** VPS. У юзеров — стандартный OpenClaw Control UI
  через выданный admin'ом HTTPS-URL.
- **Не PaaS / multi-tenant SaaS**. Это административный инструмент для
  одного VPS на одну организацию.
- **Не Web UI для overlay**. Только TUI.

### 1.3. Жёсткое разделение ролей админ vs юзер

**Админ** в overlay — это **обычный Linux-юзер** (например, `ubuntu`
на DigitalOcean / Hetzner / большинстве VPS, или специально заведённый
`opadmin`), у которого **есть возможность повышать права через sudo**
(`sudo`/`wheel` group), но **который не работает под root постоянно**.
Имя админа задаётся **по username** при первом запуске
`openclaw-multi`.

> **Важно:** root-аккаунт как постоянный admin — это антипаттерн и
> противоречит security best-practices (а у многих VPS-провайдеров
> прямой root-login по SSH вообще запрещён). Overlay поэтому
> определяет админа **по имени**, а не по UID-0, и при попытке
> запуска под `uid==0` показывает warning с рекомендацией
> «создайте обычного юзера, добавьте в sudo, перезапустите overlay
> от его имени».

Идентификация админа:

- При первом запуске `openclaw-multi` (когда в `state.db` нет
  записанного админа) — overlay читает `$SUDO_USER` (если запущено
  через `sudo openclaw-multi`); если не пусто — это и есть кандидат в
  админы. Если запущен напрямую (без sudo) — берёт `$USER` /
  результат `whoami`.
- TUI показывает: «обнаружен админ-кандидат: `<username>`. Сохранить?»
  → admin сохраняется в state.db (`admin_username`).
- При всех последующих запусках overlay сравнивает текущего юзера
  по имени (`$USER` / `whoami`) с `admin_username` из state.db.

Sudo по требованию (без постоянного root):

- `openclaw-multi` сам запускается **под обычным username** админа
  (не под root).
- Для **операций, требующих root** (write в `/etc/cloudflared/`,
  `/etc/systemd/system/`, `useradd`, `loginctl enable-linger`,
  `npm i -g`, `ufw`, `sysctl`), overlay вызывает их через `sudo
<command>`. Если sudo попросит пароль — пароль будет введён в TUI
  один раз за сессию (cached `sudo -v` на длительность сессии).
- Альтернативно (для headless/CI-сценариев): админ настраивает
  passwordless sudo для конкретного перечня команд через
  `/etc/sudoers.d/openclaw-multi` — overlay предлагает сгенерировать
  этот файл при fresh install (опционально).

Жёсткий контроль:

- При запуске `openclaw-multi` под user, чьё имя **не равно**
  `admin_username` из state.db — немедленный выход с ошибкой и audit
  log entry. Даже если этот user — root, отказ остаётся в силе
  (исключение возможно только при «recovery» режиме, см. ниже).
- При попытке `openclaw-multi bootstrap <username>` с username,
  равным `admin_username`, или с username, входящим в `sudo`/`wheel`,
  — отказ с объяснением.
- При `useradd` для openclaw-юзера overlay явно ставит ему shell
  `/bin/bash` **без** добавления в `sudo`/`wheel`. Если админ потом
  руками добавит юзера в `sudo` — `openclaw-multi audit` это заметит
  и пометит как critical-warning.

Recovery (если админ-аккаунт потерян/заблокирован):

- TUI поддерживает спец-режим `openclaw-multi --recover-admin`, который
  можно запустить **только под root**. Он позволяет переназначить
  `admin_username` в state.db. Этот режим логируется в audit как
  CRITICAL и шлёт notification.

Это разделение исключает:

1. «Обычный юзер вдруг получает доступ к admin-TUI» — отказ по имени.
2. «Админ постоянно работает под root» — overlay явно отговаривает и
   запускается под обычным username.
3. «Админ случайно становится юзером OpenClaw на той же VPS» — отказ
   bootstrap'а с admin's username.

---

## 2. Принципы

### 2.1. Невмешательство в OpenClaw core

- Ничего не патчится в `node_modules/openclaw/`.
- Ничего не редактируется в `~/.openclaw/openclaw.json` руками админа —
  только через `openclaw config set` (стандартный публичный API) или через
  свой бинарь openclaw, запущенный из-под нужного юзера.
- Все артефакты overlay живут в **двух непересекающихся** локациях:
  - `/opt/openclaw-multi/` — статические файлы overlay (бинарник,
    шаблоны, скрипты).
  - `~/.openclaw-overlay/` каждого юзера — состояние watcher'а и
    overlay-аугментации (port allocation, tunnel routes), **не
    `~/.openclaw/`**.
- Шаблоны конфигов и unit-файлов параметризованы (`envsubst`) и
  применяются строго через `openclaw config set` или `systemctl --user`.

### 2.2. Идемпотентность

- Любая команда может быть выполнена многократно без ущерба:
  `bootstrap alice` второй раз = no-op + sanity check.
- Все операции пишутся в audit log (`/var/log/openclaw-multi/audit.log`).
- При сбое посередине — overlay умеет докатить или откатить.

### 2.3. Прозрачность для конечного юзера

- Юзер `alice` запускает `openclaw` — работает обычный CLI OpenClaw,
  никаких подмен.
- Установка плагина `openclaw plugins install clawhub:gog` — работает
  обычный механизм; overlay только дополнительно поднимает tunnel route
  если плагин этого попросил, и записывает URL обратно в config.
- ClawHub плагины и skills работают как обычно.
- Webhook-каналы (Telegram webhook, Slack HTTP Request URL, MS Teams
  Bot Framework) тоже подключаются стандартным wizard'ом OpenClaw —
  overlay только публикует webhook URL через cloudflared, не подменяя
  процесс настройки на стороне внешнего сервиса (юзер сам прописывает URL
  в @BotFather, Slack App config, Azure Bot reg).

### 2.4. Минимум внешних зависимостей

- Один бинарник overlay на хост.
- На VPS должны быть: `bash` (как интерпретатор скриптов overlay,
  shebang `#!/bin/bash`), `systemd`, `npm/node`, `tailscale`,
  `cloudflared`. Overlay умеет проверить и при необходимости поставить
  отсутствующее.
- **Какой login-shell использует админ или юзеры — не имеет значения.**
  Если у админа дефолтный shell `zsh`, `fish` или любой другой —
  overlay-скрипты всё равно выполняются под `bash` через shebang
  `#!/bin/bash`. Сам `bash` в Ubuntu/Debian установлен по умолчанию
  и присутствует даже если пользовательский shell — `zsh`. Overlay
  при pre-flight check проверяет только наличие бинарника `bash` в
  PATH (`command -v bash`), не текущий `$SHELL`.
- Бинарники TUI / overlay-API / watcher написаны на Go и не зависят
  от shell вообще — они общаются с системой через `os/exec` напрямую.

### 2.5. Идемпотентность установки зависимостей

Каждый шаг pre-flight check сначала **проверяет наличие** и пропускает
уже сконфигурированное:

| Зависимость         | Проверка                                                                                     | Что делает overlay                                                                                                                                                                                  |
| ------------------- | -------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Node.js 24          | проверка выполняется внутри managed user через `~/.local/bin/openclaw` и `nvm`               | system-space не ставит Node.js; tenant runtime создается на этапе add-user                                                                                                                          |
| Tailscale           | `command -v tailscale && systemctl is-active tailscaled && tailscale status`                 | если уже работает — сохранить identity, **не делать** `tailscale up` повторно. Опционально предложить добавить ACL-tag для VPS. Если есть, но не залогинен — провести `tailscale up --ssh`          |
| Cloudflared         | `command -v cloudflared` + наличие `~/.cloudflared/cert.pem` или `/etc/cloudflared/cert.pem` | если бинарь и сертификат есть — переиспользовать; если есть свой `config.yml` — сделать timestamped backup и спросить «overwrite?». Если cloudflared не установлен — поставить из `.deb`            |
| UFW                 | `ufw status`                                                                                 | если уже active с правилами — показать текущие, спросить «применить overlay-правила (`deny incoming`/`allow outgoing` + ваш порт) и сохранить остальные?». Если inactive — настроить и активировать |
| sysctl hardening    | `sysctl kernel.yama.ptrace_scope` и т.д.                                                     | если все нужные значения уже выставлены — пропустить; иначе записать `/etc/sysctl.d/openclaw-overlay.conf` и `sysctl --system`                                                                      |
| `/proc hidepid=2`   | `mount \| grep proc`                                                                         | если уже `hidepid=2` — пропустить; иначе править `/etc/fstab` и `mount -o remount /proc`                                                                                                            |
| OpenClaw CLI        | `su - <user> -c "/home/<user>/.local/bin/openclaw --version"`                                | ставится только в tenant user-space через `nvm`; глобальный OpenClaw CLI не используется                                                                                                           |

Любая операция, изменяющая системное состояние, **сначала делает бэкап**
старого файла (`/var/lib/openclaw-multi/snapshots/<ts>/`) и пишет в audit
log. Откат возможен из меню §6.10.

---

## 3. Архитектура (high-level)

```
┌─────────────────────────────────────────────────────────────────────┐
│                          VPS (host)                                 │
│                                                                     │
│ ┌─────────────────────────────────────────────────────────────────┐ │
│ │ /usr/local/bin/openclaw-multi   (TUI bin, admin entry point)    │ │
│ └─────────────────────────────────────────────────────────────────┘ │
│                                                                     │
│ ┌─────────────────────────────────────────────────────────────────┐ │
│ │ /opt/openclaw-multi/    (статические артефакты overlay)         │ │
│ │   templates/   scripts/   schemas/                              │ │
│ └─────────────────────────────────────────────────────────────────┘ │
│                                                                     │
│ ┌─────────────────────────────────────────────────────────────────┐ │
│ │ Системные сервисы (root):                                       │ │
│ │   cloudflared.service          (один на VPS, hot-reload)        │ │
│ │   openclaw-overlay-api.service (HTTP daemon на UNIX-сокете)     │ │
│ └─────────────────────────────────────────────────────────────────┘ │
│                                                                     │
│ ┌──────────────────┐ ┌──────────────────┐ ┌──────────────────┐      │
│ │ User: alice      │ │ User: bob        │ │ User: charlie    │      │
│ │  ~/.openclaw/    │ │  ~/.openclaw/    │ │  ~/.openclaw/    │      │
│ │  ~/.openclaw-    │ │  ~/.openclaw-    │ │  ~/.openclaw-    │      │
│ │   overlay/       │ │   overlay/       │ │   overlay/       │      │
│ │ systemctl --user:│ │ systemctl --user:│ │ systemctl --user:│      │
│ │   openclaw-      │ │   openclaw-      │ │   openclaw-      │      │
│ │     gateway      │ │     gateway      │ │     gateway      │      │
│ │   openclaw-      │ │   openclaw-      │ │   openclaw-      │      │
│ │     overlay-     │ │     overlay-     │ │     overlay-     │      │
│ │     watcher      │ │     watcher      │ │     watcher      │      │
│ └──────────────────┘ └──────────────────┘ └──────────────────┘      │
│                                                                     │
│ ┌─────────────────────────────────────────────────────────────────┐ │
│ │ Сеть:                                                           │ │
│ │   tailscaled    (admin-only: SSH к VPS, доступ к overlay-API)   │ │
│ │   cloudflared   (публичные inbound: персональный Control UI     │ │
│ │                  каждого юзера + callbacks + webhooks плагинов) │ │
│ │   ufw           (default deny incoming / allow outgoing)        │ │
│ └─────────────────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────────────┘
```

### 3.1. Поток управления (control flow)

- **Админ** запускает `openclaw-multi` (TUI), выбирает действие.
- Команды → bash-обвязки в `/opt/openclaw-multi/scripts/` → стандартные
  системные команды (`useradd`, `systemctl`, `loginctl`, `ufw`, `tailscale`,
  `cloudflared`) и `openclaw` (через `su -`).
- При **bootstrap нового юзера** overlay-API сразу создаёт публичный
  ingress route `gateway-<user>.openclaw.<domain>` →
  `localhost:<user-port>`, чтобы юзер мог зайти в свой Control UI с
  любого устройства без Tailscale.
- **Per-user watcher** в `systemctl --user` следит за изменениями
  `~/.openclaw/openclaw.json` юзера. Если появился плагин с
  `needs.publicCallback` — обращается к **overlay-API daemon** через
  UNIX-сокет, тот добавляет route в cloudflared и возвращает URL.
- **Overlay-API daemon** — единственный компонент с привилегиями писать в
  `/etc/cloudflared/config.yml` и слать SIGHUP cloudflared.
  Авторизация watcher'а — через UNIX-сокет с проверкой UID (`SO_PEERCRED`).

### 3.2. Поток данных (data flow для нового публичного route)

```
Plugin install
    ↓
~/.openclaw/openclaw.json (новая запись plugins.entries.<id>)
    ↓ inotify
overlay-watcher (systemctl --user, под UID юзера)
    ↓ HTTP POST /routes (через UNIX socket /run/openclaw-overlay.sock)
overlay-api (системный, root)
    ↓ allocate port + write config + SIGHUP
cloudflared
    ↓ ingress route активен
overlay-api → response { url, port }
    ↓
overlay-watcher
    ↓ openclaw config set plugins.entries.<id>.config.callbackUrl=<url>
    ↓ openclaw config set plugins.entries.<id>.config.callbackPort=<port>
Plugin пересчитывает конфиг (hot-reload в OpenClaw)
```

---

## 4. Технологический стек

### 4.1. Язык TUI

**Рекомендация: Go + [Bubble Tea v2](https://github.com/charmbracelet/bubbletea) +
[Lip Gloss v2](https://github.com/charmbracelet/lipgloss).**

Текущий baseline репозитория: `go 1.26.2`,
`charm.land/bubbletea/v2 v2.0.6`, `charm.land/lipgloss/v2 v2.0.3`,
`modernc.org/sqlite v1.49.1`, `gopkg.in/yaml.v3 v3.0.1`.
`go.sum` коммитится вместе с `go.mod`: он фиксирует checksums модулей и их
`go.mod`, чтобы сборки были воспроизводимыми и Go мог обнаружить подмену
зависимости.

Обоснование:

- **Один статический бинарник** (`go build`), никаких runtime-зависимостей
  на VPS, кроме базового Linux.
- Богатый TUI: меню, списки, формы, прогресс-бары, нативные цвета и
  unicode.
- Активная экосистема (Charm.sh).
- Не пересекается с Node-runtime'ом OpenClaw → апдейты OpenClaw не влияют
  на overlay.

### 4.2. Сетевые слои

- **Tailscale** — admin-only (SSH к VPS, доступ к overlay-API,
  мониторинг). Юзеру не нужен.
- **Cloudflare Tunnel** (`cloudflared`) — публичные inbound:
  персональный Control UI каждого юзера, OAuth callbacks, webhook'и
  плагинов и каналов. Wildcard DNS `*.openclaw.<your-domain>` → CNAME
  tunnel (если у админа есть домен) **или** quick-tunnel
  `*.cfargotunnel.com` (без аккаунта Cloudflare).
- **UFW** — `default deny incoming` + `allow outgoing` + ваш сторонний
  порт.

### 4.3. Изоляция per-user

- `useradd` + `loginctl enable-linger`.
- `systemctl --user` для openclaw-gateway и openclaw-overlay-watcher.
- `umask 0077` через `/etc/profile.d/openclaw.sh`.
- Hardening sysctl + `/proc hidepid=2`.
- Замена Docker-sandbox на rootless Podman (опционально, по выбору при
  установке).

### 4.4. State и audit

- Состояние overlay (port pool, активные routes, юзеры) — SQLite в
  `/var/lib/openclaw-multi/state.db`. Простая схема, читаемо `sqlite3` CLI.
- Audit log — JSONL в `/var/log/openclaw-multi/audit.log` с ротацией через
  `logrotate`.

### 4.5. Бэкап и удаление через встроенный OpenClaw

- Бэкап юзера: `openclaw backup create` (`docs/cli/backup.md`) — даёт
  timestamp'ed `.tar.gz` с manifest.json, поддерживает `--verify`,
  `--only-config`, `--no-include-workspace`. Overlay поверх него
  опционально шифрует (`openssl enc -aes-256-cbc -pbkdf2`) и кладёт в
  `/var/lib/openclaw-multi/backups/<user>/`.
- Удаление юзера: `openclaw uninstall --all --yes --non-interactive`
  (`docs/cli/uninstall.md`) под `su - <user>`, потом `userdel -r`.
- Не делаем своего велосипеда — в OpenClaw уже есть надёжные команды.

---

## 5. Структура директорий

```
/usr/local/bin/
  openclaw-multi                          # TUI бинарник (Go single binary)
  openclaw-overlay-api                    # Daemon бинарник (Go)
  openclaw-overlay-watcher                # Per-user watcher бинарник (Go)
  openclaw-multi-bootstrap                # Bash launcher для одноразовой установки

/opt/openclaw-multi/
  templates/
    openclaw.json.tmpl                    # шаблон конфига Gateway per-user
    openclaw-gateway.service.tmpl         # systemd --user unit
    openclaw-overlay-watcher.service.tmpl # watcher unit
    cloudflared-config.tmpl               # начальный config.yml
    sysctl-overlay.conf                   # /etc/sysctl.d/openclaw-overlay.conf
    fstab-proc-hidepid                    # фрагмент /etc/fstab
    profile-d-openclaw.sh                 # /etc/profile.d/openclaw.sh
  scripts/
    bootstrap-user.sh
    deactivate-user.sh
    activate-user.sh
    backup-user.sh                        # обёртка над `openclaw backup create`
    restore-user.sh                       # обёртка над unpack + ownership fix
    healthcheck.sh
    network-check.sh
    uninstall-user.sh                     # обёртка над `openclaw uninstall`
    uninstall-overlay.sh                  # снимает overlay с хоста
  schemas/
    state.sql                             # схема SQLite
    config.yaml.schema.json               # JSON-Schema для overlay-config

/etc/openclaw-multi/
  config.yml                              # глобальная конфигурация overlay
                                          # (домен Cloudflare, диапазоны портов и т.д.)

/etc/cloudflared/
  config.yml                              # ingress routes (управляется overlay-api)
  <tunnel-id>.json                        # credentials cloudflared

/etc/systemd/system/
  cloudflared.service
  openclaw-overlay-api.service

/etc/sysctl.d/openclaw-overlay.conf
/etc/profile.d/openclaw.sh

/var/lib/openclaw-multi/
  state.db                                # SQLite
  backups/<username>/<timestamp>.tar.gz[.enc]
  snapshots/<timestamp>/                  # бэкапы системных файлов перед изменениями
/var/log/openclaw-multi/
  audit.log
  cloudflared-reload.log

/var/cache/openclaw-compile/              # общий Node compile cache (chmod 1777)

# Per-user (под $HOME юзера)
~/.openclaw/                              # стандартный OpenClaw, не трогается overlay-ом
~/.openclaw-overlay/                      # overlay-аугментации
  watcher.state                           # последняя обработанная версия конфига
  routes.local                            # локальный список routes этого юзера
~/.config/systemd/user/
  openclaw-gateway.service
  openclaw-overlay-watcher.service
```

---

## 6. TUI: главное меню и подменю

### 6.1. Главное меню

```
┌─ OpenClaw Multi-User Overlay ──────────────────────────── v0.1.0 ─┐
│                                                                   │
│  Хост: vps-fra1.example.com  |  Tailscale: ON  |  CF Tunnel: ON   │
│  Юзеров: 3 (active 2)        |  Gateway healthy: 2/3              │
│                                                                   │
│  > 1. Установка с нуля (fresh install)                            │
│    2. Обновление OpenClaw                                         │
│    3. Управление пользователями                                   │
│    4. Health check / Doctor                                       │
│    5. Сеть и фаервол (Tailscale, Cloudflare, UFW)                 │
│    6. Плагины и туннели (обзор по юзерам)                         │
│    7. Логи и мониторинг                                           │
│    8. Аудит безопасности                                          │
│    9. Diagnostic snapshot (для support)                           │
│   10. Удаление OpenClaw / overlay                                 │
│    0. Выход                                                       │
│                                                                   │
└───────────────────────────────────────────────────────────────────┘
  ↑/↓ навигация   Enter выбрать   q выход
```

### 6.2. (1) Fresh install

Сценарий «свежий VPS, ничего не установлено» (или частично установлено —
overlay идемпотентен, см. §2.5).

Подшаги (TUI ведёт по wizard'у):

1. **Pre-flight check.**
   - Linux distro и версия (поддерживаются Ubuntu 22.04+/Debian 12+).
   - Свободное место (≥ 5 GB), RAM (≥ 1 GB).
   - Наличие `systemd`, `useradd`, `loginctl`.
   - Открытые порты `ss -tlnp` (предупредить если что-то слушает на
     18789–19999).
2. **Установка зависимостей (идемпотентно).**
   - Node.js 24 — по логике §2.5.
   - Tailscale — если уже работает, сохранить identity и не трогать;
     иначе `curl -fsSL https://tailscale.com/install.sh | sh` →
     `tailscale up --ssh`.
   - Cloudflared — если есть `cert.pem`, переиспользуем; иначе скачать
     `.deb` и поставить.
   - UFW — если уже active, не сбрасывать чужие правила, **дописать**
     наши (`allow in <ваш-сторонний-порт>`, гарантировать
     `default deny incoming / allow outgoing`).
3. **Настройка Tailscale (admin-only).**
   - `tailscale up` (если не сделано).
   - Опционально настроить ACL-tag для VPS (например `tag:openclaw-vps`).
   - **Юзеры Tailscale-аккаунта не получают** — это инструмент админа.
4. **Настройка Cloudflare Tunnel.**

   Два варианта на выбор:

   **Вариант A. С Cloudflare-аккаунтом (рекомендуется, основной путь).**
   Бесплатный тариф Cloudflare даёт всё нужное.
   - Запросить email + API token (или `cloudflared login` через OAuth).
   - Запросить домен (`<your-domain>`) и поддомен для overlay (по
     умолчанию `openclaw`).
   - `cloudflared tunnel create openclaw-multi`.
   - Записать DNS-CNAME `*.openclaw.<your-domain>` →
     `<tunnel-id>.cfargotunnel.com`.
   - Сгенерировать `/etc/cloudflared/config.yml` с пустым ingress
     (только catch-all 404).
   - Установить `cloudflared.service` (systemd).

   **Вариант B. Quick tunnels (временный тестовый режим без аккаунта).**
   - `cloudflared tunnel --url http://localhost:18000` без login.
   - URL'ы будут эфемерными `*.cfargotunnel.com` со случайным префиксом —
     каждый раз новый при перезапуске cloudflared.
   - Подходит для разовых тестов / dev-окружения, не рекомендуется в
     production.
   - Overlay предупреждает админа о минусах и сохраняет выбор.

5. **Настройка UFW.**
   - `ufw default deny incoming / allow outgoing`.
   - Опционально: `ufw allow in 8080/tcp` (TUI спрашивает «есть ли
     сторонние порты для открытия наружу?»).
   - `ufw enable`.
6. **Hardening хоста.**
   - Записать `/etc/sysctl.d/openclaw-overlay.conf` и `sysctl --system`.
   - Изменить `/etc/fstab`: `proc /proc proc defaults,hidepid=2,gid=adm 0 0`,
     `mount -o remount /proc`.
   - Записать `/etc/profile.d/openclaw.sh` (umask 0077, NODE_COMPILE_CACHE).
   - `mkdir -p /var/cache/openclaw-compile && chmod 1777 ...`.
7. **OpenClaw CLI не ставится глобально.**
   - Fresh install ставит только system-space overlay components.
   - Node.js `24` и OpenClaw CLI создаются внутри managed user на этапе
     add-user через tenant `nvm`.
   - OpenClaw CLI ставится из `pdasilem/openclaw:latest`.
   - Команда обновления OpenClaw задается настройкой overlay.
   - Onboarding запускается только через
     `/home/<user>/.local/bin/openclaw onboard --non-interactive`.
8. **Установка overlay-API daemon.**
   - Скопировать бинарник в `/usr/local/bin/openclaw-overlay-api`.
   - Установить `openclaw-overlay-api.service` (systemd).
9. **Создание первого пользователя** — переход в подменю «Add user» (см.
   §6.4).
10. **Финальный summary**: что сделано, какие URL/токены сохранены, где
    лежат логи, ссылка на доку.

После завершения — return в главное меню.

### 6.3. (2) Обновление OpenClaw

```
┌─ Обновление OpenClaw ─────────────────────────────────────────────┐
│                                                                   │
│  Текущая версия (global):  2026.4.5                               │
│  Доступная версия (npm):   2026.4.12                              │
│                                                                   │
│  Юзеров с запущенным Gateway: 3                                   │
│  ─────────────────────────────────                                │
│  > [x] Сделать бэкап перед обновлением (через openclaw backup)    │
│    [x] Запустить обязательные пост-апдейтные тесты                │
│    [x] Автоматический откат при падении тестов                    │
│    [x] Перезапустить все Gateway после обновления                 │
│    [ ] Включить debug-режим (verbose log)                         │
│                                                                   │
│  [ Применить ]  [ Отмена ]                                        │
│                                                                   │
└───────────────────────────────────────────────────────────────────┘
```

Логика:

1. Если выбран бэкап — `openclaw backup create` под каждым юзером (см.
   §6.4.4).
2. Для каждого tenant обновить user-space OpenClaw CLI через его `nvm`
   окружение.
3. Если выбран рестарт — для каждого юзера `systemctl --user
restart openclaw-gateway`.
4. **Обязательные пост-апдейтные тесты** (см. §13 митигацию):
   - `openclaw config validate` под каждым юзером;
   - `openclaw doctor` под каждым юзером с парсингом;
   - smoke-test публикации mock-плагина с tunnel-callback (overlay-API
     поднимает временный route, проверяет доступность, удаляет).
5. **При падении хотя бы одного теста**:
   - откат tenant package в каждом затронутом user-space runtime;
   - перезапуск всех Gateway;
   - audit log entry с уровнем CRITICAL;
   - TUI notification («апдейт откачен, требуется обновление overlay-слоя
     для совместимости с OpenClaw <new>»);
   - опциональное email/Telegram-уведомление (если настроено в
     `/etc/openclaw-multi/config.yml`).
7. Audit log: версия до, версия после, кто запустил, время, результат
   тестов.

### 6.4. (3) Управление пользователями

Подменю:

```
┌─ Управление пользователями ───────────────────────────────────────┐
│                                                                   │
│  Текущие пользователи (3):                                        │
│  ┌────────┬────────┬─────────┬──────────┬─────────────────────┐   │
│  │ Имя    │ Статус │ Порт    │ Linger   │ Создан              │   │
│  ├────────┼────────┼─────────┼──────────┼─────────────────────┤   │
│  │ alice  │ active │ 18789   │ on       │ 2026-04-12          │   │
│  │ bob    │ active │ 18809   │ on       │ 2026-04-15          │   │
│  │ carol  │ paused │ 18829   │ off      │ 2026-04-20          │   │
│  └────────┴────────┴─────────┴──────────┴─────────────────────┘   │
│                                                                   │
│  > 1. Добавить пользователя                                       │
│    2. Удалить пользователя                                        │
│    3. Деактивировать (paused)                                     │
│    4. Активировать                                                │
│    5. Бэкап данных пользователя                                   │
│    6. Восстановление из бэкапа                                    │
│    7. Сменить порт пользователю                                   │
│    8. Просмотр конфига пользователя                               │
│    9. Запустить openclaw doctor для пользователя                  │
│   10. Просмотр активности routes (last_seen)                      │
│   11. Назад                                                       │
│                                                                   │
└───────────────────────────────────────────────────────────────────┘
```

#### 6.4.1. Add user

1. Запросить username (валидация: a-z0-9\_-, длина ≤ 32).
2. Аллоцировать порт из пула (например, шаг 20: 18789 → 18809 → 18829).
3. Запросить или сгенерировать первичный gateway token.
4. `useradd -m -s /bin/bash <user>`.
5. `loginctl enable-linger <user>`.
6. Прокинуть стандартное окружение (`/etc/profile.d/openclaw.sh`).
7. **Подготовка env vars** (overlay подставляет их перед запуском
   `openclaw onboard` чтобы wizard сразу взял правильные значения):

   ```bash
   export OPENCLAW_GATEWAY_PORT=<выделенный порт>
   export OPENCLAW_GATEWAY_TOKEN=<сгенерированный токен>
   export OPENCLAW_GATEWAY_BIND=loopback        # cloudflared будет публиковать
   ```

   **Что должен выбрать админ в wizard'е** (TUI показывает подсказки):
   - `--non-interactive` — обязательно.
   - `--install-daemon` — обязательно (systemd --user unit + linger).
   - Bind mode — `loopback` (overlay сам публикует через cloudflared).
   - Tailscale в `gateway.tailscale` — `mode: off` (overlay не использует
     TS для юзеров).
   - Auth mode — `token` (default), токен уже подставлен через env.
   - Каналы и провайдеры задаются параметрами non-interactive onboarding.

8. **Запустить non-interactive onboarding OpenClaw**:

   ```bash
   su - <user> -c "/home/<user>/.local/bin/openclaw onboard --non-interactive --mode local --auth-choice skip --gateway-bind loopback --gateway-auth token --gateway-token-ref-env OPENCLAW_GATEWAY_TOKEN --gateway-port $OPENCLAW_GATEWAY_PORT --install-daemon --accept-risk"
   ```

   После завершения OpenClaw Multi проверяет `openclaw doctor` и user service.

9. После завершения onboard — TUI **возвращает себе управление** и
   доделывает overlay-сторону:
   - `chmod 700 ~/.openclaw`, `chmod 600 ~/.openclaw/openclaw.json`;
   - записать unit `openclaw-overlay-watcher.service`,
     `systemctl --user enable --now`;
   - **через overlay-API создать ingress route**
     `gateway-<user>.openclaw.<domain>` → `localhost:<port>`. Это
     персональный публичный URL юзера для Control UI;
   - прописать в state.db: имя, порт, статус active, gateway URL;
   - Audit log.
10. **Финальный экран**: показать админу:
    - Публичный Control UI URL (`https://gateway-<user>.openclaw.<domain>`)
    - Gateway token (для juзера, чтобы залогиниться)
    - Все системные действия — done.

После этого admin передаёт юзеру URL+token любым удобным способом
(email, мессенджер, бумажка). Юзер открывает URL в любом браузере с
любого устройства, вводит токен — попадает в стандартный Control UI
OpenClaw. Никакого Tailscale у юзера нет.

#### 6.4.2. Remove user

1. Запросить confirmation (с typing username).
2. Опционально предложить бэкап (по умолчанию yes) — см. §6.4.4.
3. `su - <user> -c "openclaw uninstall --all --yes --non-interactive"`
   (`docs/cli/uninstall.md`) — стандартное OpenClaw удаление сервиса +
   state + workspace.
4. Через overlay-API удалить **все** ingress routes юзера (gateway +
   плагин-callbacks).
5. `systemctl --user --machine=<user>@.host stop openclaw-gateway
openclaw-overlay-watcher` (или через `su -`).
6. `loginctl disable-linger <user>`.
7. `userdel -r <user>` (с `-r` — удаляет `$HOME` со всем содержимым).
8. Освободить порт в state.db.
9. Audit log.

#### 6.4.3. Deactivate / Activate

**Deactivate.** Сетевой доступ юзера полностью блокируется, но данные
остаются:

- `systemctl --user stop openclaw-gateway openclaw-overlay-watcher`
  (Gateway-процесс юзера падает).
- `loginctl disable-linger <user>` (даже если юзер залогинится по SSH и
  попробует запустить руками — после logout сразу убьётся).
- Через overlay-API пометить **все routes** юзера (gateway + плагин-
  callbacks) как `disabled` → cloudflared удаляет ingress entries → SIGHUP.
- Снять флаг `active` в state.db (статус `paused`).
- Audit log.

Что юзер видит при deactivate:

- **Telegram-бот** перестаёт отвечать (Gateway не слушает Telegram polling).
- **Control UI** — `https://gateway-<user>.openclaw.<domain>/` начинает
  возвращать 404 от cloudflared catch-all.
- **Webhook'и плагинов** (если были) — тоже 404.
- **SSH в `<user>`** (если был доступ) всё ещё работает, но `openclaw
gateway` руками не запустить — даст ошибку про port lock или watcher
  не сможет достучаться до overlay-API.
- **Данные в `~/.openclaw/`** — нетронуты. После activate всё работает
  как было.

**Activate.** Обратное:

- Включить routes в overlay-API → cloudflared SIGHUP.
- `loginctl enable-linger <user>`.
- `systemctl --user start openclaw-gateway openclaw-overlay-watcher`.
- Снять `paused` в state.db.
- Audit log.

#### 6.4.4. Backup

- **Использует встроенный** `openclaw backup create` (`docs/cli/backup.md`).
- TUI выполняет:
  ```bash
  su - <user> -c "openclaw backup create --output ~/.openclaw-backup-tmp --verify"
  ```
- Архив `<timestamp>-openclaw-backup.tar.gz` уже содержит manifest.json
  и проверен `--verify`.
- **Опционально** overlay шифрует архив поверх:
  `openssl enc -aes-256-cbc -pbkdf2 -salt -in <archive> -out <archive>.enc`
  с паролем, который TUI спрашивает или генерирует.
- Перемещает в `/var/lib/openclaw-multi/backups/<user>/<timestamp>.tar.gz[.enc]`.
- Метаданные backup'а в state.db: ts, размер, sha256, OpenClaw version.

#### 6.4.5. Restore

- Список бэкапов с возможностью выбрать конкретный.
- Если архив зашифрован — спросить пароль:
  `openssl enc -aes-256-cbc -pbkdf2 -d -in ... -out ...`.
- Если юзер существует:
  - `systemctl --user stop openclaw-gateway`;
  - `tar -xzf <archive> -C ~/<user>/.openclaw-restore-tmp/`;
  - `chown -R <user>:<user> ...`;
  - переместить в `~/.openclaw/` (предварительно бэкап текущего state);
  - `systemctl --user start openclaw-gateway`.
- Если юзера нет — добавить + восстановить за один шаг.
- Опционально: `openclaw backup verify` перед восстановлением.

#### 6.4.6. Дополнительные операции

- **Quota management**: задать `MemoryMax`/`CPUQuota`/`IOWeight` per user
  (правка systemd unit override через `systemctl --user edit
openclaw-gateway.service`).
- **Rotate token**: перевыдать `OPENCLAW_GATEWAY_TOKEN`, прописать в
  config, restart Gateway, показать новый токен админу.
- **Show DM endpoints**: показать публичный Control UI URL юзера и все
  его plugin callback URL'ы.
- **Просмотр активности routes (last_seen)**: см. §6.4.7.

#### 6.4.7. Просмотр активности routes (last_seen)

Cloudflared пишет access log (через `journalctl -u cloudflared`).
Overlay парсит логи и показывает per-route last_seen timestamp:

```
┌─ Route activity для юзера alice ──────────────────────────────────┐
│                                                                   │
│  Hostname                                  | Target | Last seen   │
│  ─────────────────────────────────────────────────────────────── │
│  gateway-alice.openclaw.example.com        | :18789 | 2 мин назад │
│  oauth-google.alice.openclaw.example.com   | :18901 | 4 дня назад │
│  webhook-tg.alice.openclaw.example.com     | :18902 | 1 час назад │
│  teams.alice.openclaw.example.com          | :18903 | 3 нед назад │
│                                                                   │
│  > [ Удалить выбранные routes ]   [ Назад ]                       │
│                                                                   │
└───────────────────────────────────────────────────────────────────┘
```

Админ сам решает, удалять stale routes или оставить (например, OAuth
callback больше не нужен — токен уже получен).

Если в системе по какой-то причине нет cloudflared логов с access info
(например, отключено через flag) — overlay показывает в колонке
`last_seen` значение `n/a` и не делает ничего сложнее.

### 6.5. (4) Health check / Doctor

```
┌─ Health Check ────────────────────────────────────────────────────┐
│                                                                   │
│  System:                                                          │
│    [✓] Disk space (free 12.3 GB / 50 GB)                          │
│    [✓] Memory (used 1.8 GB / 4 GB)                                │
│    [✓] Load average (0.42, 0.31, 0.27)                            │
│    [⚠] Sysctl ptrace_scope = 1 (рекомендуется 2)                  │
│    [✓] /proc hidepid = 2                                          │
│                                                                   │
│  Services:                                                        │
│    [✓] cloudflared                                                │
│    [✓] openclaw-overlay-api                                       │
│    [✓] tailscaled                                                 │
│    [✓] ufw (active)                                               │
│                                                                   │
│  Per-user:                                                        │
│    alice: [✓] gateway active, [✓] watcher active, [✓] linger on   │
│    bob:   [✓] gateway active, [✓] watcher active, [✓] linger on   │
│    carol: [—] paused                                              │
│                                                                   │
│  Network:                                                         │
│    [✓] Tailscale connected (admin-only, 100.x.x.x)                │
│    [✓] Cloudflare tunnel up (3+N ingress routes active)           │
│    [✓] UFW rules: deny incoming, allow outgoing, allow 8080       │
│                                                                   │
│  > [ Запустить openclaw doctor под каждым юзером ]                │
│    [ Применить рекомендованные исправления (auto-fix) ]           │
│    [ Назад ]                                                      │
│                                                                   │
└───────────────────────────────────────────────────────────────────┘
```

Логика:

- Health-check самой VPS (system, services, network).
- Audit прав доступа: `~/.openclaw/` каждого юзера должен быть `0o700`,
  `openclaw.json` — `0o600`. Если не так — auto-fix предлагается.
- Проверка что ни один Gateway не слушает на `0.0.0.0` без auth.
- Sanity check Tailscale auth-keys / срока действия.
- Sanity check Cloudflare token (TTL).
- `openclaw doctor` под каждым юзером с парсингом вывода.

### 6.6. (5) Сеть и фаервол

```
┌─ Сеть и фаервол ──────────────────────────────────────────────────┐
│                                                                   │
│  > 1. Tailscale (admin-only): статус, identity, IP, ACL           │
│    2. Cloudflare Tunnel (admin + публикация для юзеров): routes   │
│    3. UFW: правила, статистика, лог                               │
│    4. Открытые порты на хосте (ss/lsof)                           │
│    5. Тест связности (ping / curl публичных URL)                  │
│    6. Регенерировать DNS-record для wildcard                      │
│    7. Rotate Cloudflare credentials                               │
│    8. Назад                                                       │
│                                                                   │
└───────────────────────────────────────────────────────────────────┘
```

В разделе Tailscale явно помечено «admin-only»: TUI напоминает что
юзерам Tailscale не нужен, и любая попытка добавить юзера в Tailscale
identity не нужна для overlay-функционала.

В разделе Cloudflare показывает per-route:

- hostname / target / owner-юзер / last_seen / status (active/disabled);
- кнопка «удалить stale routes» (нет связанного плагина и не gateway-route);
- кнопка «протестировать route» (curl HTTPS-URL → проверить что
  cloudflared доходит до Gateway);
- live-tail логов cloudflared.

### 6.7. (6) Плагины и туннели (обзор по юзерам)

```
┌─ Плагины и туннели по юзерам ─────────────────────────────────────┐
│                                                                   │
│  alice:                                                           │
│    Public Control UI: https://gateway-alice.openclaw.example.com  │
│      └ last_seen: 2 мин назад                                     │
│    Плагины:                                                       │
│      ├─ telegram          [polling]   [—]                         │
│      ├─ anthropic         [outbound]  [—]                         │
│      ├─ google (gemini)   [outbound]  [—]                         │
│      ├─ gog-clawhub       [needs cb]  https://gog.alice.openc...  │
│      │                                last_seen: 4 дня назад      │
│      └─ msteams           [webhook ]  https://teams.alice.open... │
│                                       last_seen: 1 час назад      │
│                                                                   │
│  bob:                                                             │
│    Public Control UI: https://gateway-bob.openclaw.example.com    │
│      └ last_seen: 8 мин назад                                     │
│    Плагины:                                                       │
│      ├─ telegram          [polling]                               │
│      ├─ slack             [socket ]                               │
│      └─ openai            [outbound]                              │
│                                                                   │
│  > Назад                                                          │
│                                                                   │
└───────────────────────────────────────────────────────────────────┘
```

Этот экран — read-only мониторинг. Управление плагинами — это уже
обязанность юзера через стандартный `openclaw plugins ...`.

### 6.8. (7) Логи и мониторинг

- Per-user `journalctl --user -u openclaw-gateway --since today` под `su -`.
- `journalctl -u cloudflared`.
- `journalctl -u openclaw-overlay-api`.
- Live-tail с цветовой подсветкой error/warn (через bubbletea viewport).

### 6.9. (8) Аудит безопасности

- Запуск `openclaw security audit` под каждым юзером (стандартное
  OpenClaw, см. `docs/gateway/security/index.md:30-43`).
- Проверка прав FS на чувствительных файлах.
- Проверка что нет процессов под нашими юзерами с `0.0.0.0` listeners.
- Проверка что нет общих групп (`docker`) у наших openclaw-юзеров.
- Проверка что нет shared SSH ключей между юзерами.
- Отчёт с pass/warn/fail и рекомендациями.

### 6.10. (9) Diagnostic snapshot

- Собирает в один tar.gz: системные характеристики, логи overlay-API,
  состояние cloudflared, statе.db, output `openclaw doctor` каждого юзера
  (с redact-ом токенов).
- Полезно для раздачи в issue tracker / поддержку.
- **Бонус-фича**: предложить сгенерировать «снимок настроек» для
  быстрого восстановления overlay-конфигурации на другой VPS (без
  пользовательских данных).

### 6.11. (10) Удаление OpenClaw / overlay

```
┌─ Удаление ────────────────────────────────────────────────────────┐
│                                                                   │
│  Что удалить?                                                     │
│                                                                   │
│  Overlay (TUI, API, watchers):                                    │
│    [x] Удалить overlay полностью                                  │
│                                                                   │
│  OpenClaw:                                                        │
│    [x] Удалить openclaw глобально (npm uninstall -g)              │
│                                                                   │
│  Пользовательские данные (по галочке per user):                   │
│    [ ] alice  — удалить ~/.openclaw                               │
│    [ ] bob    — удалить ~/.openclaw                               │
│    [ ] carol  — удалить ~/.openclaw                               │
│    [x] Удалить всех Linux-юзеров (userdel -r)                     │
│                                                                   │
│  Бэкапы:                                                          │
│    [ ] Удалить /var/lib/openclaw-multi/backups/                   │
│                                                                   │
│  Системные сервисы (по отдельности):                              │
│    [x] Cloudflared                                                │
│    [ ] Tailscale (внимание: потеряете admin-доступ!)              │
│    [ ] UFW (вернуть к default state)                              │
│    [x] Sysctl/fstab hardening (откатить)                          │
│                                                                   │
│  [ Применить ]  [ Отмена ]                                        │
│                                                                   │
└───────────────────────────────────────────────────────────────────┘
```

Логика (в порядке):

1. **Дополнительная защита**: TUI требует ввод текста подтверждения
   («yes, удалить»), при включённом «удалить Tailscale» — двойное
   подтверждение.
2. Под каждым активным юзером — **обязательный** `openclaw backup
create` перед удалением (если `~/.openclaw` помечен к удалению), даже
   если юзер не запросил — кладётся в `/var/lib/openclaw-multi/backups/`
   на случай rollback'а.
3. Для каждого юзера, помеченного к полному удалению:
   `su - <user> -c "openclaw uninstall --all --yes --non-interactive"`
   (`docs/cli/uninstall.md`) → потом `userdel -r <user>` (если
   галочка «удалить Linux-юзеров»).
4. Удаление overlay-watcher units и overlay-доп.файлов в каждом
   `~/.openclaw-overlay/`.
5. Через overlay-API удалить все ingress routes → cloudflared SIGHUP.
6. Если выбрано удаление overlay:
   - `systemctl stop openclaw-overlay-api && disable && rm unit`;
   - `rm /usr/local/bin/openclaw-{multi,overlay-api,overlay-watcher,multi-bootstrap}`;
   - `rm -rf /opt/openclaw-multi /etc/openclaw-multi /var/lib/openclaw-multi /var/log/openclaw-multi`.
7. Если выбрано удаление openclaw глобально:
   - `npm uninstall -g openclaw`.
8. Если выбраны бэкапы — `rm -rf /var/lib/openclaw-multi/backups/`.
9. Cloudflared: `systemctl stop cloudflared && disable && apt remove
cloudflared && rm -rf /etc/cloudflared`.
10. Tailscale: `tailscale logout && systemctl stop tailscaled && apt
remove tailscale && rm -rf /var/lib/tailscale`. **С предупреждением
    о потере admin-доступа.**
11. UFW: применить snapshot из `/var/lib/openclaw-multi/snapshots/...`
    (или `ufw reset` + `ufw disable`).
12. Sysctl/fstab: откатить из snapshot'ов в
    `/var/lib/openclaw-multi/snapshots/...`.
13. Финальный отчёт: что удалено, что осталось, ссылки на бэкапы (если
    остались).

---

## 7. Overlay-API daemon (детали)

Это критический компонент, отдельно от TUI.

### 7.1. Endpoints (HTTP на UNIX-сокете `/run/openclaw-overlay.sock`)

| Method | Path                               | Описание                                                |
| ------ | ---------------------------------- | ------------------------------------------------------- |
| GET    | `/health`                          | живой ли                                                |
| GET    | `/users`                           | список зарегистрированных юзеров с портами              |
| POST   | `/users/<u>/gateway-route`         | завести/обновить персональный Control UI route юзера    |
| DELETE | `/users/<u>/gateway-route`         | удалить (при remove user)                               |
| POST   | `/users/<u>/routes`                | добавить плагин-callback ingress route                  |
| DELETE | `/users/<u>/routes/<id>`           | удалить route                                           |
| GET    | `/users/<u>/routes`                | список routes юзера (с last_seen из cf-логов)           |
| GET    | `/users/<u>/routes/<id>/last-seen` | timestamp последнего hit'а                              |
| POST   | `/users/<u>/disable`               | пометить все routes юзера как disabled (для deactivate) |
| POST   | `/users/<u>/enable`                | вернуть active (для activate)                           |
| POST   | `/cloudflared/reload`              | принудительный SIGHUP (для TUI)                         |
| GET    | `/audit?since=<ts>`                | tail audit log                                          |

Авторизация:

- Через `SO_PEERCRED` сокета: для `/users/<u>/...` запрос принимается
  только если UID запрашивающего совпадает с владельцем `<u>` **или**
  запрашивающий — root (TUI).

### 7.2. Алгоритм add-route

```
1. Принять { user, plugin_id, local_port, hostname_hint } от watcher.
2. Если local_port не указан — аллоцировать из user-pool.
3. Сгенерировать subdomain: <hostname_hint>.<user>.openclaw.<domain>.
4. Записать в state.db.
5. Регенерировать /etc/cloudflared/config.yml из шаблона + state.db.
6. Validate (`cloudflared tunnel ingress validate /etc/cloudflared/config.yml`).
7. SIGHUP cloudflared.
8. Wait for cloudflared health (до 3 сек).
9. Вернуть { url, port } watcher'у.
10. Audit log.
```

### 7.3. Failure modes и rollback

- Validate fail → не trigger SIGHUP, вернуть error в watcher.
- SIGHUP отправлен но cloudflared не recovered → откатить config.yml,
  ещё SIGHUP, alert в audit.
- Watcher не может достучаться до API → exponential backoff, retry.

### 7.4. Last-seen tracking

- `/users/<u>/routes/<id>/last-seen` парсит `journalctl -u cloudflared
--since 7days --output=json` и достаёт timestamp последнего HTTP-hit'а
  для конкретного hostname.
- Если cloudflared логи без access-info — возвращает `n/a` без ошибки.
- Кэшируется в памяти overlay-API на 60 сек чтобы не дёргать journalctl
  на каждый запрос TUI.

---

## 8. Per-user watcher (детали)

### 8.1. Что слушает

- `inotify` на `~/.openclaw/openclaw.json` (modify/move).
- Опционально: `inotify` на `~/.openclaw/extensions/` если плагины
  кладут что-то туда.

### 8.2. Алгоритм при изменении конфига

```
1. Прочитать новый openclaw.json.
2. Сравнить со снимком в ~/.openclaw-overlay/watcher.state.
3. Найти новые/удалённые plugins.entries.<id>.
4. Для каждого нового плагина:
   a. Прочитать его манифест в node_modules: needs.publicCallback?
   b. Если да — POST /users/<self>/routes с { plugin_id, callback_port }.
   c. Получить { url, port } от API.
   d. Выполнить:
      openclaw config set plugins.entries.<id>.config.callbackUrl=<url>
      openclaw config set plugins.entries.<id>.config.callbackPort=<port>
5. Для удалённых плагинов:
   a. DELETE /users/<self>/routes/<id>.
6. Обновить snapshot в watcher.state.
```

### 8.3. Контракт с плагином (опциональная оптимизация)

**Важно: контракт не обязательный.** Любой плагин из ClawHub работает в
overlay-сценарии без модификаций — overlay просто наблюдает за конфигом
юзера и проактивно публикует callback URL, когда видит что плагин его
запросил.

Что делает overlay-watcher для **обычного плагина (без контракта)**:

- читает `~/.openclaw/openclaw.json` после установки плагина;
- ищет в его манифесте/конфиге **известные паттерны** webhook/callback
  полей (`webhookUrl`, `callbackUrl`, `redirectUri`, `webhookPort`,
  `callbackPort`, и т. д. — overlay держит heuristic-список самых
  частых имён);
- если паттерн найден и значение пустое или указывает на `localhost` —
  публикует endpoint через cloudflared, прописывает результат
  через `openclaw config set`.

**Опциональный контракт** для авторов плагинов, желающих явно
сигнализировать overlay о своих требованиях, — пометить в
`openclaw.plugin.json`:

```json
{
  "openclaw": {
    "overlay": {
      "needsPublicCallback": true,
      "callbackPort": 18901,
      "callbackPath": "/oauth2callback",
      "configKey": "plugins.entries.<id>.config.callbackUrl"
    }
  }
}
```

Зачем нужно — это **облегчает встраивание**:

- overlay не угадывает поля по эвристикам, а читает декларативный
  манифест;
- автор плагина точно знает что URL придёт и куда;
- ноль ложных срабатываний (overlay не открывает route там где не надо).

Что не нужно:

- никаких изменений в openclaw `main` для этого контракта — это
  свободное extension-поле в `openclaw.plugin.json`, OpenClaw его
  игнорирует, overlay читает.
- отсутствие контракта не блокирует ничего: плагин работает по
  эвристике или (если эвристика не сработала) юзер сам прописывает
  публичный URL руками через стандартный `openclaw config set` —
  overlay при следующей итерации watcher'а заметит изменение и поднимет
  route, если адрес — наш cloudflared-домен.

Для встроенных каналов с известным webhook-flow (Telegram webhook,
Slack HTTP Request URL, MS Teams Bot Framework) overlay имеет
hard-coded шаблоны (subdomain naming, путь по умолчанию) и публикует
endpoint автоматически даже без всякого контракта.

---

## 9. Phases of implementation

### Phase 0. Skeleton (1 неделя)

- Бутстрап Go-проекта, бинарники с заглушками.
- Bubble Tea TUI shell с главным меню (заглушки на действия).
- Минимальный CI (golangci-lint, go test).
- Docker-окружение для smoke-тестов (Ubuntu 22.04 в контейнере).

### Phase 1. Bootstrap fresh install (2 недели)

- Pre-flight check, install dependencies (Tailscale, Cloudflared, Node)
  с **полной идемпотентностью** (см. §2.5).
- Шаблоны конфигов и unit-файлов с envsubst.
- TUI wizard «fresh install» end-to-end до момента «overlay готов, юзеров
  нет».
- Smoke-тест в Docker.

### Phase 2. User management — add/remove/activate/deactivate (1.5 недели)

- `bootstrap-user.sh` с интеграцией стандартного `openclaw onboard` и
  pre-set env vars для консистентного выбора в wizard.
- TUI экран users + операции add/remove/pause/resume.
- Создание персонального Control UI route через overlay-API.
- state.db с миграциями.
- Audit log infrastructure.

### Phase 3. Backup/restore (1 неделя)

- Использование встроенного `openclaw backup create / verify`
  (`docs/cli/backup.md`).
- Опциональное шифрование поверх (openssl AES).
- Восстановление с проверкой checksum.
- Cron-задача (через `systemctl timers`) для авто-бэкапов.

### Phase 4. Doctor / health check (1 неделя)

- System checks.
- Wrap `openclaw doctor` под каждым юзером с парсингом.
- Auto-fix для типичных проблем (chmod, umask).

### Phase 5. Network management (1 неделя)

- TUI экраны для Tailscale, Cloudflare, UFW.
- Тесты связности.
- Управление DNS-записями через Cloudflare API.
- Cloudflared logs parser для last_seen routes.

### Phase 6. Overlay-API daemon (1.5 недели)

- HTTP-сервер на UNIX-сокете с SO_PEERCRED.
- Endpoints: gateway-route, plugin routes, disable/enable, last-seen.
- Регенерация cloudflared config + SIGHUP.
- Тесты с реальным cloudflared в Docker.

### Phase 7. Per-user watcher (1.5 недели)

- inotify watcher на openclaw.json.
- Парсинг плагин-манифестов.
- Интеграционный тест: ставим mock-плагин, проверяем что URL появился.

### Phase 8. Аудит безопасности и monitoring (1 неделя)

- Security audit экран.
- Diagnostic snapshot.
- Live-tail логов в TUI.

### Phase 9. Production hardening (1.5 недели)

- Обработка edge cases (disk full, network down, partial state).
- Logrotate для audit.log.
- Backup encryption hardening.
- **Обязательные пост-апдейтные тесты OpenClaw с автоматическим
  откатом и уведомлением админа** (см. §6.3 шаг 5-6).
- Документация для операторов.

### Phase 10. Удаление OpenClaw / overlay (1 неделя)

- TUI экран uninstall с гранулярными опциями.
- Snapshot системных файлов перед изменениями (для rollback'а).
- Интеграция с `openclaw uninstall --all` под каждым юзером.
- Smoke-тесты «full install → full uninstall → проверка чистоты VPS».

### Phase 11. End-to-end QA (1 неделя)

- Полный сценарий: fresh install → add user → install plugin from ClawHub
  с tunnel → OAuth flow → cleanup.
- Cross-user isolation тесты (попытка alice прочитать файлы bob).
- Update OpenClaw → проверка что всё ещё работает + тест автоматического
  отката при поломке.

**Итого: ~13–14 недель (3.5 месяца) для одного fullstack-разработчика на
production-grade overlay.**

Чисто MVP без cloudflared-автоматики (Phase 6+7) и без uninstall (Phase 10) —
~7–8 недель.

---

## 10. Тестирование

### 10.1. Уровни тестов

- **Unit (Go test)** — логика TUI, парсеры, allocator портов.
- **Integration (Docker)** — реальный systemd, реальный openclaw,
  mock-cloudflared.
- **End-to-end (KVM/QEMU VM)** — полный сценарий fresh install в чистой
  Ubuntu VM.

### 10.2. Smoke-сценарии

1. Fresh install на чистой Ubuntu 22.04.
2. Fresh install на VPS, где Tailscale уже стоит и работает —
   идемпотентность.
3. Add user → onboard → старт Gateway → SSH disconnect → проверка что
   Gateway жив + публичный Control UI URL отвечает.
4. Update OpenClaw до новой версии → все юзеры всё ещё работают.
5. **Update OpenClaw до версии с несовместимой схемой → тесты падают →
   автоматический откат → уведомление админа → проверка что юзеры снова
   работают.**
6. Install plugin с tunnel → проверка появления HTTPS URL и что OAuth
   flow проходит до конца.
7. Backup → remove user → restore → юзер вернулся.
8. Cross-user isolation: попытка `cat /home/bob/.openclaw/credentials/...`
   из alice → permission denied.
9. Reboot VPS → все Gateway автостартуют благодаря linger.
10. Disable Tailscale → admin-доступ потерян, но Gateway-ы и публичные URL
    юзеров работают.
11. **Full install → full uninstall со всеми галочками → проверка что
    VPS чист (никаких openclaw процессов, файлов, маршрутов).**

### 10.3. Adversarial-grade тесты (Phase 11)

- Попытка ptrace процесса соседа → блокировано `ptrace_scope=2`.
- Попытка увидеть процессы соседа в `/proc` → скрыто `hidepid=2`.
- Memory exhaustion одним юзером → не валит других благодаря `MemoryMax`.
- Forkbomb → `TasksMax` ограничивает.
- Network DOS → `IPInputBandwidth` ограничивает.

---

## 11. Документация

### 11.1. Что должно быть готово к релизу

- `README.md` (quickstart на 1 страницу).
- `docs/install.md` (полный fresh install guide).
- `docs/users.md` (как добавлять/удалять/деактивировать юзеров).
- `docs/network.md` (Tailscale + Cloudflare setup, варианты A/B для CF).
- `docs/uninstall.md` (как полностью убрать).
- `docs/troubleshooting.md` (типичные проблемы).
- `docs/security.md` (модель угроз, hardening, что закрыто/что открыто).
- `docs/architecture.md` (для разработчиков overlay).
- `docs/upgrading-openclaw.md` (как обновлять OpenClaw без поломки overlay,
  что делать при автоматическом откате).
- `docs/multi-vps-future.md` (обзор паттернов multi-VPS установки —
  **только теория**, не реализовано в v1.0).
- `man/openclaw-multi.1` (man-page).

### 11.2. Distribution

- Один репозиторий `openclaw-multi` (начинаем работу в нем).
- Релизы — GitHub Releases с pre-built бинарниками для linux-amd64 и
  linux-arm64.
- Установка — `curl -fsSL https://.../install.sh | bash`.
- Опционально: deb/rpm-пакеты.

---

## 12. Open questions (нужно решить до Phase 1)

Решённые на этапе ревью пользователем (зафиксированы в плане):

- ~~Q1. Domain для wildcard CNAME~~ → quick-tunnel default + опция «свой домен».
- ~~Q2. Cloudflared без аккаунта~~ → основной путь с аккаунтом, quick tunnels только для временных тестов.
- ~~Q3. Multi-VPS~~ → только doc-раздел в v1.0, реализация v1.1.
- ~~Q4. Teams автоматизация~~ → стандартный flow OpenClaw, без специальной overlay-логики.
- ~~Q5. Web UI~~ → не делаем.
- ~~Q6. Динамическое закрытие routes по OAuth completion~~ → показываем
  last_seen, админ решает вручную.

Решённые на этапе Phase 0:

- **OQ-1. Формат audit-log entries** → **JSONL** с обязательными
  полями `ts` (ISO-8601 UTC), `actor` (Linux UID + username),
  `action` (короткий enum: `bootstrap_user`, `delete_user`,
  `update_openclaw`, `add_route`, `delete_route`, `enable_user`,
  `disable_user`, `backup_create`, `backup_restore`, `uninstall`,
  ...), `target` (имя ресурса: `<username>` / `<route-id>` /
  `<package-version>`), `result` (`ok` / `error` / `rolled_back`).
  Опциональные: `details` (произвольный JSON), `error_message`,
  `duration_ms`. Файл — `/var/log/openclaw-multi/audit.log`,
  ротация через logrotate, формат удобен для парсинга grep'ом и для
  будущей интеграции с SIEM/ELK.

- **OQ-2. Шифрование бэкапов** → **один master-key**, настраивается
  один раз при первом fresh install. Хранится в
  `/etc/openclaw-multi/master.key` (mode `0o600`, owner root). Все
  бэкапы шифруются им автоматически (AES-256-CBC + PBKDF2 через
  `openssl enc`). Восстановление не требует ввода пароля — TUI читает
  ключ сам. Опционально: TUI поддерживает `rotate-master-key`
  (расшифровать все существующие бэкапы старым ключом и зашифровать
  новым). Master-key обязательно входит в `Diagnostic snapshot`
  (§6.10) с предупреждением админу о его секретности.

- **OQ-3. Notifications для админа** → **Telegram-бот**. При первом
  fresh install TUI предлагает шаг «настроить нотификации» (опционально,
  можно пропустить). Админ создаёт собственного Telegram-бота через
  @BotFather, вводит token и chat_id в TUI. Overlay сохраняет в
  `/etc/openclaw-multi/notifications.yml` и шлёт сообщения через прямой
  outbound HTTP к `api.telegram.org` (никаких cloudflared-routes к
  самому overlay не нужно — это outbound из overlay-API, по UFW
  default allow outgoing проходит). Что нотифицируется по умолчанию:
  откат апдейта OpenClaw, падение overlay-API, истечение Cloudflare
  токена, истечение Tailscale auth-key, падение бэкап-таймера, любые
  CRITICAL-events из audit log.

---

## 13. Риски и mitigation

| Риск                                                 | Вероятность | Импакт    | Митигация                                                                                                                                                                                                                                                                                                                                                                                                                                                                                  |
| ---------------------------------------------------- | ----------- | --------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| **OpenClaw меняет JSON-схему конфига несовместимо**  | средняя     | большой   | **Обязательные автоматические тесты после tenant-scoped обновления OpenClaw**: `openclaw config validate` под каждым юзером + smoke `openclaw doctor` + тест публикации mock-плагина с tunnel-callback. **При падении любого теста — откат tenant package** и уведомление админа (TUI notification + audit log entry CRITICAL + опциональный email/Telegram alert). Сообщение: «требуется обновление overlay-слоя для совместимости с OpenClaw <new>». |
| Cloudflare меняет API/CLI cloudflared                | низкая      | средний   | использовать только стабильные команды, читать CHANGELOG перед апдейтом                                                                                                                                                                                                                                                                                                                                                                                                                    |
| Tailscale меняет CLI                                 | низкая      | средний   | то же                                                                                                                                                                                                                                                                                                                                                                                                                                                                                      |
| systemd-user поведение меняется в новой Ubuntu       | низкая      | средний   | тестировать на основных distro в CI                                                                                                                                                                                                                                                                                                                                                                                                                                                        |
| Overlay-API падает                                   | средняя     | большой   | systemd `Restart=always`, watcher exponential backoff                                                                                                                                                                                                                                                                                                                                                                                                                                      |
| Юзер случайно удаляет `~/.openclaw-overlay/`         | низкая      | малый     | watcher умеет восстановить из state.db                                                                                                                                                                                                                                                                                                                                                                                                                                                     |
| Конфликт portов с другими сервисами на хосте         | средняя     | малый     | pre-flight check, port allocator с retry                                                                                                                                                                                                                                                                                                                                                                                                                                                   |
| OOM на маленькой VPS из-за множества Gateway         | средняя     | большой   | TUI показывает ожидаемое потребление при add user, hard cap через `MemoryMax`                                                                                                                                                                                                                                                                                                                                                                                                              |
| Cloudflare / Tailscale аккаунт юзера случайно удалён | низкая      | большой   | snapshot настроек в `/var/lib/openclaw-multi/snapshots/` + audit log                                                                                                                                                                                                                                                                                                                                                                                                                       |
| Юзер случайно делает overlay-API доступным наружу    | низкая      | критичный | API только на UNIX-сокете, не TCP-порту; bind 127.0.0.1 явно запрещён в коде                                                                                                                                                                                                                                                                                                                                                                                                               |

---

## 14. Roadmap после v1.0

### v1.1

- **Multi-VPS управление**: общий control plane для нескольких VPS, общий
  Cloudflare-аккаунт, агрегированный мониторинг.

### v1.2

- Vault / SOPS для секретов (gateway tokens, cloudflare credentials).
- Rootless Podman опция как альтернатива Docker-sandbox (если когда-то
  понадобится контейнерная изоляция плагин-tools).

### v1.3

- Per-org policy engine: какие плагины из ClawHub разрешены / запрещены,
  какие модельные провайдеры можно подключать, лимиты на каналы.

### v2.0

- Federation: несколько VPS как один логический cluster.
- Auto-scaling (вынос юзеров на разные VPS).

**Намеренно не в roadmap** (по решению пользователя):

- Web UI (только TUI, всегда).
- Plugin marketplace mirror (используем оригинальный ClawHub).

---

## 15. Резюме

`openclaw-multi` — самостоятельная административная утилита (TUI на Go +
Bubble Tea), которая поверх стандартного OpenClaw автоматизирует
multi-user развёртывание на VPS с adversarial-grade изоляцией и
автоматическим управлением сетью:

- **Tailscale** — admin-only (SSH + overlay-API).
- **Cloudflare Tunnel** — публикует и персональные Control UI юзеров, и
  все callback/webhook эндпоинты плагинов из ClawHub.
- **UFW** — `default deny incoming`, никаких ручных правок.

Утилита не модифицирует OpenClaw core, использует встроенные команды
`openclaw backup create / verify` и `openclaw uninstall --all` вместо
самописных аналогов. Tenant-scoped апдейт OpenClaw сопровождается
обязательными пост-апдейтными тестами с откатом tenant package и
уведомлением админа при поломке схемы. Юзер видит обычный OpenClaw —
плагины и каналы настраиваются стандартным wizard'ом, overlay только под
капотом обеспечивает сетевую инфраструктуру.

Production-ready реализация — ~13–14 недель для одного
fullstack-разработчика; MVP без auto-cloudflared и uninstall — ~7–8
недель.

Следующий шаг: согласовать §12 (open questions OQ-1..OQ-3) и стартовать
Phase 0.
