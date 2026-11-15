# Оформление бота «Ритм» в BotFather

Готовый пакет: тексты, команды, политика и ассеты. Всё можно менять по вкусу —
файлы лежат рядом, исходники картинок генерируются скриптом.

**Бот:** [@health_check_in_bot](https://t.me/health_check_in_bot) · **дата:** 12.11.2026

## Что где лежит

| Файл | Назначение |
|---|---|
| `assets/description.png` | Description picture, **640×360** PNG |
| `assets/description.gif` | Альтернатива: анимированная description picture, **320×180** GIF |
| `assets/botpic.png` | Аватарка (Botpic), **512×512** PNG |
| `assets/generate_assets.py` | Перегенерация картинок (Pillow, спокойная палитра) |
| `about.txt` | About / короткое описание (лимит 120 символов) |
| `description.txt` | Description / экран «Что может этот бот?» (лимит 512) |
| `commands.txt` | Список команд для `/setcommands` |
| `privacy-policy.md` | Политика конфиденциальности (текст) |
| `privacy-policy.html` | Та же политика для публикации как страница |

Требование Telegram к description picture: **640×360 px** (PNG/JPG) или **320×180** GIF.
Botpic: квадрат 512×512, важное — в центральном круге (Telegram обрезает по кругу).
Исходная заметка о поле: [sostav.ru](https://www.sostav.ru/blogs/262002/34784),
официальная справка: [Telegram Bots](https://core.telegram.org/bots/features#botfather).

## Порядок действий в BotFather

1. `/mybots` → выбрать `@health_check_in_bot`.
2. **Edit Botpic** → загрузить `assets/botpic.png` (или оставить текущую).
3. **Edit Description Picture** → загрузить `assets/description.png`
   (если хочется анимации — `assets/description.gif`, 320×180).
4. **Edit Name** → `Ритм — утренний чек-ин` (или просто `Ритм`).
5. **Edit About** → текст из `about.txt`.
6. **Edit Description** → текст из `description.txt`.
7. **Edit Commands** → вставить блок из `commands.txt`.
8. **Edit Privacy Policy** → вставить публичный URL на политику (см. ниже).
9. **Bot Settings** → **Allow Groups: Disable**, **Inline Mode: Disable**.
10. Проверить: открыть бота, `/start`, `/help`, `/settings`, `/status`; меню команд
    должно показывать `help / start / settings / set / status`.

## Поля и значения

| Поле | Сейчас у тебя | Поставить |
|---|---|---|
| Name | (текущее) | `Ритм — утренний чек-ин` |
| Username | `@health_check_in_bot` | **не менять** (смена username ломает ссылку) |
| About | — | `about.txt` (99/120 символов) |
| Description | 🚫 | `description.txt` (500/512) |
| Description picture | 🚫 | `assets/description.png` (640×360) |
| Botpic | 🖼 есть | `assets/botpic.png` (512×512), можно заменить |
| Commands | no commands yet | `commands.txt` |
| Privacy Policy | 🚫 | публичный URL (ниже) |
| Allow Groups | проверить | **Disable** (личный бот, без групп) |
| Inline Mode | проверить | **Disable** |
| Payments / Web App / Mini App / Business | — | не включать |
| Bot's domain | — | не задавать |

### Команды (точный блок для Edit Commands)

```text
help - Что умею и как начать
start - Начать заново и открыть справку
settings - Цели и зоны, переопределения *
set - Изменить настройку: ключ и значение
training - Записать тренировку текстом
week - Сводка недели: HRV, сон, питание, тренировки, вес
status - HRV_7, зона, цели, тренировка
```

Только реализованные команды: `/checkin`, `/training`, `/food`, `/measure`,
`/cancel`, `/ask`, `/review`, `/week`; `/article` — backlog.
добавим их сюда же (и в HelpText кода).

## Политика конфиденциальности: как получить URL

BotFather просит **публичную ссылку**. Перед публикацией замени в
`privacy-policy.md`/`.html` плейсхолдер `[УКАЖИ КОНТАКТ]` на свой Telegram-контакт
для запросов по данным.

Варианты хостинга (любой):

1. **Telegraph (проще всего):** открыть [telegra.ph](https://telegra.ph), создать
   страницу «Политика конфиденциальности бота Ритм», вставить текст из
   `privacy-policy.md` (или HTML), опубликовать и скопировать URL.
2. **GitHub Gist:** создать **публичный** gist с `privacy-policy.md`, взять
   `gist.githubusercontent.com/...` (проверить, что ссылка открывается без логина).
3. **GitHub Pages / свой сайт:** положить `privacy-policy.html` на публичный хост.

⚠️ Приватный репозиторий не подойдёт: raw-ссылка потребует авторизацию.
Политика должна открываться без входа.

## Что проверить после сохранения

- [ ] Аватарка читается в списке чатов (не мелкая, контрастная).
- [ ] Description picture 640×360 не обрезается (важное — в центре).
- [ ] About и Description влезли целиком (лимиты 120/512).
- [ ] Меню команд показывает семь пунктов; `/set`, `/training`, `/week` подписаны понятно.
- [ ] Политика открывается инкогнито по URL.
- [ ] Группы и inline выключены: бот отвечает только в ЛС разрешённым chat_id.
- [ ] `/start` показывает описание и команды; `/settings` и `/status` работают.

## Про картинки

Спокойная палитра «Ритма»: тёмно-зелёный фон, мята, песок. Тексты на картинке —
только название и суть, без QR и ссылок (Description в Telegram не разрешает
активные ссылки; QR можно, но для личного бота это лишнее).

Если нужна другая типографика/композиция — правится `generate_assets.py`
(шрифты DejaVu, Pillow), затем перегенерировать:

```bash
PYTHONPATH=.sandbox-cache/pylibs python3 docs/botfather/assets/generate_assets.py
```

Скрипт и картинки хранятся в репозитории, чтобы оформление можно было
воспроизвести и не потерять.
