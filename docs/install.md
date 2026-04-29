# Установка

> **Важно:** этот документ описывает установочный поток OpenClaw Multi.
> Реальная проверка выполняется владельцем на VPS по шагам из
> [`docs/vps-runbook.md`](vps-runbook.md) и
> [`docs/e2e-use-cases.md`](e2e-use-cases.md).

## Целевая VPS

- ОС: Ubuntu 24.
- Доступ: SSH уже настроен.
- Админ на VPS: пользователь `ubuntu`.
- Root-доступ: настроен без пароля через `su`.
- Multi-tenant модель: один OpenClaw tenant = один Linux user на VPS.

## Предусловия

- VPS доступна по SSH.
- У пользователя `ubuntu` есть доступ к root.
- Есть исходники `openclaw-multi` или готовые бинарники.
- Есть домен и Cloudflare account/named tunnel для стабильных публичных URL.
- VPS имеет исходящий интернет-доступ.

Минимальные ресурсы:

- 5 GB свободного диска или больше;
- 1 GB RAM или больше.

## Запуск установочного мастера

Подключиться к VPS:

```bash
ssh ubuntu@<vps-host>
```

Перейти в root shell для системной установки:

```bash
su -
```

Переключиться внутрь tenant из root shell:

```bash
su - <username>
```

Запустить OpenClaw Multi:

```bash
openclaw-multi
```

В TUI выбрать:

```text
1. Fresh install
```

## Что делает Fresh Install

Мастер выполняет установочные шаги по порядку.

| Шаг | Что проверяется или настраивается |
| --- | --- |
| Pre-flight | Дистрибутив, диск, RAM, системные команды, конфликты портов |
| Node.js | Не ставится в system-space; Node.js `24` ставится позже внутри tenant через `nvm` |
| Tailscale | Системная установка и admin-only диагностика |
| Cloudflare Tunnel | Account/named tunnel или quick tunnel для тестов |
| UFW | Консервативная firewall-настройка без сброса чужих правил |
| Host hardening | sysctl, hidepid, `/etc/profile.d/openclaw.sh`, `/var/cache/openclaw-compile` |
| OpenClaw CLI | CLI ставится через `nvm`/npm в user context из `pdasilem/openclaw:latest`; tenant onboarding выполняется отдельно |
| overlay-API | Systemd unit для `/usr/local/bin/openclaw-overlay-api` |
| State/config | `/etc/openclaw-multi/config.yml`, `state.db`, audit log |

Fresh Install не является созданием tenant. Tenant создается позже через:

```text
3. User management
```

## Cloudflare Tunnel

Рекомендуемый вариант для VPS:

- Cloudflare account/named tunnel.
- Wildcard DNS:

```text
*.openclaw.<domain> -> <tunnel_id>.cfargotunnel.com
```

Quick tunnel допустим только для временной проверки, потому что URL меняется
после рестарта `cloudflared`.

## После установки

Проверить системные сервисы:

```bash
systemctl is-active tailscaled cloudflared openclaw-overlay-api
systemctl status cloudflared openclaw-overlay-api --no-pager
```

Проверить основные файлы:

```bash
test -f /etc/openclaw-multi/config.yml
test -f /var/lib/openclaw-multi/state.db
test -f /var/log/openclaw-multi/audit.log
test -S /run/openclaw-overlay.sock
```

Дальше создать первого tenant:

```text
3. User management -> Add user
```

После создания Linux пользователя OpenClaw Multi запускает non-interactive
OpenClaw onboarding под этим пользователем. Этот порядок описан в
[`docs/vps-runbook.md`](vps-runbook.md).

## Типовые проблемы

**Проверка системных команд**

Сначала проверить наличие команд:

```bash
for cmd in useradd userdel loginctl systemctl ss sqlite3 git make; do
  command -v "$cmd" >/dev/null || echo "missing: $cmd"
done
```

Только если команда отсутствует, поставить соответствующий пакет через `apt`.
Для `useradd`/`userdel` пакет:

```bash
apt-get update
apt-get install -y passwd
```

**Node.js и OpenClaw CLI**

Fresh Install не ставит Node.js или OpenClaw CLI в system-space. Единственный
актуальный путь: при создании managed user OpenClaw Multi готовит tenant
runtime внутри `/home/<username>` через `nvm`, ставит Node.js `24`,
tenant-scoped OpenClaw CLI из `pdasilem/openclaw:latest`, затем запускает
non-interactive onboarding через `/home/<username>/.local/bin/openclaw`.
Команда обновления OpenClaw задается настройкой `openclaw_update_command`.

**Tailscale уже есть**

Перед изменением проверять:

```bash
command -v tailscale
systemctl is-active tailscaled
tailscale status
```

Если Tailscale уже авторизован, identity не сбрасывать и `tailscale up` повторно
не запускать. Если binary есть, но login отсутствует, использовать `tailscale up
--ssh` только для admin-доступа.

**Cloudflared уже есть**

Перед изменением проверять:

```bash
command -v cloudflared
test -f ~/.cloudflared/cert.pem || test -f /etc/cloudflared/cert.pem
test -f /etc/cloudflared/config.yml
```

Если `/etc/cloudflared/config.yml` существует, перед перезаписью делать
timestamped backup и подтверждать overwrite. Credentials file по умолчанию:
`/etc/cloudflared/<tunnel_id>.json`; перед публикацией маршрутов файл должен
существовать.

**Конфликт портов**

Найти слушателя:

```bash
ss -ltnup
```

Освободить порт или поменять диапазон в `/etc/openclaw-multi/config.yml`.

**UFW уже настроен**

Мастер не должен сбрасывать существующие правила. Он добавляет только
недостающие разрешения. Если нужна чистая firewall-конфигурация, решение о
`ufw reset` принимается вручную владельцем VPS.
