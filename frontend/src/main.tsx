import React, { useState, useEffect, useCallback, useRef, lazy } from 'react'
import {createRoot} from 'react-dom/client'
import './i18n'
import './style.css'
import Launcher from './components/Launcher'
import { GetSettings, SetSettings } from '../bindings/cmdex/settingsservice'
import { ResetAllData } from '../bindings/cmdex/commandservice'
import { THEMES, type CustomTheme } from './types'
import { Events } from '@wailsio/runtime'
import { eventNames } from './wails/events'
import { toast } from 'sonner'
import { applyTheme, applyDensity, applyFonts, parseCustomThemes, resolveActiveCustomTheme } from './lib/theme-apply'

const App = lazy(() => import('./App'))
const SettingsPage = lazy(() => import('./components/SettingsPage'))

const container = document.getElementById('root')

const windowKind = new URLSearchParams(window.location.search).get('window')
const isSettingsWindow = windowKind === 'settings'
const isLauncherWindow = windowKind === 'launcher'

const root = createRoot(container!)

if (isLauncherWindow) {
    // The global quick launcher window. It only needs theming applied — all of
    // its behaviour lives in <Launcher />, which stays mounted for the lifetime
    // of the app because the Go side shows/hides the window rather than
    // creating and destroying it.
    function LauncherWindow() {
        const [theme, setTheme] = useState('vscode-dark')
        const settingsGenerationRef = useRef(0)

        const applySettings = useCallback((s: Awaited<ReturnType<typeof GetSettings>> | null) => {
            if (!s) return
            const t = s.theme || 'vscode-dark'
            const custom = resolveActiveCustomTheme(s.customThemes, t)
            setTheme(t)
            applyTheme(t, custom?.colors)
            applyDensity(s.density || 'comfortable')
            applyFonts(s.uiFont || 'Inter', s.monoFont || 'JetBrains Mono')
        }, [])

        useEffect(() => {
            const generation = ++settingsGenerationRef.current
            GetSettings().then(settings => {
                if (generation === settingsGenerationRef.current) applySettings(settings)
            }).catch(() => {})
        }, [applySettings])

        // Stay in sync when preferences change in the settings window.
        useEffect(() => {
            const cleanup = Events.On(eventNames.settingsChanged, () => {
                const generation = ++settingsGenerationRef.current
                GetSettings().then(settings => {
                    if (generation === settingsGenerationRef.current) applySettings(settings)
                }).catch(() => {})
            })
            return () => cleanup()
        }, [applySettings])

        return <Launcher theme={theme} />
    }

    root.render(
        <React.StrictMode>
            <LauncherWindow />
        </React.StrictMode>
    )
} else if (isSettingsWindow) {
    function SettingsWindow() {
        const [theme, setTheme] = useState('vscode-dark')
        const [density, setDensity] = useState('comfortable')
        const [uiFont, setUiFont] = useState('Inter')
        const [monoFont, setMonoFont] = useState('JetBrains Mono')
        const [customThemes, setCustomThemes] = useState<CustomTheme[]>([])
        const [locale, setLocale] = useState('en')
        const [lastDarkTheme, setLastDarkTheme] = useState('vscode-dark')
        const [lastLightTheme, setLastLightTheme] = useState('vscode-light')
        const [windowX, setWindowX] = useState(-1)
        const [windowY, setWindowY] = useState(-1)
        const [windowWidth, setWindowWidth] = useState(640)
        const [windowHeight, setWindowHeight] = useState(520)
        const customThemesStrRef = useRef('[]')
        // Mirrors `customThemes` so handlers that run before the next render
        // (import -> onThemeChange in the same tick) still see the latest list.
        const customThemesRef = useRef<CustomTheme[]>([])

        const syncCustomThemes = useCallback((list: CustomTheme[]) => {
            customThemesRef.current = list
            customThemesStrRef.current = JSON.stringify(list)
            setCustomThemes(list)
        }, [])

        useEffect(() => {
            GetSettings().then(s => {
                if (!s) return
                const t = s.theme || 'vscode-dark'
                setTheme(t)
                setDensity(s.density || 'comfortable')
                setUiFont(s.uiFont || 'Inter')
                setMonoFont(s.monoFont || 'JetBrains Mono')
                setLocale(s.locale || 'en')
                if (s.windowX !== undefined) setWindowX(s.windowX)
                if (s.windowY !== undefined) setWindowY(s.windowY)
                if (s.windowWidth !== undefined) setWindowWidth(s.windowWidth)
                if (s.windowHeight !== undefined) setWindowHeight(s.windowHeight)
                const parsed = parseCustomThemes(s.customThemes)
                // Synchronize the empty list too: a settings refresh must not
                // leave themes from an earlier state visible after they were
                // removed or reset elsewhere.
                syncCustomThemes(parsed)
                const loadedCustom = parsed.find(c => c.id === t)
                applyTheme(t, loadedCustom?.colors)
                applyDensity(s.density || 'comfortable')
                applyFonts(s.uiFont || 'Inter', s.monoFont || 'JetBrains Mono')
            }).catch(() => {})
        }, [syncCustomThemes])

        const persistSettings = useCallback(async (newSettings: Record<string, unknown>) => {
            try {
                await SetSettings(JSON.stringify(newSettings))
                Events.Emit(eventNames.settingsChanged, newSettings)
            } catch (err) {
                toast.error('Failed to save settings: ' + String(err))
                console.error('SetSettings error:', err)
            }
        }, [])

        const handleThemeChange = useCallback((newTheme: string) => {
            const builtIn = THEMES.find(t => t.id === newTheme)
            const custom = customThemesRef.current.find(t => t.id === newTheme)
            const themeType = builtIn?.type ?? custom?.type ?? 'dark'
            applyTheme(newTheme, custom?.colors)
            if (themeType === 'dark') {
                setLastDarkTheme(newTheme)
                document.documentElement.style.setProperty('--cmdex-last-dark-theme', newTheme)
            } else {
                setLastLightTheme(newTheme)
                document.documentElement.style.setProperty('--cmdex-last-light-theme', newTheme)
            }
            setTheme(newTheme)
            const newSettings = {
                locale, theme: newTheme,
                lastDarkTheme: themeType === 'dark' ? newTheme : lastDarkTheme,
                lastLightTheme: themeType === 'light' ? newTheme : lastLightTheme,
                customThemes: customThemesStrRef.current,
                uiFont, monoFont, density,
                windowX, windowY, windowWidth, windowHeight,
            }
            persistSettings(newSettings)
        }, [locale, uiFont, monoFont, density, persistSettings, lastDarkTheme, lastLightTheme, windowX, windowY, windowWidth, windowHeight])

        const handleImportTheme = useCallback((newTheme: CustomTheme) => {
            const updated = [...customThemesRef.current, newTheme]
            syncCustomThemes(updated)
            const newSettings = {
                locale, theme, lastDarkTheme, lastLightTheme,
                customThemes: customThemesStrRef.current, uiFont, monoFont, density,
                windowX, windowY, windowWidth, windowHeight,
            }
            persistSettings(newSettings)
        }, [syncCustomThemes, locale, theme, uiFont, monoFont, density, persistSettings, lastDarkTheme, lastLightTheme, windowX, windowY, windowWidth, windowHeight])

        const handleRemoveCustomTheme = useCallback((themeId: string) => {
            const updated = customThemesRef.current.filter(t => t.id !== themeId)
            syncCustomThemes(updated)
            // Removing the selected theme must persist exactly once. Saving the
            // deleted id here and then falling back in a second async save races:
            // whichever RPC lands last decides, and the DB (or the main window)
            // can end up pointing at a theme that no longer exists.
            if (theme === themeId) {
                handleThemeChange('vscode-dark')
                return
            }
            const newSettings = {
                locale, theme, lastDarkTheme, lastLightTheme,
                customThemes: customThemesStrRef.current, uiFont, monoFont, density,
                windowX, windowY, windowWidth, windowHeight,
            }
            persistSettings(newSettings)
        }, [syncCustomThemes, locale, theme, uiFont, monoFont, density, persistSettings, lastDarkTheme, lastLightTheme, windowX, windowY, windowWidth, windowHeight, handleThemeChange])

        const handleResetAllData = useCallback(async () => {
            await ResetAllData()
            // db.ResetAll truncates app_settings too, so every field this
            // window holds in local state is now stale — not just
            // customThemes. handleThemeChange/handleImportTheme/
            // handleRemoveCustomTheme all close over theme/lastDarkTheme/
            // lastLightTheme/uiFont/monoFont/density/locale and persist them
            // verbatim; a stale `theme` in particular can point at a
            // since-deleted custom theme id, and importing a new theme
            // afterward fires two unordered persistSettings() writes
            // (one from handleImportTheme, one from the onThemeChange call
            // right after it) where the stale one can finish last and
            // persist a dangling theme id. Re-sync everything from the
            // fresh post-reset settings before telling the main window to
            // reload too.
            const s = await GetSettings().catch(() => null)
            const t = s?.theme || 'vscode-dark'
            setTheme(t)
            setDensity(s?.density || 'comfortable')
            setUiFont(s?.uiFont || 'Inter')
            setMonoFont(s?.monoFont || 'JetBrains Mono')
            setLocale(s?.locale || 'en')
            setLastDarkTheme(s?.lastDarkTheme || 'vscode-dark')
            setLastLightTheme(s?.lastLightTheme || 'vscode-light')
            syncCustomThemes([])
            applyTheme(t)
            applyDensity(s?.density || 'comfortable')
            applyFonts(s?.uiFont || 'Inter', s?.monoFont || 'JetBrains Mono')
            // The main window is holding stale commands/categories *and*
            // stale settings — tell it to reload both rather than requiring a
            // restart.
            Events.Emit(eventNames.dataReset)
        }, [syncCustomThemes])

        return (
            <SettingsPage
                theme={theme}
                onThemeChange={handleThemeChange}
                customThemes={customThemes}
                onImportTheme={handleImportTheme}
                onRemoveCustomTheme={handleRemoveCustomTheme}
                onResetAllData={handleResetAllData}
                density={density}
                uiFont={uiFont}
                monoFont={monoFont}
            />
        )
    }

    root.render(
        <React.StrictMode>
            <React.Suspense fallback={null}>
                <SettingsWindow />
            </React.Suspense>
        </React.StrictMode>
    )
} else {
    root.render(
        <React.StrictMode>
            <React.Suspense fallback={null}>
                <App/>
            </React.Suspense>
        </React.StrictMode>
    )
}
