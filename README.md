# Apifox MCP Server

[![Python](https://img.shields.io/badge/Python-3.10%2B-blue?logo=python&logoColor=white)](https://www.python.org/)
[![Go](https://img.shields.io/badge/Go-CLI-00ADD8?logo=go&logoColor=white)](https://go.dev/)
[![uv](https://img.shields.io/badge/uv-Compatible-purple?logo=python&logoColor=white)](https://docs.astral.sh/uv/)
[![MCP](https://img.shields.io/badge/MCP-Compatible-green?logo=data:image/svg+xml;base64,PHN2ZyB4bWxucz0iaHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmciIHZpZXdCb3g9IjAgMCAyNCAyNCI+PHBhdGggZmlsbD0id2hpdGUiIGQ9Ik0xMiAyQzYuNDggMiAyIDYuNDggMiAxMnM0LjQ4IDEwIDEwIDEwIDEwLTQuNDggMTAtMTBTMTcuNTIgMiAxMiAyek0xMiAyMGMtNC40MSAwLTgtMy41OS04LThzMy41OS04IDgtOCA4IDMuNTkgOCA4LTMuNTkgOC04IDh6Ii8+PC9zdmc+)](https://modelcontextprotocol.io/)
[![License](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![Apifox](https://img.shields.io/badge/Apifox-Integration-orange?logo=swagger&logoColor=white)](https://apifox.com/)

---

这是一个基于 [Model Context Protocol (MCP)](https://modelcontextprotocol.io/) 的服务器，用于通过 LLM (如 Claude) 直接管理 [Apifox](https://apifox.com/) 项目。

它允许你通过自然语言指令来查看、创建、更新和删除 Apifox 中的 API 接口、数据模型 (Schema)、文件夹等，并能检查 API 定义的完整性。

## ✨ 功能特性

*   **API 接口管理**:
    *   列出接口 (`list_api_endpoints`)
    *   获取接口详情 (`get_api_endpoint_detail`)
    *   创建接口 (`create_api_endpoint`) - 自动处理标准错误响应
    *   更新接口 (`update_api_endpoint`)
    *   删除接口 (`delete_api_endpoint`)
    *   接口完整性检查 (`check_api_responses`, `audit_all_api_responses`)
*   **数据模型 (Schema) 管理**:
    *   列出模型 (`list_schemas`)
    *   获取模型详情 (`get_schema_detail`)
    *   创建模型 (`create_schema`)
    *   更新模型 (`update_schema`)
    *   删除模型 (`delete_schema`)
*   **其他管理**:
    *   目录管理 (`list_folders`, `create_folder`, `delete_folder`)
    *   标签管理 (`list_tags`)
    *   按标签获取接口 (`get_apis_by_tag`, `add_tag_to_api`)
    *   配置检查 (`check_apifox_config`)

## 🛠️ 安装

CLI 使用 Go 编写，MCP server 仍使用 Python。开发环境需要 Go 1.22+ 和 Python 3.10+。

1.  **克隆项目**
    ```bash
    git clone <repository_url>
    cd <repository_name>
    ```

2.  **构建 Go CLI**

    **Homebrew (发布后推荐)**

    ```bash
    brew tap iwen-conf/tap
    brew install --cask apifox-mcp
    apifox-mcp --version
    ```

    **源码构建**

    ```bash
    go build -o ./bin/apifox-mcp ./cmd/apifox-mcp
    ./bin/apifox-mcp --help
    ```

    如果希望安装到 `PATH`：

    ```bash
    go install ./cmd/apifox-mcp
    apifox-mcp --help
    ```

3.  **安装 Python MCP server 依赖**

    **使用 uv**
    ```bash
    uv venv
    uv sync
    uv run python -m apifox_mcp.main
    ```

    **使用 pip (传统方式)**
    ```bash
    pip install -e .
    python -m apifox_mcp.main
    ```

## ⚙️ 配置

在使用前，你需要设置以下环境变量来连接你的 Apifox 项目。

| 环境变量 | 描述 | 获取方式 |
| :--- | :--- | :--- |
| `APIFOX_TOKEN` | Apifox 开放 API 令牌 | Apifox 客户端 -> 账号设置 -> API 访问令牌 |
| `APIFOX_PROJECT_ID` | 目标项目 ID | 项目概览页 -> 项目设置 -> 基本设置 -> ID |

## 💻 使用方法 (CLI + Skill)

构建 Go CLI 后会得到 `apifox-mcp` 命令。它直接调用 Apifox 官方开放 API，适合给 Codex Skill 或普通脚本使用；MCP server 继续用 Python 模块运行。

```bash
# 检查配置和连接
apifox-mcp call check_apifox_config

# 查看可用工具和参数
apifox-mcp list-tools
apifox-mcp describe create_api_endpoint
apifox-mcp versions

# 读取接口、模型、标签
apifox-mcp call list_api_endpoints --param limit=20
apifox-mcp call get_api_endpoint_detail --param path=/orders --param method=GET
apifox-mcp call list_schemas --param limit=20

# AI 批量编写 Apifox/OpenAPI 接口文档（推荐主流程）
apifox-mcp docs-template -o .apifox-docs.json
apifox-mcp validate-docs --file .apifox-docs.json
apifox-mcp apply-docs --file .apifox-docs.json --dry-run
apifox-mcp apply-docs --file .apifox-docs.json

# 单个接口文档
apifox-mcp endpoint-template --method POST -o .apifox-endpoint.json
apifox-mcp validate-endpoint --file .apifox-endpoint.json
apifox-mcp create-endpoint --file .apifox-endpoint.json --dry-run
apifox-mcp create-endpoint --file .apifox-endpoint.json
apifox-mcp upsert-endpoint --file .apifox-endpoint.json

# AI 结构化生成一组 RESTful CRUD 接口文档
apifox-mcp crud-template -o .apifox-crud.json
apifox-mcp validate-crud --file .apifox-crud.json
apifox-mcp generate-crud --file .apifox-crud.json --dry-run
apifox-mcp generate-crud --file .apifox-crud.json

# 官方开放 API 导入导出（迁移、备份或兼容场景；不是 AI 写文档主流程）
apifox-mcp export-openapi --format JSON --oas-version 3.1 -o .apifox-openapi.json
apifox-mcp export-openapi --scope tags --tag 订单管理 --format YAML -o .apifox-orders.yaml
apifox-mcp import-openapi --file .apifox-openapi.json --endpoint-overwrite-behavior AUTO_MERGE
apifox-mcp import-openapi --url https://example.com/openapi.yaml --prepend-base-path
apifox-mcp import-postman --file .postman-collection.json

# 标准化调试：先预览请求，不调用 Apifox
apifox-mcp export-openapi --scope tags --tag 订单管理 --dry-run
apifox-mcp import-openapi --file .apifox-openapi.json --print-payload

# 调用未来新增的 Apifox /v1 开放 API
apifox-mcp request GET /versions --json
apifox-mcp request POST /projects/123/export-openapi --data-file .apifox-export-payload.json

# 复杂参数建议写到隐藏 JSON 文件
apifox-mcp call create_api_endpoint --args-file .apifox-create-order.json
```

凭证默认读取环境变量，也可以在命令中传入：

```bash
apifox-mcp --token "$APIFOX_TOKEN" --project-id "$APIFOX_PROJECT_ID" call check_apifox_config
```

仓库内置 Codex Skill：`skills/apifox-cli`。需要让 Codex 自动发现时，可将该目录放到 `$CODEX_HOME/skills` 或 `~/.codex/skills` 下；在仓库内使用时，也可以直接让 Codex 使用这个路径下的 `$apifox-cli` Skill。

> 对 Go 项目，推荐让 AI 从代码、路由、DTO、校验逻辑和需求文档中提取接口信息，写入 `.apifox-docs.json`、`.apifox-endpoint.json` 或 `.apifox-crud.json`，再通过 CLI 写入 Apifox。不建议把 Swagger 文档维护在 Go 标记注释中作为主流程，也不建议把“导入已有 OpenAPI 文件”当成主流程。

### AI 文档规格

`.apifox-docs.json` 是面向 AI 的批量文档输入格式。`endpoints` 中每个接口支持 `action` 字段，取值为 `upsert`、`create` 或 `update`，默认推荐 `upsert`；`crud` 中每个对象会调用 CRUD 批量生成。

```json
{
  "endpoints": [
    {
      "action": "upsert",
      "title": "创建订单",
      "path": "/orders",
      "method": "POST",
      "description": "创建订单，需要用户已登录",
      "tags": ["订单管理"],
      "request_body_schema": {
        "type": "object",
        "properties": {
          "item_id": {"type": "integer", "description": "商品ID"},
          "quantity": {"type": "integer", "description": "购买数量"}
        },
        "required": ["item_id", "quantity"]
      },
      "request_body_example": {"item_id": 1001, "quantity": 2},
      "response_schema": {
        "type": "object",
        "properties": {
          "order_id": {"type": "integer", "description": "订单ID"},
          "status": {"type": "string", "description": "订单状态"}
        },
        "required": ["order_id", "status"]
      },
      "response_example": {"order_id": 90001, "status": "pending"}
    }
  ],
  "crud": []
}
```

## 重点⚠️
### APIFOX_TOKEN获取方式
<img width="1594" height="1029" alt="截屏2025-12-17 01 58 51" src="https://github.com/user-attachments/assets/aad5da36-a99d-484b-959c-116918897487" />


### APIFOX_PROJECT_ID获取方式

<img width="2032" height="1167" alt="截屏2025-12-17 01 57 06" src="https://github.com/user-attachments/assets/a381baf8-7da0-4d88-950c-ac8b78c7af8d" />


### 设置项目文档为公开
ps:我实际使用发现只有设置为文档发布才能正常操作项目

 <img width="1594" height="1029" alt="截屏2025-12-17 01 55 12" src="https://github.com/user-attachments/assets/59cb26ea-26af-47a4-8329-aabe4ec63bce" />



## ⚙️ 配置

在使用前，你需要获取以下凭证来连接你的 Apifox 项目。

| 环境变量 | 描述 | 获取方式 |
| :--- | :--- | :--- |
| `APIFOX_TOKEN` | Apifox 开放 API 令牌 | Apifox 客户端 -> 账号设置 -> API 访问令牌 |
| `APIFOX_PROJECT_ID` | 目标项目 ID | 项目概览页 -> 项目设置 -> 基本设置 -> ID |

## 🐳 使用方法 (Docker)

### 方法一：从源码构建

```bash
git clone https://github.com/iwen-conf/apifox-mcp.git
cd apifox-mcp
docker build -t apifox-mcp .
```

### 方法二：使用预构建镜像

从 [Releases](https://github.com/iwen-conf/apifox-mcp/releases) 下载 `apifox-mcp.tar`，然后加载：

```bash
docker load -i apifox-mcp.tar
```

### 配置 Claude Desktop

编辑 Claude Desktop 的配置文件：
- **macOS**: `~/Library/Application Support/Claude/claude_desktop_config.json`
- **Windows**: `%APPDATA%\Claude\claude_desktop_config.json`

#### 方式一：使用 Docker (推荐用于生产环境)

```json
{
  "mcpServers": {
    "apifox": {
      "command": "docker",
      "args": [
        "run",
        "-i",
        "--rm",
        "-e", "APIFOX_TOKEN",
        "-e", "APIFOX_PROJECT_ID",
        "apifox-mcp"
      ],
      "env": {
        "APIFOX_TOKEN": "your_token_here",
        "APIFOX_PROJECT_ID": "your_project_id_here"
      }
    }
  }
}
```

#### 方式二：使用 uv (推荐用于本地开发)

```json
{
  "mcpServers": {
    "apifox": {
      "command": "uv",
      "args": [
        "run",
        "--directory",
        "/path/to/apifox-mcp",
        "python",
        "-m",
        "apifox_mcp.main"
      ],
      "env": {
        "APIFOX_TOKEN": "your_token_here",
        "APIFOX_PROJECT_ID": "your_project_id_here"
      }
    }
  }
}
```

> **注意**: 
> - 请将 `your_token_here` 和 `your_project_id_here` 替换为你的实际凭证
> - 使用 uv 方式时，请将 `/path/to/apifox-mcp` 替换为实际的项目路径

### 3. 命令行运行 (可选)

你也可以直接在命令行中测试：

```bash
# 使用环境变量
docker run -i --rm \
  -e APIFOX_TOKEN=your_token \
  -e APIFOX_PROJECT_ID=your_project_id \
  apifox-mcp

# 或者使用 .env 文件
docker run -i --rm --env-file .env apifox-mcp
```

## 📝 编写规范

本工具在创建和更新接口时强制执行以下规范，以确保文档质量：

1.  **中文描述**: 必须提供中文的 `title` 和 `description`。
2.  **完整 Schema**: `response_schema` 和 `request_body_schema` 中的每个字段必须包含 `description`。
3.  **真实示例**: 示例数据 (`example`) 必须是真实值，不能是简单的类型占位符 (如 "string")。
4.  **错误响应**: 系统会自动为你补充标准的 4xx/5xx 错误响应，无需手动定义。

## 🤝 贡献

欢迎提交 Issue 和 Pull Request！
