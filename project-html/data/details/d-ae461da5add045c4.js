window.BOARD_DETAILS = window.BOARD_DETAILS || {};
window.BOARD_DETAILS["d-ae461da5add045c4"] = {
  "delivery": {
    "plan": {
      "status": "passed",
      "summary": "把 Python MCP 重构为 Go CLI 的受控 Agent 门面，以统一契约、安全计划和结构化审计提供企业级 Apifox 管理能力。",
      "background": "当前仓库同时维护 Go CLI 和遗留 Python MCP，两者分别实现 Apifox/OpenAPI 调用，工具输出也主要面向人类文本。随着 Agent 直接管理企业接口文档，重复实现会带来行为漂移，缺少统一 dry-run、写权限、结构化错误和审计还会放大外部写入风险。",
      "goals": [
        "提供 Agent 可稳定发现和解析的强类型 MCP 工具",
        "让所有 Apifox 行为复用 Go CLI 的结构化命令契约",
        "用服务端 plan/apply 策略、一次性计划和脱敏审计约束真实写入",
        "支持本地 stdio，并为受控 Streamable HTTP 部署提供安全基座"
      ],
      "scopeIn": [
        "MCP 服务工厂、CLI gateway、结构化模型、写策略、计划存储和审计",
        "接口、Schema、标签、审计和批量文档的正式 Agent 工具面",
        "stdio 与受控 HTTP 启动、Docker、CI、测试和兼容迁移"
      ],
      "scopeOut": [
        "新增第二套直接调用 Apifox HTTP 的业务实现",
        "实现公开 API 不支持的接口、Schema 或目录删除",
        "首阶段公网部署和自建 OAuth 授权服务器"
      ],
      "solution": "MCP 层只负责协议、校验、权限和观测，通过安全异步子进程把结构化请求从 stdin 交给 apifox-cli。读取工具直接返回 CLI JSON；变更先强制 dry-run 并冻结为有 TTL 的计划，只有显式 apply 模式才能用计划 ID 执行。SDK lifespan 统一注入配置、gateway、计划存储和审计器，避免 import-time 全局状态。",
      "dataFlowSummary": "Agent MCP 请求进入强类型工具后，先完成参数和权限校验；读取请求直接交给 CLI gateway，变更请求先生成 dry-run 计划并冻结 payload hash；获准 apply 时消费一次性计划，再由 apifox-cli 调用官方 Apifox API；CLI JSON 被转换为稳定 MCP 结果，同时写入不含 Token 和业务 payload 的审计事件。",
      "coreDesign": "以 CLI 为单一执行核心消除双实现漂移，以默认 plan-only 和服务端冻结计划控制外部副作用。ToolAnnotations 只用于客户端提示，真正授权由服务端 write mode 和计划校验强制执行。远程 HTTP 默认关闭，只有认证验证器和 Host/Origin 白名单完整时才允许非 loopback 绑定。",
      "keyImpl": [
        {
          "title": "双实现漂移",
          "desc": "问题是 Python MCP 与 Go CLI 各自拼装 OpenAPI；选择 MCP 通过 stdin 调用 CLI；原因是复用现有 validation、dry-run、structured JSON 和官方 import/export 路径。"
        },
        {
          "title": "Agent 误写外部系统",
          "desc": "问题是 confirm 布尔值可被 Agent 自行填写；选择 disabled/plan/apply 服务端策略和一次性计划 ID；原因是让写授权与 payload 冻结成为可验证门禁。"
        },
        {
          "title": "工具结果不稳定",
          "desc": "问题是旧工具主要返回自由文本；选择 Pydantic input/output 和统一 error envelope；原因是方便 Agent、脚本和测试按 schema 消费。"
        },
        {
          "title": "凭据与审计风险",
          "desc": "问题是子进程、错误和日志可能带出 Token 或业务数据；选择 env allowlist、stdin、集中脱敏和写前 fail-closed 审计；原因是保护企业凭据并保留追责证据。"
        },
        {
          "title": "远程传输暴露",
          "desc": "问题是 Streamable HTTP 可能被误绑定公网；选择 stdio 默认、loopback 默认、认证和 Host/Origin 门禁；原因是先满足本地 Agent，再逐步接入企业 IdP。"
        }
      ],
      "flowchart": "flowchart TD\n    A[Agent MCP Client] --> B[Typed MCP Tool]\n    B --> C[Validation and Policy]\n    C --> D{Read or Change}\n    D -->|Read| E[CliGateway]\n    D -->|Plan| F[CLI dry-run]\n    F --> G[Freeze plan ID and payload hash]\n    D -->|Apply| H{Valid one-time plan}\n    H -->|No| I[Structured policy error]\n    H -->|Yes| E\n    G --> E\n    E --> J[apifox-cli JSON]\n    J --> K[Official Apifox API]\n    J --> L[Structured MCP result]\n    L --> M[Redacted audit event]",
      "acceptance": [
        "无凭据和网络即可完成工具发现与全部 Hermetic 测试",
        "默认 plan 模式不能触发真实 Apifox 写入",
        "计划篡改、过期、跨项目和重复消费均返回稳定错误码",
        "stdio stdout 不含普通日志，Token 不进入参数、结果或审计",
        "Go CLI 原有测试和命令契约保持通过"
      ],
      "next": "按方案进入 Implementation Gate，先完成 SDK v2 spike、结构化模型和 CliGateway。"
    }
  }
};
