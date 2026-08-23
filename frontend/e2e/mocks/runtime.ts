// Mock @wailsio/runtime for Playwright E2E tests.
// Replaces the Wails IPC bridge with an in-memory backend.

/* eslint-disable @typescript-eslint/no-explicit-any */

let categories: any[] = [];
let commands: any[] = [];
const presets: Record<string, any[]> = {};
const settings: Record<string, any> = {
  locale: 'en',
  theme: 'vscode-dark',
  lastDarkTheme: 'vscode-dark',
  lastLightTheme: 'vscode-light',
  customThemes: '[]',
  uiFont: 'Inter',
  monoFont: 'JetBrains Mono',
  density: 'comfortable',
};

let nextId = 0;
function uid() {
  return `mock-${++nextId}-${Math.random().toString(36).slice(2, 8)}`;
}

const now = () => new Date().toISOString();

// ── Terminal ─────────────────────────────────────────────
// Mirrors the real backend's actual behavior (TerminalService.ServiceStartup
// and CreateSession both synchronously start the shell before returning) so
// tests can catch a regression of the bug fixed in Terminal.tsx: every
// mount used to unconditionally call Start() even when the backend had
// already started the session, tearing down the healthy PTY and spawning a
// redundant second shell. See callCounts below.
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

const eventListeners: Record<string, Array<(data: any) => void>> = {};

export const Events = {
  On(eventName: string, callback: (data: any) => void) {
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
  Emit(eventName: string, data: any) {
    (eventListeners[eventName] || []).forEach((fn) => fn(data));
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

const handlers: Record<number, (...args: any[]) => any> = {
  // ── Categories ──────────────────────────────────────────
  // GetCategories
  1124386808: () => categories,

  // CreateCategory(name, icon, color)
  3920645540: (name: string, icon: string, color: string) => {
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

  // UpdateCategory(id, name, icon, color)
  871939973: (id: string, name: string, icon: string, color: string) => {
    const idx = categories.findIndex((c) => c.id === id);
    if (idx < 0) throw new Error('Category not found');
    categories[idx] = { ...categories[idx], name, icon, color, updatedAt: now() };
    return categories[idx];
  },

  // DeleteCategory(id)
  2228038743: (id: string) => {
    categories = categories.filter((c) => c.id !== id);
    commands.forEach((cmd) => {
      if (cmd.categoryId === id) cmd.categoryId = '';
    });
  },

  // ── Commands ─────────────────────────────────────────────
  // GetCommands
  2230805162: () => commands,

  // GetCommandsByCategory(categoryID)
  3544855671: (categoryID: string) =>
    commands.filter((c) => (c.categoryId || '') === (categoryID || '')),

  // CreateCommand(title, desc, scriptBody, categoryID, tags, variables, workingDir)
  3040387109: (
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
    return cmd;
  },

  // UpdateCommand(id, title, desc, scriptBody, categoryID, tags, variables, workingDir)
  2942553414: (
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
    return commands[idx];
  },

  // DeleteCommand(id)
  1888656992: (id: string) => {
    commands = commands.filter((c) => c.id !== id);
  },

  // RenameCommand(id, newTitle)
  3511040027: (id: string, newTitle: string) => {
    const idx = commands.findIndex((c) => c.id === id);
    if (idx < 0) throw new Error('Command not found');
    commands[idx] = {
      ...commands[idx],
      title: { String: newTitle, Valid: true },
      updatedAt: now(),
    };
    return commands[idx];
  },

  // ReorderCommand(id, newPosition, newCategoryId)
  2371488912: (id: string, newPosition: number, newCategoryId: string) => {
    const cmd = findCommand(id);
    if (cmd) {
      cmd.position = newPosition;
      cmd.categoryId = newCategoryId || '';
    }
    return commands;
  },

  // GetScriptBody(commandID)
  707578151: (commandID: string) => {
    const cmd = findCommand(commandID);
    if (!cmd) return '';
    return cmd.scriptContent.replace(/^#!.*\n/, '');
  },

  // GetScriptContent(commandID)
  1214515992: (commandID: string) => {
    const cmd = findCommand(commandID);
    return cmd ? cmd.scriptContent : '';
  },

  // SearchCommands(query)
  2165520554: (query: string) => {
    if (!query) return commands;
    const q = query.toLowerCase();
    return commands.filter(
      (c) =>
        (c.title.String || '').toLowerCase().includes(q) ||
        c.scriptContent.toLowerCase().includes(q),
    );
  },

  // ── Presets ──────────────────────────────────────────────
  // GetPresets(commandID)
  2933858456: (commandID: string) => presets[commandID] || [],

  // SavePreset(commandID, name, values)
  2518009278: (commandID: string, name: string, values: Record<string, string>) => {
    if (!presets[commandID]) presets[commandID] = [];
    const preset = { id: uid(), name, position: presets[commandID].length, values: values || {} };
    presets[commandID].push(preset);
    return preset;
  },

  // UpdatePreset(commandID, presetID, name, values)
  1219258890: (
    commandID: string,
    presetID: string,
    name: string,
    values: Record<string, string>,
  ) => {
    const list = presets[commandID] || [];
    const idx = list.findIndex((p) => p.id === presetID);
    if (idx < 0) throw new Error('Preset not found');
    list[idx] = { ...list[idx], name, values: values || {} };
    return list[idx];
  },

  // DeletePreset(commandID, presetID)
  1347137556: (commandID: string, presetID: string) => {
    if (presets[commandID]) {
      presets[commandID] = presets[commandID].filter((p) => p.id !== presetID);
    }
  },

  // ReorderPresets(commandID, presetIDs)
  4123798965: (commandID: string, presetIDs: string[]) => {
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
  // GetSettings
  3034808949: () => ({ ...settings }),

  // SetSettings(jsonStr)
  287946425: (jsonStr: string) => {
    try {
      Object.assign(settings, JSON.parse(jsonStr));
    } catch {
      /* ignore */
    }
  },

  // ── Execution ────────────────────────────────────────────
  // GetVariables(commandID)
  4101005934: (commandID: string) => {
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

  // RunCommand(commandID, variables)
  4143621145: (commandID: string, variables: Record<string, string>) => {
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
  // ExportCommands(commandIDs)
  3360644818: () => {},

  // ImportCommands
  840325137: () => [],

  // SaveThemeTemplate
  1489453142: () => {},

  // ── Terminal ─────────────────────────────────────────────
  // CreateSession
  2914743863: () => createMockTerminalSession(),

  // ListSessions
  1878753896: () => terminalSessions,

  // GetActiveSession
  1716291155: () => terminalSessions.find((s) => s.id === activeTerminalSessionId) || null,

  // SetActiveSession(id)
  4186280199: (id: string) => {
    activeTerminalSessionId = id;
  },

  // CloseSession(id)
  3305403653: (id: string) => {
    terminalSessions = terminalSessions.filter((s) => s.id !== id);
    if (activeTerminalSessionId === id) {
      activeTerminalSessionId = terminalSessions[0]?.id || null;
    }
  },

  // RenameSession(id, name)
  911254629: (id: string, name: string) => {
    const s = terminalSessions.find((s) => s.id === id);
    if (s) s.name = name;
  },

  // Start(sessionId, cols, rows)
  3774501399: (sessionId: string) => {
    terminalCallCounts.Start++;
    const s = terminalSessions.find((s) => s.id === sessionId);
    if (s) s.running = true;
  },

  // Stop(sessionId)
  2555571709: (sessionId: string) => {
    const s = terminalSessions.find((s) => s.id === sessionId);
    if (s) s.running = false;
  },

  // Write(sessionId, data)
  371925220: () => {
    terminalCallCounts.Write++;
  },

  // Resize(sessionId, cols, rows)
  270758441: () => {
    terminalCallCounts.Resize++;
  },

  // Clear(sessionId)
  493646090: () => {
    terminalCallCounts.Clear++;
  },

  // GetLastOutput(sessionId)
  4011093730: () => ({ available: false, text: '', exitCode: 0, truncated: false }),

  // ── Events ───────────────────────────────────────────────
  // GetEventNames
  2407475739: () => ({
    openSettings: 'open-settings',
    openShortcuts: 'open-shortcuts',
    settingsChanged: 'settings-changed',
    settingsWindowClosing: 'settings-window-closing',
  }),

  // ── App ──────────────────────────────────────────────────
  // GetOS
  816844233: () => 'darwin',

  // PickDirectory
  1347829059: () => '/mock/path',

  // ShowSettingsWindow
  2596981913: () => {},

  // ── Misc ─────────────────────────────────────────────────
  // ResetAllData
  121210722: () => {
    categories = [];
    commands = [];
    Object.keys(presets).forEach((k) => delete presets[k]);
  },
};

export const Call = {
  ByID(id: number, ...args: any[]) {
    const handler = handlers[id];
    if (!handler) {
      console.warn(`[e2e mock] no handler for method ID ${id}`);
      return Promise.resolve(null);
    }
    try {
      return Promise.resolve(handler(...args));
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
    nextId = 0;
    terminalSessions = [];
    activeTerminalSessionId = null;
    terminalSessionCounter = 0;
    terminalCallCounts.CreateSession = 0;
    terminalCallCounts.Start = 0;
    terminalCallCounts.Write = 0;
    terminalCallCounts.Resize = 0;
    terminalCallCounts.Clear = 0;
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
  // Deliver an event to the app as the real runtime would. Callers pass the
  // full `WailsEvent` shape ({ name, data, sender }) because that is what
  // window-to-window emits look like on the receiving side.
  emit(eventName: string, data: any) {
    Events.Emit(eventName, data);
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
};

// Read seed data injected via addInitScript before app initializes
const seed = (globalThis as any).__cmdexE2E_SEED__;
if (seed) {
  if (seed.categories) categories = seed.categories;
  if (seed.commands) commands = seed.commands;
  if (seed.presets) Object.assign(presets, seed.presets);
  if (seed.settings) Object.assign(settings, seed.settings);
  nextId = 100;
}
