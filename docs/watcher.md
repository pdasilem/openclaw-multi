# Наблюдатель Overlay

`openclaw-overlay-watcher` - это per-user daemon. Он запускается внутри
managed Linux пользователя, то есть внутри tenant boundary на целевой Ubuntu 24
VPS.

Watcher следит за OpenClaw конфигом этого пользователя:

```text
/home/<user>/.openclaw/openclaw.json
```

Когда watcher находит plugin callback config, он обращается в overlay-API через
UNIX socket:

```text
/run/openclaw-overlay.sock
```

Watcher отправляет:

- `plugin_id`;
- `hostname_hint`;
- `local_port`.

Финальный route ID не задается клиентом. Его выводит daemon на стороне
overlay-API.

## Граница Tenant

OpenClaw Multi использует модель:

```text
1 OpenClaw tenant = 1 Linux user на VPS
```

Поэтому watcher:

- запускается не от root;
- запускается не от admin `ubuntu`;
- запускается от managed пользователя, например `alice`;
- имеет доступ только к `~/.openclaw/openclaw.json` своего пользователя;
- авторизуется в overlay-API через `SO_PEERCRED`.

## Callback-контракт

Phase 7 поддерживает такой явный формат plugin config:

```json
{
  "plugins": {
    "entries": {
      "calendar": {
        "config": {
          "callbackPort": 19000,
          "callbackHostnameHint": "oauth"
        }
      }
    }
  }
}
```

Watcher вызывает overlay-API и получает публичный callback URL. После этого он
записывает результат обратно в OpenClaw config через CLI:

```text
openclaw config set plugins.entries.<plugin_id>.config.callbackUrl <url>
openclaw config set plugins.entries.<plugin_id>.config.callbackPort <port>
```

Плагин может переопределить ключи, куда нужно записывать callback values:

```json
{
  "overlay": {
    "callback": {
      "local_port": 19000,
      "hostname_hint": "oauth",
      "url_config_key": "plugins.entries.calendar.config.callbackUrl",
      "port_config_key": "plugins.entries.calendar.config.callbackPort"
    }
  }
}
```

## Состояние

Watcher хранит per-user snapshot:

```text
/home/<user>/.openclaw-overlay/watcher.state
```

В state фиксируются:

- plugin ID;
- hostname hint;
- local port;
- route ID, возвращенный overlay-API;
- public URL;
- статус записи callback values в OpenClaw config.

Если state отсутствует, watcher считает snapshot пустым и делает полный sync.

## Systemd-сервис

User service запускает watcher так:

```text
/usr/local/bin/openclaw-overlay-watcher \
  --username <user> \
  --config /home/<user>/.openclaw/openclaw.json \
  --snapshot /home/<user>/.openclaw-overlay/watcher.state \
  --socket /run/openclaw-overlay.sock
```

Проверка сервиса под managed пользователем:

```bash
su - <user>
systemctl --user status openclaw-overlay-watcher --no-pager
exit
```

На VPS админский пользователь - `ubuntu`, root shell доступен без пароля через
`su -`. Для ручной проверки tenant boundary из root shell:

```bash
su - <user>
id
test -f ~/.openclaw/openclaw.json
exit
```

## Логи

Логи watcher:

```bash
su - <user>
journalctl --user -u openclaw-overlay-watcher --no-pager -n 300
journalctl --user -u openclaw-overlay-watcher -f
exit
```

Логи overlay-API:

```bash
journalctl -u openclaw-overlay-api --no-pager -n 300
```

Watcher state:

```bash
su - <user>
cat ~/.openclaw-overlay/watcher.state
exit
```

OpenClaw config с секретами нужно смотреть осторожно:

```bash
su - <user>
sed -n '1,220p' ~/.openclaw/openclaw.json
exit
```

Перед передачей логов или config нужно редактировать:

- provider keys;
- gateway tokens;
- OAuth material;
- channel credentials;
- личные account identifiers.

## Что проверять в E2E

- Watcher стартует как managed Linux user.
- Watcher видит изменения в `~/.openclaw/openclaw.json`.
- Plugin route создается через overlay-API.
- Route ID daemon-derived, а не client-supplied.
- Callback URL записывается обратно через `openclaw config set`.
- После рестарта watcher не создает duplicate routes.
- После удаления plugin callback config route становится неактивным или
  удаляется согласно текущей реализации.
