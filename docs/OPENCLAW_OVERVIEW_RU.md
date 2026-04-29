# OpenClaw — Обзор архитектуры, режимов работы и сценариев VPS-развёртывания

> Аналитический отчёт по ветке `main` репозитория `openclaw/openclaw`.
> Все ссылки даны в формате `путь:строка` относительно корня репозитория.

---

## 1. Что такое OpenClaw

**OpenClaw** — это персональный AI-ассистент, который запускается на собственных
устройствах пользователя и подключается к мессенджерам, через которые человек
уже общается. Цель проекта — «личный, локальный, всегда-онлайн» ассистент, а не
multi-tenant SaaS.

Ключевые свойства из `README.md:21-34`:

- **Управляющая плоскость** — единственный долгоживущий процесс **Gateway**
  (Node.js), который владеет состоянием каналов, агентов и сессий.
- **Поддерживаемые каналы (≈ 24)**: WhatsApp, Telegram, Slack, Discord, Google
  Chat, Signal, iMessage, BlueBubbles, IRC, Microsoft Teams, Matrix, Feishu,
  LINE, Mattermost, Nextcloud Talk, Nostr, Synology Chat, Tlon, Twitch, Zalo,
  Zalo Personal, WeChat, QQ, WebChat.
- **Узлы (nodes)** — клиенты на macOS / iOS / Android / headless подключаются к
  Gateway по WebSocket и предоставляют локальные возможности (`screen`,
  `camera`, `system.run`).
- **Control UI (Canvas)** — браузерная админка, отдаётся тем же HTTP-сервером
  Gateway на том же порту:
  - `/__openclaw__/canvas/`
  - `/__openclaw__/a2ui/`
    (`docs/gateway/network-model.md:21-24`).
- **Trust-модель** — _single trusted operator boundary_ per Gateway.
  Прямая цитата из `docs/gateway/security/index.md:9-15`:
  «OpenClaw is **not** a hostile multi-tenant security boundary for multiple
  adversarial users sharing one agent/gateway».

### Где живёт состояние

Единый каталог `~/.openclaw/` (override через `OPENCLAW_STATE_DIR`,
см. `src/config/paths.ts:60-89`):

| Подпуть                                     | Назначение                                    |
| ------------------------------------------- | --------------------------------------------- |
| `openclaw.json`                             | главный конфиг (JSON5, валидируется по схеме) |
| `credentials/oauth.json`                    | OAuth-токены (`src/config/paths.ts:236-252`)  |
| `agents/<agentId>/agent/auth-profiles.json` | model-auth для конкретного агента             |
| `workspace/`                                | рабочее пространство агента (default)         |
| `memory/` (если используется)               | SQLite-базы памяти                            |

Конфиг — JSON5, описание схемы — `docs/gateway/configuration.md:11-69`.
Gateway watch'ит файл и применяет правки горячо.

---

## 2. Архитектура управляющей плоскости

| Компонент      | Где                                      | Что делает                                            |
| -------------- | ---------------------------------------- | ----------------------------------------------------- |
| **Gateway**    | Node.js процесс на хосте/в Docker        | WS+HTTP control plane, владеет каналами и агентами    |
| **CLI**        | `openclaw` команда                       | управление конфигом, onboarding, диагностика          |
| **Control UI** | `/__openclaw__/canvas/` на порту Gateway | веб-админка, отдаётся самим Gateway                   |
| **Nodes**      | клиенты (Mac/iOS/Android/headless)       | подключаются к WS Gateway, дают локальные возможности |
| **Channels**   | подключаемые мессенджеры                 | живут внутри Gateway-процесса                         |
| **Agents**     | сущности с моделью + workspace           | один Gateway может держать несколько агентов          |

Базовые команды OpenClaw:

- `openclaw onboard` — мастер первичной настройки.
- `openclaw gateway --port 18789 --bind <mode>` — запуск Gateway.

Overlay onboarding для managed tenant:

- `/home/<user>/.local/bin/openclaw onboard --non-interactive --install-daemon`
  — единственный onboarding path OpenClaw Multi.

---

## 3. Режимы работы Gateway (bind modes)

Канонический выбор сделан в `src/gateway/net.ts:287-373`
(функция `resolveGatewayBindHost`). По дефолту:

- На обычном Linux-хосте — `loopback`.
- В контейнере (Docker / Podman / K8s) — `auto` → `0.0.0.0`.
- Если активна Tailscale Serve — всегда `loopback` (Tailscale Serve
  архитектурно требует loopback-bind).

| Режим      | Bind-host                                | Use-case                 | Подходит ли для публичного VPS                                          |
| ---------- | ---------------------------------------- | ------------------------ | ----------------------------------------------------------------------- |
| `loopback` | `127.0.0.1` (+ `[::1]`)                  | дефолт, локальный доступ | ✅ через SSH-туннель / Tailscale Serve                                  |
| `lan`      | `0.0.0.0`                                | вся локальная сеть       | ⚠️ публично без TLS = опасно (plaintext `ws://`, токен в открытом виде) |
| `tailnet`  | Tailscale IPv4 (CGNAT `100.x.x.x`)       | приватный VPN Tailscale  | ✅ безопасно (трафик в L3-туннеле)                                      |
| `auto`     | `0.0.0.0` в контейнере, иначе `loopback` | авто-выбор для контейнера | контекстно — то же что выбранный фактический режим                      |

Дополнительно — **надстройки поверх bind**:

- **Tailscale Serve** (`docs/gateway/tailscale.md:11-46`) — Gateway остаётся на
  loopback, Tailscale-демон даёт `https://<magicdns>/` на tailnet,
  плюс identity-headers (`tailscale-user-login`). Если установлено
  `gateway.auth.allowTailscale: true`, для пользователей tailnet auth
  происходит без токена/пароля — по верифицированной идентичности от
  `tailscale whois`.
- **Tailscale Funnel** — публичный HTTPS, требует обязательного
  `gateway.auth.password` (явно описано в `docs/gateway/tailscale.md:83-95`).
- **Trusted-proxy** — identity-aware reverse-proxy
  (`gateway.auth.mode: trusted-proxy`); Gateway доверяет identity-заголовкам от
  nginx/caddy перед собой.

### Порты по умолчанию

- Gateway: **`18789`** (`src/config/paths.ts:214`,
  `export const DEFAULT_GATEWAY_PORT = 18789`).
- Browser control: `gateway.port + 2` (loopback only).
- CDP-диапазон браузера: `controlPort + 9 .. + 108`
  (`docs/gateway/multiple-gateways.md:131-139`).
- Bridge legacy (если включён): `${OPENCLAW_BRIDGE_PORT:-18790}`
  (`docker-compose.yml:25-26`).
- Override порта Gateway: `OPENCLAW_GATEWAY_PORT` или CLI-флаг `--port`
  (`src/config/paths.ts:285-301`).

### Аутентификация

Из `docs/gateway/tailscale.md:23-46` и `docs/gateway/configuration.md`:

| Режим auth             | Как настраивается                                                                       | Когда уместен                                 |
| ---------------------- | --------------------------------------------------------------------------------------- | --------------------------------------------- |
| `none`                 | дефолт только при private ingress (loopback / SSH tunnel)                               | localhost / SSH-only                          |
| `token`                | `OPENCLAW_GATEWAY_TOKEN` или конфиг; Gateway сам генерирует случайный при первом старте | базовый сценарий, любая non-loopback ситуация |
| `password`             | `OPENCLAW_GATEWAY_PASSWORD`                                                             | shared-secret (особенно для Funnel)           |
| `trusted-proxy`        | identity-headers от reverse-proxy                                                       | nginx/caddy с OIDC/SSO                        |
| `allowTailscale: true` | поверх Tailscale Serve                                                                  | tailnet identity без отдельного секрета       |

### Опасный break-glass — `OPENCLAW_ALLOW_INSECURE_PRIVATE_WS`

Из `src/commands/onboard-remote.ts:37-42`:

> «Break-glass: OPENCLAW_ALLOW_INSECURE_PRIVATE_WS=1 for trusted private
> networks.»

Этот флаг разрешает `ws://` (plaintext) к приватным сетям. Допустим только в
доверенной LAN или dev-режиме. **На публичном VPS включать категорически
нельзя** — даже если Gateway за `0.0.0.0`, токен полетит по сети открытым
текстом.

---

## 4. Сценарии развёртывания на VPS

| Сценарий                      | Bind       | Auth                            | Транспорт клиента                     | Защита                 | Когда выбирать                         |
| ----------------------------- | ---------- | ------------------------------- | ------------------------------------- | ---------------------- | -------------------------------------- |
| **SSH tunnel**                | `loopback` | `token`                         | `ws://127.0.0.1:18789` через `ssh -L` | SSH-ключи              | базовый, минимальная поверхность атаки |
| **Tailscale Serve**           | `loopback` | `password` или `allowTailscale` | `https://<magicdns>/`                 | TS + auth              | команда уже на Tailscale               |
| **Tailscale Funnel**          | `loopback` | `password` обязательно          | `https://<funnel>`                    | TS + password          | публичный HTTPS без своего домена      |
| **Tailnet bind**              | `tailnet`  | `token`                         | `ws://100.x.x.x:18789`                | VPN + token            | прямое соединение с tailnet-узлов      |
| **Reverse proxy nginx/caddy** | `loopback` | `trusted-proxy`                 | `https://<domain>/`                   | TLS + identity headers | свой домен + SSO/OIDC                  |
| **Прямой `lan` без TLS**      | `lan`      | `token`                         | `ws://VPS:18789`                      | ❌ ничем               | антипаттерн, не использовать           |

Источники сценариев: `docs/install/hetzner.md` (SSH tunnel + Docker),
`docs/install/exe-dev.md`, `docs/install/fly.md`, `docs/gateway/remote.md`,
`docs/gateway/tailscale.md`, `docs/gateway/network-model.md`.

### Эталон от Hetzner-гайда

`docs/install/hetzner.md:189-204` явно показывает безопасный публикуемый порт:

```yaml
ports:
  # Recommended: keep the Gateway loopback-only on the VPS; access via SSH tunnel.
  # To expose it publicly, remove the `127.0.0.1:` prefix and firewall accordingly.
  - "127.0.0.1:${OPENCLAW_GATEWAY_PORT}:18789"
```

Доступ оттуда:

```bash
ssh -N -L 18789:127.0.0.1:18789 root@YOUR_VPS_IP
# затем в браузере: http://127.0.0.1:18789/
```

То есть Gateway внутри контейнера слушает `0.0.0.0:18789`, но Docker публикует
порт **только на loopback хоста**, а пользователь добирается через SSH.

---

## 5. Docker-стек на VPS

Содержимое `docker-compose.yml`:

- Сервис `openclaw-gateway` — образ `${OPENCLAW_IMAGE:-openclaw:local}`,
  слушает `--bind ${OPENCLAW_GATEWAY_BIND:-lan}` на порту `18789`
  (`docker-compose.yml:30-38`). Дефолт bind — `lan` именно для контейнера
  (внутри namespace это безопасно, наружу решает публикация порта).
- Сервис `openclaw-cli` — тот же образ, но `network_mode: "service:openclaw-gateway"`
  (общая сеть), `cap_drop: NET_RAW, NET_ADMIN`,
  `security_opt: no-new-privileges:true` (`docker-compose.yml:52-78`).
- Persist-маунты:
  - `${OPENCLAW_CONFIG_DIR}` → `/home/node/.openclaw`
  - `${OPENCLAW_WORKSPACE_DIR}` → `/home/node/.openclaw/workspace`
- Healthcheck: `fetch('http://127.0.0.1:18789/healthz')`
  (`docker-compose.yml:39-50`).

### Sandbox (опциональный)

`docker-compose.yml:16-23` оставляет закомментированными строки для DooD-доступа:

```yaml
# - /var/run/docker.sock:/var/run/docker.sock
# group_add:
#   - "${DOCKER_GID:-999}"
```

Sandbox активируется через `OPENCLAW_SANDBOX=1` в `scripts/docker/setup.sh`,
требует пробрасывать docker-сокет и группу `docker` хоста. Это даёт агентам
запускать инструменты в изолированных контейнерах через DooD.

### Переменные `.env` (по `docs/install/hetzner.md:132-156`)

```env
OPENCLAW_IMAGE=openclaw:latest
OPENCLAW_GATEWAY_TOKEN=          # пусто → Gateway сгенерирует сам
OPENCLAW_GATEWAY_BIND=lan        # рекомендуется в Docker
OPENCLAW_GATEWAY_PORT=18789
OPENCLAW_CONFIG_DIR=/root/.openclaw
OPENCLAW_WORKSPACE_DIR=/root/.openclaw/workspace
GOG_KEYRING_PASSWORD=            # сгенерировать openssl rand -hex 32
XDG_CONFIG_HOME=/home/node/.openclaw
```

---

## 6. Несколько Linux-юзеров на одном VPS — что получится

Это ответ на вопрос #2 пользователя. Разбираем сценарий: на одной VPS заведены
системные пользователи `user1`, `user2`, `user3`, и каждый ставит OpenClaw
независимо (без какого-либо общего стека).

### 6.1 Естественная изоляция через `$HOME`

В `src/config/paths.ts:51-89` функция `resolveStateDir` явно идёт от
`os.homedir()` пользователя:

```text
OPENCLAW_STATE_DIR (env)  >  ~/.openclaw  >  legacy ~/.clawdbot
```

Каждый Linux-юзер автоматически получает **свой** `~/.openclaw`. Никаких общих
файлов в `/etc` или `/var` OpenClaw не создаёт. Каталоги-результаты:

| Подпуть                                                 | Что внутри                               | Сегрегация |
| ------------------------------------------------------- | ---------------------------------------- | ---------- |
| `~/.openclaw/openclaw.json`                             | конфиг (модели, каналы, агенты, плагины) | per-user   |
| `~/.openclaw/credentials/oauth.json`                    | OAuth-токены каналов и моделей           | per-user   |
| `~/.openclaw/agents/<agentId>/agent/auth-profiles.json` | API-ключи моделей                        | per-user   |
| `~/.openclaw/workspace/`                                | рабочие файлы агента                     | per-user   |
| `~/.openclaw/memory/*.sqlite` (если есть)               | память агентов                           | per-user   |

### 6.2 Lock-каталог per-uid

`src/config/paths.ts:220-225`:

```ts
export function resolveGatewayLockDir(tmpdir = os.tmpdir): string {
  const base = tmpdir();
  const uid = typeof process.getuid === "function" ? process.getuid() : undefined;
  const suffix = uid != null ? `openclaw-${uid}` : "openclaw";
  return path.join(base, suffix);
}
```

То есть `/tmp/openclaw-1001` для UID 1001, `/tmp/openclaw-1002` для UID 1002 —
**автоматическая изоляция блокировок Gateway** между Linux-юзерами. Один юзер
не сможет случайно «потеснить» Gateway другого.

### 6.3 Конфликты по ресурсам

Это места, где «всё само не разделится» и нужна ручная настройка:

| Ресурс                            | Проблема                                                                                        | Решение                                                                                                                                                          |
| --------------------------------- | ----------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| **Порт `18789`**                  | при дефолте все юзеры будут пытаться слушать один порт                                          | каждому задать свой `OPENCLAW_GATEWAY_PORT` (например 18789, 18809, 18829 — gap ≥ 20)                                                                            |
| **CDP-порты браузера**            | `controlPort = base + 2`, CDP-диапазон `+9..+108` (`docs/gateway/multiple-gateways.md:131-139`) | gap между базовыми портами ≥ 20, иначе пересечение                                                                                                               |
| **Группа `docker`**               | если sandbox включён, оба юзера должны быть в группе `docker`                                   | известный footgun: участник группы `docker` фактически имеет root через `docker run -v /:/host`. Для adversarial-юзеров — недопустимо                            |
| **Глобальные сервисы**            | systemd / Tailscale / launchd зачастую глобальны                                                | использовать `systemctl --user` (`docs/vps.md:84-116`) — каждому юзеру отдельный user-юнит                                                                       |
| **Параллельная установка пакета** | общий системный OpenClaw CLI смешивает tenants и ломает staged rollout                          | OpenClaw CLI ставится только в tenant user-space через `nvm`; system-space overlay не требует global `openclaw`                                                 |

### 6.4 Защита на уровне файловой системы

В коде проекта **нет** явного `chmod 700` на `~/.openclaw` или `umask` setup —
полагается на дефолт ОС. На большинстве дистрибутивов дефолт `umask 0022` →
файлы создаются как `0o644`, **читаются другими локальными юзерами**.

Что это значит на VPS с несколькими юзерами:

- `user2` сможет прочитать `~user1/.openclaw/credentials/oauth.json`,
  если каталог `~user1` имеет execute-бит для `other`.
- На большинстве дистрибутивов home-каталоги создаются с `0o755` или `0o700`
  (зависит от `useradd` дистрибутива и `/etc/login.defs:HOME_MODE`).

**Ручная защита** (рекомендация для каждого юзера):

```bash
umask 0077                                 # для всех новых файлов
chmod 700 ~/.openclaw                       # запрет execute другим
chmod 600 ~/.openclaw/openclaw.json
chmod 600 ~/.openclaw/credentials/oauth.json
chmod -R go-rwx ~/.openclaw/agents          # на всякий случай
```

Это критично, потому что в `oauth.json` и `auth-profiles.json` лежат живые
токены к WhatsApp / Telegram / OpenAI / Anthropic и т. д.

### 6.5 Память и ресурсы

Из `docs/vps.md:65-116`:

- Один Gateway-процесс в idle ≈ **100–300 MB RSS**, под нагрузкой
  (активная сессия с моделью) — до **400–500 MB**.
- `node_modules` глобального пакета ≈ **500–800 MB** на пользователя при
  per-user prefix; при системной установке `/usr/lib/node_modules` — общий.
- **Рекомендуется общий compile-cache**:

  ```bash
  export NODE_COMPILE_CACHE=/var/tmp/openclaw-compile-cache
  mkdir -p /var/tmp/openclaw-compile-cache
  export OPENCLAW_NO_RESPAWN=1
  ```

  Каталог можно сделать с `0o1777` (sticky), тогда все юзеры будут писать туда
  свои bytecode-кэши, не мешая друг другу.

Грубый sizing для VPS:

| RAM VPS | Сколько одновременных Gateway комфортно |
| ------- | --------------------------------------- |
| 1 GB    | 1 (тесно)                               |
| 2 GB    | 2–3                                     |
| 4 GB    | 4–5                                     |
| 8 GB    | 6–8 (зависит от активности и моделей)   |

### 6.6 Уникальность настроек на пользователя

Полностью уникальные на юзера:

- Список и конфигурация **каналов** (Telegram-бот, WhatsApp-аккаунт, Slack workspace и т. д.).
- **Модели** и провайдеры (свои API-ключи).
- **Агенты** (`agents.list[]` в конфиге, каждый со своим `agentId`,
  workspace-каталогом, моделью, сессионной политикой).
- **Плагины** — конфигурация каждого юзера (`plugins.entries`), хотя сам код
  bundled-плагинов лежит в общем `node_modules`.
- **OAuth-сессии**, **автоматизация** (cron, hooks), **разрешения**.

Общее (если пакет ставится глобально):

- Бинарник `openclaw` и `node_modules`.
- Compile-cache (если общий, для скорости старта).
- Образ Docker `openclaw:local` (если используется Docker-стек).

### 6.7 Альтернатива: один OS-юзер + `--profile`

Если адверсарной изоляции не нужно, но требуется несколько Gateway —
существует встроенный механизм профилей
(`docs/gateway/multiple-gateways.md:80-117`):

```bash
openclaw --profile main onboard
openclaw --profile main gateway --port 18789

openclaw --profile ops onboard
openclaw --profile ops gateway --port 19789
```

Каждый профиль получает изолированно:

- `OPENCLAW_CONFIG_PATH` (отдельный конфиг)
- `OPENCLAW_STATE_DIR` (state, креды, кэши)
- `agents.defaults.workspace` (workspace)
- свой базовый порт (плюс производные browser/CDP)

Это **не заменяет multi-user изоляцию** для adversarial-сценариев — security-доки
прямо говорят (`docs/gateway/security/index.md:14-15`):

> «If adversarial-user isolation is required, split by trust boundary
> (separate gateway + credentials, and ideally separate OS users/hosts).»

### 6.8 Итог по multi-user

Технически OpenClaw устанавливается несколько раз под разных Linux-юзеров
без специальной настройки и **естественно изолируется через `$HOME`**.
Изоляция данных — адекватная для команды доверенных коллег. Но требует ручной
настройки от администратора:

1. Уникальные `OPENCLAW_GATEWAY_PORT` (с gap ≥ 20 для browser/CDP).
2. `umask 0077` + `chmod 700 ~/.openclaw` под каждым юзером.
3. Отдельные `systemctl --user` юниты (а не один system-юнит).
4. Если sandbox/Docker — понимать footgun про общую группу `docker`.
5. Бэкап делать **по каталогам каждого юзера** отдельно (нет единой точки).

Это **не** «multi-tenant security boundary» в смысле взаимно недоверчивых
пользователей — для такого требуется отдельный VPS / VM на каждого.

---

## 7. Сравнительная таблица сетевых режимов на VPS

| Критерий             | `loopback` + SSH         | Tailscale Serve                 | Tailscale Funnel                            | `tailnet` bind                              | nginx + trusted-proxy      | `lan` без TLS  |
| -------------------- | ------------------------ | ------------------------------- | ------------------------------------------- | ------------------------------------------- | -------------------------- | -------------- |
| Public-доступ        | ❌ только через SSH      | ❌ только tailnet               | ✅ публичный HTTPS                          | ❌ только tailnet                           | ✅ публичный HTTPS         | ✅ открыто     |
| TLS                  | SSH-канал                | ✅ TS терминирует               | ✅ TS терминирует                           | ❌ только token в `ws://`                   | ✅ proxy терминирует       | ❌             |
| Установка            | минимум                  | средне (TS на VPS+клиенте)      | средне                                      | средне                                      | сложнее (proxy + TLS-серт) | минимум        |
| Identity             | по SSH-ключу             | TS identity (`tailscale whois`) | password                                    | token                                       | reverse-proxy headers      | token          |
| Утечка токена в сети | нет                      | нет                             | нет                                         | да (если `ws://`)                           | нет                        | **да**         |
| Подходит для VPS     | ✅ базовый рекомендуемый | ✅ если есть TS                 | ✅ для публичного доступа без своего домена | ⚠️ только если tailnet полностью доверенный | ✅ при своём домене и SSO  | ❌ антипаттерн |

---

## 8. Чек-лист рекомендаций для VPS

### Базовый, безопасный сценарий

1. Один Linux-юзер на trust-границу (1 человек = 1 пользователь = 1 Gateway).
2. Gateway `bind: loopback`, порт публикуется только на `127.0.0.1` хоста.
3. Доступ — `ssh -L 18789:127.0.0.1:18789 user@vps`.
4. `gateway.auth.mode: token`, `OPENCLAW_GATEWAY_TOKEN` сгенерирован Gateway-ом
   при первом старте, лежит только в `~/.openclaw/openclaw.json`.
5. `OPENCLAW_ALLOW_INSECURE_PRIVATE_WS` — никогда не выставлен.
6. Бэкап `~/.openclaw` (включая `credentials/` и `agents/`) — регулярно
   (`docs/install/hetzner.md:33-37`).

### Если нужна команда на tailnet

1. Установить Tailscale на VPS и на клиенты.
2. `gateway.bind: loopback`, `gateway.tailscale.mode: serve`,
   `gateway.auth.allowTailscale: true`.
3. Открывать `https://<magicdns>/` — auth по Tailscale identity без отдельного
   секрета.
4. `gateway.auth.allowTailscale: false` если на хосте может работать
   недоверенный локальный код.

### Если нужно несколько Linux-юзеров на одном VPS

1. Каждому — свой `OPENCLAW_GATEWAY_PORT` (gap ≥ 20).
2. `umask 0077` в `~/.bashrc`, `chmod 700 ~/.openclaw` после установки.
3. Отдельные `systemctl --user enable openclaw-gateway`.
4. Общий `NODE_COMPILE_CACHE=/var/tmp/openclaw-compile-cache` (sticky-bit
   каталог) — для скорости.
5. Если планируется sandbox — понимать что общая группа `docker` снимает FS-изоляцию;
   для adversarial-юзеров вместо общей VPS лучше **отдельные VM/VPS**.
6. Бэкапы — отдельно по каталогу каждого юзера.

### Чего точно не делать

- ❌ Bind `lan` или `0.0.0.0` без TLS на публичном VPS.
- ❌ `OPENCLAW_ALLOW_INSECURE_PRIVATE_WS=1` в production.
- ❌ Один общий Gateway для нескольких людей с правами модификации
  `~/.openclaw` — это автоматически делает их «trusted operators»
  (`docs/gateway/security/index.md:55-72`).
- ❌ Расшаривать workspace (`~/.openclaw/workspace`) между юзерами.

---

## 9. Ключевые исходные файлы и документы

Если потребуется углубиться, основные точки входа:

- `README.md:21-120` — описание продукта и quick-start.
- `docs/vps.md:1-117` — VPS-tuning и провайдер-пикер.
- `docs/install/hetzner.md:1-263` — пример полного развёртывания на VPS.
- `docs/install/index.md` — индекс провайдеров (DigitalOcean, Hetzner, Fly,
  Oracle, GCP, Azure, exe.dev и др.).
- `docs/gateway/network-model.md:1-26` — bind-модель.
- `docs/gateway/multiple-gateways.md:1-175` — паттерн нескольких Gateway на
  одном хосте.
- `docs/gateway/tailscale.md:1-100` — Tailscale Serve / Funnel / bind tailnet.
- `docs/gateway/security/index.md:9-72` — trust model.
- `docs/gateway/configuration.md:11-69` — конфиг-обзор.
- `src/config/paths.ts:50-302` — резолвинг state-dir, port, lock-dir, OAuth-dir.
- `src/gateway/net.ts:287-373` — `resolveGatewayBindHost`.
- `src/commands/onboard-remote.ts:37-42` — `OPENCLAW_ALLOW_INSECURE_PRIVATE_WS`.
- `docker-compose.yml:1-79` — официальный Docker-стек.
- `scripts/docker/setup.sh` — sandbox-флаги и сборка локального образа.

---

## 10. Краткие ответы на вопросы пользователя

### Q1. Какие режимы работы и как они применимы к VPS?

OpenClaw поддерживает 4 режима bind-а Gateway: `loopback`, `lan`, `tailnet`,
`auto`, плюс две надстройки — Tailscale Serve (приватный HTTPS) и Tailscale
Funnel (публичный HTTPS), плюс trusted-proxy для своего reverse-proxy. На
публичном VPS правильный выбор — **`loopback` + один из защищённых
транспортов**: SSH-туннель (минимум), Tailscale Serve (рекомендация для
команды), Tailscale Funnel или nginx/caddy с TLS (если нужен публичный URL).
Прямой `lan`-bind без TLS — антипаттерн, токен и трафик уйдут в открытом виде.
Дефолтный порт — `18789` (override `OPENCLAW_GATEWAY_PORT`).

### Q2. Можно ли поставить OpenClaw нескольким Linux-юзерам на один VPS?

Да, технически. **Изоляция данных** работает естественно через `$HOME`: каждый
Linux-юзер получает свой `~/.openclaw/` с конфигом, OAuth-токенами,
agents/workspace, плюс per-uid lock-каталог `/tmp/openclaw-<uid>`. **Уникальные
настройки** (каналы, модели, плагины, агенты) — автоматически per-user.
**Память** — каждый Gateway ≈ 100–300 MB RSS в idle.

Однако три вещи **не разделяются автоматически** и требуют ручной настройки:

1. **Порт `18789`** — общий по умолчанию; нужно задать разные
   `OPENCLAW_GATEWAY_PORT` (gap ≥ 20 для browser/CDP).
2. **Права на `~/.openclaw`** — дефолтный umask `0022` оставляет файлы
   читаемыми соседями; нужно `umask 0077` и/или `chmod 700`.
3. **Группа `docker`** (если используется sandbox) — общая, что снимает
   FS-изоляцию между юзерами.

OpenClaw официально не позиционируется как multi-tenant security boundary для
adversarial-юзеров: для такого сценария security-документация рекомендует
**отдельные VM/VPS на каждого**. Для команды доверенных коллег схема рабочая
при перечисленных выше ручных мерах.
