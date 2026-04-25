# OpenClaw — Точечные ответы (VPS, Tailscale, linger, multi-user overlay)

> Дополнение к [OPENCLAW_OVERVIEW_RU.md](OPENCLAW_OVERVIEW_RU.md). Здесь —
> прямые ответы на три практических вопроса по эксплуатации на VPS.
> Все ссылки в формате `путь:строка` относятся к ветке `main`.

---

## Вопрос 1. Как обеспечить максимум функциональности при максимальной безопасности — без ручного вмешательства в порты

### 1.1. Что я упустил в предыдущих ответах

Расширения OpenClaw ставятся не только из встроенной папки `extensions/`.
В проекте есть **ClawHub** — публичный registry, через который агент ставит
любой кастомный плагин:

```bash
openclaw plugins install clawhub:<package>     # docs/tools/clawhub.md:31-49
```

Кастомные плагины могут декларировать **что им нужен публичный inbound**
(callback URL, webhook). Например, плагин `voice-call` (`extensions/voice-call/`)
имеет в манифесте `tunnel.provider` (`extensions/voice-call/openclaw.plugin.json:310-327`)
со встроенными значениями:

```text
"enum": ["none", "ngrok", "tailscale-serve", "tailscale-funnel"]
```

То есть OpenClaw уже из коробки умеет поднимать туннель для плагина,
которому он нужен. Это и есть точка интеграции, через которую overlay
автоматизирует всю историю с публичными портами.

### 1.2. Сводная карта inbound-потребностей

| Поток                                               | Кому нужно                                                            | Как организовать без ручной правки UFW                                                                        |
| --------------------------------------------------- | --------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------- |
| OAuth callback при первичной авторизации провайдера | Google/Gemini, GitHub Copilot, Microsoft, любой OAuth-плагин          | туннель (`cloudflared` / `ngrok` / `tailscale-funnel`) с маршрутом `oauth.openclaw.<domain> → localhost:8085` |
| Webhook от мессенджера                              | Telegram (если выбран webhook), Slack (HTTP Request URL), Google Chat | тот же туннель с `webhook-tg.<user>.openclaw.<domain> → localhost:<port>`                                     |
| Microsoft Teams Bot Framework                       | обязательно (Microsoft не умеет polling)                              | туннель с `teams.<user>.openclaw.<domain> → localhost:3978`                                                   |
| Push-notifications от внешних API                   | редкие случаи (Calendar push, Twilio voice events)                    | плагин декларирует `tunnel.provider`, overlay поднимает маршрут                                               |
| Кастомные ClawHub-плагины                           | любые, которые так сделаны                                            | автоматически через тот же `tunnel.provider` контракт                                                         |

Главная мысль: **ровно одна универсальная точка** — постоянно работающий
сервис-туннель, в который overlay автоматически добавляет маршруты по мере
установки плагинов. UFW при этом продолжает быть закрыт наглухо
(`default deny incoming`).

### 1.3. Стандартная архитектура «Tailscale + Cloudflare Tunnel»

Эти две технологии **ортогональны** и не конфликтуют — они закрывают разные
задачи:

| Слой                                  | Что закрывает                                                                                                      | Outbound-only?                                                                                             |
| ------------------------------------- | ------------------------------------------------------------------------------------------------------------------ | ---------------------------------------------------------------------------------------------------------- |
| **Tailscale** (`tailscaled`)          | приватный admin-доступ: Control UI Gateway, SSH, доступ из tailnet к локальным сервисам VPS                        | да — `tailscaled` сам поднимает UDP-канал к координатору                                                   |
| **Cloudflare Tunnel** (`cloudflared`) | публичные inbound-эндпоинты для плагинов и каналов: OAuth callback, webhook URL, Bot Framework, push notifications | да — `cloudflared` держит исходящий QUIC к Cloudflare, через который Cloudflare пробрасывает входящий HTTP |

Оба работают **через outbound-соединения наружу**, поэтому UFW в обоих
случаях ничего не открывает. Это позволяет VPS оставаться полностью
закрытым с точки зрения inbound (`default deny incoming`).

Конфигурация UFW для всего сценария:

```bash
sudo ufw default deny incoming
sudo ufw default allow outgoing
sudo ufw allow in 8080/tcp        # ваш сторонний сайт
sudo ufw enable
```

Никаких правил для Tailscale (`tailscaled` работает мимо UFW), никаких
правил для cloudflared (он только outbound). Никаких правил для OpenClaw
(всё inbound — через cloudflared, всё админ-inbound — через Tailscale).

### 1.4. Как `cloudflared` крепится к OpenClaw

Встроенной поддержки `cloudflared` в `enum` `tunnel.provider` нет —
официально OpenClaw умеет `ngrok`, `tailscale-serve`, `tailscale-funnel`.
Но это **не блокер**, потому что cloudflared с точки зрения плагина
выглядит как обычный reverse-proxy перед локальным портом:

- плагин слушает на `localhost:<port>`;
- cloudflared принимает HTTPS на `<hostname>` и пробрасывает на
  `http://localhost:<port>`;
- плагин видит `Host`-header — просто настраивается
  `webhookSecurity.allowedHosts: ["<hostname>"]`
  (`extensions/voice-call/openclaw.plugin.json:329-348`).

То есть с точки зрения OpenClaw cloudflared — это «трасту-нгрок» с другим
бэкендом. Никакой модификации main не требуется. Overlay управляет
конфигом `cloudflared`, а в `openclaw.json` плагина просто вписывает уже
готовый публичный URL.

### 1.5. Конфигурация: один cloudflared на VPS, wildcard DNS

`cloudflared` поднимается как **системный сервис** на VPS:

```ini
# /etc/systemd/system/cloudflared.service (создаёт overlay при инсталляции)
[Service]
ExecStart=/usr/local/bin/cloudflared tunnel --config /etc/cloudflared/config.yml run
Restart=on-failure
User=cloudflared
Group=cloudflared
```

Wildcard-DNS в Cloudflare:

```
*.openclaw.example.com    CNAME    <tunnel-id>.cfargotunnel.com
```

Конфигурация туннеля (управляется overlay-ом, в `/etc/cloudflared/config.yml`):

```yaml
tunnel: <tunnel-id>
credentials-file: /etc/cloudflared/<tunnel-id>.json

# Per-user, per-плагин routes — overlay добавляет/удаляет автоматически
ingress:
  - hostname: oauth.alice.openclaw.example.com
    service: http://127.0.0.1:18901
  - hostname: webhook-tg.alice.openclaw.example.com
    service: http://127.0.0.1:18902
  - hostname: teams.alice.openclaw.example.com
    service: http://127.0.0.1:18903
  - hostname: oauth.bob.openclaw.example.com
    service: http://127.0.0.1:19001
  # catch-all
  - service: http_status:404
```

Cloudflared умеет **hot-reload**: после того как overlay меняет config.yml,
ему отправляется `systemctl reload cloudflared` (либо `SIGHUP`), и новый
маршрут начинает работать без рестарта.

### 1.6. Как взаимодействуют Tailscale и Cloudflare Tunnel в одной системе

| Зона                                | Кто принимает                                       | Как работает                                                                     |
| ----------------------------------- | --------------------------------------------------- | -------------------------------------------------------------------------------- |
| Control UI Gateway (админка)        | **Tailscale** (Tailscale Serve или `bind: tailnet`) | админ заходит из своего ноутбука в tailnet, никаких public URLs                  |
| Webhook/callback URLs плагинов      | **Cloudflared**                                     | публичный HTTPS, доступен с любых внешних серверов (Google, Microsoft, Telegram) |
| SSH к VPS                           | **Tailscale**                                       | SSH через tailnet, public-22 закрыт                                              |
| Outbound: API моделей, мессенджеров | напрямую через сеть VPS                             | `default allow outgoing` в UFW, ничего настраивать не надо                       |

Ничего не дублируется и ничего не конфликтует. На уровне сетевого стека —
два отдельных tun/tap-интерфейса от двух разных демонов; OpenClaw обоих
видит просто как «приходит запрос на `localhost:<port>`».

### 1.7. Конкретный flow: бот устанавливает custom-плагин с ClawHub

Сценарий, который вы описали: вы пишете в Telegram «установи плагин
`gog-clawhub` для работы с Google API», агент сам всё делает, никакого
ручного вмешательства в права/порты.

Полный flow:

**Шаг 1.** Telegram-канал доставляет сообщение в Gateway. Агент по tool-call
(в OpenClaw агенту доступны системные tools, см.
`docs/tools/clawhub.md:31-49`) выполняет:

```text
plugins.install(spec="clawhub:gog-clawhub")
```

OpenClaw скачивает архив, валидирует `pluginApi` и `minGatewayVersion`,
ставит в active workspace (`docs/tools/clawhub.md:47-49`).

**Шаг 2.** В манифесте плагина (предположим он сделан по контракту, как
voice-call) есть:

```json
{
  "config": {
    "tunnel": { "provider": "tailscale-funnel" },
    "callback": { "port": 18901, "path": "/oauth2callback" }
  }
}
```

или просто (что вероятнее для custom-плагина):

```json
{
  "needs": { "publicCallback": true, "callbackPort": 18901 }
}
```

**Шаг 3.** **Overlay watcher** (постоянно работающий компонент overlay-я,
запущенный как `systemctl --user openclaw-overlay-watcher.service` для
каждого юзера) ловит событие установки плагина. Источник события — один
из:

- watch на `~/.openclaw/openclaw.json` (плагин при установке записывает
  туда свою секцию);
- post-install hook от `openclaw plugins install` (если такая возможность
  будет добавлена; на момент main её нет — overlay использует watch как
  fallback).

Watcher делает следующее:

1. Читает манифест плагина: «нужен publicCallback на порту 18901».
2. Аллоцирует уникальное subdomain имя:
   `<plugin-id>.<linux-username>.openclaw.<domain>`
   (например, `gog.alice.openclaw.example.com`).
3. Через короткий API (overlay-сервис на хосте, см. §1.8) добавляет в
   `/etc/cloudflared/config.yml` новый ingress entry и шлёт `SIGHUP`
   процессу cloudflared.
4. Записывает выданный публичный URL обратно в config плагина:
   ```bash
   openclaw config set \
     plugins.entries.gog-clawhub.config.callbackUrl \
     https://gog.alice.openclaw.example.com/oauth2callback
   ```

**Шаг 4.** Плагин запускает OAuth-flow: формирует authorization URL,
отдаёт его агенту, агент пишет юзеру в Telegram «открой эту ссылку».

**Шаг 5.** Юзер открывает ссылку с телефона/ноутбука. Google → редирект на
`https://gog.alice.openclaw.example.com/oauth2callback?code=...`.
Cloudflare принимает HTTPS, передаёт через cloudflared-туннель на
`localhost:18901`, плагин ловит code, обменивает на refresh-token,
сохраняет в `~/.openclaw/credentials/`.

**Шаг 6.** Плагин рапортует «готов», агент пишет в Telegram «плагин
установлен и подключён».

**Шаг 7. (cleanup, опционально).** Если плагин нужен только для outbound
(после авторизации public callback больше не нужен), overlay-watcher через
сутки/после первого refresh удаляет ingress route → cloudflared снова
reload. Если плагин постоянно слушает webhook (как Teams) — route
остаётся.

На каждом шаге **никаких ручных операций с UFW, портами, правами**.
Юзер только нажимает «авторизовать» в браузере у Google.

### 1.8. Что должен сделать overlay (минимальный набор компонентов)

| Компонент               | Где                                                                                         | Делает                                                                                                                             |
| ----------------------- | ------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------- |
| `cloudflared` сервис    | `/etc/systemd/system/cloudflared.service` (системный, не user)                              | один процесс на VPS, держит все маршруты для всех юзеров                                                                           |
| `cloudflared` config    | `/etc/cloudflared/config.yml`                                                               | обновляется overlay-API                                                                                                            |
| Overlay-API daemon      | `/usr/local/bin/openclaw-overlay-api` (HTTP на `127.0.0.1:18000`, доступен только локально) | принимает заявки `add-route`, `remove-route`, генерирует config.yml, шлёт SIGHUP                                                   |
| Per-user watcher        | `systemctl --user openclaw-overlay-watcher.service`                                         | следит за `~/.openclaw/openclaw.json`, при появлении плагина с `needs.publicCallback` дёргает overlay-API                          |
| Pre-allocated port pool | в overlay-API                                                                               | каждому юзеру выдан диапазон, например 18900-18999 для alice, 19000-19099 для bob; внутри диапазона аллоцируются порты под плагины |
| Wildcard DNS            | в Cloudflare DNS                                                                            | `*.openclaw.<domain>` → tunnel CNAME, настраивается **один раз** при установке overlay                                             |

В таком виде вся инфра «принимает всё что нужно для любого плагина из
ClawHub» становится **set-and-forget**.

### 1.9. Microsoft Teams в этой схеме

Точно так же, как любой другой плагин с inbound:

1. `openclaw plugins install @openclaw/msteams` (`docs/channels/msteams.md:387`).
2. Watcher видит «msteams нужен публичный HTTPS на порту 3978».
3. Overlay-API добавляет ingress
   `teams.alice.openclaw.example.com → localhost:3978`, `SIGHUP`
   cloudflared.
4. Watcher прописывает webhook URL в Azure Bot Framework (через
   Microsoft Graph API, если у юзера есть токен) **или** показывает
   юзеру в Telegram сообщение «зайдите в Azure Bot и впишите этот URL» —
   зависит от того, насколько глубоко overlay интегрируется с Azure.

Важно: для Teams есть ограничение, что Microsoft Bot Framework сам
дёргает webhook URL — поэтому endpoint должен быть **постоянно доступен**.
В отличие от OAuth callback, route для Teams не удаляется. Overlay просто
держит его в `config.yml` как «постоянный».

### 1.10. Резюме по вопросу 1

- Стандартная безопасная настройка для VPS: UFW `deny incoming / allow
outgoing` + ваш сторонний порт. **Никаких правил для OpenClaw в UFW.**
- Tailscale — для приватного admin-доступа.
- Cloudflare Tunnel — для всех публичных endpoint'ов плагинов и каналов
  (OAuth callback, webhook, Teams Bot Framework).
- Overlay управляет cloudflared-конфигом автоматически: при установке
  плагина с `needs.publicCallback` overlay добавляет ingress route, шлёт
  SIGHUP cloudflared, прописывает URL в плагин. Пользователь и агент в
  это не вмешиваются.
- Никаких ограничений функциональности, потому что любой плагин с любым
  webhook-требованием получает готовый HTTPS-URL без ручного открытия
  портов.

---

## Вопрос 2. Linger — короткое подтверждение

На VPS — держать включённым всегда. Без него Gateway убивается systemd
через ~5 секунд после `exit` из SSH. С linger переживает logout и
автоматически стартует при reboot. OpenClaw сам пробует включить через
`loginctl enable-linger $USER` в финале onboarding-визарда
(`src/wizard/setup.finalize.ts:92-102`,
`src/daemon/systemd-linger.ts`); если требуется sudo — спрашивает пароль.
Проверка: `loginctl show-user $USER -p Linger` или `openclaw doctor`
(`docs/gateway/doctor.md:442-445`).

---

## Вопрос 3. Multi-user overlay — полноценная изоляция, расширенная за счёт cloudflared

### 3.1. Где была неточность в моих предыдущих формулировках

В первом отчёте я написал что overlay «подходит только для дружественной
команды» и «не является multi-tenant security boundary». Это было
неаккуратно. Перечитав security-доку (`docs/gateway/security/index.md:22`):

> «one user/trust boundary per gateway (prefer **one OS user**/host/VPS
> per boundary)»

…security-модель OpenClaw прямо **рекомендует** разделение по Linux-юзерам
как способ изоляции. Adversarial-сценарий, против которого предупреждает
doc, — это **shared Gateway** (один процесс на всех юзеров). Если у
каждого Linux-юзера свой Gateway-процесс (а overlay делает именно это) —
никакого «shared» нет, и trust boundary разделены так же, как разделяются
учётки в любой UNIX-системе.

То есть overlay через UID-разделение — это **не компромисс**, это
«официальный способ» в терминологии самой OpenClaw.

### 3.2. Что даёт overlay в плане изоляции (стандартный UNIX hardening)

| Ресурс                    | Механизм                                                                |
| ------------------------- | ----------------------------------------------------------------------- |
| Файлы `~/.openclaw/`      | UID-владелец + `chmod 700` + `umask 0077`                               |
| Lock-каталог Gateway      | `/tmp/openclaw-<uid>` per-uid (`src/config/paths.ts:220-225`)           |
| Процессы Gateway          | разные UIDs + `systemctl --user`                                        |
| Видимость чужих процессов | `/proc hidepid=2`, `kernel.yama.ptrace_scope=2`                         |
| Память/CPU/IO             | systemd cgroup-квоты в unit-файле (`MemoryMax`, `CPUQuota`, `IOWeight`) |
| Сеть                      | Per-user порты (gap ≥ 20 для browser/CDP)                               |
| Cloudflared routes        | каждому юзеру свой набор subdomain'ов под `<user>.openclaw.<domain>`    |
| Tailscale identity        | каждому юзеру свой Tailscale-аккаунт или ACL-tag                        |

Это эквивалент изоляции в shared-VPS у любого хостера.

### 3.3. Что overlay реально делает при `bootstrap <username>`

```text
1. useradd -m -s /bin/bash <username>             # системный
2. loginctl enable-linger <username>              # systemd --user живёт после logout
3. su - <username>:
     umask 0077
     mkdir -p ~/.openclaw && chmod 700 ~/.openclaw
     mkdir -p ~/.openclaw/workspace
     # шаблон конфига (PORT, TOKEN, TAILSCALE_MODE, OVERLAY_API_URL)
     envsubst < /opt/openclaw-multi/templates/openclaw.json.tmpl > ~/.openclaw/openclaw.json
     chmod 600 ~/.openclaw/openclaw.json
     # systemd unit с hardening
     envsubst < /opt/openclaw-multi/templates/openclaw-gateway.service.tmpl \
         > ~/.config/systemd/user/openclaw-gateway.service
     # watcher для динамических tunnel-маршрутов
     envsubst < /opt/openclaw-multi/templates/openclaw-overlay-watcher.service.tmpl \
         > ~/.config/systemd/user/openclaw-overlay-watcher.service
     systemctl --user daemon-reload
     systemctl --user enable --now openclaw-gateway.service
     systemctl --user enable --now openclaw-overlay-watcher.service
4. pre-allocate port range для cloudflared (например 18900-18999 для alice)
5. вписать port range в overlay-API (locally on 127.0.0.1:18000)
```

После этого:

- `alice` имеет рабочий Gateway на `127.0.0.1:18900` (или на её
  Tailscale IP) с собственным токеном;
- любой плагин, который ставится из ClawHub в `alice` workspace и просит
  публичный callback, получает route `<plugin>.alice.openclaw.<domain>`
  автоматически;
- никакие файлы и процессы `alice` не видны `bob`.

### 3.4. Hardening-блок в systemd-unit

Overlay прописывает в шаблон unit:

```ini
[Service]
PrivateTmp=yes
ProtectSystem=strict
ProtectHome=tmpfs
NoNewPrivileges=yes
RestrictAddressFamilies=AF_UNIX AF_INET AF_INET6
LockPersonality=yes
MemoryDenyWriteExecute=yes
MemoryMax=2G
CPUQuota=200%
TasksMax=512
IPAccounting=yes
```

`ProtectHome=tmpfs` означает: gateway-процесс не видит ни одного `/home/...`
кроме своего собственного (`ReadWritePaths=/home/<username>`). Это режет
любую попытку «прочитать `/home/bob/.openclaw/credentials/oauth.json`» из
кода плагина в `alice`.

### 3.5. Sysctl-hardening для VPS

Overlay при инсталляции записывает (`/etc/sysctl.d/openclaw-overlay.conf`):

```
kernel.yama.ptrace_scope=2     # запрет cross-user ptrace
kernel.kptr_restrict=2         # скрыть kernel pointers
kernel.dmesg_restrict=1        # dmesg только root
fs.protected_hardlinks=1
fs.protected_symlinks=1
```

И `/etc/fstab`:

```
proc /proc proc defaults,hidepid=2,gid=<adm-group> 0 0
```

### 3.6. Один реальный footgun: classic Docker sandbox

Если активировать sandbox через `OPENCLAW_SANDBOX=1` в multi-user
сценарии (`docker-compose.yml:16-23`), все юзеры попадают в группу
`docker`, что фактически даёт root каждому из них (через `docker run -v /:/host`).

Решение в overlay: **отказаться от Docker-sandbox** и заменить на:

- **rootless Podman** per-user (`podman --user`) — нет общей привилегированной
  группы;
- либо **bubblewrap / firejail** для изоляции tool-вызовов на уровне OS
  без контейнерной плоскости.

Overlay-инсталлятор по умолчанию выключает `OPENCLAW_SANDBOX=1` и
конфигурирует rootless Podman.

### 3.7. Архитектура overlay в полном виде

| Компонент                | Где живёт                                                               | Зачем                                                       |
| ------------------------ | ----------------------------------------------------------------------- | ----------------------------------------------------------- |
| Глобальный `openclaw`    | `/usr/local/bin/openclaw` (`npm i -g`)                                  | один бинарь и один `node_modules` на всю VPS                |
| Wrapper `openclaw-multi` | `/usr/local/bin/openclaw-multi` (bash)                                  | подкоманды `bootstrap`, `remove`, `list`, `update`, `audit` |
| Bootstrap-скрипт         | `/opt/openclaw-multi/bootstrap-user.sh`                                 | создание юзера, конфиг, units, права                        |
| Шаблоны конфига и units  | `/opt/openclaw-multi/templates/`                                        | parameterized envsubst                                      |
| Overlay-API daemon       | `/usr/local/bin/openclaw-overlay-api` (системный, на `127.0.0.1:18000`) | управление cloudflared config, port allocation, audit log   |
| Per-user watcher         | `~/.config/systemd/user/openclaw-overlay-watcher.service`               | следит за плагинами, заявляет ingress routes                |
| `cloudflared`            | системный сервис                                                        | один на всю VPS, hot-reload через SIGHUP                    |
| Wildcard DNS             | в Cloudflare                                                            | `*.openclaw.<domain>` → tunnel CNAME                        |
| Hardening sysctl         | `/etc/sysctl.d/openclaw-overlay.conf`                                   | ptrace_scope, kptr_restrict, etc                            |
| Hardening fstab          | `/etc/fstab`                                                            | `proc hidepid=2`                                            |
| Common compile cache     | `/var/cache/openclaw-compile` (`chmod 1777`)                            | bytecode-кэш Node, общий для всех                           |
| Per-user state           | `~/.openclaw/` каждого юзера, `0o700`                                   | конфиг, креды, agents, workspace                            |

### 3.8. Что переживает обновление главного пакета

```bash
sudo npm install -g openclaw@latest    # обновляет общий бинарь
openclaw-multi update                  # пробегает по юзерам, openclaw doctor для каждого
```

`openclaw-multi update` под каждым юзером:

1. `openclaw doctor` — проверяет что сервис здоров;
2. `openclaw config validate` — конфиг валиден против новой схемы;
3. перегенерирует unit-файл из шаблона (если шаблон обновился);
4. `systemctl --user daemon-reload && restart`.

Per-user конфиги в `~/.openclaw/` остаются нетронутыми.
`/opt/openclaw-multi/` — отдельная сущность, версионируется через
overlay-репо.

Точки риска (что может потребовать adapt overlay):

| Риск                                                                 | Как мониторить                     | Митигация                                                |
| -------------------------------------------------------------------- | ---------------------------------- | -------------------------------------------------------- |
| Изменение env vars (`src/cli/profile.ts`, `src/config/paths.ts`)     | smoke-test после апдейта           | `openclaw doctor` для каждого юзера                      |
| Breaking changes в JSON-схеме конфига                                | `openclaw config validate`         | держать конфиг минимальным, использовать стабильные поля |
| Изменение пути `~/.openclaw`                                         | следить за CHANGELOG               | overlay умеет миграцию (rename + symlink)                |
| Появление официальной поддержки cloudflared в `tunnel.provider` enum | хорошая новость — упростит overlay | подменить на родной `tunnel.provider: cloudflared`       |
| Поломка `--non-interactive` onboard                                  | smoke-test                         | держать last-known-good версию пакета как fallback       |

### 3.9. Что НЕ закроет даже идеальный overlay

- **Ядерные уязвимости** — local privilege escalation; помогает только
  отдельный хост.
- **Side-channel атаки** (Meltdown/Spectre) — современный микрокод и патчи
  ядра.
- **Disk I/O contention** — `IOWeight=` в systemd-unit, но не абсолютная
  гарантия.
- **Сетевая полоса** — `IPInputBandwidth=`/`IPOutputBandwidth=` в
  systemd-unit, ограниченная гарантия.

Эти ограничения — общие для любой shared Linux-системы, не специфика
OpenClaw. Для compliance (PCI/HIPAA) нужен отдельный хост; для всех
остальных задач overlay даёт нормальную изоляцию shared-VPS.

### 3.10. Оценка трудоёмкости (с учётом cloudflared)

| Компонент                                            | Чел.-дни       | Что входит                                                                                    |
| ---------------------------------------------------- | -------------- | --------------------------------------------------------------------------------------------- |
| Wrapper `openclaw-multi` (bash)                      | 1–2            | подкоманды, port allocator, error handling                                                    |
| Bootstrap + non-interactive onboard wiring           | 3–4            | основная сложность — стабильный `onboard --non-interactive` под `su -`                        |
| Шаблоны unit с hardening-директивами                 | 1–2            | PrivateTmp/ProtectHome/cgroup quotas                                                          |
| Sysctl/fstab hardening                               | 1              | `/etc/sysctl.d/`, `/etc/fstab`                                                                |
| Cloudflared сервис + wildcard DNS bootstrap          | 2              | install cloudflared, login, tunnel create, DNS-CNAME                                          |
| **Overlay-API daemon** (manage cloudflared config)   | 3–4            | HTTP daemon, port allocator, config writer, SIGHUP handler                                    |
| **Per-user watcher** (детект новых плагинов)         | 3–4            | watch на `~/.openclaw/openclaw.json`, парсинг `needs.publicCallback`, обращение к overlay-API |
| Замена Docker sandbox (rootless Podman / bubblewrap) | 2–3            | per-user runtime, тесты что tools работают                                                    |
| Документация для админа                              | 2              | README + troubleshooting + audit-чеклист                                                      |
| Smoke-тесты (изолированный VM)                       | 3–4            | bootstrap → старт → установка плагина с tunnel → попытка cross-user атаки → teardown          |
| Production hardening (логи, ротация, бэкап)          | 2–3            | journal-логи, rotation `~/.openclaw`, миграция при upgrade                                    |
| **MVP (без cloudflared автоматики)**                 | **10–15 дней** | базовая изоляция, ручной cloudflared конфиг                                                   |
| **Full с cloudflared-автоматикой**                   | **20–25 дней** | + overlay-API + watcher + интеграционные тесты                                                |
| **Adversarial-grade с аудитом**                      | **30–40 дней** | + полный hardening, аудит, тесты атак                                                         |

### 3.11. Резюме по вопросу 3

- Overlay через нескольких Linux-юзеров — **полноценная изоляция**,
  не «только для доверенных». Security-доки OpenClaw это явно
  поддерживают.
- Стандартный UNIX hardening (UID/GID, FS permissions, systemd cgroups,
  hidepid, ptrace_scope, отдельные `--user` юниты, уникальные порты,
  отключённый Docker-sandbox в пользу rootless Podman) даёт изоляцию,
  эквивалентную shared-VPS у хостера.
- Overlay интегрирует cloudflared как системный сервис с wildcard DNS,
  per-user watcher автоматически добавляет ingress routes для плагинов,
  декларирующих `needs.publicCallback`. Никакого ручного UFW или открытия
  портов — UFW остаётся `default deny incoming`.
- Overlay живёт **полностью вне** `node_modules` и репозитория OpenClaw.
  `npm i -g openclaw@latest` его не ломает. Pre-allocated port pools и
  overlay-API контролируют всю динамику.
- Трудоёмкость: 20–25 дней на full с cloudflared-автоматикой; 30–40 на
  adversarial-grade с аудитом.

---

## Краткое TL;DR

1. **Tailscale + UFW + расширения с inbound.** Стандартная схема:
   `ufw default deny incoming / allow outgoing`. Tailscale закрывает
   приватный admin-доступ к Gateway. Cloudflare Tunnel (системный
   `cloudflared`-сервис с wildcard DNS) принимает все публичные inbound
   для плагинов и каналов (OAuth callback, webhook URL, Microsoft Teams).
   Оба работают через outbound, в UFW открывать наружу ничего не нужно.
   Overlay управляет cloudflared-конфигом автоматически: установка нового
   плагина из ClawHub запускает watcher → overlay-API → ingress route →
   SIGHUP cloudflared → URL вписывается в config плагина. Ручного
   вмешательства в порты/права нет.

2. **Linger.** На VPS включать всегда; OpenClaw сам пробует.

3. **Multi-user overlay.** Полноценная adversarial-grade изоляция через
   стандартный UNIX hardening (UIDs, permissions, systemd cgroups,
   hidepid, ptrace_scope, отдельные `--user` юниты, отключённый
   Docker-sandbox в пользу rootless Podman). Cloudflared интегрирован как
   часть overlay для автоматического publishing inbound-эндпоинтов
   плагинов. Overlay не ломается при `npm i -g openclaw@latest`.
   MVP с cloudflared-автоматикой — 20–25 чел.-дней; adversarial-grade с
   аудитом — 30–40.
