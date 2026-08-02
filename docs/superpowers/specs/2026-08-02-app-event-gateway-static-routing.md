# App Event Gateway Static Routing

## Goal

- `app` 作为统一入口/出口网关，负责事件适配与路由
- 领域层只负责产生领域事件，不再直接依赖旧 `bus`
- 当前阶段只走内存通道，`DeliveryMQ` 仅保留路由语义，后续再接 RabbitMQ

## Event Model

- `EventType` 表示事件族：`session`、`message`、`agent`、`provider`
- `EventName` 表示具体事件：例如 `session.created`、`provider.text_delta`
- 领域事件构造函数放在各自事件文件中，业务代码不手写事件名
- `app.NewEvent(name, payload)` 根据静态 catalog 自动补齐 `EventType`

## Routing

- 路由表由 `internal/app` 的静态 catalog 维护
- 每个 `EventName` 映射到 `EventType + RoutePolicy`
- 当前所有事件默认 `DeliveryMemory`
- 不保留 `memoryEvent` 兼容路径
- 不再接受仅含 `EventType` 的旧事件作为发布输入

## Current Phase

- 已移除 `internal/bus`
- 已把 session / message / agent / provider 的发布点收口到领域构造函数
- `app.Dispatcher` 只负责内存广播订阅
