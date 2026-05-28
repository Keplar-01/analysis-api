package docs

const SwaggerHTML = `<!doctype html>
<html lang="ru">
<head>
  <meta charset="utf-8">
  <title>analysis-api swagger</title>
  <link rel="stylesheet" href="https://unpkg.com/swagger-ui-dist@5/swagger-ui.css">
</head>
<body>
  <div id="swagger-ui"></div>
  <script src="https://unpkg.com/swagger-ui-dist@5/swagger-ui-bundle.js"></script>
  <script>
    window.ui = SwaggerUIBundle({
      url: '/swagger/openapi.json',
		  dom_id: '#swagger-ui',
		  persistAuthorization: true
    })
  </script>
</body>
</html>
`

const OpenAPIJSON = `{
  "openapi": "3.0.3",
  "info": {
    "title": "analysis-api-service",
    "version": "1.0.0",
    "description": "API гибридного анализа кеш-эффективности: статика + кэш-симуляция. Для локальной работы без core-api получите JWT через GET /api/v1/analysis/dev-token."
  },
  "components": {
    "securitySchemes": {
      "BearerAuth": {
        "type": "http",
        "scheme": "bearer",
        "bearerFormat": "JWT"
      }
    }
  },
  "security": [
    {
      "BearerAuth": []
    }
  ],
  "paths": {
    "/api/v1/analysis/dev-token": {
      "get": {
        "summary": "Получить JWT для работы без core-api",
        "description": "Возвращает готовый JWT-токен для локальной и гибридной интеграции с analysis-api. Используйте его в Swagger Authorize (без префикса Bearer) или в заголовке Authorization: Bearer <token>. Не требует core-api и учётных данных.",
        "security": [],
        "responses": {
          "200": {
            "description": "JWT token issued",
            "content": {
              "application/json": {
                "schema": {
                  "type": "object",
                  "properties": {
                    "token": { "type": "string" },
                    "expires_in": { "type": "integer", "format": "int64", "example": 86400 },
                    "user_id": { "type": "string" },
                    "email": { "type": "string", "example": "dev@analysis.local" },
                    "role": { "type": "string", "example": "admin" }
                  }
                }
              }
            }
          },
          "500": { "description": "Failed to issue token" }
        }
      }
    },
    "/health": {
      "get": {
        "summary": "Проверка живости сервиса",
        "security": [],
        "responses": {
          "200": {
            "description": "OK"
          }
        }
      }
    },
    "/api/v1/analysis/upload": {
      "post": {
        "summary": "Загрузить C-файл и запустить анализ",
		"description": "Загружает новый файл и создаёт задачу анализа. project_id опционален: если не передан, используется внутренний системный namespace. В ответе возвращается task c task_id и file_id.",
        "requestBody": {
          "required": true,
          "content": {
            "multipart/form-data": {
              "schema": {
                "type": "object",
				"required": ["file"],
                "properties": {
                  "project_id": {
                    "type": "string",
					"description": "Идентификатор проекта; можно не передавать"
                  },
                  "file": {
                    "type": "string",
                    "format": "binary",
                    "description": "Исходный C-файл"
                  },
                  "l1_size_kb": { "type": "integer", "format": "int32" },
                  "l1_line_size": { "type": "integer", "format": "int32" },
                  "l1_associativity": { "type": "integer", "format": "int32" },
                  "l2_size_kb": { "type": "integer", "format": "int32" },
                  "l2_line_size": { "type": "integer", "format": "int32" },
                  "l2_associativity": { "type": "integer", "format": "int32" }
                }
              }
            }
          }
        },
        "responses": {
          "202": { "description": "Analysis started; response contains created task with file_id and task_id" },
          "400": { "description": "Bad request" },
          "500": { "description": "Failed to start analysis" }
        }
      }
    },
    "/api/v1/analysis/files/{file_id}/content": {
      "get": {
        "summary": "Текст исходника по file_id",
        "parameters": [
          {
            "name": "file_id",
            "in": "path",
            "required": true,
            "schema": { "type": "string" }
          }
        ],
        "responses": {
          "200": {
            "description": "Исходник как text/plain",
            "content": {
              "text/plain": {
                "schema": { "type": "string" }
              }
            }
          },
          "404": { "description": "Файл не найден или мягко удалён" }
        }
      }
    },
    "/api/v1/analysis/files/{file_id}": {
      "delete": {
        "summary": "Мягко скрыть файл (не удалять объект MinIO)",
        "description": "Устанавливает deleted_at. Скрытые файлы не попадают в список проекта; повторная загрузка того же содержимого может восстановить запись.",
        "parameters": [
          {
            "name": "file_id",
            "in": "path",
            "required": true,
            "schema": { "type": "string" }
          }
        ],
        "responses": {
          "204": { "description": "Файл скрыт (или уже был скрыт)" },
          "403": { "description": "Нет прав удалить файл" },
          "404": { "description": "file_id не найден" }
        }
      }
    },
    "/api/v1/analysis/files/{file_id}/analyze": {
      "post": {
        "summary": "Запустить анализ для уже загруженного файла",
        "description": "Создаёт новую задачу анализа поверх существующего file_id без повторной загрузки файла. В ответе возвращается task c тем же file_id и новым task_id.",
        "parameters": [
          {
            "name": "file_id",
            "in": "path",
            "required": true,
            "schema": { "type": "string" }
          }
        ],
        "requestBody": {
          "required": false,
          "content": {
            "application/x-www-form-urlencoded": {
              "schema": {
                "type": "object",
                "properties": {
                  "l1_size_kb": { "type": "integer", "format": "int32" },
                  "l1_line_size": { "type": "integer", "format": "int32" },
                  "l1_associativity": { "type": "integer", "format": "int32" },
                  "l2_size_kb": { "type": "integer", "format": "int32" },
                  "l2_line_size": { "type": "integer", "format": "int32" },
                  "l2_associativity": { "type": "integer", "format": "int32" }
                }
              }
            }
          }
        },
        "responses": {
          "202": { "description": "Analysis started for existing file; response contains file_id and new task_id" },
          "400": { "description": "Bad request" },
          "404": { "description": "File not found" },
          "500": { "description": "Failed to start analysis" }
        }
      }
    },
    "/api/v1/analysis/tasks/{task_id}/metrics": {
      "get": {
        "summary": "Сводные метрики задачи по всем уровням кэша",
        "description": "Возвращает массив levels с метриками и optimization_score для каждого уровня кэша (L1/L2/L3). Источник — cache-out.json в MinIO.",
        "parameters": [
          {
            "name": "task_id",
            "in": "path",
            "required": true,
            "schema": { "type": "string" }
          }
        ],
        "responses": {
          "200": {
            "description": "Task metrics",
            "content": {
              "application/json": {
                "schema": {
                  "type": "object",
                  "properties": {
                    "task_id": { "type": "string" },
                    "status": { "type": "string" },
                    "levels": {
                      "type": "array",
                      "items": {
                        "type": "object",
                        "properties": {
                          "cache_level": { "type": "string", "example": "L1" },
                          "total_memory_accesses": { "type": "integer", "format": "int64" },
                          "cache_hits": { "type": "integer", "format": "int64" },
                          "cache_misses": { "type": "integer", "format": "int64" },
                          "hit_rate": { "type": "number", "format": "double" },
                          "miss_rate": { "type": "number", "format": "double" },
                          "optimization_score": { "type": "number", "format": "double", "description": "Hit rate уровня × 100" }
                        }
                      }
                    }
                  }
                }
              }
            }
          },
          "404": { "description": "Task not found" }
        }
      }
    },
    "/api/v1/analysis/files/{file_id}/metrics": {
      "get": {
        "summary": "Сводные метрики последней задачи по file_id",
        "parameters": [
          {
            "name": "file_id",
            "in": "path",
            "required": true,
            "schema": { "type": "string" }
          }
        ],
        "responses": {
          "200": { "description": "File metrics (same shape as task metrics)" },
          "404": { "description": "File or analysis results not found" }
        }
      }
    },
    "/api/v1/analysis/tasks/{task_id}": {
      "get": {
        "summary": "Статус задачи",
        "parameters": [
          {
            "name": "task_id",
            "in": "path",
            "required": true,
            "schema": { "type": "string" }
          }
        ],
        "responses": {
          "200": { "description": "Task status" },
          "404": { "description": "Task not found" }
        }
      }
    },
    "/api/v1/analysis/tasks/{task_id}/static-patterns": {
      "get": {
        "summary": "Статические паттерны задачи",
        "parameters": [
          {
            "name": "task_id",
            "in": "path",
            "required": true,
            "schema": { "type": "string" }
          }
        ],
        "responses": {
          "200": { "description": "Static patterns" },
          "404": { "description": "Task not found" }
        }
      }
    },
    "/api/v1/analysis/tasks/{task_id}/aggregated": {
      "get": {
        "summary": "Агрегированные статические и динамические метрики",
        "parameters": [
          {
            "name": "task_id",
            "in": "path",
            "required": true,
            "schema": { "type": "string" }
          }
        ],
        "responses": {
          "200": { "description": "Aggregated patterns" },
          "404": { "description": "Task not found" }
        }
      }
    },
    "/api/v1/analysis/files/{file_id}/simulation-results": {
      "get": {
        "summary": "Полные результаты симуляции по файлу",
        "description": "Возвращает последнюю задачу анализа для файла, общие cache-метрики и pattern-level результаты симуляции с misses по уровням кэша.",
        "parameters": [
          {
            "name": "file_id",
            "in": "path",
            "required": true,
            "schema": { "type": "string" }
          }
        ],
        "responses": {
          "200": { "description": "File simulation results with metrics and pattern-level cache misses" },
          "404": { "description": "File or analysis results not found" }
        }
      }
    },
    "/api/v1/analysis/admin/debug/static-patterns": {
      "post": {
        "summary": "Синхронный debug-запуск статического анализатора",
        "requestBody": {
          "required": true,
          "content": {
            "multipart/form-data": {
              "schema": {
                "type": "object",
                "required": ["file"],
                "properties": {
                  "file": {
                    "type": "string",
                    "format": "binary"
                  }
                }
              }
            }
          }
        },
        "responses": {
          "200": { "description": "Analyzer patterns" },
          "400": { "description": "Bad request" },
          "500": { "description": "Analyzer failed" }
        }
      }
    }
  }
}`
