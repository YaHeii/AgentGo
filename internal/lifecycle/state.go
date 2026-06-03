package lifecycle

// State is the process-wide runtime state singleton.
//
// NOTE: State is intentionally public and mutable. It is not concurrency-safe.
var State *GlobalState

// GlobalState is the process-wide runtime state singleton.
//
// NOTE: This object is intentionally mutable and publicly writable.
// Direct field access like `lifecycle.State.Model = "x"` is allowed by design.
// It is not concurrency-safe and callers must not assume locked access.
type GlobalState struct {
	AppVersion              string
	StartTime               string
	Cwd                     string
	ProjectRoot             string
	PermissionLevel         PermissionLevel
	SessionID               string
	ParentSessionID         string
	InitialEnv              map[string]string
	ModelLimit              int
	MaxTurn                 int
	Model                   string
	CumulativeInputTokens   int
	CumulativeOutputTokens  int
	CumulativeTotalTokens   int
	CurrentTurnInputTokens  int
	CurrentTurnOutputTokens int
	CurrentTurnTotalTokens  int
	EstimatedContextTokens  int
	ActualContextTokens     int
	EstimatedContextChars   int
	CurrentMessageCount     int
	Temperature             float32
}

type PermissionLevel int

const (
	SafeLevel PermissionLevel = iota
	AttentionLevel
	DangerLevel
)

// NOTE: copy from Claudecode Uncomment when using a field
// originalCwd: string
// // Stable project root - set once at startup (including by --worktree flag),
// // never updated by mid-session EnterWorktreeTool.
// // Use for project identity (history, skills, sessions) not file operations.
// projectRoot: string
// totalCostUSD: number
// totalAPIDuration: number
// totalAPIDurationWithoutRetries: number
// totalToolDuration: number
// turnHookDurationMs: number
// turnToolDurationMs: number
// turnClassifierDurationMs: number
// turnToolCount: number
// turnHookCount: number
// turnClassifierCount: number
// startTime: number
// lastInteractionTime: number
// totalLinesAdded: number
// totalLinesRemoved: number
// hasUnknownModelCost: boolean
// cwd: string
// modelUsage: { [modelName: string]: ModelUsage }
// mainLoopModelOverride: ModelSetting | undefined
// initialMainLoopModel: ModelSetting
// modelStrings: ModelStrings | null
// isInteractive: boolean
// kairosActive: boolean
// // When true, ensureToolResultPairing throws on mismatch instead of
// // repairing with synthetic placeholders. HFI opts in at startup so
// // trajectories fail fast rather than conditioning the model on fake
// // tool_results.
// strictToolResultPairing: boolean
// sdkAgentProgressSummariesEnabled: boolean
// userMsgOptIn: boolean
// clientType: string
// sessionSource: string | undefined
// questionPreviewFormat: 'markdown' | 'html' | undefined
// flagSettingsPath: string | undefined
// flagSettingsInline: Record<string, unknown> | null
// allowedSettingSources: SettingSource[]
// sessionIngressToken: string | null | undefined
// oauthTokenFromFd: string | null | undefined
// apiKeyFromFd: string | null | undefined
// // Telemetry state
// meter: Meter | null
// sessionCounter: AttributedCounter | null
// locCounter: AttributedCounter | null
// prCounter: AttributedCounter | null
// commitCounter: AttributedCounter | null
// costCounter: AttributedCounter | null
// tokenCounter: AttributedCounter | null
// codeEditToolDecisionCounter: AttributedCounter | null
// activeTimeCounter: AttributedCounter | null
// statsStore: { observe(name: string, value: number): void } | null
// sessionId: SessionId
// // Parent session ID for tracking session lineage (e.g., plan mode -> implementation)
// parentSessionId: SessionId | undefined
// // Logger state
// loggerProvider: LoggerProvider | null
// eventLogger: ReturnType<typeof logs.getLogger> | null
// // Meter provider state
// meterProvider: MeterProvider | null
// // Tracer provider state
// tracerProvider: BasicTracerProvider | null
// // Agent color state
// agentColorMap: Map<string, AgentColorName>
// agentColorIndex: number
// // Last API request for bug reports
// lastAPIRequest: Omit<BetaMessageStreamParams, 'messages'> | null
// // Messages from the last API request (ant-only; reference, not clone).
// // Captures the exact post-compaction, CLAUDE.md-injected message set sent
// // to the API so /share's serialized_conversation.json reflects reality.
// lastAPIRequestMessages: BetaMessageStreamParams['messages'] | null
// // Last auto-mode classifier request(s) for /share transcript
// lastClassifierRequests: unknown[] | null
// // CLAUDE.md content cached by context.ts for the auto-mode classifier.
// // Breaks the yoloClassifier → claudemd → filesystem → permissions cycle.
// cachedClaudeMdContent: string | null
// // In-memory error log for recent errors
// inMemoryErrorLog: Array<{ error: string; timestamp: string }>
// // Session-only plugins from --plugin-dir flag
// inlinePlugins: Array<string>
// // Explicit --chrome / --no-chrome flag value (undefined = not set on CLI)
// chromeFlagOverride: boolean | undefined
// // Use cowork_plugins directory instead of plugins (--cowork flag or env var)
// useCoworkPlugins: boolean
// // Session-only bypass permissions mode flag (not persisted)
// sessionBypassPermissionsMode: boolean
// // Session-only flag gating the .claude/scheduled_tasks.json watcher
// // (useScheduledTasks). Set by cronScheduler.start() when the JSON has
// // entries, or by CronCreateTool. Not persisted.
// scheduledTasksEnabled: boolean
// // Session-only cron tasks created via CronCreate with durable: false.
// // Fire on schedule like file-backed tasks but are never written to
// // .claude/scheduled_tasks.json — they die with the process. Typed via
// // SessionCronTask below (not importing from cronTasks.ts keeps
// // bootstrap a leaf of the import DAG).
// sessionCronTasks: SessionCronTask[]
// // Teams created this session via TeamCreate. cleanupSessionTeams()
// // removes these on gracefulShutdown so subagent-created teams don't
// // persist on disk forever (gh-32730). TeamDelete removes entries to
// // avoid double-cleanup. Lives here (not teamHelpers.ts) so
// // resetStateForTests() clears it between tests.
// sessionCreatedTeams: Set<string>
// // Session-only trust flag for home directory (not persisted to disk)
// // When running from home dir, trust dialog is shown but not saved to disk.
// // This flag allows features requiring trust to work during the session.
// sessionTrustAccepted: boolean
// // Session-only flag to disable session persistence to disk
// sessionPersistenceDisabled: boolean
// // Track if user has exited plan mode in this session (for re-entry guidance)
// hasExitedPlanMode: boolean
// // Track if we need to show the plan mode exit attachment (one-time notification)
// needsPlanModeExitAttachment: boolean
// // Track if we need to show the auto mode exit attachment (one-time notification)
// needsAutoModeExitAttachment: boolean
// // Track if LSP plugin recommendation has been shown this session (only show once)
// lspRecommendationShownThisSession: boolean
