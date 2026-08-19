# Инструкция Для Independent Reviewer

## Цель

Проверить шесть canonical task solutions и определить, можно ли использовать их как корректные эталонные реализации для эксперимента `Specs Don't Hallucinate`.

Human review должен подтвердить не только прохождение автоматических тестов, но и следующее:

- реализация соответствует требованиям;
- Direct и Codegen варианты имеют одинаковое публичное поведение;
- соблюдены ограничения treatment;
- formal inputs и canonical SQL не были произвольно изменены;
- решения не подогнаны специально под evaluator;
- код достаточно корректен, чтобы использовать его как reference перед pilots.

Reviewer не должен исправлять код самостоятельно. Все обнаруженные проблемы записываются как findings.

## 1. Объект Review

Task 9 baseline, который обязательно должен входить в проверяемую историю:

```text
f2a1910 Complete known-broken coverage
```

Рабочий repository:

```text
experiment/
```

Authoritative design document находится рядом с repository:

```text
../experiment-decisions.md
```

В начале проверки выполнить:

```sh
cd experiment
git status --short --branch
git rev-parse HEAD
git log -1 --oneline
git merge-base --is-ancestor f2a1910 HEAD
```

Ожидаемый результат:

```text
branch: main
worktree: clean
f2a1910 is an ancestor of HEAD
```

Reviewer записывает полный фактический `HEAD` в итоговый отчёт. Это должен быть согласованный commit, содержащий данную инструкцию и не изменяющий canonical solutions после `f2a1910` без отдельного повторного согласования. Если ancestry check возвращает ненулевой exit code или worktree не чист, остановить review и сообщить владельцу эксперимента.

## 2. Заявление О Независимости

В итоговом отчёте reviewer должен подтвердить:

```text
I did not author the canonical solutions under review.
I reviewed the implementation personally and did not delegate the final judgment to an AI system.
```

Допустимо использовать IDE, поиск, diff и другие инструменты. Итоговое решение должен принимать человек.

## 3. Что Не Нужно Проверять

Этот review не является optional review будущих measured candidates из `review/README.md`.

Сейчас не нужно:

- проверять 28 measured candidates;
- выставлять оценки по `review/rubric.md`;
- проводить blind review;
- рандомизировать порядок;
- анализировать token usage;
- анализировать model behavior;
- запускать agents;
- запускать pilots;
- запускать measured runs;
- обращаться к model provider.

Проверяются только шесть canonical solutions.

## 4. Проверяемые Solutions

| Task | Direct | Codegen |
|---|---|---|
| Nullable PATCH | `fixtures/task-solutions/nullable-patch-direct/` | `fixtures/task-solutions/nullable-patch-codegen/` |
| Optimistic Locking | `fixtures/task-solutions/optimistic-locking-direct/` | `fixtures/task-solutions/optimistic-locking-codegen/` |
| Cursor Pagination | `fixtures/task-solutions/cursor-pagination-direct/` | `fixtures/task-solutions/cursor-pagination-codegen/` |

Canonical starting points:

```text
fixtures/base2-direct/
fixtures/base2-codegen/
```

## 5. Сначала Прочитать

Читать документы в следующем порядке:

```text
../experiment-decisions.md
ROADMAP.md
fixtures/manifest.md
fixtures/task-solutions/README.md
tasks/problem-details.md
```

Затем требования задач:

```text
tasks/full/nullable-patch.md
tasks/full/optimistic-locking.md
tasks/full/cursor-pagination.md
```

Затем propagation requirements:

```text
tasks/propagation/README.md
tasks/propagation/nullable-patch.md
tasks/propagation/optimistic-locking.md
tasks/propagation/cursor-pagination.md
```

После этого README каждого solution:

```text
fixtures/task-solutions/nullable-patch-direct/README.md
fixtures/task-solutions/nullable-patch-codegen/README.md
fixtures/task-solutions/optimistic-locking-direct/README.md
fixtures/task-solutions/optimistic-locking-codegen/README.md
fixtures/task-solutions/cursor-pagination-direct/README.md
fixtures/task-solutions/cursor-pagination-codegen/README.md
```

При расхождениях использовать приоритет:

```text
../experiment-decisions.md
ROADMAP.md
experiment config/schema
implementation
scaffold documentation
старые conceptual documents
```

## 6. Автоматические Проверки

Из корня `experiment/` выполнить:

```sh
make validate-task-targets
make verify-task-solutions
make evaluate-task-solutions
```

Ожидается:

- canonical propagation targets validated;
- все Go modules verified;
- tests прошли;
- `go vet` прошёл;
- Codegen outputs регенерируются byte-for-byte;
- все шесть evaluator results имеют `completeSuccess: true`.

Результаты evaluator находятся в:

```text
results/task-solutions/
```

Для каждого JSON проверить:

```sh
jq '{task, completeSuccess, setup, failed: [.behaviorCases[] | select(.applicable and (.passed != true)) | .id]}' results/task-solutions/*.json
```

Ожидается:

```text
completeSuccess: true
failed: []
```

Успешные automated checks не заменяют чтение реализации.

Если команда не запускается из-за отсутствующего Docker image или локальной инфраструктуры, записать это в отчёте. Не считать infrastructure problem дефектом solution без дополнительного подтверждения.

## 7. Сравнение С Starting Fixtures

Reviewer должен посмотреть изменения каждого solution относительно соответствующего Base 2.

Команды для Direct:

```sh
git diff --no-index -- fixtures/base2-direct fixtures/task-solutions/nullable-patch-direct
git diff --no-index -- fixtures/base2-direct fixtures/task-solutions/optimistic-locking-direct
git diff --no-index -- fixtures/base2-direct fixtures/task-solutions/cursor-pagination-direct
```

Команды для Codegen:

```sh
git diff --no-index -- fixtures/base2-codegen fixtures/task-solutions/nullable-patch-codegen
git diff --no-index -- fixtures/base2-codegen fixtures/task-solutions/optimistic-locking-codegen
git diff --no-index -- fixtures/base2-codegen fixtures/task-solutions/cursor-pagination-codegen
```

Для `git diff --no-index` exit code `1` означает наличие ожидаемых различий и сам по себе не является ошибкой.

Reviewer должен убедиться, что изменения связаны с соответствующей задачей и не содержат постороннего redesign.

## 8. Проверка Formal Inputs

Внутри каждой Direct/Codegen пары должны совпадать:

```text
api/openapi.yaml
db/migrations/
db/queries/tasks.sql
```

Проверить:

```sh
diff -q \
  fixtures/task-solutions/nullable-patch-direct/api/openapi.yaml \
  fixtures/task-solutions/nullable-patch-codegen/api/openapi.yaml

diff -qr \
  fixtures/task-solutions/nullable-patch-direct/db/migrations \
  fixtures/task-solutions/nullable-patch-codegen/db/migrations

diff -q \
  fixtures/task-solutions/nullable-patch-direct/db/queries/tasks.sql \
  fixtures/task-solutions/nullable-patch-codegen/db/queries/tasks.sql
```

Повторить для:

```text
optimistic-locking
cursor-pagination
```

`go.mod` и `go.sum` не обязаны быть byte-identical: Codegen может иметь дополнительные generator/runtime dependencies. При этом общие Go, framework и database dependencies должны использовать согласованные версии.

Проверить отсутствие различий в:

- business requirements;
- OpenAPI operations;
- migrations;
- canonical SQL;
- Problem Details semantics.

## 9. Общий Checklist Для Всех Solutions

Для каждого из шести solutions проверить:

- [ ] Используется Go, `chi` и `pgx/v5`.
- [ ] `id` генерируется приложением как UUID.
- [ ] `createdAt` генерируется PostgreSQL.
- [ ] Timestamps возвращаются в UTC с шестью fractional digits.
- [ ] Title обрезается по краям.
- [ ] Title проверяется как 1–200 Unicode code points.
- [ ] Unknown JSON fields отклоняются там, где этого требует contract.
- [ ] Errors возвращаются как RFC 9457 Problem Details.
- [ ] Content type ошибок равен `application/problem+json`.
- [ ] Problem Details соответствуют `tasks/problem-details.md`.
- [ ] HTTP state соответствует PostgreSQL state.
- [ ] HTTP, service и repository responsibilities разделены.
- [ ] Нет hardcoded ответов для известных evaluator inputs.
- [ ] Нет evaluator-specific environment checks.
- [ ] Нет обхода canonical SQL через посторонний inline SQL.
- [ ] Candidate-authored tests проверяют основную task semantics.
- [ ] Код не содержит очевидных race conditions или lost updates.
- [ ] Реализация не меняет baseline behavior без требования задачи.

Точные package names и внутренняя архитектура не фиксированы.

## 10. Direct Treatment Checklist

Для каждого Direct solution проверить:

- [ ] Нет `oapi-codegen.yaml`.
- [ ] Нет `sqlc.yaml`.
- [ ] Нет generator commands.
- [ ] Нет сгенерированного HTTP server implementation.
- [ ] Нет сгенерированных database wrappers.
- [ ] HTTP request/response types написаны вручную.
- [ ] Routing написан вручную.
- [ ] Strict JSON decoding реализован вручную.
- [ ] Repository wrappers написаны вручную.
- [ ] Canonical SQL используется без изменения semantics.
- [ ] Generated code не был скопирован и объявлен handwritten.

Итог должен быть записан отдельно для каждого Direct solution.

## 11. Codegen Treatment Checklist

Для каждого Codegen solution проверить:

- [ ] Есть `oapi-codegen.yaml`.
- [ ] Есть `sqlc.yaml`.
- [ ] HTTP types/routes созданы `oapi-codegen`.
- [ ] Database models/queries созданы `sqlc`.
- [ ] Generated files регенерируются byte-for-byte.
- [ ] Generated files не содержат ручных semantic corrections.
- [ ] Business logic находится в handwritten service/handlers/adapters.
- [ ] Mapping между generated types и domain types написан явно.
- [ ] Candidate scripts не подменяют canonical generation.
- [ ] Handwritten код не обходит generated repository через посторонний SQL.

Expected generated files включают:

```text
internal/httpapi/generated.gen.go
internal/repository/generated/db.go
internal/repository/generated/models.go
internal/repository/generated/querier.go
internal/repository/generated/tasks.sql.go
```

Не все задачи обязательно меняют каждый generated file, но существующие outputs должны быть canonical.

# Nullable PATCH Review

## 12. Требуемое Поведение

Проверить оба nullable solutions:

- [ ] Existing и newly created rows начинают с `dueAt = null`.
- [ ] Каждая Task response содержит обязательный member `dueAt`.
- [ ] Unset value кодируется как JSON `null`.
- [ ] `POST /tasks` продолжает принимать только `title`.
- [ ] `POST` с `dueAt` отклоняется.
- [ ] `PATCH /tasks/{taskId}` поддерживает `title`, `dueAt` или оба поля.
- [ ] Пустой PATCH отклоняется.
- [ ] Unknown fields отклоняются.
- [ ] `title: null` отклоняется.
- [ ] Unknown task возвращает `404`.
- [ ] Invalid input возвращает `400`.
- [ ] Success возвращает `200` и обновлённую Task.

## 13. Три Состояния Nullable

Это центральная часть review.

Reviewer должен найти request representation и проследить весь путь до SQL:

```text
JSON request
-> HTTP request type
-> handler
-> service input
-> repository operation
-> database state
-> response mapping
```

Проверить:

| JSON input | Ожидаемая операция |
|---|---|
| `dueAt` отсутствует | Сохранить текущее значение |
| `"dueAt": null` | Очистить значение |
| `"dueAt": "<timestamp>"` | Установить новое значение |

Убедиться, что omitted и null не сводятся к одному Go zero value.

Для Codegen solution отдельно проверить использование generated nullable type и отсутствие ручной правки generated representation.

## 14. Nullable Database Consistency

Проверить:

- [ ] Set сохраняет timestamp в PostgreSQL.
- [ ] Clear записывает SQL `NULL`.
- [ ] Omitted не изменяет колонку.
- [ ] Response после PATCH соответствует database row.
- [ ] Следующий GET возвращает то же состояние.
- [ ] Invalid PATCH не изменяет row.
- [ ] Timestamp нормализуется в UTC с шестью fractional digits.

## 15. Nullable Решение

Reviewer записывает:

```text
nullable-patch-direct: approved | changes-required
nullable-patch-codegen: approved | changes-required
pair behaviorally equivalent: yes | no
```

# Optimistic Locking Review

## 16. ETag Поведение

Проверить оба locking solutions:

- [ ] Version начинается с `1`.
- [ ] Task JSON содержит integer `version`.
- [ ] `POST` возвращает ETag `"1"`.
- [ ] Item `GET` возвращает ETag, соответствующий version.
- [ ] Успешный `PUT` возвращает новый ETag.
- [ ] Collection не обязана иметь ETag.
- [ ] Response ETag всегда strong.
- [ ] Response ETag не содержит leading zeros.

## 17. If-Match Parsing

Допустимо ровно одно strong integer ETag:

```text
"[1-9][0-9]*"
```

Reviewer проверяет rejection следующих форм:

```text
"0"
"+1"
"-1"
"01"
"9223372036854775808"
1
W/"1"
*
"1", "2"
два отдельных If-Match headers
```

Ожидаемые statuses:

| Ситуация | Status |
|---|---:|
| Missing `If-Match` | `428` |
| Malformed `If-Match` | `400` |
| Valid ETag, unknown task | `404` |
| Valid stale ETag | `412` |
| Valid current ETag | `200` |

Проверить precedence:

```text
header syntax
-> resource existence
-> version match
```

## 18. Atomic Update

Reviewer должен найти canonical update query.

Операция должна быть эквивалентна:

```sql
UPDATE tasks
SET title = $new_title,
    version = version + 1
WHERE id = $id
  AND version = $expected_version
RETURNING ...;
```

Недопустим небезопасный алгоритм:

```text
SELECT version
if version matches:
    UPDATE without version predicate
```

Проверить:

- [ ] Match и update выполняются одной atomic database operation.
- [ ] Version increment выполняется в той же operation.
- [ ] При stale ETag row не изменяется.
- [ ] Два concurrent request с одним ETag не могут оба победить.
- [ ] Final database version увеличивается только один раз.
- [ ] Final title принадлежит единственному winner.
- [ ] Loser получает `412`.

## 19. Locking Решение

Reviewer записывает:

```text
optimistic-locking-direct: approved | changes-required
optimistic-locking-codegen: approved | changes-required
pair behaviorally equivalent: yes | no
```

# Cursor Pagination Review

## 20. Основное Поведение

Проверить:

- [ ] `limit` optional.
- [ ] Default limit равен `20`.
- [ ] Minimum limit равен `1`.
- [ ] Maximum limit равен `100`.
- [ ] Invalid limit возвращает `400`.
- [ ] Cursor optional.
- [ ] Malformed cursor возвращает `400`.
- [ ] Cursor рассматривается HTTP layer как opaque value.
- [ ] Response envelope содержит `items`.
- [ ] `nextCursor` присутствует только при наличии следующей страницы.
- [ ] На final page `nextCursor` отсутствует.
- [ ] `nextCursor` не равен JSON `null`.
- [ ] `nextCursor` не равен пустой строке.

## 21. Stable Ordering

Canonical order:

```text
createdAt ASC, id ASC
```

Reviewer должен проверить SQL `ORDER BY` и cursor predicate.

Правильная операция должна быть эквивалентна:

```sql
WHERE (created_at, id) > ($cursor_created_at, $cursor_id)
ORDER BY created_at ASC, id ASC
LIMIT $limit_plus_one;
```

Допустима эквивалентная развёрнутая форма:

```sql
WHERE created_at > $cursor_created_at
   OR (
       created_at = $cursor_created_at
       AND id > $cursor_id
   )
```

Недостаточно использовать только:

```sql
WHERE created_at > $cursor_created_at
```

Такой вариант теряет rows с одинаковым timestamp.

## 22. Cursor Encoding

Проверить:

- [ ] Cursor содержит `createdAt` и `id`.
- [ ] Cursor сериализуется как JSON.
- [ ] JSON кодируется unpadded Base64URL.
- [ ] Cursor decode проверяет структуру и значения.
- [ ] Invalid Base64 отклоняется.
- [ ] Invalid JSON отклоняется.
- [ ] Invalid timestamp отклоняется.
- [ ] Invalid UUID отклоняется.
- [ ] Cursor формируется из последнего возвращённого item.
- [ ] Internal cursor fields не раскрываются как отдельные query parameters.

## 23. Duplicates И Gaps

Мысленно проверить последовательность:

```text
row 1: timestamp A, id 1
row 2: timestamp A, id 2
row 3: timestamp A, id 3
row 4: timestamp B, id 4
```

При `limit=2` ожидается:

```text
page 1: id 1, id 2
page 2: id 3, id 4
```

Reviewer подтверждает:

- [ ] Timestamp ties разрешаются через UUID.
- [ ] Первый item следующей страницы строго больше cursor tuple.
- [ ] Последний item предыдущей страницы не повторяется.
- [ ] Первый пропущенный item не исчезает.
- [ ] Pagination завершается.
- [ ] Final page не выдаёт лишний cursor.
- [ ] Удаление rows после cursor не делает cursor невалидным.

## 24. Pagination Решение

Reviewer записывает:

```text
cursor-pagination-direct: approved | changes-required
cursor-pagination-codegen: approved | changes-required
pair behaviorally equivalent: yes | no
```

# Findings И Итог

## 25. Классификация Findings

Использовать два уровня:

| Severity | Значение |
|---|---|
| `blocking` | Solution нельзя считать canonical |
| `non-blocking` | Улучшение желательно, но semantic correctness не нарушена |

Blocking findings включают:

- нарушение task requirements;
- Direct/Codegen behavioral mismatch;
- неатомарный optimistic update;
- nullable state confusion;
- pagination duplicates или gaps;
- изменение canonical formal inputs;
- ручную правку generated files;
- generator usage в Direct;
- evaluator-specific hardcoding;
- HTTP/database inconsistency.

## 26. Формат Finding

Каждый finding оформляется так:

```text
ID: REVIEW-NNN
Severity: blocking | non-blocking
Solution: <directory name>
File: <relative path>
Line: <line or range>
Requirement: <requirement being checked>
Observation: <what is wrong or questionable>
Required change: <what must be corrected>
```

Пример:

```text
ID: REVIEW-001
Severity: blocking
Solution: optimistic-locking-direct
File: internal/repository/postgres/tasks.go
Line: 84-103
Requirement: exactly one concurrent request using the current ETag may succeed
Observation: version is read before an unconditional UPDATE
Required change: compare and increment version in one atomic SQL operation
```

Каждое решение `changes-required` должно ссылаться минимум на один finding.

## 27. Итоговый Report

Reviewer возвращает один заполненный Markdown-report. Рекомендуемое имя:

```text
canonical-solutions-review.md
```

Report должен быть самостоятельным документом: владелец эксперимента должен иметь возможность определить reviewer, проверенный revision, выполненные команды, решение по каждому solution, equivalence каждой пары, treatment constraints, findings и итоговое заключение без обращения к дополнительной переписке.

Reviewer предоставляет отчёт следующего формата:

```markdown
# Canonical Task Solutions Independent Review

Reviewer: <name or stable reviewer identifier>
Review date: <YYYY-MM-DD>
Reviewed revision: <full Git SHA from git rev-parse HEAD>
Relevant experience: <short description>

## Independence Statement

I did not author the canonical solutions under review.
I reviewed the implementation personally and did not delegate the final judgment to an AI system.

## Automated Checks

| Command | Result | Notes |
|---|---|---|
| make validate-task-targets | passed/failed/not-run | |
| make verify-task-solutions | passed/failed/not-run | |
| make evaluate-task-solutions | passed/failed/not-run | |

## Solution Decisions

| Solution | Decision | Blocking findings |
|---|---|---|
| nullable-patch-direct | approved/changes-required | |
| nullable-patch-codegen | approved/changes-required | |
| optimistic-locking-direct | approved/changes-required | |
| optimistic-locking-codegen | approved/changes-required | |
| cursor-pagination-direct | approved/changes-required | |
| cursor-pagination-codegen | approved/changes-required | |

## Pair Equivalence

| Pair | Equivalent | Notes |
|---|---|---|
| Nullable PATCH | yes/no | |
| Optimistic Locking | yes/no | |
| Cursor Pagination | yes/no | |

## Treatment Constraints

| Check | Result | Notes |
|---|---|---|
| Direct solutions contain no generator usage or generated implementation | confirmed/rejected | |
| Codegen generated files regenerate byte-for-byte | confirmed/rejected | |
| Codegen generated files contain no manual semantic edits | confirmed/rejected | |
| Formal inputs match within each Direct/Codegen pair | confirmed/rejected | |

## Findings

<Findings in REVIEW-NNN format, or "No findings">

## Final Conclusion

All six canonical task solutions are suitable as experiment references: yes/no

Remaining concerns:

<text or "None">
```

Обязательные правила заполнения:

- `Reviewed revision` содержит полный 40-character Git SHA, а не branch name или сокращённый hash.
- Для каждой automated command указывается `passed`, `failed` или `not-run`.
- Для `failed` и `not-run` обязательно указывается причина в `Notes`.
- Все шесть solutions присутствуют в таблице решений.
- Каждая Direct/Codegen пара имеет отдельное решение об equivalence.
- Каждый `changes-required` ссылается хотя бы на один finding.
- Если findings отсутствуют, записывается `No findings`.
- Final Conclusion содержит однозначное `yes` или `no`.
- Неопределённые placeholders из шаблона не должны оставаться в готовом report.

Report можно передать одним из способов:

- отдельным файлом `canonical-solutions-review.md` владельцу эксперимента;
- GitHub Issue с полным неизменённым текстом report;
- отдельной branch или pull request, содержащими только report;
- другим каналом, сохраняющим полный Markdown без потери file/line references.

Reviewer не изменяет canonical solutions, `ROADMAP.md` или experiment configuration. Если report передаётся через branch или pull request, единственным содержательным изменением должен быть review report.

## 28. Что Именно Возвращается Владельцу

Reviewer передаёт:

1. Заполненный `canonical-solutions-review.md`.
2. Полный reviewed Git SHA.
3. Результат каждой из трёх automated commands или объяснение `not-run`.
4. Шесть отдельных решений `approved` или `changes-required`.
5. Три отдельных решения об equivalence Direct/Codegen пар.
6. Подтверждение Direct, Codegen и formal-input constraints.
7. Все findings с severity и file/line references либо явное `No findings`.
8. Финальный ответ, пригодны ли все шесть solutions как experiment references.

Не требуется передавать:

- локальные Docker images или containers;
- generated evaluator results, если они не нужны для объяснения failure;
- копию repository;
- credentials;
- `.env`;
- Task 6 OCI artifacts;
- pilot или measured-run artifacts.

Если automated command завершилась ошибкой, reviewer прикладывает только достаточный diagnostic excerpt без secrets и больших runtime artifacts.

## 29. Условия Успешного Review

Task 8 можно закрыть только если:

- все шесть solutions имеют решение `approved`;
- все три пары признаны behaviorally equivalent;
- Direct constraints подтверждены;
- Codegen constraints подтверждены;
- formal inputs подтверждены;
- нет unresolved blocking findings;
- reviewer явно заявил независимость;
- указан проверенный Git revision.

Non-blocking findings допустимы, если reviewer явно подтверждает, что они не делают solution некорректным.

## 30. Если Найдена Проблема

Reviewer не должен исправлять solution самостоятельно.

Нужно:

1. Записать finding.
2. Отметить solution как `changes-required`.
3. Передать report владельцу эксперимента.
4. Дождаться исправления.
5. Проверить новый diff.
6. Повторно проверить affected solution.
7. Записать finding как `resolved` или оставить открытым.
8. Указать новый reviewed revision.

Task 8 остаётся открытым до повторного подтверждения.

Если исправление меняет task semantics, OpenAPI, migrations или canonical SQL, reviewer должен отдельно отметить это. Такие изменения требуют более широкой revalidation и не должны выполняться как обычная локальная правка implementation.

## 31. После Успешного Review

Reviewer передаёт итоговый report владельцу эксперимента.

После этого:

- report добавляется как repository evidence;
- `ROADMAP.md` обновляется;
- Task 8 отмечается завершённым;
- automated checks запускаются повторно;
- изменения коммитятся отдельным commit;
- pilots всё ещё не начинаются автоматически.
