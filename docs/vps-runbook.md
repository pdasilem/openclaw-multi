# VPS Runbook: деплой, запуск, логи, мониторинг

Документ описывает ручной owner-run процесс для реальной VPS: как развернуть
OpenClaw Multi, как запустить сервисы, как создать tenant, как пройти OpenClaw
onboarding внутри tenant, и какие логи/снимки состояния собирать.

Для самих проверок использовать [`docs/e2e-use-cases.md`](e2e-use-cases.md).

## Контекст VPS

- ОС: Ubuntu 24.
- SSH доступ есть через Tailscale.
- Админский пользователь на VPS: `ubuntu`.
- Root-доступ: passwordless `sudo`; root shell через `sudo -i`.
- `tailscaled` используется только для admin-доступа к VPS.
- UFW сохраняет public inbound закрытым; SSH должен оставаться доступен через
  tailnet/Tailscale interface. Текущий валидный layout: `ALLOW IN` на весь
  `tailscale0`, без отдельного `22/tcp` правила.
- Multi-tenant модель: один OpenClaw tenant = один Linux user на VPS.
- `openclaw-multi` владеет lifecycle managed Linux user: create, activate,
  pause, remove.
- `openclaw onboard` не создает tenant. Он выполняется позже внутри уже
  созданного Linux пользователя.

Роли в runbook:

- `ubuntu` — SSH admin и владелец checkout `/home/ubuntu/openclaw-multi`.
- `root` — системная установка overlay, запись `/usr/local/bin`, `/etc`,
  `/var/lib`, `/var/log`, systemd units, cloudflared config, создание Linux
  users.
- `<username>` — tenant Linux user, который появляется только после
  `User management -> Add user`. До создания tenant переключаться некуда.

Подключиться к VPS как admin:

```bash
ssh ubuntu@<vps-host>
```

Перейти в root shell нужно только для ручных команд, которые меняют системное
состояние вне TUI:

```bash
sudo -i
```

Соглашение:

- checkout и сборка выполняются как `ubuntu`;
- установка бинарников и системные операции выполняются как `root`;
- `openclaw-multi` запускать как `ubuntu` без `sudo`;
- root-права внутри TUI используются только точечно через явные `sudo`
  команды;
- если sudo попросит пароль, вводить его во встроенном terminal panel;
- не запускать interactive TUI через `sudo openclaw-multi` или из `sudo -i`;
- внутрь tenant переключаться только после создания user:
  `su - <username>`;
- в примерах `<username>` заменить на реально созданного tenant, например
  `alice`.

## Перед запуском

Перед этим runbook выполнить [`docs/install.md`](install.md).

Проверить готовность:

```bash
command -v openclaw-multi
command -v openclaw-overlay-api
command -v openclaw-overlay-watcher
command -v cloudflared
test -d /home/ubuntu/openclaw-multi
test -f /etc/cloudflared/<tunnel_id>.json
systemctl is-active tailscaled
tailscale status
sudo -i
ufw status verbose
exit
```

Если любой пункт не проходит, вернуться в [`docs/install.md`](install.md).

## Конфиг OpenClaw Multi

Основной конфиг:

```text
/etc/openclaw-multi/config.yml
```

Поля, которые нужны для публичной публикации routes:

```yaml
domain: example.com
subdomain: openclaw
tunnel_name: <tunnel-name>
tunnel_id: <cloudflare-tunnel-id>
tunnel_mode: account
cloudflare_zone_id: <zone-id>
cloudflare_api_token: <token-with-dns-edit>
cloudflared_credentials_file: /etc/cloudflared/<tunnel_id>.json
port_range_start: 18789
port_range_step: 20
node_version_min: "24"
openclaw_update_source: "pdasilem/openclaw:latest"
openclaw_update_command: "npm install --global github:pdasilem/openclaw#latest"
notifications:
  telegram_token: ""
  telegram_chat_id: ""
```

Правила:

- `cloudflared_credentials_file` можно не указывать, тогда overlay-API выводит
  путь как `/etc/cloudflared/<tunnel_id>.json`.
- Перед публикацией overlay-API проверяет, что credentials file существует.
- `cloudflare_api_token` нельзя прикладывать к логам без редактирования.

Проверка credentials file:

```bash
test -f /etc/cloudflared/<tunnel_id>.json
ls -l /etc/cloudflared/<tunnel_id>.json
```

## Установка с нуля

Запустить OpenClaw Multi как admin `ubuntu` без `sudo`.
Fresh install и add-user внутри TUI получают root-права только для конкретных
системных операций через явные `sudo` команды: запись `/etc`, systemd units,
`useradd`, `chown`, cloudflared config. Если sudo запросит пароль, prompt
появится во встроенном terminal panel.

```bash
ssh ubuntu@<vps-host>
openclaw-multi
```

Выполнить:

```text
1. Fresh install
```

После мастера проверить:

```bash
systemctl is-active tailscaled cloudflared openclaw-overlay-api
systemctl status cloudflared openclaw-overlay-api --no-pager
test -S /run/openclaw-overlay.sock
ls -l /run/openclaw-overlay.sock
test -f /etc/openclaw-multi/config.yml
test -f /var/lib/openclaw-multi/state.db
test -f /var/log/openclaw-multi/audit.log
```

## Systemd-сервисы

System unit overlay-API:

```bash
sudo -i
install -m 0644 /home/ubuntu/openclaw-multi/templates/openclaw-overlay-api.service.tmpl \
  /etc/systemd/system/openclaw-overlay-api.service
systemctl daemon-reload
systemctl enable --now openclaw-overlay-api
exit
```

Текущий unit запускает:

```text
/usr/local/bin/openclaw-overlay-api \
  --config /etc/openclaw-multi/config.yml \
  --state /var/lib/openclaw-multi/state.db \
  --socket /run/openclaw-overlay.sock \
  --cloudflared-config /etc/cloudflared/config.yml \
  --audit-log /var/log/openclaw-multi/audit.log
```

User units создаются для каждого managed Linux user:

```text
openclaw-gateway.service
openclaw-overlay-watcher.service
```

## Создание managed Linux user

Tenant создается не через OpenClaw onboarding, а через OpenClaw Multi.

В TUI:

```text
3. User management -> Add user
```

Ожидаемый flow из `docs/users.md`:

1. validate username;
2. validate `domain` and `subdomain`;
3. allocate gateway port from `18789 + n*20`;
4. generate gateway token automatically;
5. run `useradd -m -s /bin/bash <username>`;
6. run `loginctl enable-linger <username>`;
7. run non-interactive OpenClaw onboarding under `su - <username>`;
8. pass `OPENCLAW_GATEWAY_PORT`, `OPENCLAW_GATEWAY_TOKEN`, and
   `OPENCLAW_GATEWAY_BIND=loopback`;
9. harden `~/.openclaw` as `0700` and `~/.openclaw/openclaw.json` as `0600`;
10. write per-user watcher unit;
11. publish gateway route through overlay-API;
12. record user and gateway route in `state.db`;
13. emit `bootstrap_user` and route publication audit events;
14. show admin the public gateway URL and generated token.

Проверить Linux boundary:

```bash
id <username>
getent passwd <username>
loginctl show-user <username> -p Linger
su - <username>
pwd
id
node --version
test -x ~/.local/bin/openclaw
~/.local/bin/openclaw doctor
exit
```

Ожидаемо:

- `<username>` существует как реальный Linux user.
- Home directory: `/home/<username>`.
- `su - <username>` переключает в boundary этого tenant.
- Для active user включен linger, чтобы user services жили после logout/reboot.

## OpenClaw onboarding внутри tenant

После создания Linux user OpenClaw Multi запускает настоящий non-interactive
OpenClaw onboarding внутри этого пользователя.

Важно:

- не запускать onboarding от root;
- не запускать onboarding от admin `ubuntu`, если настраивается tenant;
- запускать только non-interactive onboarding через `su - <username>` из root
  shell;
- считать tenant готовым только после успешного `openclaw doctor`.

Onboarding настраивает:

- model provider и auth;
- workspace и bootstrap files;
- gateway mode/bind/auth;
- channels;
- daemon/user service, если выбран daemon install.

Принятые решения для onboarding:

- `--install-daemon` обязателен: gateway должен жить как `systemd --user`
  сервис и переживать SSH logout/reboot через linger.
- `--non-interactive` обязателен: это единственный onboarding path overlay.
- Gateway bind mode: `loopback`.
- Gateway auth mode: `token`.
- Gateway token генерирует OpenClaw Multi и передает через
  `OPENCLAW_GATEWAY_TOKEN`.
- Gateway port передается через `OPENCLAW_GATEWAY_PORT`.
- Gateway bind передается через `OPENCLAW_GATEWAY_BIND=loopback`.
- Tailscale для managed users выключен; публичный вход идет через cloudflared.
- Provider/auth/channels задаются через non-interactive OpenClaw onboarding
  внутри tenant; overlay не меняет OpenClaw core.
- OpenClaw CLI устанавливается и обновляется из `pdasilem/openclaw:latest`.
- Команда обновления задается настройкой `openclaw_update_command`.

OpenClaw Multi должен автоматически подготовить tenant runtime до onboarding:

```bash
su - <username>
test -x ~/.local/bin/openclaw
test -x ~/.local/bin/openclaw-gateway-start
~/.local/bin/openclaw --version
exit
```

Команда onboarding:

```bash
su - <username>
export OPENCLAW_GATEWAY_TOKEN='<gateway-token-generated-by-openclaw-multi>'
export OPENCLAW_GATEWAY_PORT='<gateway-port-allocated-by-openclaw-multi>'
~/.local/bin/openclaw onboard --non-interactive \
  --mode local \
  --auth-choice skip \
  --gateway-bind loopback \
  --gateway-auth token \
  --gateway-token-ref-env OPENCLAW_GATEWAY_TOKEN \
  --gateway-port "$OPENCLAW_GATEWAY_PORT" \
  --install-daemon \
  --accept-risk
~/.local/bin/openclaw doctor
exit
```

Ожидаемый config:

```text
/home/<username>/.openclaw/openclaw.json
```

Собрать evidence после onboarding:

```bash
su - <username>
node --version
~/.local/bin/openclaw --version
~/.local/bin/openclaw doctor
test -f ~/.openclaw/openclaw.json
sed -n '1,220p' ~/.openclaw/openclaw.json
find ~/.config/systemd/user -maxdepth 1 -type f -name '*openclaw*' -print
systemctl --user list-units '*openclaw*' --no-pager
exit
```

После успешного onboarding TUI/OpenClaw Multi должен вернуться в overlay flow:

1. применить `chmod 700 ~/.openclaw` и `chmod 600 ~/.openclaw/openclaw.json`;
2. записать `openclaw-overlay-watcher.service`;
3. включить watcher через `systemctl --user enable --now`;
4. создать route `gateway-<username>.<subdomain>.<domain>` через overlay-API;
5. записать route/user состояние в `state.db`;
6. записать audit events;
7. показать админу public Control UI URL и gateway token.

Перед передачей evidence редактировать:

- provider keys;
- gateway tokens;
- OAuth material;
- channel credentials;
- personal account identifiers.

## Запуск user services

Для active managed user:

```bash
loginctl show-user <username> -p Linger
su - <username>
systemctl --user daemon-reload
systemctl --user enable --now openclaw-gateway
systemctl --user enable --now openclaw-overlay-watcher
exit
```

Проверка:

```bash
su - <username>
systemctl --user status openclaw-gateway --no-pager
systemctl --user status openclaw-overlay-watcher --no-pager
exit
```

Rendered gateway unit должен запускать tenant wrapper:

```text
ExecStart=/home/<username>/.local/bin/openclaw-gateway-start
```

System-space OpenClaw binary не является допустимым путем для tenant gateway.

## Публикация cloudflared

Проверить rendered config:

```bash
cloudflared tunnel ingress validate --config /etc/cloudflared/config.yml
sed -n '1,220p' /etc/cloudflared/config.yml
```

Проверить процесс:

```bash
systemctl show cloudflared -p MainPID --value
cloudflared --version
journalctl -u cloudflared --no-pager -n 300
```

Phase 6 использует `SIGHUP` для reload. В E2E нужно подтвердить, что
установленная версия `cloudflared` реально перечитывает локальный ingress
config:

```bash
CLOUDFLARED_PID="$(systemctl show cloudflared -p MainPID --value)"
kill -HUP "$CLOUDFLARED_PID"
journalctl -u cloudflared --no-pager -n 100
```

Ожидаемо:

- `cloudflared` остается active;
- новый route начинает обслуживаться без полного `systemctl restart`.

## Логи

System services:

```bash
journalctl -u openclaw-overlay-api --no-pager -n 300
journalctl -u cloudflared --no-pager -n 300
journalctl -u tailscaled --no-pager -n 300
journalctl -u openclaw-overlay-api -f
journalctl -u cloudflared -f
```

Per-user services:

```bash
su - <username>
journalctl --user -u openclaw-gateway --no-pager -n 300
journalctl --user -u openclaw-overlay-watcher --no-pager -n 300
journalctl --user -u openclaw-gateway -f
journalctl --user -u openclaw-overlay-watcher -f
exit
```

OpenClaw Multi audit log:

```bash
tail -n 300 /var/log/openclaw-multi/audit.log
tail -f /var/log/openclaw-multi/audit.log
```

Watcher state:

```bash
su - <username>
cat ~/.openclaw-overlay/watcher.state
sed -n '1,220p' ~/.openclaw/openclaw.json
exit
```

## Снимок мониторинга

Снимать до и после каждого E2E прохода:

```bash
date -Is
hostnamectl
uptime
free -h
df -h
systemctl list-units 'openclaw*' 'cloudflared*' 'tailscaled*' --no-pager
systemctl is-active tailscaled cloudflared openclaw-overlay-api
ss -ltnup
tailscale status
ufw status verbose
cloudflared --version
cloudflared tunnel ingress validate --config /etc/cloudflared/config.yml
```

Route state:

```bash
sqlite3 /var/lib/openclaw-multi/state.db \
  "select id, username, kind, plugin_id, local_port, hostname, enabled, last_seen_value from routes order by username, kind, plugin_id, hostname;"
```

User service state:

```bash
loginctl show-user <username> -p Linger
su - <username>
systemctl --user status openclaw-gateway --no-pager
systemctl --user status openclaw-overlay-watcher --no-pager
exit
```

## Порядок E2E

Source of truth: [`docs/e2e-use-cases.md`](e2e-use-cases.md).

Первый VPS проход:

1. `UC-0001: Build and Run Binaries`
2. `UC-0101: Fresh Install on Clean VPS`
3. `UC-0102: Idempotent Fresh Install Rerun`
4. `UC-0201: Add Managed User and Gateway State`
5. `UC-0601: Publish Gateway Route and Reload cloudflared with SIGHUP`
6. `UC-0602: SO_PEERCRED Authorization`
7. `UC-0603: Daemon-Derived Plugin Route IDs`
8. `UC-0604: Rollback on Invalid cloudflared Publication`
9. `UC-0605: Sign-Up Flow Publishes Gateway Route`
10. `UC-0701: Watcher Publishes Callback Route`
11. `UC-0702: Watcher Updates Existing Callback Route`
12. `UC-0703: Watcher Removes Stale Callback Route`
13. `UC-0704: Watcher Survives overlay-API Restart`

Для каждого failed case сохранить:

- use-case ID;
- command transcript;
- relevant `journalctl`;
- redacted `/etc/openclaw-multi/config.yml`;
- redacted `/etc/cloudflared/config.yml`;
- relevant state DB rows;
- watcher state affected user.

## Рестарт и восстановление

Overlay API:

```bash
systemctl restart openclaw-overlay-api
systemctl status openclaw-overlay-api --no-pager
```

Cloudflared reload через Phase 6 path:

```bash
CLOUDFLARED_PID="$(systemctl show cloudflared -p MainPID --value)"
kill -HUP "$CLOUDFLARED_PID"
systemctl is-active cloudflared
```

Если E2E покажет, что `SIGHUP` не перечитывает ingress config, сначала
зафиксировать:

- `cloudflared --version`;
- unit status;
- journal;
- route behavior.

Только после evidence принимать решение о переходе на `systemctl restart`.

Managed user services:

```bash
su - <username>
systemctl --user restart openclaw-gateway
systemctl --user restart openclaw-overlay-watcher
exit
```

Snapshots generated cloudflared config:

```text
/var/lib/openclaw-multi/snapshots/
```
