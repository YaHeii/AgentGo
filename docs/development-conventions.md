# agentGo 开发约定

日期：2026-05-11

## 目标

本文档只记录当前仓库应长期遵守的开发约定。

它不承担项目结构总览职责；项目结构、分层现状和运行链路请看
[docs/struct.md](/root/agentGo/docs/struct.md)。

## 手动维护与抽象准则
1. 函数封装原则（Encapsulation Logic）
禁止冗余抽象：若逻辑仅涉及原子赋值或直接字段映射（Direct Field Mapping），禁止封装为 Helper 方法。

封装触发阈值：仅当逻辑满足以下任一条件时方可封装：

  包含条件分支控制流（Conditional Branching）。

  涉及跨领域类型转换或非琐碎的数据清洗。

  存在不可变性约束（Immutability Constraints）或需维护内部状态一致性。

判定基准：若去掉抽象后，原位逻辑的认知负荷（Cognitive Load）未显著增加且未导致代码重复，则该抽象为无效冗余。

2. 跨层协作与接口定义规范（Cross-layer Collaboration）
既有实现优先：在发起跨层协作前，必须检索目标层（Target Layer）是否已存在功能等价的实现。若存在，应通过接口扩展（Interface Extension）将其纳入当前层的依赖抽象中，禁止重复实现。

  最小接口原则（Interface Segregation）：若目标层缺失必要逻辑，应遵循以下步骤：

  职责划分：在目标层定义满足该逻辑的最小功能单元（Atomic Implementation）。

  依赖注入：在本层定义满足业务意图的消费者接口（Consumer Interface）。

  解耦引用：通过接口引用目标层实现，确保物理包依赖方向始终指向抽象而非具体实现。

3. 文件命名与架构分布规范 (File Naming & Structure)
领域接口定义（Domain Interface）：在各包（Package）中，与包同名的文件（如 session/session.go）仅允许定义该领域的 核心接口（Interfaces） 与 基础数据模型（Models/Structs）。严禁在此文件中包含具体的业务逻辑实现。

服务实现命名（Service Implementation）：具体的业务逻辑实现必须放置于以 _service.go 后缀命名的文件中（如 session_service.go）。该文件应包含：

具体的结构体定义（通常为私有，如 type sessionService struct）。

对本包内核心接口及 跨层依赖接口 的完整实现。

事件驱动契约（Event Contract）：

定义载体：包内所有业务事件的负载结构体（Payloads）必须集中定义在 events.go 中。

通信约束：Service 结构体必须在构造阶段（New 函数）强制注入 app.Dispatcher 接口实例，并作为其核心字段。

发布职责：Service 在完成状态变更（如持久化成功）后，必须通过该 Dispatcher 发布对应的业务事件，确保系统的异步解耦能力。

## 1. 每层通过 service 暴露对外方法

每个领域层对外应以 service 方法作为唯一入口。

也就是说：

- `session` 对外暴露 session 相关用例
- `message` 对外暴露 message 相关用例
- `agent` 对外暴露 query 相关用例
- `app` 只暴露少量面向 UI 的 facade 方法

不推荐的做法：

- UI 直接调用 `db`
- `app` 直接操作 `sqlc` 生成代码
- 上层绕过 service 直接依赖下层具体实现

## 2. 每层通过消费者侧接口定义依赖

当一层需要依赖另一层时，应在消费者侧定义最小接口，而不是直接依赖具体类型。

当前约定是：

- 对外暴露能力靠 service
- 对内声明依赖靠 store/interface

例如：

- `session` 定义自己的 `sessionStore`
- `message` 定义自己的 `messageStore`
- `agent` 定义自己消费的 conversation/session interface
- `app` 只定义自己需要的最小依赖接口

核心原则是：

- 接口定义在消费者侧
- 接口只保留最小能力集
- 不为了“将来可能会用”提前做大接口

### 2.1 contract 包使用规范

当某一层在解耦过程中，出现“必须跨越模块边界、且至少被两个独立模块共同使用”的纯结构体或窄契约时，可以在该层下建立 `contract` 子包。

- 严格的零依赖原则（Zero Dependency）
单向终结：contract 包处于依赖拓扑的最底层，绝对禁止 import 项目内的任何其他业务包或外领域的实现包。

被动汇聚：上层调用方和本层底层实现方，都必须单向 import 该 contract 包。

在数据结构与领域模型上，解耦（Decoupling）的优先级永远高于不要重复自己（DRY - Don't Repeat Yourself）绝不允许一个结构体穿透所有架构层。在不同包中定义独立的结构体,
- 结构体纯洁性要求（Tech Stack Agnosticism）
拒绝框架污染：结构体内仅保留业务字段。严禁携带底层框架的侵入式标签（如 gorm:"primaryKey" 或与特定数据库绑定的 DB Tag）或特定网络协议的标签（除非该 contract 就是为特定网关层设计的 DTO，但在核心领域模型中应避免）。

- 无资源句柄：结构体中严禁包含数据库连接指针（如 *sql.DB）、文件句柄或网络会话对象。

- 强制的下层防腐义务（Mandatory Mapping）
隔离内部模型：下层模块（如数据库访问层）在内部必须使用私有的数据对象（DO/PO）与底层基础设施交互。

- 阻断泄漏：下层在实现 contract 定义的接口时，必须在自身模块内部完成数据转换（Mapping）。对外返回数据时，必须将底层的私有 DO 转换为 contract 中定义的纯结构体，将底层细节死死封锁在实现层内部。

## 3. 跨层事件统一通过 app.Dispatcher 发送

当前统一事件通道是 `app.Dispatcher`。

约定如下：

- `Dispatcher` 在 `main.go` 的启动期 wiring 中创建
- 通过依赖注入传给各 service / query loop
- 下层不自行创建新的全局事件总线
- UI 只订阅统一事件流，不分别订阅多个领域通道

推荐模式：

```go
dispatcher := app.NewDispatcher(128)
messageSvc := message.NewMessageService(st, dispatcher)
sessionSvc := session.NewSessionService(st, messageSvc, dispatcher)
agentSvc := agent.NewQueryLoop(sessionSvc, llm, dispatcher)
```

## 4. 事件结构定义放在各层自己的 events.go 中

每个领域层自己的事件载荷、状态枚举、事件字段，应定义在本层自己的
`events.go` 或 `event.go` 中。

例如：

- `internal/session/event.go`
- `internal/message/events.go`
- `internal/agent/events.go`
- `internal/lifecycle/events.go`

这里定义的是领域事件的真实 payload，而不是在 `app` 层再复制一份 DTO。

## 5. app/events.go 维护统一事件契约

`internal/app/events.go` 负责维护跨层统一事件协议，而不是维护每个领域的业务字段。

它至少应作为以下内容的真相源：

- `EventType`
- `Event` interface
- `BaseEvent`
- 各领域对应的统一事件类别常量

因此，新增或调整领域事件时，应同步检查 `app/events.go` 是否需要更新：

- 如果新增了新的领域事件类别，需要在 `app/events.go` 增加对应 `EventType`
- 如果只是修改某个领域事件 payload 字段，则在该领域自己的 `events.go` 修改
- 不要在 `app/events.go` 再复制一遍 `session/message/agent` 的 payload struct

一句话：

- 领域字段归各层自己的 `events.go`
- 统一事件分类归 `app/events.go`

## 6. wiring 只放在 main.go

当前约定：

- `app` 不负责 new 下层 service
- `ui` 不负责装配 provider/db/session/message/agent
- `main.go` 作为 composition root，负责完整依赖拼装

也就是说，concrete wiring 的唯一入口应是 `main.go`。

`internal/lifecycle` 当前只负责：

- 全局 `State` 单例
- 全局 `CurrentSupervisor` 单例
- 启动期的 runtime state 初始化
- 基于统一事件流的上下文/token 用量监控

注意：

- `lifecycle.State` 允许直接字段写入，例如 `lifecycle.State.Model = "x"`
- 这是当前明确接受的设计选择
- `State` 不是并发安全对象，调用方不得假设其带锁


## 8. app 层保持 thin facade

`app` 当前应保持 thin facade。

这意味着：

- `app` 不再拥有独立的 `Session` / `Message` / `QueryState` DTO
- `app` 不再做“把下层事件重新翻译成另一套结构”的映射层
- `app` 只维护统一事件外壳和面向 UI 的少量动作入口
**app层仅依赖其他层的contract, 定义其他层的服务接口,向ui层提供服务.**
**各层想要使用其他层的服务均需要从app中引入, 另外contract可以直接用,**