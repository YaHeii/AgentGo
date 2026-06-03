# 项目级 AGENTS 规范整合设计

日期：2026-06-03

## 背景

当前仓库同时存在根级 `AGENTS.md` 与 `docs/development-conventions.md`。

前者更像协作入口，后者承载了较多项目级开发规范。两者并存后，项目规范真相源不够集中；同时 `docs/struct.md` 已经不存在，后续细节约束也计划下沉到 `docs/superpowers/specs/`。

因此需要把项目级、长期稳定的开发规范统一收敛到根级 `AGENTS.md`，并删除 `docs/development-conventions.md`。

## 目标

- 根级 `AGENTS.md` 成为当前唯一的项目级开发规范
- 子目录允许存在自己的 `AGENTS.md`，只补充或收紧该子树约定，并在作用域内优先生效
- 根级文档保持简短、抽象、可执行，不写具体实现细节或逐文件说明
- 阶段性实现细节与设计讨论，后续下沉到 `docs/superpowers/specs/`

## 文档边界

新的根级 `AGENTS.md` 只保留以下类型的规则：

- 目标与作用域
- 文档优先级与局部 `AGENTS.md` 规则
- 沟通、文档与流程约定
- 分层开发规范：
  - `service`
  - 文件命名与架构分布
  - 消费者侧接口
  - `contract`
  - 跨层事件
- 高层边界：
  - `app`
  - `lifecycle`
  - `ui`
- 开发与验证约定

以下内容不再保留在根级 `AGENTS.md` 中：

- 具体代码示例
- 当前实现细节
- 逐文件说明
- 容易频繁变化的局部交互规则

## 架构结论

- `app` 的定位是面向 `ui` 的 thin facade
- 非 UI 层之间的协作不默认经由 `app` 中转
- `app` 不应成为所有模块之间的统一服务入口
- 某层需要另一层能力时，应由消费者侧定义最小接口，并直接依赖必要的抽象和 `contract`

## 影响

- 根级 `AGENTS.md` 将吸收 `docs/development-conventions.md` 的核心内容
- `docs/development-conventions.md` 删除
- `internal/ui/AGENTS.md` 对项目级规范的引用需要从旧文档改到新的根级 `AGENTS.md`
