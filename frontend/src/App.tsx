import { useState, useEffect, useCallback, useRef, useMemo, lazy, Suspense } from 'react';
import { useTranslation } from 'react-i18next';
import './style.css';
import { useSyncedRef } from './hooks/useSyncedRef';
import { useResizable } from './hooks/useResizable';
import Sidebar from './components/Sidebar';
import CategoryEditor from './components/CategoryEditor';
import VariablePrompt from './components/VariablePrompt';
import type { TerminalHandle } from './components/Terminal';
import ResizablePanel from './components/ResizablePanel';
import TabBar, { type Tab } from './components/TabBar';
import TerminalTabBar from './components/TerminalTabBar';
import CommandPalette from './components/CommandPalette';
import WelcomeTab from './components/WelcomeTab';
import KeyboardShortcutsDialog from './components/KeyboardShortcutsDialog';
import CommandDetailTab from './components/CommandDetailTab';
import { useKeyboardShortcuts, cmdOrCtrl } from './hooks/useKeyboardShortcuts';
import { TooltipProvider } from '@/components/ui/tooltip';
import { Toaster } from '@/components/ui/sonner';
import { toast } from 'sonner';
import { Badge } from '@/components/ui/badge';

import {
    AlertDialog,
    AlertDialogContent,
    AlertDialogHeader,
    AlertDialogTitle,
    AlertDialogDescription,
    AlertDialogFooter,
    AlertDialogCancel,
    AlertDialogAction,
} from '@/components/ui/alert-dialog';
import { Events } from '@wailsio/runtime';
import { eventNames, initEventNames } from './wails/events';
import {
    type Category,
    type Command,
    type VariableDefinition,
    type VariablePrompt as VarPromptType,
    type VariablePreset,
    type TabDraft,
    type SessionInfo,
    type SettingsPayload,
    type OSPathMap,
    type OSKey,
    type CustomTheme,
} from './types';
import { createNewTabId, isNewCommandTabId, getCommandDisplayTitle } from './utils/tab';
import { normalizeOS } from './utils/path';

import {
    GetCategories,
    CreateCategory,
    UpdateCategory,
    DeleteCategory,
    GetCommands,
    CreateCommand,
    UpdateCommand,
    DeleteCommand,
    GetPresets,
    SavePreset,
    UpdatePreset,
    DeletePreset,
    ReorderCommand,
    GetScriptBody,
    ReorderPresets,
} from '../bindings/cmdex/commandservice';
import {
    GetSettings,
    SetSettings,
} from '../bindings/cmdex/settingsservice';
import {
    ShowSettingsWindow,
    GetOS,
} from '../bindings/cmdex/app';
import {
    GetVariables,
    RunCommand,
} from '../bindings/cmdex/executionservice';
import { CreateSession, ListSessions, CloseSession, RenameSession, SetActiveSession, GetActiveSession } from '../bindings/cmdex/terminalservice';
import i18n from './i18n';
import {
    emptyDraft,
    draftFromCommand,
    draftsEqual,
    cloneDraft,
    makePlaceholderCommand,
} from './utils/tabDraft';
import { buildVariablesFromScript, variableDefinitionsToPrompts } from './utils/templateVars';
import { copyText } from './utils/clipboard';
import { MainLogo } from './assets/images/main-logo';
import { applyTheme, applyDensity, applyFonts } from './lib/theme-apply';

const TerminalComponent = lazy(() => import('./components/Terminal'));

type ModalState =
    | { type: 'none' }
    | { type: 'categoryEditor'; category?: Category }
    | { type: 'managePresets'; variables: VarPromptType[]; commandId: string; presets: VariablePreset[] }
    | { type: 'fillVariables'; variables: VarPromptType[]; commandId: string; initialValues: Record<string, string> }
    | { type: 'confirmDiscard' }
    | { type: 'confirmVarRemoval'; removedVars: string[]; tabId: string };

// Legacy localStorage keys — used only for one-time migration on startup
const THEME_STORAGE_KEY = 'cmdex-theme';
const LAST_DARK_THEME_KEY = 'cmdex-last-dark-theme';
const LAST_LIGHT_THEME_KEY = 'cmdex-last-light-theme';
const CUSTOM_THEMES_KEY = 'cmdex-custom-themes';
const FONT_SANS_KEY = 'cmdex-ui-font';
const FONT_MONO_KEY = 'cmdex-mono-font';
const DENSITY_KEY = 'cmdex-density';


function App() {
    const { t } = useTranslation();
    const [categories, setCategories] = useState<Category[]>([]);
    const [commands, setCommands] = useState<Command[]>([]);
    const allCommandsRef = useRef<Command[]>([]);
    const [selectedCommand, setSelectedCommand] = useState<Command | null>(null);
    const [modal, setModal] = useState<ModalState>({ type: 'none' });

    const [serverVariables, setServerVariables] = useState<VarPromptType[]>([]);
    const [currentResolvedValues, setCurrentResolvedValues] = useState<Record<string, string>>({});
    const [lastSelectedPresetId, setLastSelectedPresetId] = useState<string>('');
    const executingTabIdRef = useRef<string | null>(null);
    const [executingTabIdState, setExecutingTabIdState] = useState<string | null>(null);

    const selectedCommandRef = useSyncedRef(selectedCommand);
    const selectedCommandId = selectedCommand?.id;

    const [openTabs, setOpenTabs] = useState<Tab[]>([]);
    const openTabsRef = useSyncedRef(openTabs);

    const [shortcutsDialogOpen, setShortcutsDialogOpen] = useState(false);
    const scriptFetchGenRef = useRef<Record<string, number>>({});
    const [activeTabId, setActiveTabId] = useState<string | null>(null);
    const activeTabIdRef = useSyncedRef(activeTabId);
    const prevTabIdRef = useRef<string | null>(null);
    const [tabDrafts, setTabDrafts] = useState<Record<string, TabDraft>>({});
    const [tabBaselines, setTabBaselines] = useState<Record<string, TabDraft>>({});
    const tabDraftsRef = useSyncedRef(tabDrafts);
    const tabBaselinesRef = useSyncedRef(tabBaselines);

    const [paletteOpen, setPaletteOpen] = useState(false);
    const [currentOS, setCurrentOS] = useState<OSKey>('unknown');
    const pendingCloseTabIdRef = useRef<string | null>(null);
    const mainContentRef = useRef<HTMLDivElement>(null);
    const terminalRefs = useRef<Record<string, TerminalHandle>>({});

    const [theme, setTheme] = useState<string>('vscode-dark');
    // Custom themes must live in state (not only in settingsRef) so the theme
    // effect below re-runs when they are imported/removed in the settings window.
    const [customThemes, setCustomThemes] = useState<CustomTheme[]>([]);

    const [uiFont, setUiFont] = useState<string>('Inter');
    const [monoFont, setMonoFont] = useState<string>('JetBrains Mono');
    const [density, setDensity] = useState<string>('comfortable');
    const [defaultWorkingDir, setDefaultWorkingDir] = useState<OSPathMap>({});

    // Terminal split pane state
    const TERM_STORAGE_KEY = 'cmdex-terminal-height';
    const MIN_TERM_HEIGHT = 100;
    const MAX_TERM_HEIGHT_PCT = 0.85;

    const viewportHeight = window.innerHeight;
    const defaultTermHeight = Math.round(viewportHeight * 0.40);
    const maxTermHeight = Math.round(viewportHeight * MAX_TERM_HEIGHT_PCT);

    const { size: terminalHeight, isDragging, handleStart } = useResizable({
        axis: 'y',
        direction: -1,
        minSize: MIN_TERM_HEIGHT,
        maxSize: maxTermHeight,
        defaultSize: defaultTermHeight,
        storageKey: TERM_STORAGE_KEY,
    });

    const [terminalCollapsed, setTerminalCollapsed] = useState<boolean>(() => {
        return localStorage.getItem(`${TERM_STORAGE_KEY}-collapsed`) === 'true';
    });

    const [activeSessionId, setActiveSessionId] = useState<string>('');
    const [sessions, setSessions] = useState<SessionInfo[]>([]);
    const terminalOrderRef = useRef<string[]>([]);

    const collapseTerminal = useCallback(() => {
        setTerminalCollapsed(true);
        localStorage.setItem(`${TERM_STORAGE_KEY}-collapsed`, 'true');
    }, []);

    const expandTerminal = useCallback(() => {
        setTerminalCollapsed(false);
        localStorage.setItem(`${TERM_STORAGE_KEY}-collapsed`, 'false');
    }, []);

    // -- Session CRUD callbacks (Task 1) --

    const createTerminalSession = useCallback(async () => {
        try {
            const info = await CreateSession();
            if (info) {
                // Mutate terminalOrderRef BEFORE the await so the re-render
                // triggered by setSessions sees the new id. Refs are not
                // reactive — mutating after the await would leave the ref
                // missing the new id during the re-render, so the
                // TerminalComponent for this id would never mount (regression
                // from 0c5b41a when SetActiveSession became awaited).
                terminalOrderRef.current = [...terminalOrderRef.current, info.id];
                setSessions(prev => [...prev, info]);
                try {
                    await SetActiveSession(info.id);
                    setActiveSessionId(info.id);
                } catch (activeErr) {
                    console.error('Failed to set active session after create:', activeErr);
                    toast.error('Session created but could not be set as active.');
                }
            }
        } catch (err) {
            console.error('Failed to create terminal session:', err);
            const msg = String(err);
            if (msg.includes('max sessions reached')) {
                toast.error(t('toast.maxSessionsReached', { limit: 10 }));
            } else {
                toast.error('Could not create session. Check that the terminal backend is running.');
            }
        }
    }, [t]);

    const closeTerminalSession = useCallback(async (id: string) => {
        if (sessions.length <= 1) return; // D-02: last tab not closeable
        // Snapshot sessions before the await so we don't read stale closure
        // state if another action mutates `sessions` mid-flight.
        const wasActive = activeSessionId === id;
        const remaining = wasActive ? sessions.filter(s => s.id !== id) : [];
        const next = remaining[0];
        try {
            await CloseSession(id);
            setSessions(prev => prev.filter(s => s.id !== id));
            terminalOrderRef.current = terminalOrderRef.current.filter(tid => tid !== id);
            if (wasActive && next) {
                try {
                    await SetActiveSession(next.id);
                    setActiveSessionId(next.id);
                } catch (activeErr) {
                    console.error('Failed to set next active session after close:', activeErr);
                    toast.error('Session closed but could not switch active session.');
                }
            }
        } catch (err) {
            console.error('Failed to close terminal session:', err);
        }
    }, [sessions, activeSessionId]);

    const renameTerminalSession = useCallback(async (id: string, name: string) => {
        const trimmed = name.trim();
        if (!trimmed) return;
        try {
            await RenameSession(id, trimmed);
            setSessions(prev => prev.map(s =>
                s.id === id ? { ...s, name: trimmed } : s
            ));
        } catch (err) {
            console.error('Failed to rename session:', err);
            toast.error('Could not rename session. Please try again.');
        }
    }, []);

    const switchTerminalSession = useCallback(async (id: string) => {
        try {
            await SetActiveSession(id);
            setActiveSessionId(id);
        } catch (err) {
            console.error('Failed to set active session:', err);
            toast.error('Could not switch active session.');
        }
    }, []);

    const handleReorderTerminalTabs = useCallback((reordered: SessionInfo[]) => {
        setSessions(reordered);
    }, []);

    // Focus detection: returns true if keyboard focus is anywhere inside the
    // terminal pane (including the xterm.js hidden textarea used for shell input).
    // The xterm-helper-textarea is rendered as a descendant of the mount point,
    // so the single .terminal-pane ancestor check covers all terminal focus states.
    const isFocusInTerminalPane = useCallback((): boolean => {
        const el = document.activeElement as HTMLElement | null;
        if (!el) return false;
        return !!el.closest?.('.terminal-pane');
    }, []);

    const handleTerminalResizeStart = useCallback((e: React.MouseEvent) => {
        e.preventDefault();
        handleStart(e.clientY);
    }, [handleStart]);

    // Tracks whether settings have been loaded from DB (prevents premature saves before load)
    const settingsLoadedRef = useRef(false);
    // Holds latest settings values for use in flushSettings without stale closures
    const settingsRef = useRef({
        locale: 'en',
        terminal: '',
        theme: 'vscode-dark',
        lastDarkTheme: 'vscode-dark',
        lastLightTheme: 'vscode-light',
        customThemes: [] as CustomTheme[],
        uiFont: 'Inter',
        monoFont: 'JetBrains Mono',
        density: 'comfortable',
        defaultWorkingDir: {} as OSPathMap,
        windowX: -1,
        windowY: -1,
        windowWidth: 640,
        windowHeight: 520,
    });

    // Persists all current settings from settingsRef to the DB.
    // Must only be called after settingsLoadedRef.current === true.
    const flushSettings = () => {
        if (!settingsLoadedRef.current) return;
        const r = settingsRef.current;
        SetSettings(JSON.stringify({
            locale: r.locale,
            terminal: r.terminal,
            theme: r.theme,
            lastDarkTheme: r.lastDarkTheme,
            lastLightTheme: r.lastLightTheme,
            customThemes: JSON.stringify(r.customThemes),
            uiFont: r.uiFont,
            monoFont: r.monoFont,
            density: r.density,
            defaultWorkingDir: r.defaultWorkingDir,
            windowX: r.windowX,
            windowY: r.windowY,
            windowWidth: r.windowWidth,
            windowHeight: r.windowHeight,
        })).catch(() => {});
    };

    // Tracks whether event names have been initialized from backend
    const [eventsInitialized, setEventsInitialized] = useState(false);

    useEffect(() => {
        initEventNames().then(() => setEventsInitialized(true));
        // Load all sessions and active session on mount
        ListSessions().then((list) => {
            const loaded = ((list || []) as SessionInfo[]).filter(Boolean);
            setSessions(loaded);
            terminalOrderRef.current = loaded.map(s => s.id);
            // Return loaded for the next .then(), and chain GetActiveSession
            return GetActiveSession().then((info) => ({ loaded, info }));
        }).then(({ loaded, info }) => {
            if (info) {
                setActiveSessionId(info.id);
            } else if (loaded.length === 0) {
                // No active session and no sessions loaded — create a default session
                CreateSession().then((newInfo) => {
                    if (newInfo) {
                        setSessions([newInfo]);
                        setActiveSessionId(newInfo.id);
                        SetActiveSession(newInfo.id);
                        terminalOrderRef.current = [newInfo.id];
                    }
                }).catch(() => {});
            }
        }).catch(() => {});
    }, []);

    // Track per-session running status via pty-exit events
    const sessionIdsKey = sessions.map(s => s.id).join(',');
    useEffect(() => {
        if (!eventsInitialized) return;
        const cleanups: (() => void)[] = [];

        const subscribeSession = (sessionId: string) => {
            const ptyExitEvent = 'pty-exit:' + sessionId;
            const cleanup = Events.On(ptyExitEvent, () => {
                setSessions(prev => prev.map(s =>
                    s.id === sessionId ? { ...s, running: false } : s
                ));
            });
            cleanups.push(cleanup);
            return cleanup;
        };

        sessions.forEach(s => subscribeSession(s.id));

        return () => cleanups.forEach(fn => fn());
    // eslint-disable-next-line react-hooks/exhaustive-deps -- use sessionIdsKey to avoid re-subscribing when only session.running changes
    }, [sessionIdsKey, eventsInitialized]);

    // A custom theme has no `[data-theme="custom-..."]` CSS rule, so its colors
    // must be passed to applyTheme() to be written as inline CSS variables on
    // the root element. Without them the main window silently falls back to the
    // default palette while the settings window (which previews the imported
    // colors directly) still looks correct.
    useEffect(() => {
        const custom = customThemes.find((c) => c.id === theme);
        applyTheme(theme, custom?.colors ?? null);
        settingsRef.current.theme = theme;
        settingsRef.current.customThemes = customThemes;
        flushSettings();
    }, [theme, customThemes]);

    useEffect(() => {
        const mediaQuery = window.matchMedia('(prefers-color-scheme: dark)');
        const handler = (e: MediaQueryListEvent) => {
            const r = settingsRef.current;
            setTheme(e.matches ? r.lastDarkTheme : r.lastLightTheme);
        };
        mediaQuery.addEventListener('change', handler);
        return () => mediaQuery.removeEventListener('change', handler);
    }, []);

    useEffect(() => {
        applyFonts(uiFont, monoFont);
        settingsRef.current.uiFont = uiFont;
        settingsRef.current.monoFont = monoFont;
        flushSettings();
    }, [uiFont, monoFont]);

    useEffect(() => {
        applyDensity(density);
        settingsRef.current.density = density;
        flushSettings();
    }, [density]);

    // Tab switch fade: trigger opacity fade-in on the main-content area when activeTabId changes.
    // With per-tab mounts, this animates the entire main-content area on tab switch.
    // Inactive shells are display:none so only the active shell is visible during the fade.
    useEffect(() => {
        const el = mainContentRef.current;
        if (!el) return;
        el.classList.remove('tab-content-fade-in');
        // Force reflow so the class removal takes effect before re-adding
        void el.offsetWidth;
        el.classList.add('tab-content-fade-in');
        const timer = setTimeout(() => {
            el.classList.remove('tab-content-fade-in');
        }, 160);
        return () => clearTimeout(timer);
    }, [activeTabId]);

    const resolvedVariables = useMemo(() => {
        if (!selectedCommand) return [];
        if (isNewCommandTabId(selectedCommandId)) {
            const d = tabDrafts[selectedCommandId];
            if (!d) return [];
            return variableDefinitionsToPrompts(d.variables);
        }
        const d = tabDrafts[selectedCommandId];
        if (d) {
            const serverMap = new Map(serverVariables.map(v => [v.name, v]));
            return d.variables.map(dv => {
                const sv = serverMap.get(dv.name);
                if (sv) return sv;
                return {
                    name: dv.name,
                    placeholder: dv.name,
                    description: dv.description,
                    example: dv.example,
                    defaultExpr: dv.default,
                    defaultValue: dv.default ?? '',
                };
            });
        }
        return serverVariables;
    }, [selectedCommand, selectedCommandId, tabDrafts, serverVariables]);

    const variablesRequestIdRef = useRef(0);
    useEffect(() => {
        if (!selectedCommand) {
            // eslint-disable-next-line react-hooks/set-state-in-effect
            setServerVariables([]);
            return;
        }
        if (isNewCommandTabId(selectedCommandId)) {
            setServerVariables([]);
            return;
        }
        const requestId = ++variablesRequestIdRef.current;
        GetVariables(selectedCommandId)
            .then((v) => {
                if (variablesRequestIdRef.current === requestId) {
                    setServerVariables(v || []);
                }
            })
            .catch(() => {
                if (variablesRequestIdRef.current === requestId) {
                    setServerVariables([]);
                }
            });
    }, [selectedCommand, selectedCommandId]);

    useEffect(() => {
        if (!selectedCommand || isNewCommandTabId(selectedCommandId)) return;
        const d = tabDrafts[selectedCommandId];
        const b = tabBaselines[selectedCommandId];
        if (d && b && !draftsEqual(d, b)) return;
        const fresh = commands.find((c) => c.id === selectedCommandId);
        // eslint-disable-next-line react-hooks/set-state-in-effect -- syncing selected command from fresh data after external reload
        if (fresh) setSelectedCommand(fresh);
    }, [selectedCommand, commands, selectedCommandId, tabDrafts, tabBaselines]);

    const loadData = useCallback(async () => {
        try {
            const [cats, cmds] = await Promise.all([GetCategories(), GetCommands()]);
            setCategories(cats || []);
            setCommands(cmds || []);
            allCommandsRef.current = cmds || [];
            return (cmds as Command[]) || [];
        } catch (err) {
            console.error('Failed to load data:', err);
            return [] as Command[];
        }
    }, []);

    useEffect(() => {
        /* eslint-disable react-hooks/set-state-in-effect -- one-time init data loading */
        GetOS().then((os) => setCurrentOS(normalizeOS(os))).catch(() => setCurrentOS('unknown'));
        loadData();
        GetSettings()
            .then((s) => {
                if (!s) return;

                // One-time localStorage migration: if DB has the default value but localStorage
                // has a user-set value, prefer the localStorage value (migrates existing users).
                const migrateField = (dbVal: string, lsKey: string, defaultVal: string): string => {
                    if (dbVal === defaultVal) {
                        return localStorage.getItem(lsKey) || defaultVal;
                    }
                    return dbVal;
                };

                const prefersDark = window.matchMedia('(prefers-color-scheme: dark)').matches;
                const migratedTheme = migrateField(s.theme, THEME_STORAGE_KEY, 'vscode-dark') ||
                    (prefersDark
                        ? (localStorage.getItem(LAST_DARK_THEME_KEY) || 'vscode-dark')
                        : (localStorage.getItem(LAST_LIGHT_THEME_KEY) || 'vscode-light'));
                const migratedLastDark = migrateField(s.lastDarkTheme, LAST_DARK_THEME_KEY, 'vscode-dark');
                const migratedLastLight = migrateField(s.lastLightTheme, LAST_LIGHT_THEME_KEY, 'vscode-light');
                const migratedUiFont = migrateField(s.uiFont, FONT_SANS_KEY, 'Inter');
                const migratedMonoFont = migrateField(s.monoFont, FONT_MONO_KEY, 'JetBrains Mono');
                const migratedDensity = migrateField(s.density, DENSITY_KEY, 'comfortable');

                let migratedCustomThemes: CustomTheme[] = [];
                try {
                    const rawCustomThemes = s.customThemes && s.customThemes !== '[]'
                        ? s.customThemes
                        : localStorage.getItem(CUSTOM_THEMES_KEY);
                    if (rawCustomThemes) {
                        const parsed = JSON.parse(rawCustomThemes);
                        // A non-array payload must not reach state — the theme
                        // effect calls .find() on it.
                        if (Array.isArray(parsed)) migratedCustomThemes = parsed;
                    }
                } catch { /* ignore parse errors */ }

                // Apply locale
                if (s.locale && s.locale !== i18n.language) {
                    i18n.changeLanguage(s.locale);
                }

                // Sync settingsRef before marking loaded (prevents flushSettings no-ops)
                settingsRef.current = {
                    locale: s.locale || 'en',
                    terminal: s.terminal || '',
                    theme: migratedTheme,
                    lastDarkTheme: migratedLastDark,
                    lastLightTheme: migratedLastLight,
                    customThemes: migratedCustomThemes,
                    uiFont: migratedUiFont,
                    monoFont: migratedMonoFont,
                    density: migratedDensity,
                    defaultWorkingDir: s.defaultWorkingDir || {},
                    windowX: s.windowX ?? -1,
                    windowY: s.windowY ?? -1,
                    windowWidth: s.windowWidth ?? 640,
                    windowHeight: s.windowHeight ?? 520,
                };
                settingsLoadedRef.current = true;

                // Apply state — each setter triggers its effect which calls flushSettings
                setCustomThemes(migratedCustomThemes);
                setTheme(migratedTheme);
                setUiFont(migratedUiFont);
                setMonoFont(migratedMonoFont);
                setDensity(migratedDensity);
                setDefaultWorkingDir(s.defaultWorkingDir || {});

                // Clear legacy localStorage keys after successful migration
                [THEME_STORAGE_KEY, LAST_DARK_THEME_KEY, LAST_LIGHT_THEME_KEY,
                 CUSTOM_THEMES_KEY, FONT_SANS_KEY, FONT_MONO_KEY, DENSITY_KEY].forEach(k =>
                    localStorage.removeItem(k)
                );

                // Persist migrated values to DB (covers the case where migration pulled from localStorage)
                SetSettings(JSON.stringify({
                    locale: settingsRef.current.locale,
                    terminal: settingsRef.current.terminal,
                    theme: migratedTheme,
                    lastDarkTheme: migratedLastDark,
                    lastLightTheme: migratedLastLight,
                    customThemes: JSON.stringify(migratedCustomThemes),
                    uiFont: migratedUiFont,
                    monoFont: migratedMonoFont,
                    density: migratedDensity,
                    defaultWorkingDir: settingsRef.current.defaultWorkingDir,
                    windowX: settingsRef.current.windowX,
                    windowY: settingsRef.current.windowY,
                    windowWidth: settingsRef.current.windowWidth,
                    windowHeight: settingsRef.current.windowHeight,
                })).catch(() => {});
            })
            .catch(() => {
                // Allow saves even if initial load fails
                settingsLoadedRef.current = true;
            });
        setOpenTabs([]);
        setActiveTabId(null);
    }, [loadData]);
    /* eslint-enable react-hooks/set-state-in-effect */

    const openSettingsWithToast = async () => {
        try {
            await ShowSettingsWindow();
        } catch (err) {
            toast.error('Failed to open settings window');
            console.error('ShowSettingsWindow error:', err);
        }
    };

    useEffect(() => {
        if (!eventsInitialized) return;
        const cleanup = Events.On(eventNames.openSettings, async () => {
            await openSettingsWithToast();
        });
        return cleanup;
    }, [eventsInitialized]);

    useEffect(() => {
        if (!eventsInitialized) return;
        const cleanup = Events.On(eventNames.openShortcuts, () => {
            setShortcutsDialogOpen(true);
        });
        return cleanup;
    }, [eventsInitialized]);

    useEffect(() => {
        if (!eventsInitialized) return;
        // Wails v3 `Events.On` delivers a `WailsEvent` wrapper: { name, data, sender }.
        // The emitted payload is at `event.data`, not on the event object itself.
        // Reading the payload fields directly off `event` returns undefined and would
        // cause `||` fallbacks to kick in, overwriting user's just-saved settings
        // with defaults. Always unwrap `.data`.
        const cleanup = Events.On(eventNames.settingsChanged, (event: { name: string; data: unknown; sender: string }) => {
            const payload = event?.data as Partial<SettingsPayload> | undefined;
            if (!payload) return;
            // Keep settingsRef in sync BEFORE state setters fire their auto-save
            // useEffects — those effects read state and persist, so settingsRef
            // must hold the correct non-theme fields first or a stale value (e.g.
            // lastDarkTheme, locale, terminal) could be written back to the DB.
            const current = settingsRef.current;
            settingsRef.current = {
                ...current,
                locale: payload.locale ?? current.locale,
                terminal: payload.terminal ?? current.terminal,
                theme: payload.theme ?? current.theme,
                lastDarkTheme: payload.lastDarkTheme ?? current.lastDarkTheme,
                lastLightTheme: payload.lastLightTheme ?? current.lastLightTheme,
                uiFont: payload.uiFont ?? current.uiFont,
                monoFont: payload.monoFont ?? current.monoFont,
                density: payload.density ?? current.density,
                defaultWorkingDir: payload.defaultWorkingDir ?? current.defaultWorkingDir,
            };
            // Parse custom themes before setTheme so the theme effect always sees
            // the list the incoming theme id may refer to (a freshly imported one).
            if (payload.customThemes !== undefined) {
                try {
                    const parsed = typeof payload.customThemes === 'string'
                        ? JSON.parse(payload.customThemes)
                        : payload.customThemes;
                    if (Array.isArray(parsed)) {
                        settingsRef.current.customThemes = parsed;
                        setCustomThemes(parsed);
                    }
                } catch {
                    // Do not overwrite existing customThemes on parse failure
                }
            }
            if (payload.locale) i18n.changeLanguage(payload.locale);
            if (payload.theme) setTheme(payload.theme);
            if (payload.uiFont) setUiFont(payload.uiFont);
            if (payload.monoFont) setMonoFont(payload.monoFont);
            if (payload.density) setDensity(payload.density);
            if (payload.defaultWorkingDir) setDefaultWorkingDir(payload.defaultWorkingDir);
        });
        return cleanup;
    }, [eventsInitialized]);

    const updateDraft = useCallback((tabId: string, partial: Partial<TabDraft>) => {
        setTabDrafts((prev) => {
            const cur = prev[tabId];
            if (!cur) return prev;
            const next: TabDraft = { ...cur, ...partial };
            return { ...prev, [tabId]: next };
        });
    }, []);

    const handleDiscardTab = useCallback(
        (tabId: string) => {
            const b = tabBaselines[tabId];
            if (b) setTabDrafts((prev) => ({ ...prev, [tabId]: cloneDraft(b) }));
        },
        [tabBaselines],
    );

     
    const finalizeCloseTab = useCallback(
        (tabId: string) => {
            const prevTabs = openTabsRef.current;
            const newTabs = prevTabs.filter((t) => t.id !== tabId);
            if (activeTabId === tabId) {
                const idx = prevTabs.findIndex((t) => t.id === tabId);
                const nextTab = newTabs[Math.min(idx, newTabs.length - 1)];
                if (nextTab) {
                    if (isNewCommandTabId(nextTab.id)) {
                        const d = tabDraftsRef.current[nextTab.id];
                        setSelectedCommand(makePlaceholderCommand(nextTab.id, d?.categoryId));
                    } else {
                        const cmd = allCommandsRef.current.find((c) => c.id === nextTab.id);
                        setSelectedCommand(cmd ?? null);
                    }
                    setActiveTabId(nextTab.id);
                } else {
                    setSelectedCommand(null);
                    setActiveTabId(null);
                                    }
            }
            setOpenTabs(newTabs);
            setTabDrafts((prev) => {
                const n = { ...prev };
                delete n[tabId];
                return n;
            });
            setTabBaselines((prev) => {
                const n = { ...prev };
                delete n[tabId];
                return n;
            });
            delete scriptFetchGenRef.current[tabId];
        },
        // eslint-disable-next-line react-hooks/exhaustive-deps -- refs via useSyncedRef are stable
        [activeTabId],
    );
     
    const openNewCommandTab = useCallback(
        (defaultCategoryId?: string) => {
            const prevTabId = activeTabIdRef.current;
            if (prevTabId) {
                prevTabIdRef.current = prevTabId;
            }
            const id = createNewTabId();
            const initial = emptyDraft(defaultCategoryId);
            const baseline = cloneDraft(initial);
            setTabDrafts((prev) => ({ ...prev, [id]: initial }));
            setTabBaselines((prev) => ({ ...prev, [id]: baseline }));
            setSelectedCommand(makePlaceholderCommand(id, defaultCategoryId));
                        setActiveTabId(id);
            setOpenTabs((prev) => [...prev, { id, title: t('commandEditor.newCommand') }]);
        },
        // eslint-disable-next-line react-hooks/exhaustive-deps -- refs via useSyncedRef are stable
        [t],
    );

    const openTab = useCallback((cmd: Command) => {
        const prevTabId = activeTabIdRef.current;
        if (prevTabId && prevTabId !== cmd.id) {
            prevTabIdRef.current = prevTabId;
        }
        setSelectedCommand(cmd);
        setActiveTabId(cmd.id);
        const isExisting = !!tabBaselinesRef.current[cmd.id];
        setOpenTabs((prev) => {
            const tabTitle = getCommandDisplayTitle(cmd);
            const exists = prev.find((t) => t.id === cmd.id);
            if (exists) {
                return prev.map((t) => (t.id === cmd.id ? { ...t, title: tabTitle } : t));
            }
            return [...prev, { id: cmd.id, title: tabTitle }];
        });
        if (isExisting) {
            return;
        }
        const g = (scriptFetchGenRef.current[cmd.id] = (scriptFetchGenRef.current[cmd.id] ?? 0) + 1);
        void GetScriptBody(cmd.id)
            .then((body) => {
                if (scriptFetchGenRef.current[cmd.id] !== g) return;
                const d = draftFromCommand(cmd, body);
                setTabDrafts((prev) => prev[cmd.id] ? prev : { ...prev, [cmd.id]: d });
                setTabBaselines((prev) => prev[cmd.id] ? prev : { ...prev, [cmd.id]: cloneDraft(d) });
            })
            .catch(() => {
                if (scriptFetchGenRef.current[cmd.id] !== g) return;
                const d = draftFromCommand(cmd, '');
                setTabDrafts((prev) => prev[cmd.id] ? prev : { ...prev, [cmd.id]: d });
                setTabBaselines((prev) => prev[cmd.id] ? prev : { ...prev, [cmd.id]: cloneDraft(d) });
            });
    // eslint-disable-next-line react-hooks/exhaustive-deps -- refs via useSyncedRef are stable
    }, []);

    const skipVarRemovalCheckRef = useRef(false);
    const pendingDirectSaveBodyRef = useRef<string | null>(null);

    const computeRemovedVarsWithPresets = (
        tabId: string,
        newVars: VariableDefinition[],
    ): string[] => {
        const existingCmd = allCommandsRef.current.find((c) => c.id === tabId);
        if (!existingCmd || !existingCmd.presets || existingCmd.presets.length === 0) return [];
        const newVarNames = new Set(newVars.map((v) => v.name));
        const removedVars = existingCmd.variables
            .filter((v) => !newVarNames.has(v.name))
            .map((v) => v.name);
        return removedVars.filter((name) =>
            existingCmd.presets!.some((p) => {
                const val = p.values[name];
                return typeof val === 'string' && val.trim() !== '';
            }),
        );
    };

    const handleSaveTab = useCallback(
        async (tabId: string) => {
            const d = tabDraftsRef.current[tabId];
            if (!d || !d.scriptBody.trim()) return;
            const title = d.title.trim();
            const description = d.description.trim();
            const body = d.scriptBody.replace(/^\s+|\s+$/g, '');
            const tags = d.tags.map((tag) => tag.trim()).filter(Boolean);
            const vars = buildVariablesFromScript(body, d.variables);

            if (!isNewCommandTabId(tabId) && !skipVarRemovalCheckRef.current) {
                const removedWithPresets = computeRemovedVarsWithPresets(tabId, vars);
                if (removedWithPresets.length > 0) {
                    pendingDirectSaveBodyRef.current = null;
                    setModal({ type: 'confirmVarRemoval', removedVars: removedWithPresets, tabId });
                    return;
                }
            }

            try {
                if (isNewCommandTabId(tabId)) {
                    const cmd = await CreateCommand(
                        title,
                        description,
                        body,
                        d.categoryId,
                        tags,
                        vars,
                        d.workingDir,
                    );
                    await loadData();
                    const savedBody = await GetScriptBody(cmd.id);
                    const saved = draftFromCommand(cmd, savedBody);
                    setTabDrafts((prev) => {
                        const next = { ...prev };
                        delete next[tabId];
                        next[cmd.id] = saved;
                        return next;
                    });
                    setTabBaselines((prev) => {
                        const next = { ...prev };
                        delete next[tabId];
                        next[cmd.id] = cloneDraft(saved);
                        return next;
                    });
                    setOpenTabs((prev) =>
                        prev.map((tt) =>
                            tt.id === tabId ? { id: cmd.id, title: getCommandDisplayTitle(cmd) } : tt,
                        ),
                    );
                    // Only switch to the newly created command if the original tab is still active
                    if (activeTabIdRef.current === tabId) {
                        setActiveTabId(cmd.id);
                        setSelectedCommand(cmd);
                    }
                    toast.success(t('toast.commandCreated'));
                } else {
                    await UpdateCommand(
                        tabId,
                        title,
                        description,
                        body,
                        d.categoryId,
                        tags,
                        vars,
                        d.workingDir,
                    );
                    await loadData();
                    const cmd = allCommandsRef.current.find((c) => c.id === tabId);
                    if (cmd) {
                        const body = await GetScriptBody(cmd.id);
                        const saved = draftFromCommand(cmd, body);
                        setTabDrafts((prev) => ({ ...prev, [tabId]: saved }));
                        setTabBaselines((prev) => ({ ...prev, [tabId]: cloneDraft(saved) }));
                        // Only update selectedCommand if this tab is still active
                        if (activeTabIdRef.current === tabId) {
                            setSelectedCommand(cmd);
                        }
                    }
                    toast.success(t('toast.commandSaved'));
                }
            } catch (err) {
                console.error('Failed to save command:', err);
            }
        },
        // eslint-disable-next-line react-hooks/exhaustive-deps -- refs via useSyncedRef are stable
        [loadData, t],
    );

    const closeTab = (tabId: string) => {
        const d = tabDrafts[tabId];
        const b = tabBaselines[tabId];
        const dirty = d && b && !draftsEqual(d, b);
        if (dirty) {
            pendingCloseTabIdRef.current = tabId;
            setModal({ type: 'confirmDiscard' });
            return;
        }
        finalizeCloseTab(tabId);
    };

    const handleSelectTab = (tabId: string) => {
        if (tabId === activeTabId) return;
        if (activeTabId) {
            prevTabIdRef.current = activeTabId;
        }
        setActiveTabId(tabId);
        if (isNewCommandTabId(tabId)) {
            const d = tabDraftsRef.current[tabId];
            setSelectedCommand(makePlaceholderCommand(tabId, d?.categoryId));
        } else {
            const cmd = allCommandsRef.current.find((c) => c.id === tabId);
            if (cmd) setSelectedCommand(cmd);
        }
    };

    const tabsForBar = useMemo(
        () =>
            openTabs
                .filter((tab) => tab.id !== '__welcome__')
                .map((tab) => {
                    const d = tabDrafts[tab.id];
                    const b = tabBaselines[tab.id];
                    const dirty = !!(d && b && !draftsEqual(d, b));
                    let title = tab.title;
                    if (d) {
                        const trimmedTitle = d.title.trim();
                        if (trimmedTitle) {
                            title = trimmedTitle;
                        } else {
                            const body = d.scriptBody.replace(/\n/g, ' ').trim();
                            if (body.length > 0) {
                                title = body.length > 50 ? body.slice(0, 50) + '...' : body;
                            } else if (isNewCommandTabId(tab.id)) {
                                title = t('commandEditor.newCommand');
                            } else {
                                title = t('common.untitled');
                            }
                        }
                    }
                    return { ...tab, title, isDirty: dirty };
                }),
        [openTabs, tabDrafts, tabBaselines, t],
    );

    const activeDraft = activeTabId ? tabDrafts[activeTabId] : null;
    const activeDirty =
        activeTabId && activeDraft && tabBaselines[activeTabId]
            ? !draftsEqual(activeDraft, tabBaselines[activeTabId])
            : false;

    const handleCreateCategory = async (data: { name: string; color: string }) => {
        try {
            await CreateCategory(data.name, '', data.color);
            await loadData();
            setModal({ type: 'none' });
            toast.success(t('toast.categoryCreated'));
        } catch (err) {
            console.error('Failed to create category:', err);
        }
    };

    const handleUpdateCategory = async (data: { name: string; color: string }) => {
        if (modal.type !== 'categoryEditor' || !modal.category) return;
        try {
            await UpdateCategory(modal.category.id, data.name, '', data.color);
            await loadData();
            setModal({ type: 'none' });
            toast.success(t('toast.categorySaved'));
        } catch (err) {
            console.error('Failed to update category:', err);
        }
    };

    const handleDeleteCategory = async (catId: string) => {
        try {
            await DeleteCategory(catId);
            if (selectedCommand?.categoryId === catId) {
                setSelectedCommand(null);
            }
            await loadData();
            toast.success(t('toast.categoryDeleted'));
        } catch (err) {
            console.error('Failed to delete category:', err);
        }
    };

    const isSavedCommandDraftDirty = useCallback((commandId: string) => {
        const d = tabDraftsRef.current[commandId];
        const b = tabBaselinesRef.current[commandId];
        return !!(d && b && !draftsEqual(d, b));
    // eslint-disable-next-line react-hooks/exhaustive-deps -- refs via useSyncedRef are stable
    }, []);

    const runCommandDirect = useCallback(async (commandId: string, variables: Record<string, string>) => {
        const execTabId = activeTabIdRef.current;
        executingTabIdRef.current = execTabId;
        setExecutingTabIdState(execTabId);
        expandTerminal();

        try {
            const result = await RunCommand(commandId, variables);
            if (result.error || result.exitCode === -1) {
                if (execTabId === activeTabIdRef.current) {
                    toast.error(t('toast.commandFailed', { code: result.exitCode ?? -1 }));
                }
            }
        } catch {
            if (execTabId === activeTabIdRef.current) {
                toast.error(t('toast.commandFailed', { code: -1 }));
            }
        } finally {
            executingTabIdRef.current = null;
            setExecutingTabIdState(null);
        }
    // eslint-disable-next-line react-hooks/exhaustive-deps -- refs via useSyncedRef are stable
    }, [t]);

    const handleExecute = useCallback(async (tabId: string, values: Record<string, string>) => {
        if (isNewCommandTabId(tabId)) return;
        if (isSavedCommandDraftDirty(tabId)) {
            toast.message(t('toast.saveBeforeExecute'));
            return;
        }
        runCommandDirect(tabId, values);
    }, [isSavedCommandDraftDirty, runCommandDirect, t]);

    const handleDeleteCommand = async (cmd: Command) => {
        try {
            await DeleteCommand(cmd.id);
            if (selectedCommand?.id === cmd.id) {
                closeTab(cmd.id);
            }
            await loadData();
            toast.success(t('toast.commandDeleted'));
        } catch (err) {
            console.error('Failed to delete command:', err);
        }
    };

    const handleReorderCommand = async (id: string, newPosition: number, newCategoryId: string) => {
        const prev = commands;
        const prevAll = allCommandsRef.current;
        const optimistic = prev.map(cmd => {
            if (cmd.id === id) {
                return { ...cmd, categoryId: newCategoryId, position: newPosition };
            }
            return cmd;
        });
        allCommandsRef.current = optimistic;
        setCommands(optimistic);
        try {
            const reordered = await ReorderCommand(id, newPosition, newCategoryId);
            if (reordered) {
                allCommandsRef.current = reordered;
                setCommands(reordered);
            }
        } catch (err) {
            console.error('Failed to reorder command:', err);
            allCommandsRef.current = prevAll;
            setCommands(prev);
        }
    };

    const handleFillVariables = async (initialValues: Record<string, string>) => {
        if (!selectedCommand || isNewCommandTabId(selectedCommand.id)) return;
        const vars = await GetVariables(selectedCommand.id);
        setModal({
            type: 'fillVariables',
            variables: vars || [],
            commandId: selectedCommand.id,
            initialValues,
        });
    };

    const handleFillVariablesByTab = useCallback(async (tabId: string, initialValues: Record<string, string>) => {
        if (isNewCommandTabId(tabId)) return;
        const vars = await GetVariables(tabId);
        setModal({
            type: 'fillVariables',
            variables: vars || [],
            commandId: tabId,
            initialValues,
        });
    }, []);

    const handleVariableSubmit = async (values: Record<string, string>) => {
        if (!selectedCommand || isNewCommandTabId(selectedCommand.id)) return;
        if (isSavedCommandDraftDirty(selectedCommand.id)) {
            toast.message(t('toast.saveBeforeExecute'));
            return;
        }
        setModal({ type: 'none' });
        runCommandDirect(selectedCommand.id, values);
    };

    const handleSavePreset = async (name: string, values: Record<string, string>) => {
        if (modal.type !== 'managePresets') return;
        await SavePreset(modal.commandId, name, values);
        const presets = await GetPresets(modal.commandId);
        setModal({ ...modal, presets: presets || [] });
        toast.success(t('toast.presetCreated'));
    };

    const handleUpdatePreset = async (presetId: string, name: string, values: Record<string, string>) => {
        if (modal.type !== 'managePresets') return;
        await UpdatePreset(modal.commandId, presetId, name, values);
        const presets = await GetPresets(modal.commandId);
        setModal({ ...modal, presets: presets || [] });
        toast.success(t('toast.presetSaved'));
    };

    const handleDeletePreset = async (presetId: string) => {
        if (modal.type !== 'managePresets') return;
        await DeletePreset(modal.commandId, presetId);
        const presets = await GetPresets(modal.commandId);
        setModal({ ...modal, presets: presets || [] });
    };

    const refreshCommand = useCallback(async (commandId: string): Promise<Command | null> => {
        const cmds = await GetCommands();
        allCommandsRef.current = cmds || [];
        setCommands(cmds || []);
        const refreshed = (cmds || []).find((c: Command) => c.id === commandId) ?? null;
        if (refreshed && selectedCommandRef.current?.id === refreshed.id) {
            setSelectedCommand(refreshed);
        }
        return refreshed;
    // eslint-disable-next-line react-hooks/exhaustive-deps -- refs via useSyncedRef are stable
    }, []);

    const handleAddPresetForTab = useCallback(async (tabId: string, initialValues?: Record<string, string>): Promise<string> => {
        if (isNewCommandTabId(tabId)) return '';
        try {
            const created = await SavePreset(tabId, t('commandDetail.newPresetName'), initialValues ?? {});
            await refreshCommand(tabId);
            return created.id;
        } catch (err) {
            console.error('Failed to add preset:', err);
            toast.error(t('toast.presetAddFailed'));
            return '';
        }
    }, [t, refreshCommand]);

    const handleRenamePresetForTab = useCallback(async (tabId: string, presetId: string, newName: string) => {
        if (isNewCommandTabId(tabId)) return;
        const cmd = allCommandsRef.current.find((c) => c.id === tabId);
        const preset = cmd?.presets.find((p) => p.id === presetId);
        if (!preset) return;
        try {
            await UpdatePreset(tabId, presetId, newName, preset.values);
            await refreshCommand(tabId);
        } catch (err) {
            console.error('Failed to rename preset:', err);
            toast.error(t('toast.presetRenameFailed'));
        }
    }, [refreshCommand, t]);

    const handleDeletePresetForTab = useCallback(async (tabId: string, presetId: string) => {
        if (isNewCommandTabId(tabId)) return;
        try {
            await DeletePreset(tabId, presetId);
            await refreshCommand(tabId);
        } catch (err) {
            console.error('Failed to delete preset:', err);
            toast.error(t('toast.presetDeleteFailed'));
        }
    }, [refreshCommand, t]);

    const handleReorderPresetsForTab = useCallback(async (tabId: string, presetIds: string[]) => {
        if (isNewCommandTabId(tabId)) return;
        const cmd = allCommandsRef.current.find((c) => c.id === tabId);
        if (!cmd) return;
        const reordered = presetIds
            .map((id) => cmd.presets.find((p) => p.id === id))
            .filter(Boolean) as typeof cmd.presets;
        setSelectedCommand((prev) => (prev && prev.id === tabId) ? { ...prev, presets: reordered } : prev);
        try {
            await ReorderPresets(tabId, presetIds);
            await refreshCommand(tabId);
        } catch (err) {
            console.error('Failed to reorder presets:', err);
            toast.error(t('toast.presetReorderFailed'));
            await refreshCommand(tabId);
        }
    }, [refreshCommand, t]);

    const handleSavePresetValuesForTab = useCallback(async (tabId: string, presetId: string, values: Record<string, string>) => {
        if (isNewCommandTabId(tabId)) return;
        const cmd = allCommandsRef.current.find((c) => c.id === tabId);
        const preset = cmd?.presets.find((p) => p.id === presetId);
        if (!preset) return;
        try {
            await UpdatePreset(tabId, presetId, preset.name, values);
            await refreshCommand(tabId);
            toast.success(t('toast.presetSaved'));
        } catch (err) {
            console.error('Failed to save preset values:', err);
            toast.error(t('toast.presetSaveFailed'));
        }
    }, [t, refreshCommand]);

    const handleCloseManagePresets = async () => {
        setModal({ type: 'none' });
        const cmds = await loadData();
        if (selectedCommand) {
            const refreshed = cmds.find((c: Command) => c.id === selectedCommand.id);
            if (refreshed) setSelectedCommand(refreshed);
        }
    };

    const handleSaveScript = useCallback(async (tabId: string, scriptBody: string) => {
        if (isNewCommandTabId(tabId)) return;
        const d = tabDraftsRef.current[tabId];
        if (!d) return;
        const strippedBody = scriptBody.replace(/^\s+|\s+$/g, '');
        const vars = buildVariablesFromScript(strippedBody, d.variables);

        if (!skipVarRemovalCheckRef.current) {
            const removedWithPresets = computeRemovedVarsWithPresets(tabId, vars);
            if (removedWithPresets.length > 0) {
                pendingDirectSaveBodyRef.current = scriptBody;
                setModal({ type: 'confirmVarRemoval', removedVars: removedWithPresets, tabId });
                return;
            }
        }

        try {
            await UpdateCommand(tabId, d.title.trim(), d.description.trim(), strippedBody, d.categoryId, d.tags.map(tag => tag.trim()).filter(Boolean), vars, d.workingDir);
            await loadData();
            const cmd = allCommandsRef.current.find(c => c.id === tabId);
            if (cmd) {
                const body = await GetScriptBody(cmd.id);
                const saved = draftFromCommand(cmd, body);
                setTabDrafts(prev => ({ ...prev, [tabId]: saved }));
                setTabBaselines(prev => ({ ...prev, [tabId]: cloneDraft(saved) }));
                if (activeTabIdRef.current === tabId) {
                    setSelectedCommand(cmd);
                }
            }
            toast.success(t('toast.commandSaved'));
        } catch (err) {
            console.error('Failed to save script:', err);
        }
    // eslint-disable-next-line react-hooks/exhaustive-deps -- refs via useSyncedRef are stable
    }, [loadData, t]);

    const handleSelectCommand = (cmd: Command) => {
        openTab(cmd);
    };

    /* eslint-disable react-hooks/refs -- keyboard shortcuts use ref-based handlers (not called during render) */
    useKeyboardShortcuts({
        [`${cmdOrCtrl}+p`]: () => setPaletteOpen(true),
        'ctrl+p': () => setPaletteOpen(true),

        [`${cmdOrCtrl}+s`]: () => {
            if (modal.type !== 'none' || !activeTabId || !activeDirty) return;
            void handleSaveTab(activeTabId);
        },

        [`${cmdOrCtrl}+enter`]: () => {
            const el = document.activeElement;
            if (el && (el.tagName === 'TEXTAREA' || el.tagName === 'INPUT')) return;
            if (!selectedCommand || modal.type !== 'none' || isNewCommandTabId(selectedCommand.id)) return;
            if (resolvedVariables.length === 0) {
                handleExecute(selectedCommand!.id, {});
            } else {
                const hasEmpty = resolvedVariables.some((v) => !currentResolvedValues[v.name]);
                if (hasEmpty) {
                    handleFillVariables(currentResolvedValues);
                } else {
                    handleExecute(selectedCommand!.id, currentResolvedValues);
                }
            }
        },

        [`${cmdOrCtrl}+n`]: () => openNewCommandTab(),
        [`${cmdOrCtrl}+t`]: () => {
            if (isFocusInTerminalPane()) {
                createTerminalSession(); // D-01: new terminal session
            } else {
                openNewCommandTab(); // existing behavior: new command tab
            }
        },

        [`${cmdOrCtrl}+f`]: () => setPaletteOpen(true),

        [`${cmdOrCtrl}+,`]: async () => {
            await openSettingsWithToast();
        },

        'ctrl+w': () => {
            if (isFocusInTerminalPane()) {
                // D-02: close active terminal session, no-op if last tab
                if (sessions.length > 1 && activeSessionId) {
                    closeTerminalSession(activeSessionId);
                }
            } else {
                if (activeTabId) closeTab(activeTabId);
            }
        },
        'meta+w': () => {
            if (isFocusInTerminalPane()) {
                if (sessions.length > 1 && activeSessionId) {
                    closeTerminalSession(activeSessionId);
                }
            } else {
                if (activeTabId) closeTab(activeTabId);
            }
        },

        'ctrl+tab': () => {
            if (isFocusInTerminalPane()) {
                // D-05: cycle terminal sessions forward, wrap around
                if (sessions.length < 2) return;
                const idx = sessions.findIndex(s => s.id === activeSessionId);
                const next = sessions[(idx + 1) % sessions.length];
                if (next) {
                    switchTerminalSession(next.id);
                    requestAnimationFrame(() => {
                        terminalRefs.current[next.id]?.focus();
                    });
                }
            } else {
                // existing behavior: cycle command tabs forward
                if (openTabs.length < 2) return;
                const idx = openTabs.findIndex((t) => t.id === activeTabId);
                const next = openTabs[(idx + 1) % openTabs.length];
                if (next) handleSelectTab(next.id);
            }
        },

        'ctrl+shift+tab': () => {
            if (isFocusInTerminalPane()) {
                // D-05: cycle terminal sessions backward, wrap around
                if (sessions.length < 2) return;
                const idx = sessions.findIndex(s => s.id === activeSessionId);
                const prev = sessions[(idx - 1 + sessions.length) % sessions.length];
                if (prev) {
                    switchTerminalSession(prev.id);
                    requestAnimationFrame(() => {
                        terminalRefs.current[prev.id]?.focus();
                    });
                }
            } else {
                // existing behavior: cycle command tabs backward
                if (openTabs.length < 2) return;
                const idx = openTabs.findIndex((t) => t.id === activeTabId);
                const prev = openTabs[(idx - 1 + openTabs.length) % openTabs.length];
                if (prev) handleSelectTab(prev.id);
            }
        },

        'ctrl+`': () => {
            if (terminalCollapsed) {
                expandTerminal();
            } else {
                collapseTerminal();
            }
        },

        [`${cmdOrCtrl}+shift+backspace`]: () => {
            if (activeTabId && activeDirty) {
                handleDiscardTab(activeTabId);
            }
        },

        // Cmd+1-6: jump to nth tab
        ...Object.fromEntries(
            [1, 2, 3, 4, 5, 6].map((n) => [
                `${cmdOrCtrl}+${n}`,
                () => {
                    const tabs = openTabs.filter((tt) => tt.id !== '__welcome__');
                    if (tabs.length >= n) handleSelectTab(tabs[n - 1].id);
                },
            ]),
        ),
        // Cmd+9: toggle between current and previous tab
        [`${cmdOrCtrl}+9`]: () => {
            const prev = prevTabIdRef.current;
            if (prev && openTabs.some((t) => t.id === prev)) {
                handleSelectTab(prev);
            }
        },
        // Cmd+0: jump to last tab
        [`${cmdOrCtrl}+0`]: () => {
            const tabs = openTabs.filter((tt) => tt.id !== '__welcome__');
            if (tabs.length > 0) handleSelectTab(tabs[tabs.length - 1].id);
        },

        ...(paletteOpen ? { escape: () => setPaletteOpen(false) } : {}),
    });
    /* eslint-enable react-hooks/refs */

    // Memoize per-tab variable definitions so inactive tabs get stable references
    // (prevents React.memo bypass from new array on every App render).
    const tabVariablesMap = useMemo(() => {
        const map: Record<string, VarPromptType[]> = {};
        for (const tab of openTabs) {
            if (tab.id === '__welcome__') continue;
            const draft = tabDrafts[tab.id];
            if (draft && !isNewCommandTabId(tab.id)) {
                map[tab.id] = variableDefinitionsToPrompts(draft.variables);
            }
        }
        return map;
    }, [tabDrafts, openTabs]);

    // Only the executing tab should receive isExecuting=true (prevents React.memo
    // bypass on all mounted CommandDetail instances when execution state changes).
    // Driven by state so the executing tab remains pinned even if user switches tabs.
    const executingTabId = executingTabIdState;

    return (
        <TooltipProvider disableHoverableContent>
            <div className="app-layout">
                <div className="app-body">
                    <ResizablePanel
                        side="left"
                        defaultWidth={280}
                        minWidth={190}
                        maxWidth={460}
                        storageKey="cmdex-sidebar"
                        collapsedIcon={
                            <div className="logo-icon" style={{ width: 22, height: 22 }}>
                                <MainLogo width="22" height="22" />
                            </div>
                        }
                    >
                        <Sidebar
                            categories={categories}
                            commands={commands}
                            selectedCommandId={selectedCommand?.id || null}
                            onSelectCommand={handleSelectCommand}
                            onAddCategory={() => setModal({ type: 'categoryEditor' })}
                            onEditCategory={(cat) => setModal({ type: 'categoryEditor', category: cat })}
                            onDeleteCategory={handleDeleteCategory}
                            onAddCommand={(catId) => openNewCommandTab(catId)}
                            onDeleteCommand={handleDeleteCommand}
                            onReorderCommand={handleReorderCommand}
                            onOpenSettings={() => openSettingsWithToast()}
                            onImport={async () => {
                                const [cats, cmds] = await Promise.all([GetCategories(), GetCommands()]);
                                setCategories(cats || []);
                                setCommands(cmds || []);
                            }}
                        />
                    </ResizablePanel>

                    <div className="center-area">
                        <TabBar
                            tabs={tabsForBar}
                            activeTabId={activeTabId}
                            onSelectTab={handleSelectTab}
                            onCloseTab={closeTab}
                        />

                        <div className="center-area-split">
                            <div className="center-area-editor">
                                <div className="main-content" ref={mainContentRef}>
                                    {/* Loading state: selectedCommand exists but draft hasn't hydrated yet */}
                                    {selectedCommand && !activeDraft && (
                                        <div className="main-body">
                                            <p className="text-muted-foreground text-sm p-4">{t('common.loading')}</p>
                                        </div>
                                    )}

                                    {/* Welcome state: no command selected and no active draft */}
                                    {!selectedCommand && !activeDraft && (
                                        <div className="main-body">
                                            <WelcomeTab onNewCommand={() => openNewCommandTab()} />
                                        </div>
                                    )}

                                    {/* Per-tab mounts: one CommandDetailTab per open command tab.
                                        Inactive tabs are hidden via display:none so their DOM state
                                        (scroll, cursor, textarea undo) survives across tab switches. */}
                                    {openTabs
                                        .filter((tab) => tab.id !== '__welcome__')
                                        .map((tab) => {
                                            const draft = tabDrafts[tab.id];
                                            const baseline = tabBaselines[tab.id];
                                            const isTabNew = isNewCommandTabId(tab.id);
                                            const command = isTabNew
                                                ? makePlaceholderCommand(tab.id, draft?.categoryId)
                                                : commands.find((c) => c.id === tab.id) ?? null;
                                            const isTabDirty = !!(draft && baseline && !draftsEqual(draft, baseline));
                                            const isTabActive = tab.id === activeTabId;

                                            const tabVariables = isTabActive
                                                ? resolvedVariables
                                                : (tabVariablesMap[tab.id] ?? []);

                                            if (!command || !draft) return null;

                                            return (
                                                <CommandDetailTab
                                                    key={tab.id}
                                                    tabId={tab.id}
                                                    command={command}
                                                    draft={draft}
                                                    baseline={baseline}
                                                    isTabNew={isTabNew}
                                                    isTabActive={isTabActive}
                                                    isTabDirty={isTabDirty}
                                                    isExecuting={tab.id === executingTabId}
                                                    variables={tabVariables}
                                                    currentOS={currentOS}
                                                    defaultWorkingDir={defaultWorkingDir}
                                                    onDraftChange={(partial) => updateDraft(tab.id, partial)}
                                                    onExecute={handleExecute}
                                                    onFillVariables={handleFillVariablesByTab}
                                                    onRenamePreset={handleRenamePresetForTab}
                                                    onDeletePreset={handleDeletePresetForTab}
                                                    onAddPreset={handleAddPresetForTab}
                                                    onSavePresetValues={handleSavePresetValuesForTab}
                                                    onReorderPresets={handleReorderPresetsForTab}
                                                    onSaveScript={handleSaveScript}
                                                    onResolvedValuesChange={isTabActive ? setCurrentResolvedValues : undefined}
                                                    onSave={() => void handleSaveTab(tab.id)}
                                                    onDiscard={() => handleDiscardTab(tab.id)}
                                                />
                                            );
                                        })}

                                </div>
                            </div>

                            {!terminalCollapsed && (
                                <>
                                <TerminalTabBar
                                    sessions={sessions}
                                    activeSessionId={activeSessionId}
                                    onSelectTab={switchTerminalSession}
                                    onCloseTab={closeTerminalSession}
                                    onReorderTabs={handleReorderTerminalTabs}
                                    onCreateSession={createTerminalSession}
                                    onRenameSession={renameTerminalSession}
                                />
                                <div
                                    className={`terminal-divider ${isDragging ? 'dragging' : ''}`}
                                    onMouseDown={handleTerminalResizeStart}
                                >
                                    <button
                                        className="terminal-collapse-btn"
                                        onMouseDown={(e) => e.stopPropagation()}
                                        onClick={collapseTerminal}
                                        aria-label="Collapse terminal panel"
                                    >
                                        ▼
                                    </button>
                                    <button
                                        className="terminal-clear-btn"
                                        onMouseDown={(e) => e.stopPropagation()}
                                        onClick={() => {
                                            const ref = terminalRefs.current[activeSessionId];
                                            if (ref) ref.clear();
                                        }}
                                        aria-label="Clear terminal"
                                        title="Clear terminal (Ctrl+L)"
                                    >
                                        {t('common.clear')}
                                    </button>
                                    <button
                                        className="terminal-copy-btn"
                                        onMouseDown={(e) => e.stopPropagation()}
                                        onClick={() => {
                                            const ref = terminalRefs.current[activeSessionId];
                                            const output = ref?.getLastOutput() || '';
                                            if (!output) return;
                                            copyText(output).then(() => {
                                                toast.success('Output copied');
                                            }).catch((e) => {
                                                console.error('Failed to copy:', e);
                                                toast.error('Failed to copy');
                                            });
                                        }}
                                        aria-label="Copy terminal output"
                                        title="Copy last command output"
                                    >
                                        {t('common.copyLastOutput')}
                                    </button>
                                </div>
                            </>
                            )}

                            <div
                                className="terminal-pane"
                                style={terminalCollapsed
                                    ? { height: 8, minHeight: 8, maxHeight: 8 }
                                    : { height: terminalHeight, minHeight: MIN_TERM_HEIGHT, maxHeight: maxTermHeight }
                                }
                            >
                                <Suspense fallback={null}>
                                {/* eslint-disable-next-line react-hooks/refs -- intentional: terminalOrderRef tracks stable iteration order; pair with setSessions() updates to trigger re-render */}
                                {terminalOrderRef.current.map((id) => {
                                    const session = sessions.find(s => s.id === id);
                                    if (!session) return null;
                                    return (
                                        <TerminalComponent
                                            key={id}
                                            ref={(el) => {
                                                if (el) {
                                                    terminalRefs.current[id] = el;
                                                } else {
                                                    delete terminalRefs.current[id];
                                                }
                                            }}
                                            isVisible={id === activeSessionId && !terminalCollapsed}
                                            theme={theme}
                                            sessionId={id}
                                            onShellExit={() => {
                                                // Mark session as stopped
                                                setSessions(prev => prev.map(s =>
                                                    s.id === id ? { ...s, running: false } : s
                                                ));
                                                // If this was the active session and it exited, collapse terminal
                                                if (id === activeSessionId) {
                                                    collapseTerminal();
                                                }
                                            }}
                                        />
                                    );
                                })}
                                </Suspense>
                                {terminalCollapsed && (
                                    <button
                                        className="terminal-collapsed-rail"
                                        onClick={expandTerminal}
                                        aria-label="Expand terminal panel"
                                    >
                                        ▲
                                    </button>
                                )}
                            </div>
                        </div>
                    </div>
                </div>

                {modal.type === 'categoryEditor' && (
                    <CategoryEditor
                        category={modal.category}
                        onSave={modal.category ? handleUpdateCategory : handleCreateCategory}
                        onCancel={() => setModal({ type: 'none' })}
                    />
                )}

                {modal.type === 'managePresets' && (
                    <VariablePrompt
                        mode="manage"
                        variables={modal.variables}
                        presets={modal.presets}
                        defaultPresetId={lastSelectedPresetId}
                        onPresetChange={setLastSelectedPresetId}
                        onSubmit={handleVariableSubmit}
                        onCancel={handleCloseManagePresets}
                        onSavePreset={handleSavePreset}
                        onUpdatePreset={handleUpdatePreset}
                        onDeletePreset={handleDeletePreset}
                    />
                )}

                {modal.type === 'fillVariables' && (
                    <VariablePrompt
                        mode="fill"
                        variables={modal.variables}
                        presets={[]}
                        initialValues={modal.initialValues}
                        onSubmit={handleVariableSubmit}
                        onCancel={() => setModal({ type: 'none' })}
                        onSavePreset={async () => {}}
                        onUpdatePreset={async () => {}}
                        onDeletePreset={async () => {}}
                    />
                )}

                <AlertDialog
                    open={modal.type === 'confirmDiscard'}
                    onOpenChange={(open) => {
                        if (!open) {
                            pendingCloseTabIdRef.current = null;
                            setModal({ type: 'none' });
                        }
                    }}
                >
                    <AlertDialogContent data-testid="confirm-dialog">
                        <AlertDialogHeader>
                            <AlertDialogTitle>{t('app.discardTitle')}</AlertDialogTitle>
                            <AlertDialogDescription>{t('app.discardDescription')}</AlertDialogDescription>
                        </AlertDialogHeader>
                        <AlertDialogFooter>
                            <AlertDialogCancel data-testid="confirm-dialog-cancel">{t('app.cancel')}</AlertDialogCancel>
                        <AlertDialogAction
                            variant="destructive"
                            data-testid="confirm-dialog-confirm"
                            onClick={() => {
                                    setModal({ type: 'none' });
                                    const tabId = pendingCloseTabIdRef.current;
                                    pendingCloseTabIdRef.current = null;
                                    if (tabId) finalizeCloseTab(tabId);
                                }}
                            >
                                {t('app.discard')}
                            </AlertDialogAction>
                        </AlertDialogFooter>
                    </AlertDialogContent>
                </AlertDialog>
                <AlertDialog
                    open={modal.type === 'confirmVarRemoval'}
                    onOpenChange={(open) => { if (!open) setModal({ type: 'none' }); }}
                >
                    <AlertDialogContent>
                        <AlertDialogHeader>
                            <AlertDialogTitle>{t('commandDetail.varRemovalTitle')}</AlertDialogTitle>
                            <AlertDialogDescription className="space-y-2">
                                <span>{t('commandDetail.varRemovalDescription')}</span>
                                <div className="mt-2 flex flex-wrap gap-1.5">
                                    {modal.type === 'confirmVarRemoval' && modal.removedVars.map((v) => (
                                        <Badge key={v} variant="destructive" className="font-mono text-xs">
                                            {'{{' + v + '}}'}
                                        </Badge>
                                    ))}
                                </div>
                                <span className="block mt-2 text-xs text-muted-foreground">
                                    {t('commandDetail.varRemovalNote')}
                                </span>
                            </AlertDialogDescription>
                        </AlertDialogHeader>
                        <AlertDialogFooter>
                            <AlertDialogCancel onClick={() => setModal({ type: 'none' })}>
                                {t('commandDetail.cancel')}
                            </AlertDialogCancel>
                            <AlertDialogAction
                                variant="destructive"
                                onClick={async () => {
                                    if (modal.type !== 'confirmVarRemoval') return;
                                    setModal({ type: 'none' });
                                    skipVarRemovalCheckRef.current = true;
                                    if (pendingDirectSaveBodyRef.current) {
                                        await handleSaveScript(modal.tabId, pendingDirectSaveBodyRef.current);
                                        pendingDirectSaveBodyRef.current = null;
                                    } else {
                                        await handleSaveTab(modal.tabId);
                                    }
                                    skipVarRemovalCheckRef.current = false;
                                }}
                            >
                                {t('commandEditor.save')}
                            </AlertDialogAction>
                        </AlertDialogFooter>
                    </AlertDialogContent>
                </AlertDialog>
                <CommandPalette
                    open={paletteOpen}
                    commands={commands}
                    categories={categories}
                    onClose={() => setPaletteOpen(false)}
                    onOpen={handleSelectCommand}
                />
                <Toaster position="bottom-right" richColors closeButton duration={3000} />
                <KeyboardShortcutsDialog open={shortcutsDialogOpen} onOpenChange={setShortcutsDialogOpen} />
            </div>
        </TooltipProvider>
    );
}

export default App;
