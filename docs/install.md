# Установка до запуска OpenClaw Multi

Этот документ доводит VPS до состояния, когда можно выполнить команду
`openclaw-multi`. Дальше запуск TUI, `Fresh install`, создание tenant users,
публикация routes, логи и E2E идут по [`docs/vps-runbook.md`](vps-runbook.md).

## Роли

- `ubuntu` — SSH admin и владелец checkout `/home/ubuntu/openclaw-multi`.
- `root` — установка бинарников в `/usr/local/bin`, установка системных
  пакетов, подготовка `/etc/cloudflared`.
- `tailscaled` — admin-only доступ к VPS по tailnet. Managed users не получают
  Tailscale identity.
- `<username>` — tenant Linux user.

## Подключение

```bash
ssh ubuntu@<vps-host>
```

Проверить ОС и shell context:

```bash
cat /etc/os-release
id
pwd
```

Ожидаемо:

- Ubuntu 24.
- текущий пользователь: `ubuntu`.
- root shell доступен через passwordless `sudo -i`.

## Tailscale и UFW

На VPS admin SSH идет через Tailscale. UFW должен сохранять закрытый public
inbound и не должен блокировать текущую SSH-сессию.

Перед любыми установочными действиями проверить текущий SSH endpoint:

```bash
echo "$SSH_CONNECTION"
tailscale ip -4
tailscale status
systemctl is-active tailscaled
sudo -i
ufw status verbose
exit
```

Ожидаемо:

- `tailscaled` active;
- `tailscale status` показывает logged-in node;
- remote IP из `SSH_CONNECTION` находится в tailnet;
- UFW active;
- SSH разрешен через tailnet/Tailscale interface. Валидный текущий вариант:
  `Anywhere on tailscale0 ALLOW IN Anywhere`;
- public inbound остается закрытым.

Если в UFW уже есть `ALLOW IN` на весь `tailscale0`, отдельное правило для
`22/tcp` не добавлять. Если нет ни широкого `tailscale0` allow, ни явного SSH
allow на `tailscale0`, добавить SSH-доступ до любых дальнейших изменений
firewall:

```bash
sudo -i
ufw allow in on tailscale0 to any port 22 proto tcp comment 'admin ssh via tailscale'
ufw status verbose
exit
```

Не выполнять `ufw reset`. Не выполнять `tailscale up` при уже авторизованном
tailnet node. OpenClaw Multi не выдает Tailscale identity managed users:
Tailscale здесь только admin-доступ к VPS.

Не удалять существующие не-OpenClaw правила без отдельного решения владельца
VPS.

Если `tailscale` или `tailscaled` отсутствует, остановить установку
OpenClaw Multi, поставить и авторизовать Tailscale по официальной инструкции
для Ubuntu 24.04, затем вернуться к этому разделу:
<https://tailscale.com/kb/1031/install-linux>

## Системные пакеты

Проверить команды:

```bash
for cmd in git make go sqlite3 useradd userdel loginctl systemctl ss ufw tailscale curl; do
  command -v "$cmd" >/dev/null || echo "missing: $cmd"
done
```

Поставить отсутствующие пакеты из root shell:

```bash
sudo -i
apt-get update
apt-get install -y git make golang-go sqlite3 passwd systemd ufw curl ca-certificates
exit
```

`tailscale` не ставится этим `apt-get install`: он должен уже быть
установлен и авторизован до продолжения установки OpenClaw Multi.

## Установка cloudflared

Проверить наличие:

```bash
command -v cloudflared
cloudflared --version
```

Если `cloudflared` отсутствует, поставить stable package из официального
Cloudflare apt repository для Ubuntu 24.04 `noble`:

```bash
curl -fsSL https://pkg.cloudflare.com/cloudflare-main.gpg -o /tmp/cloudflare-main.gpg
sudo install -d -m 0755 /usr/share/keyrings
sudo install -m 0644 /tmp/cloudflare-main.gpg /usr/share/keyrings/cloudflare-main.gpg
printf '%s\n' 'deb [signed-by=/usr/share/keyrings/cloudflare-main.gpg] https://pkg.cloudflare.com/cloudflared noble main' | sudo tee /etc/apt/sources.list.d/cloudflared.list >/dev/null
sudo apt-get update
sudo apt-get install -y cloudflared
cloudflared --version
```

Официальная страница установки:
<https://pkg.cloudflare.com/>.

## Репозиторий

Клонировать repo как `ubuntu`:

```bash
cd /home/ubuntu
git clone https://github.com/pdasilem/openclaw-multi.git
cd /home/ubuntu/openclaw-multi
git status --short
```

Если repo уже есть:

```bash
cd /home/ubuntu/openclaw-multi
git status --short
git pull --ff-only
```

## Сборка

Собрать бинарники как `ubuntu`:

```bash
cd /home/ubuntu/openclaw-multi
make build
```

Ожидаемые артефакты:

```text
bin/openclaw-multi
bin/openclaw-overlay-api
bin/openclaw-overlay-watcher
```

## Установка бинарников

Установить бинарники в system-space из обычной SSH-сессии `ubuntu`:

```bash
cd /home/ubuntu/openclaw-multi
sudo make install
sudo openclaw-multi system-prepare
```

Проверить под пользователем `ubuntu`. Бинарники лежат в `/usr/local/bin`,
поэтому должны находиться из обычной SSH-сессии:

```bash
command -v openclaw-multi
command -v openclaw-overlay-api
command -v openclaw-overlay-watcher
```

## Cloudflare named tunnel

Для стабильных публичных URL нужен named tunnel и wildcard DNS:

```text
https://gateway-<username>.<subdomain>.<domain>
https://<plugin-route>.<subdomain>.<domain>
```

Подготовить в Cloudflare:

- domain `<domain>` добавлен в Cloudflare zone и имеет статус `Active`;
- у регистратора домена выставлены Cloudflare nameservers для этой zone;
- есть Zone ID этой zone;
- есть API token с правом `Zone:DNS:Edit` для этой zone;
- есть named tunnel с выбранным именем, например `oc-multi`;
- wildcard DNS указывает на tunnel:
  `*.<subdomain>.<domain> -> <tunnel_id>.cfargotunnel.com`.

Что значит Cloudflare zone:

- `<domain>` — реальный домен, например `example.com`;
- zone — запись этого домена в Cloudflare account;
- после добавления домена Cloudflare выдает nameservers;
- эти nameservers нужно прописать у регистратора домена;
- когда Cloudflare подтвердит nameservers, zone станет `Active`;
- Zone ID берется в dashboard на странице домена.

Официальные инструкции Cloudflare:

- Add domain / onboard zone:
  <https://developers.cloudflare.com/fundamentals/manage-domains/add-site/>
- Cloudflare Tunnel overview:
  <https://developers.cloudflare.com/tunnel/>
- Create locally-managed tunnel:
  <https://developers.cloudflare.com/tunnel/advanced/local-management/create-local-tunnel/>
- Tunnel DNS routing:
  <https://developers.cloudflare.com/tunnel/routing/>

Создать locally-managed tunnel через `cloudflared` CLI на VPS.
Cloudflare-команды выполнять как `ubuntu`, не из root shell. Это важно:
`cloudflared` пишет `cert.pem` и `<tunnel_id>.json` в default directory
текущего пользователя.

```bash
cloudflared tunnel login
cloudflared tunnel create <tunnel_name>
cloudflared tunnel route dns <tunnel_name> "*.<subdomain>.<domain>"
cloudflared tunnel list
```

После `cloudflared tunnel create` взять `tunnel_id` и точный путь к credentials
file из вывода команды. При запуске под `ubuntu` ожидаемые пути:

```text
/home/ubuntu/.cloudflared/cert.pem
/home/ubuntu/.cloudflared/<tunnel_id>.json
```

Для OpenClaw Multi credentials file должен быть установлен в system path:

```text
/etc/cloudflared/<tunnel_id>.json
```

Причина: файл в `/home/ubuntu/.cloudflared` принадлежит интерактивному
admin-пользователю и нужен для CLI-операций. Runtime `cloudflared` service и
OpenClaw Multi используют стабильный system path из `/etc/cloudflared`, чтобы
работа tunnel не зависела от home directory пользователя `ubuntu`.

Это обязательный шаг после `cloudflared tunnel create`:

```bash
sudo install -d -m 0755 /etc/cloudflared
sudo install -m 0600 /home/ubuntu/.cloudflared/<tunnel_id>.json /etc/cloudflared/<tunnel_id>.json
ls -l /home/ubuntu/.cloudflared/<tunnel_id>.json
sudo ls -l /etc/cloudflared/<tunnel_id>.json
```

Данные для `openclaw-multi`:

```yaml
domain: <domain>
subdomain: <subdomain>
tunnel_name: <tunnel_name>
tunnel_id: <tunnel_id>
tunnel_mode: account
cloudflare_zone_id: <zone-id>
cloudflare_api_token: <token-with-dns-edit>
cloudflared_credentials_file: /etc/cloudflared/<tunnel_id>.json
terminal_history_lines: 1000
```

## Готовность к запуску

Перед переходом в runbook должно выполняться:

```bash
command -v openclaw-multi
command -v openclaw-overlay-api
command -v openclaw-overlay-watcher
command -v cloudflared
test -d /home/ubuntu/openclaw-multi
test -f /etc/cloudflared/<tunnel_id>.json
test -w /var/lib/openclaw-multi
test -w /var/log/openclaw-multi
test ! -e /var/lib/openclaw-multi/state.db || test -w /var/lib/openclaw-multi/state.db
```

Следующий документ: [`docs/vps-runbook.md`](vps-runbook.md).
