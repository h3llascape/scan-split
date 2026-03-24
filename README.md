# ScanSplit

Десктопное приложение для автоматического разбиения отсканированных учебных работ по студентам. Принимает PDF с несколькими работами, распознаёт имена студентов и группы через OCR и создаёт отдельный PDF-файл для каждого студента.

## Скачать готовое приложение

Собирать руками ничего не нужно — готовые бинарники публикуются в [GitHub Releases](../../releases/latest):

| Платформа | Файл |
|-----------|------|
| macOS (Apple Silicon) | `scansplit-mac-arm64.zip` |
| Windows (64-bit) | `scansplit-windows-amd64.zip` |

Распакуйте архив и запустите приложение. Никаких дополнительных установок не требуется — Tesseract и все зависимости уже встроены.

## Как это работает

Обработка проходит в 5 этапов:

1. **Разбиение** — PDF разбивается на отдельные страницы (pdfcpu)
2. **Рендеринг** — каждая страница конвертируется в PNG с разрешением 300 DPI (MuPDF)
3. **OCR** — Tesseract распознаёт русский текст на изображениях (до 4 параллельных воркеров)
4. **Парсинг** — из текста извлекаются ФИО студента и код группы с помощью регулярных выражений
5. **Группировка и сборка** — страницы объединяются в PDF-файлы по студентам

Алгоритм группировки учитывает нечёткое совпадение имён (склонение: Малышев/Малышева), безымянные страницы перед титульными и страницы без распознанного имени. Нераспознанные страницы сохраняются отдельно для ручной проверки.

## Сборка из исходников

### Требования

- Go 1.21+
- Node.js 20+
- [Wails v2](https://wails.io/docs/gettingstarted/installation): `go install github.com/wailsapp/wails/v2/cmd/wails@latest`

#### macOS

```bash
brew install tesseract leptonica dylibbundler
go env -w CGO_CXXFLAGS="-I/opt/homebrew/include"
go env -w CGO_LDFLAGS="-L/opt/homebrew/lib"
```

#### Windows

Нужен [MSYS2](https://www.msys2.org/). В терминале MSYS2 MinGW64:

```bash
pacman -S mingw-w64-x86_64-gcc mingw-w64-x86_64-tesseract-ocr mingw-w64-x86_64-leptonica
```

Добавьте `C:\msys64\mingw64\bin` в системный PATH.

### Команды

```bash
# Скачать русскую языковую модель Tesseract (один раз)
make tessdata

# Запустить в режиме разработки (горячая перезагрузка)
make dev

# Собрать и упаковать для macOS (результат: build/bin/scansplit.app)
make bundle-mac

# Собрать для Windows (результат: build/bin/scansplit.exe + DLL)
make bundle-win
```

## Отладка OCR

В репозитории есть CLI-инструмент для диагностики распознавания:

```bash
go run ./cmd/debug <input.pdf>
```

Выводит результаты OCR и группировки постранично.

## Структура проекта

```
scan-split/
├── main.go                  # точка входа, инициализация Wails
├── Makefile                 # сборка, tessdata, бандлинг
├── cmd/
│   ├── app/app.go           # методы, доступные из JS (ProcessFile и др.)
│   └── debug/main.go        # CLI для отладки
├── internal/
│   ├── models/              # общие типы данных
│   ├── pipeline/            # оркестрация 5-этапного пайплайна
│   ├── ocr/                 # Tesseract, парсинг имён/групп, mock-режим
│   └── pdf/                 # разбиение, рендеринг, сборка PDF
├── frontend/                # Svelte-приложение
│   └── src/
│       ├── App.svelte        # главный компонент, управление состоянием
│       └── lib/             # FileUpload, ProcessingView, ResultsView
└── third_party/gosseract/   # локальный форк gosseract
```
