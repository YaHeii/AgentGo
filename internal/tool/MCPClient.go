package tool

// TODO: Transport Layer Abstraction: 
// Do not limit yourself to a Stdio-only transport implementation. Instead, define a generic `Transport` 
// interface to enable seamless switching to SSE (Server-Sent Events) or WebSockets in the future.

//Asynchronous Safety: MCP requests must support Context cancellation. When the LLM's decision changes or 
// the user forcibly interrupts the process, the corresponding tool process must be terminated immediately.

//Automatic Schema Mapping: Leverage Go's struct tags or the `reflect` package to automatically generate the JSON Schemas 
// required for MCP, thereby avoiding inconsistencies that arise from manually maintaining two separate sets of definitions.