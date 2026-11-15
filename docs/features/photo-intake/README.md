# Фича: приём данных — скриншоты (vision) и текст чек-ина

**Ветка:** `feat/photo-intake` · **Статус:** в разработке · **Дата:** 2026-11-12
**Связанные пункты ТЗ:** FR-6.8 (vision), FR-1.x (приём в свободной форме), NFR-2/NFR-3, §13.

## 1. Зачем

Пользователь присылает боту скриншоты приложений (Amazfit/Zepp, дневник питания,
тренировки и т.п.). Нужно автоматически извлекать из них данные и отвечать в чат
(дальше — сохранять в файловую БЗ на M1).

Особенности, которые обязательны:

- скриншоты бывают **одиночные и по два** (альбом Telegram);
- к альбому может быть **подпись-вопрос** («что тут по питанию?», «вчера норм?»);
- два скриншота могут прийти **и подряд одним альбомом, и отдельными сообщениями**;
- содержимое может быть разным (готовность, питание, тренировка, замеры), но с
  нужными данными;
- vision-модель может быть **той же, что чат-модель**, или чат — локальная
  (Ollama), а vision — через **внешний OpenAI-совместимый API**;
- на машине мало RAM → vision-модель грузится только под задачу, вызовы
  сериализуются, `keep_alive` короткий, таймаут увеличен (180 с);
- **текстовый чек-ин** тоже принимается: сообщение «сон 7:33, HRV 38, пульс 58,
  настроение 7» сначала разбирается **детерминированными правилами** (надёжно для
  маленькой модели), LLM `Extract` — fallback для нестандартного текста;
  свободный чат не ломается;
- скриншоты обрабатываются **по одному изображению за вызов** (тип определяется по
  `kind`: nutrition/checkin/other), результаты объединяются; дату модель пишет
  дословно (`date_text`), а код переводит «вчера/11 ноября» детерминированно.

## 2. Acceptance criteria

| # | Критерий | Проверка |
|---|---|---|
| AC1 | Конфиг vision отдельно от чата: `LLM_VISION_MODE` (пусто = как чат), `LLM_VISION_MODEL` (пусто = как чат), `LLM_VISION_API_URL/KEY` (пусто = общие). Доступны режимы `ollama` и `api`; неверные значения → ошибка на старте | `config` тесты + TZ §9 |
| AC2 | `Provider.ExtractVision(prompt, schema, images)`: Ollama — `images` base64 в сообщении + `format`; OpenAI-совместимый — `image_url` data-URL + `response_format=json_object`; JSON валидируется | `llm/vision_test.go` |
| AC3 | Экстрактор: промпт с **сегодняшней датой** (для «вчера»), JSON-схема (kinds/date/nutrition/checkin/notes), нормализация (сон `06:55`→6.92 ч, температура `+0,4`→0.4) и проверка диапазонов; нет данных → понятная ошибка | `vision/extract_test.go` |
| AC4 | Альбом (`media_group_id`): фото собираются в окне 1.2 с, обрабатываются **одним** вызовом со всеми изображениями; подпись берётся с любого фото; одиночное фото — сразу; два отдельных сообщения — два вызова | `vision/service_test.go` |
| AC5 | Роутер: `photo`/`document` (image/*) → передача в vision-сервис (`file_id`, caption, media_group_id); не-image документы игнорируются; если vision не настроен — вежливый ответ «пришли текстом» | `router/photo_test.go` |
| AC6 | Ответ в чат: краткая русская сводка распознанного + вопрос «верно?»; ошибки (модель недоступна/не распознала) → мягкий текст, бот живёт | `vision/service_test.go`, router |
| AC7 | CLI-смоук `-image <путь[,путь]>` — распознать локальный скриншот(ы) без Telegram, напечатать JSON | `app/image_test.go` + ручной прогон |
| AC7.1 | Текст чек-ина: `LooksLikeCheckin` (маркер+число), `Extract` по схеме Checkin, нормализация, ответ-подтверждение; вопрос без маркеров → чат | `intake/*_test.go`, роутер |
| AC8 | Память: vision-вызовы сериализованы (одна модель за раз), таймаут 120 с, `keep_alive` настраивается; после теста модель выгружается | код + ручной прогон |
| AC9 | Спека v1.5 (FR-6.8, §9, §13), фич-доки, README/отчёт обновлены | diff |
| AC10 | `make check`, `-race`, `govulncheck` чистые; живой смоук на реальных скриншотах; PR | CI/ручной прогон |

## 3. Архитектура

```
Telegram update (photo/document, caption, media_group_id)
  → router: PhotoIntake.Submit(chat_id, media_group_id, file_id, caption)
  → vision.Service: сборка альбома (debounce 1.2с) → скачивание файлов
  → vision.Extractor: prompt(+today) + JSON-schema → Provider.ExtractVision(images)
  → нормализация/валидация → ответ в чат (Sender)
```

- `internal/vision` — `Extractor` (промпт, схема, нормализация) и `Service`
  (альбомы, скачивание, сериализация, ответы). Не знает про telego.
- `llm.Provider.ExtractVision` — новый метод; Ollama и OpenAI-совместимый его
  реализуют, fallback делегирует.
- `router` получает интерфейс `PhotoIntake` (без импорта vision), скачивание
  файлов — в `app` (`telego.GetFile` + HTTP через прокси).
- **Дата:** в промпт передаётся «сегодня=YYYY-MM-DD», модель переводит «вчера,
  11 ноября» в `2026-11-11`; при отсутствии даты — оставляем пустой и сообщаем.

### Схема извлечения (JSON)

`kinds: [nutrition|checkin|training|measurement|other]`, `date`, `nutrition`
(`kcal`, `protein_g`, `fat_g`, `carbs_g`, `meals[]`), `checkin` (`sleep`,
`hrv_ms`, `rhr`, `fatigue`, `temperature_c`, `load_goal`), `notes`.

## 4. Матрица тестов

| Тест | Что проверяет |
|---|---|
| `TestConfigVisionDefaults` | vision = как чат, если переменные пусты |
| `TestConfigVisionSeparate` | chat=ollama, vision=api (свой URL/ключ/модель) |
| `TestVisionFromOptions` | сборка vision-провайдера по режимам, ошибки конфигурации |
| `TestOllamaExtractVision` | images base64 + format, JSON-ответ |
| `TestOpenAIExtractVision` | image_url data-URL + response_format json_object |
| `TestExtractNutritionScreenshot` | schema/JSON → нормализованный `Extracted` (ккал/Б/Ж/У, приёмы) |
| `TestExtractReadinessScreenshot` | сон→часы, HRV/пульс/усталость/температура |
| `TestExtractDateYesterday` | «вчера, 11 ноября» → дата относительно today |
| `TestExtractInvalidData` | нет данных/мусор → ошибка, не паника |
| `TestServiceAlbumCombinesPhotos` | 2 фото одного media_group → 1 вызов с 2 images, caption с любого |
| `TestServiceSinglePhoto` | одиночное фото → отдельный вызов |
| `TestServiceSeparateMessages` | два отдельных сообщения → два вызова |
| `TestServiceVisionError` | ошибка модели → вежливый ответ |
| `TestRouterPhotoSubmits` | фото (largest size) → Submit с file_id/caption/group |
| `TestRouterDocumentImage` | документ image/* → Submit; pdf — игнор |
| `TestRouterNoVisionReplies` | vision выключен → ответ «пришли текстом» |
| `TestScanImageCLI` | `-image` с mock Ollama vision → JSON |
| `TestLooksLikeCheckin` | маркер+число → true; вопрос/без чисел → false |
| `TestExtractTextCheckin` | «сон 7:33, HRV 38, пульс 58, настроение 7» → нормализованный Checkin |
| `TestRouterTextCheckin` | текст-чек-ин → SubmitText, вопрос → чат |

## 5. Живая проверка (после кода)

```bash
make image IMG="/path/readiness.jpg"
make image IMG="/path/nutrition1.jpg,/path/nutrition2.jpg"
# в Telegram: отправить один скриншот; затем альбом из двух с подписью — один ответ
```

Ожидание: поля распознаны (готовность: сон/HRV/пульс/усталость/температура;
питание: ккал/Б/Ж/У и приёмы), ответ краткий, с подтверждением.

## 6. Что НЕ входит

- Сохранение в `данные_*/` — после M1 (writer); сейчас только извлечение и ответ.
- Распознавание рукописного текста и произвольных фото — за пределами; для
  скриншотов приложений.
- Загрузка full-text/OCR — см. отдельно; при нехватке RAM рекомендуется OCR на
  стороне бота (вариант B), но в эту фичу не входит.
