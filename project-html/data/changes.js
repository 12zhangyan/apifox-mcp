// ─── 研发变更档案目录 ────────────────────────────────────────────────────────
// yan-dev-doc 创建一事一档主记录；yan-code-review package/check/repair/loop 更新同一 deliveryId。
// bug-fix、code-reading、biz-flow 保留各自记录类型；渲染逻辑在 js/board.js。
// 数据只能通过 board-add.js 写入；模板保持空目录。

const htmlChangelog = [
  {
    "date": "2026-08-03",
    "desc": "外壳数据迁移：0 条记录拆分为轻量目录 + 人类方案详情"
  },
  {
    "date": "2026-08-03",
    "desc": "新增文档：企业级 Agent MCP"
  },
  // ─── 在此行上方追加变更日志 ───
];


// ─── 轻量目录数据 ────────────────────────────────────────────────────────────
// 首页、搜索和筛选只读取这里；人类方案正文位于 data/details/<detailId>.js，点击后加载。
// md 是 Agent 执行文档；看板详情是独立撰写的人类方案，不是 md 摘录。
// Agent 专属字段 changeList / todos / stackTrace / codeLocation 禁止进入目录和详情。
// 目录字段由 board-add.js 白名单控制；禁止手工整体重写本文件。
const changes = [
  {
    "updatedAt": "2026-08-03",
    "service": "apifox-mcp",
    "module": "MCP Server",
    "title": "企业级 Agent MCP",
    "date": "2026-08-03",
    "type": "重构",
    "complexity": "复杂",
    "status": "已完成",
    "branch": "main",
    "docPath": "docs/2026-08-03/enterprise-agent-mcp.md",
    "apis": [],
    "currentGate": "plan",
    "gateStatus": "passed",
    "deliveryId": "DLV-AE461DA5AD",
    "detailId": "d-ae461da5add045c4",
    "detailPath": "data/details/d-ae461da5add045c4.js",
    "summary": "把 Python MCP 重构为 Go CLI 的受控 Agent 门面，以统一契约、安全计划和结构化审计提供企业级 Apifox 管理能力。",
    "searchText": "企业级 Agent MCP apifox-mcp MCP Server 重构 把 Python MCP 重构为 Go CLI 的受控 Agent 门面，以统一契约、安全计划和结构化审计提供企业级 Apifox 管理能力。 passed 把 Python MCP 重构为 Go CLI 的受控 Agent 门面，以统一契约、安全计划和结构化审计提供企业级 Apifox 管理能力。 当前仓库同时维护 Go CLI 和遗留 Python MCP，两者分别实现 Apifox/OpenAPI 调用，工具输出也主要面向人类文本。随着 Agent 直接管理企业接口文档，重复实现会带来行为漂移，缺少统一 dry-run、写权限、结 提供 Agent 可稳定发现和解析的强类型 MCP 工具 让所有 Apifox 行为复用 Go CLI 的结构化命令契约 用服务端 plan/apply 策略、一次性计划和脱敏审计约束真实写入 支持本地 stdio，并为受控 Streamable HTTP 部署提供安全基座 MCP 服务工厂、CLI gateway、结构化模型、写策略、计划存储和审计 接口、Schema、标签、审计和批量文档的正式 Agent 工具面 stdio 与受控 HTTP 启动、Docker、CI、测试和兼容迁移 新增第二套直接调用 Apifox HTTP 的业务实现 实现公开 API 不支持的接"
  },
  // ─── 在此行上方追加新记录 ───
];
