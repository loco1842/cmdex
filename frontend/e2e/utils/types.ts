// Type declaration for the test seed global injected via page.addInitScript.
// The `declare global` augments the global Window interface so the cast is
// unnecessary at call sites in addInitScript callbacks (which are serialized
// to the browser context and cannot close over module-scope helpers).

export interface CmdexE2ETerminalSession {
    id: string;
    name: string;
    running: boolean;
    shellPath: string;
    workingDir: string;
}

export interface CmdexE2ESeed {
    categories?: Array<Record<string, unknown>>;
    commands?: Array<Record<string, unknown>>;
    presets?: Record<string, Array<Record<string, unknown>>>;
    settings?: Record<string, unknown>;
    terminalSessions?: CmdexE2ETerminalSession[];
}

// Keep in sync with the method names in e2e/mocks/runtime.ts's METHOD_IDS.
export type CmdexE2EMethodName =
    | 'GetCategories'
    | 'CreateCategory'
    | 'UpdateCategory'
    | 'DeleteCategory'
    | 'GetCommands'
    | 'GetCommandsByCategory'
    | 'CreateCommand'
    | 'UpdateCommand'
    | 'DeleteCommand'
    | 'RenameCommand'
    | 'ReorderCommand'
    | 'GetScriptBody'
    | 'GetScriptContent'
    | 'SearchCommands'
    | 'GetPresets'
    | 'SavePreset'
    | 'UpdatePreset'
    | 'DeletePreset'
    | 'ReorderPresets'
    | 'GetSettings'
    | 'SetSettings'
    | 'GetVariables'
    | 'RunCommand'
    | 'ExportCommands'
    | 'ImportCommands'
    | 'SaveThemeTemplate'
    | 'CreateSession'
    | 'ListSessions'
    | 'GetActiveSession'
    | 'SetActiveSession'
    | 'CloseSession'
    | 'RenameSession'
    | 'Start'
    | 'Stop'
    | 'Write'
    | 'Resize'
    | 'Clear'
    | 'GetLastOutput'
    | 'GetEventNames'
    | 'GetOS'
    | 'PickDirectory'
    | 'ShowSettingsWindow'
    | 'ResetAllData';

declare global {
    interface Window {
        __cmdexE2E_SEED__?: CmdexE2ESeed;
        // Exposed by the runtime mock so tests can drive Wails events
        // (e.g. `settings-changed`, normally emitted by the settings window),
        // inject faults, and inspect what was called.
        __cmdexE2E?: {
            reset(): void;
            seed(data: CmdexE2ESeed): void;
            // Deliver a raw payload as a frontend-originated cross-window
            // emit would — the mock wraps it in the WailsEvent envelope.
            emit(eventName: string, data: unknown): void;
            // Simulate backend-emitted per-session terminal events.
            emitPtyOutput(sessionId: string, data: string): void;
            emitPtyExit(sessionId: string, exitCode: number, wasIntentional: boolean): void;
            emitPtyCleared(sessionId: string): void;
            hasListener(eventName: string): boolean;
            // Call counters for TerminalService methods — see
            // terminal.spec.ts for the regressions these guard against.
            terminalCallCounts: {
                CreateSession: number;
                Start: number;
                Write: number;
                Resize: number;
                Clear: number;
            };
            // One-shot / sticky fault injection. Refused for methods the
            // real backend can never fail (log-and-return-empty reads, and
            // RunCommand — see NEVER_REJECTS in runtime.ts).
            failNext(method: CmdexE2EMethodName, message?: string): void;
            setFailure(method: CmdexE2EMethodName, message: string | null): void;
            // Every dispatched call in order, `{ method, args }`.
            callLog: Array<{ method: CmdexE2EMethodName; args: unknown[] }>;
            clearCallLog(): void;
            // Configure ImportCommands' next successful result (default:
            // null, i.e. the user cancelled the dialog).
            setImportResult(result: Array<Record<string, unknown>> | null): void;
            // Configure PickDirectory's return value ('' simulates cancel).
            setPickDirectoryResult(path: string): void;
            // Configure GetLastOutput's next return value.
            setLastOutput(data: { available: boolean; text: string; exitCode: number; truncated: boolean }): void;
        };
    }
}

export {};
