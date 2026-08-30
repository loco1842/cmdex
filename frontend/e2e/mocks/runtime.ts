// Mock @wailsio/runtime for Playwright E2E tests.
// Replaces the Wails IPC bridge with an in-memory backend.

/* eslint-disable @typescript-eslint/no-explicit-any */

// ── Method IDs ───────────────────────────────────────────────────────────
// Wails' generated bindings call `Call.ByID(<numeric hash>, ...args)`. The
// hash is derived from the Go method name and carries no meaning of its own,
// so it is named here once; everything below (dispatch, fault injection,
// the call log) addresses methods by name. `mock-contract.spec.ts` asserts
// this table stays in sync with `frontend/bindings/cmdex/*.js` — if you
// regenerate bindings and a method's ID changes, that test fails loudly
// instead of the historical silent `[e2e mock] no handler for method ID`.
const METHOD_IDS = {
  GetCategories: 1124386808,
  CreateCategory: 3920645540,
  UpdateCategory: 871939973,
  DeleteCategory: 2228038743,
  GetCommands: 2230805162,
  GetCommandsByCategory: 3544855671,
  CreateCommand: 3040387109,
  UpdateCommand: 2942553414,
  DeleteCommand: 1888656992,
  RenameCommand: 3511040027,
  ReorderCommand: 2371488912,
  GetScriptBody: 707578151,
  GetScriptContent: 1214515992,
  SearchCommands: 2165520554,
  GetPresets: 2933858456,
  SavePreset: 2518009278,
  UpdatePreset: 1219258890,
  DeletePreset: 1347137556,
  ReorderPresets: 4123798965,
  GetSettings: 3034808949,
  SetSettings: 287946425,
  GetVariables: 4101005934,
  RunCommand: 4143621145,
  ExportCommands: 3360644818,
  ImportCommands: 840325137,
  SaveThemeTemplate: 1489453142,
  CreateSession: 2914743863,
  ListSessions: 1878753896,
  GetActiveSession: 1716291155,
  SetActiveSession: 4186280199,
  CloseSession: 3305403653,
  RenameSession: 911254629,
  Start: 3774501399,
  Stop: 2555571709,
  Write: 371925220,
  Resize: 270758441,
  Clear: 493646090,
  GetLastOutput: 4011093730,
  GetEventNames: 2407475739,
  GetOS: 816844233,
  PickDirectory: 1347829059,
  ShowSettingsWindow: 2596981913,
  ResetAllData: 121210722,
} as const;

export type MethodName = keyof typeof METHOD_IDS;

const ID_TO_NAME = new Map<number, MethodName>(
  (Object.entries(METHOD_IDS) as [MethodName, number][]).map(([name, id]) => [id, name]),
);

// Methods the real Go backend logs-and-returns-empty (or, for RunCommand,
// never rejects at all — failures are in-band ExitCode:-1 records). Fault
// injection against these would test a path that cannot happen in
// production, so failNext/setFailure refuse and warn instead of silently
// no-oping.
const NEVER_REJECTS = new Set<MethodName>([
  'GetCategories',
  'GetCommands',
  'GetCommandsByCategory',
  'SearchCommands',
  'GetPresets',
  'GetVariables',
  'GetSettings',
  'RunCommand',
]);

let categories: any[] = [];
let commands: any[] = [];
const presets: Record<string, any[]> = {};
const DEFAULT_SETTINGS: Record<string, any> = {
  locale: 'en',
  theme: 'vscode-dark',
  lastDarkTheme: 'vscode-dark',
  lastLightTheme: 'vscode-light',
  customThemes: '[]',
  uiFont: 'Inter',
  monoFont: 'JetBrains Mono',
  density: 'comfortable',
};
const settings: Record<string, any> = { ...DEFAULT_SETTINGS };

function resetSettings() {
  Object.keys(settings).forEach((k) => delete settings[k]);
  Object.assign(settings, DEFAULT_SETTINGS);
}

let nextId = 0;
function uid() {
  return `mock-${++nextId}-${Math.random().toString(36).slice(2, 8)}`;
}

const now = () => new Date().toISOString();

// ── Fault injection ──────────────────────────────────────────────────────
const failureMap = new Map<MethodName, { message: string; once: boolean }>();

function failNext(method: MethodName, message = 'Mock injected failure') {
  if (NEVER_REJECTS.has(method)) {
    console.warn(`[e2e mock] ${method} cannot fail in production (log-and-return-empty); ignoring failNext`);
    return;
  }
  failureMap.set(method, { message, once: true });
}

function setFailure(method: MethodName, message: string | null) {
  if (message === null) {
    failureMap.delete(method);
    return;
  }
  if (NEVER_REJECTS.has(method)) {
    console.warn(`[e2e mock] ${method} cannot fail in production (log-and-return-empty); ignoring setFailure`);
    return;
  }
  failureMap.set(method, { message, once: false });
}

// ── Call log ─────────────────────────────────────────────────────────────
// Every dispatched call, in order, including args — lets a test assert e.g.
// "Write was called with this exact sessionId" without the mock needing a
// bespoke counter per method.
const callLog: Array<{ method: MethodName; args: any[] }> = [];

// ── Import / Export / PickDirectory configurables ───────────────────────
// The real backend returns nil/("", nil) on a cancelled native dialog
// (importexport_service.go:82-84, :187-189; app.go:79). Default to that
// cancel semantic — a test opts into "the user picked a file" explicitly.
let pendingImportResult: any[] | null = null;
let pickDirectoryResult = '/mock/path';
let lastOutputResult: { available: boolean; text: string; exitCode: number; truncated: boolean } = {
  available: false,
  text: '',
  exitCode: 0,
  truncated: false,
};

// ── Terminal ─────────────────────────────────────────────────────────────
// Mirrors the real backend's actual behavior (TerminalService.ServiceStartup
// and CreateSession both synchronously start the shell before returning) so
// tests can catch a regression of the bug fixed in Terminal.tsx: every
// mount used to unconditionally call Start() even when the backend had
// already started the session, tearing down the healthy PTY and spawning a
// redundant second shell. See callCounts below.
const MAX_SESSIONS = 10;
let terminalSessions: Array<{
  id: string;
  name: string;
  running: boolean;
  shellPath: string;
  workingDir: string;
}> = [];
let activeTerminalSessionId: string | null = null;
let terminalSessionCounter = 0;
const terminalCallCounts = { CreateSession: 0, Start: 0, Write: 0, Resize: 0, Clear: 0 };
const ptyOutputSeq: Record<string, number> = {};

function terminalUid() {
  terminalSessionCounter++;
  return `term-${terminalSessionCounter}`;
}

function createMockTerminalSession() {
  terminalCallCounts.CreateSession++;
  const info = {
    id: terminalUid(),
    name: `Terminal ${terminalSessionCounter}`,
    running: true,
    shellPath: '/bin/mock-shell',
    workingDir: '/mock/path',
  };
  terminalSessions.push(info);
  if (!activeTerminalSessionId) activeTerminalSessionId = info.id;
  return info;
}

// Empty sessionId means "the active session" for Write/Resize/Clear/Start/
// Stop, mirroring the real backend's resolveSession (terminal_service.go:208).
function resolveSessionId(id: string): string | null {
  return id || activeTerminalSessionId;
}

const eventListeners: Record<string, Array<(event: any) => void>> = {};

export const Events = {
  On(eventName: string, callback: (event: any) => void) {
    if (!eventListeners[eventName]) eventListeners[eventName] = [];
    eventListeners[eventName].push(callback);
    return () => {
      const list = eventListeners[eventName];
      if (list) {
        const idx = list.indexOf(callback);
        if (idx >= 0) list.splice(idx, 1);
      }
    };
  },
  // Real Wails always wraps an emitted payload in a `WailsEvent`
  // ({ name, data, sender }) before delivering it to listeners — the app
  // relies on this and unwraps `.data` (App.tsx's settings-changed handler,
  // Terminal.tsx's pty-* handlers). Every emit, whether it originates here
  // (simulated backend events below) or from `__cmdexE2E.emit` (simulating
  // a frontend cross-window emit), goes through this same wrap so both
  // paths are faithful to production.
  Emit(eventName: string, data: any) {
    const event = { name: eventName, data, sender: 'e2e-mock' };
    (eventListeners[eventName] || []).forEach((fn) => fn(event));
  },
  Off(eventName: string) {
    delete eventListeners[eventName];
  },
};

export const Create = {
  Any(source: any) {
    return source;
  },
  ByteSlice(source: any) {
    return source == null ? '' : source;
  },
  Array:
    (element: (source: any) => any) =>
    (source: any[]) => {
      if (source === null) return [];
      if (element === Create.Any) return source;
      for (let i = 0; i < source.length; i++) {
        source[i] = element(source[i]);
      }
      return source;
    },
  Map:
    (_key: any, value: (source: any) => any) =>
    (source: any) => {
      if (source === null) return {};
      if (value === Create.Any) return source;
      for (const k in source) {
        source[k] = value(source[k]);
      }
      return source;
    },
  Nullable:
    (element: (source: any) => any) =>
    (source: any) => {
      if (element === Create.Any) return Create.Any;
      return source === null ? null : element(source);
    },
  Struct:
    (_createField: Record<string, (source: any) => any>) =>
    (source: any) => {
      return source;
    },
};

function findCommand(id: string) {
  return commands.find((c) => c.id === id);
}

// Seed convenience: a test may seed a command with an embedded `.presets`
// array instead of the separate top-level `presets` map. Populate the
// `presets` store from it (without clobbering an explicit top-level seed,
// which is applied first) so GetPresets and withLivePresets both see it.
function seedPresetsFromCommands(cmds: any[]) {
  for (const cmd of cmds) {
    if (cmd.presets?.length && !presets[cmd.id]) {
      presets[cmd.id] = cmd.presets;
    }
  }
}

// The real backend joins a command's presets in at read time
// (db.go's GetCommand/GetCommands), so SavePreset/UpdatePreset/DeletePreset/
// ReorderPresets — which mutate the separate `presets` store, keyed by
// command id — are immediately reflected the next time that command is
// fetched. Every command-returning handler below must overlay `presets`
// from that store rather than trust a command object's own (possibly
// stale) `.presets` field.
function withLivePresets<T extends { id: string; presets?: unknown }>(cmd: T): T {
  return { ...cmd, presets: presets[cmd.id] ?? cmd.presets ?? [] };
}

const handlersByName: Record<MethodName, (...args: any[]) => any> = {
  // ── Categories ──────────────────────────────────────────
  GetCategories: () => categories,

  CreateCategory: (name: string, icon: string, color: string) => {
    const cat = {
      id: uid(),
      name,
      icon: icon || '',
      color: color || '#7c6aef',
      createdAt: now(),
      updatedAt: now(),
    };
    categories.push(cat);
    return cat;
  },

  UpdateCategory: (id: string, name: string, icon: string, color: string) => {
    const idx = categories.findIndex((c) => c.id === id);
    if (idx < 0) throw new Error('Category not found');
    categories[idx] = { ...categories[idx], name, icon, color, updatedAt: now() };
    return categories[idx];
  },

  DeleteCategory: (id: string) => {
    categories = categories.filter((c) => c.id !== id);
    commands.forEach((cmd) => {
      if (cmd.categoryId === id) cmd.categoryId = '';
    });
  },

  // ── Commands ─────────────────────────────────────────────
  GetCommands: () => commands.map(withLivePresets),

  GetCommandsByCategory: (categoryID: string) =>
    commands.filter((c) => (c.categoryId || '') === (categoryID || '')).map(withLivePresets),

  CreateCommand: (
    title: string,
    description: string,
    scriptBody: string,
    categoryID: string,
    tags: string[],
    variables: any[],
    workingDir: any,
  ) => {
    const cmd = {
      id: uid(),
      title: { String: title || '', Valid: !!title },
      description: { String: description || '', Valid: !!description },
      scriptContent: scriptBody || '',
      tags: tags || [],
      variables: variables || [],
      presets: [],
      workingDir: workingDir || {},
      categoryId: categoryID || '',
      position: commands.length,
      createdAt: now(),
      updatedAt: now(),
    };
    commands.push(cmd);
    return withLivePresets(cmd);
  },

  UpdateCommand: (
    id: string,
    title: string,
    description: string,
    scriptBody: string,
    categoryID: string,
    tags: string[],
    variables: any[],
    workingDir: any,
  ) => {
    const idx = commands.findIndex((c) => c.id === id);
    if (idx < 0) throw new Error('Command not found');
    commands[idx] = {
      ...commands[idx],
      title: { String: title || '', Valid: !!title },
      description: { String: description || '', Valid: !!description },
      scriptContent: scriptBody || '',
      tags: tags || [],
      variables: variables || [],
      workingDir: workingDir || {},
      categoryId: categoryID || '',
      updatedAt: now(),
    };
    return withLivePresets(commands[idx]);
  },

  DeleteCommand: (id: string) => {
    commands = commands.filter((c) => c.id !== id);
    delete presets[id];
  },

  RenameCommand: (id: string, newTitle: string) => {
    const idx = commands.findIndex((c) => c.id === id);
    if (idx < 0) throw new Error('Command not found');
    commands[idx] = {
      ...commands[idx],
      title: { String: newTitle, Valid: true },
      updatedAt: now(),
    };
    return withLivePresets(commands[idx]);
  },

  ReorderCommand: (id: string, newPosition: number, newCategoryId: string) => {
    const cmd = findCommand(id);
    if (cmd) {
      cmd.position = newPosition;
      cmd.categoryId = newCategoryId || '';
    }
    return commands.map(withLivePresets);
  },

  GetScriptBody: (commandID: string) => {
    const cmd = findCommand(commandID);
    if (!cmd) return '';
    return cmd.scriptContent.replace(/^#!.*\n/, '');
  },

  GetScriptContent: (commandID: string) => {
    const cmd = findCommand(commandID);
    return cmd ? cmd.scriptContent : '';
  },

  SearchCommands: (query: string) => {
    if (!query) return commands.map(withLivePresets);
    const q = query.toLowerCase();
    return commands
      .filter(
        (c) =>
          (c.title.String || '').toLowerCase().includes(q) ||
          c.scriptContent.toLowerCase().includes(q),
      )
      .map(withLivePresets);
  },

  // ── Presets ──────────────────────────────────────────────
  GetPresets: (commandID: string) => presets[commandID] || [],

  SavePreset: (commandID: string, name: string, values: Record<string, string>) => {
    if (!presets[commandID]) presets[commandID] = [];
    const preset = { id: uid(), name, position: presets[commandID].length, values: values || {} };
    presets[commandID].push(preset);
    return preset;
  },

  UpdatePreset: (commandID: string, presetID: string, name: string, values: Record<string, string>) => {
    const list = presets[commandID] || [];
    const idx = list.findIndex((p) => p.id === presetID);
    if (idx < 0) throw new Error('Preset not found');
    list[idx] = { ...list[idx], name, values: values || {} };
    return list[idx];
  },

  DeletePreset: (commandID: string, presetID: string) => {
    if (presets[commandID]) {
      presets[commandID] = presets[commandID].filter((p) => p.id !== presetID);
    }
  },

  ReorderPresets: (commandID: string, presetIDs: string[]) => {
    if (!presets[commandID]) return;
    const map = new Map(presets[commandID].map((p) => [p.id, p]));
    presets[commandID] = presetIDs
      .map((id, i) => {
        const p = map.get(id);
        if (p) p.position = i;
        return p;
      })
      .filter(Boolean);
  },

  // ── Settings ─────────────────────────────────────────────
  GetSettings: () => ({ ...settings }),

  SetSettings: (jsonStr: string) => {
    try {
      Object.assign(settings, JSON.parse(jsonStr));
    } catch {
      /* ignore */
    }
  },

  // ── Execution ────────────────────────────────────────────
  GetVariables: (commandID: string) => {
    const cmd = findCommand(commandID);
    if (!cmd) return [];
    return (cmd.variables || []).map((v: any) => ({
      name: v.name,
      placeholder: v.description || v.name,
      description: v.description || '',
      example: v.example || '',
      defaultExpr: v.default || '',
      defaultValue: v.default || '',
    }));
  },

  // RunCommand never rejects in production — failures are in-band
  // ExitCode:-1 records (execution_service.go:119-171). This mock always
  // takes the success path; fault-injecting it would model an impossible
  // rejection, so it's in NEVER_REJECTS above.
  RunCommand: (commandID: string, variables: Record<string, string>) => {
    const cmd = findCommand(commandID);
    const record = {
      id: uid(),
      commandId: commandID,
      scriptContent: cmd?.scriptContent || '',
      finalCmd: 'echo mock execution',
      output: `Mock output: ${JSON.stringify(variables)}`,
      error: '',
      exitCode: 0,
      workingDir: '',
      executedAt: now(),
    };
    return record;
  },

  // ── Import / Export ──────────────────────────────────────
  ExportCommands: () => {},

  // Returns null on "cancel" (the default) — matching ImportCommands'
  // (nil, nil) on a cancelled dialog. A test that wants a successful import
  // must call __cmdexE2E.setImportResult([...]) first. On success this
  // mirrors the real backend by returning the full post-import command list
  // (db.GetCommands(), importexport_service.go:230), not just the imported
  // rows.
  ImportCommands: () => {
    if (pendingImportResult === null) return null;
    commands = commands.concat(pendingImportResult);
    pendingImportResult = null;
    return commands;
  },

  SaveThemeTemplate: () => {},

  // ── Terminal ─────────────────────────────────────────────
  CreateSession: () => {
    if (terminalSessions.length >= MAX_SESSIONS) {
      throw new Error(`CreateSession: max sessions reached (${MAX_SESSIONS})`);
    }
    return createMockTerminalSession();
  },

  ListSessions: () => terminalSessions,

  GetActiveSession: () => terminalSessions.find((s) => s.id === activeTerminalSessionId) || null,

  SetActiveSession: (id: string) => {
    activeTerminalSessionId = id;
  },

  CloseSession: (id: string) => {
    terminalSessions = terminalSessions.filter((s) => s.id !== id);
    if (activeTerminalSessionId === id) {
      activeTerminalSessionId = terminalSessions[0]?.id || null;
    }
  },

  RenameSession: (id: string, name: string) => {
    const s = terminalSessions.find((s) => s.id === id);
    if (s) s.name = name;
  },

  Start: (sessionId: string) => {
    terminalCallCounts.Start++;
    const s = terminalSessions.find((s) => s.id === sessionId);
    if (s) s.running = true;
  },

  Stop: (sessionId: string) => {
    const s = terminalSessions.find((s) => s.id === sessionId);
    if (s) s.running = false;
  },

  Write: (_sessionId: string, _data: string) => {
    terminalCallCounts.Write++;
  },

  Resize: (_sessionId: string, _cols: number, _rows: number) => {
    terminalCallCounts.Resize++;
  },

  // Emits pty-cleared:<id> (payload nil), matching terminal_service.go:906.
  Clear: (sessionId: string) => {
    terminalCallCounts.Clear++;
    const id = resolveSessionId(sessionId);
    if (id) Events.Emit(`pty-cleared:${id}`, null);
  },

  GetLastOutput: () => ({ ...lastOutputResult }),

  // ── Events ───────────────────────────────────────────────
  GetEventNames: () => ({
    openSettings: 'open-settings',
    openShortcuts: 'open-shortcuts',
    settingsChanged: 'settings-changed',
    settingsWindowClosing: 'settings-window-closing',
    dataReset: 'data-reset',
  }),

  // ── App ──────────────────────────────────────────────────
  GetOS: () => 'darwin',

  PickDirectory: () => pickDirectoryResult,

  ShowSettingsWindow: () => {},

  // ── Misc ─────────────────────────────────────────────────
  // db.ResetAll truncates app_settings too (db.go:1413-1422) — the frontend
  // relies on the data-reset event to reload settings, not just commands.
  ResetAllData: () => {
    categories = [];
    commands = [];
    Object.keys(presets).forEach((k) => delete presets[k]);
    resetSettings();
  },
};

export const Call = {
  ByID(id: number, ...args: any[]) {
    const name = ID_TO_NAME.get(id);
    if (!name) {
      console.warn(`[e2e mock] no handler for method ID ${id}`);
      return Promise.resolve(null);
    }
    callLog.push({ method: name, args });

    const failure = failureMap.get(name);
    if (failure) {
      if (failure.once) failureMap.delete(name);
      return Promise.reject(new Error(failure.message));
    }

    try {
      return Promise.resolve(handlersByName[name](...args));
    } catch (err) {
      return Promise.reject(err);
    }
  },
};

export class CancellablePromise<T> extends Promise<T> {
  cancel() {}
}

;(globalThis as any).__cmdexE2E = {
  reset() {
    categories = [];
    commands = [];
    Object.keys(presets).forEach((k) => delete presets[k]);
    resetSettings();
    nextId = 0;
    terminalSessions = [];
    activeTerminalSessionId = null;
    terminalSessionCounter = 0;
    terminalCallCounts.CreateSession = 0;
    terminalCallCounts.Start = 0;
    terminalCallCounts.Write = 0;
    terminalCallCounts.Resize = 0;
    terminalCallCounts.Clear = 0;
    Object.keys(ptyOutputSeq).forEach((k) => delete ptyOutputSeq[k]);
    failureMap.clear();
    callLog.length = 0;
    pendingImportResult = null;
    pickDirectoryResult = '/mock/path';
    lastOutputResult = { available: false, text: '', exitCode: 0, truncated: false };
  },
  seed(data: {
    categories?: any[];
    commands?: any[];
    presets?: Record<string, any[]>;
    settings?: Record<string, any>;
    terminalSessions?: Array<{ id: string; name: string; running: boolean; shellPath: string; workingDir: string }>;
  }) {
    if (data.categories) categories = data.categories;
    if (data.commands) commands = data.commands;
    if (data.presets) Object.assign(presets, data.presets);
    if (data.commands) seedPresetsFromCommands(data.commands);
    // Settings are merged, not replaced, so a partial seed keeps the defaults
    // for every field it does not mention — same as the init-script path below.
    if (data.settings) Object.assign(settings, data.settings);
    if (data.terminalSessions) {
      terminalSessions = data.terminalSessions;
      activeTerminalSessionId = data.terminalSessions[0]?.id ?? null;
    }
    nextId = Math.max(
      ...categories.map((c) => parseInt(c.id) || 0),
      ...commands.map((c) => parseInt(c.id) || 0),
      0,
    );
  },
  // Deliver an event to the app as a frontend-originated cross-window emit
  // would (main.tsx / SettingsPage.tsx call Events.Emit with a raw payload,
  // and Events.Emit itself now builds the WailsEvent envelope) — so callers
  // here pass the raw payload, not a pre-wrapped one.
  emit(eventName: string, data: any) {
    Events.Emit(eventName, data);
  },
  // Simulate a backend-emitted pty-output:<id> event. `seq` auto-increments
  // per session, mirroring terminal_service.go's 1-based monotonic sequence.
  emitPtyOutput(sessionId: string, data: string) {
    ptyOutputSeq[sessionId] = (ptyOutputSeq[sessionId] || 0) + 1;
    Events.Emit(`pty-output:${sessionId}`, { data, seq: ptyOutputSeq[sessionId] });
  },
  // Simulate a backend-emitted pty-exit:<id> event.
  emitPtyExit(sessionId: string, exitCode: number, wasIntentional: boolean) {
    Events.Emit(`pty-exit:${sessionId}`, { exitCode, wasIntentional });
  },
  // Simulate a backend-emitted pty-cleared:<id> event directly (payload nil),
  // without going through the Clear RPC.
  emitPtyCleared(sessionId: string) {
    Events.Emit(`pty-cleared:${sessionId}`, null);
  },
  // True once the app has subscribed to `eventName`. Tests must wait for this
  // before emitting: the app registers its listeners only after the async
  // event-name lookup resolves, and an emit before that is dropped silently.
  hasListener(eventName: string) {
    return (eventListeners[eventName] || []).length > 0;
  },
  // Call counters for TerminalService methods — regression coverage for the
  // "redundant Start() on an already-running session" bug (see
  // Terminal.tsx's initiallyRunning prop).
  terminalCallCounts,
  // ── Fault injection ─────────────────────────────────────
  // One-shot rejection for the next call to `method`. Refused (with a
  // console.warn) for methods the real backend can never fail — see
  // NEVER_REJECTS above.
  failNext,
  // Sticky rejection for every future call to `method` until cleared by
  // passing `null`.
  setFailure,
  // Every dispatched call in order, `{ method, args }` — read-only from the
  // test's perspective; use __cmdexE2E.reset() or clearCallLog() to reset it.
  callLog,
  clearCallLog() {
    callLog.length = 0;
  },
  // Configure ImportCommands' next successful result. Leave unset (or pass
  // null) to keep the default "user cancelled the dialog" behavior.
  setImportResult(result: any[] | null) {
    pendingImportResult = result;
  },
  // Configure PickDirectory's return value — pass '' to simulate a
  // cancelled directory picker.
  setPickDirectoryResult(path: string) {
    pickDirectoryResult = path;
  },
  // Configure GetLastOutput's next return value.
  setLastOutput(data: { available: boolean; text: string; exitCode: number; truncated: boolean }) {
    lastOutputResult = { ...data };
  },
};

// Read seed data injected via addInitScript before app initializes
const seed = (globalThis as any).__cmdexE2E_SEED__;
if (seed) {
  if (seed.categories) categories = seed.categories;
  if (seed.commands) commands = seed.commands;
  if (seed.presets) Object.assign(presets, seed.presets);
  if (seed.commands) seedPresetsFromCommands(seed.commands);
  if (seed.settings) Object.assign(settings, seed.settings);
  if (seed.terminalSessions) {
    terminalSessions = seed.terminalSessions;
    activeTerminalSessionId = seed.terminalSessions[0]?.id ?? null;
  }
  nextId = 100;
}
