import React, { useState, useEffect, useCallback, useRef } from 'react';
import { useTranslation } from 'react-i18next';
import { useSyncedRef } from '../hooks/useSyncedRef';
import { Button } from '@/components/ui/button';
import { Label } from '@/components/ui/label';
import { Switch } from '@/components/ui/switch';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { Tabs, TabsList, TabsTrigger, TabsContent } from '@/components/ui/tabs';
import { ToggleGroup, ToggleGroupItem } from '@/components/ui/toggle-group';
import { Upload, Download, X, FolderOpen } from 'lucide-react';
import { SetSettings, GetSettings } from '../../bindings/cmdex/settingsservice';
import { PickDirectory, GetOS } from '../../bindings/cmdex/app';
import { SaveThemeTemplate } from '../../bindings/cmdex/importexportservice';
import { THEMES, type OSKey, type CustomTheme } from '../types';
import { getOSPath, setOSPath, normalizeOS } from '../utils/path';
import { toast } from 'sonner';
import { Events } from '@wailsio/runtime';
import { eventNames } from '../wails/events';
import { applyTheme, applyDensity, applyFonts } from '../lib/theme-apply';

const LANGUAGES = [
  { code: 'en', label: 'English' },
];

const UI_FONTS = [
  { id: 'Inter', label: 'Inter', fontFamily: "'Inter', system-ui, sans-serif" },
  { id: 'Geist', label: 'Geist', fontFamily: "'Geist', system-ui, sans-serif" },
  { id: 'Nunito', label: 'Nunito', fontFamily: "'Nunito', system-ui, sans-serif" },
  { id: 'System Default', label: 'System Default', fontFamily: 'system-ui, -apple-system, sans-serif' },
] as const;

const MONO_FONTS = [
  { id: 'JetBrains Mono', label: 'JetBrains Mono', fontFamily: "'JetBrains Mono', monospace" },
  { id: 'Fira Code', label: 'Fira Code', fontFamily: "'Fira Code', monospace" },
  { id: 'Cascadia Code', label: 'Cascadia Code', fontFamily: "'Cascadia Code', monospace" },
  { id: 'Monaspace Neon', label: 'Monaspace Neon', fontFamily: "'Monaspace Neon', monospace" },
] as const;

const THEME_DOTS: Record<string, [string, string, string, string]> = {
  'vscode-dark':        ['#1e1e1e', '#252526', '#007acc', '#d4d4d4'],
  'vscode-light':       ['#ffffff', '#f3f3f3', '#0078d4', '#1f1f1f'],
  'monokai':            ['#272822', '#2d2e27', '#a6e22e', '#f8f8f2'],
  'tokyo-night':        ['#1a1b26', '#16161e', '#7aa2f7', '#a9b1d6'],
  'one-dark':           ['#282c34', '#21252b', '#61afef', '#abb2bf'],
  'classic':            ['#0f0f14', '#16161e', '#7c6aef', '#e8e8f0'],
  'catppuccin-mocha':   ['#1e1e2e', '#181825', '#cba6f7', '#cdd6f4'],
  'dracula':            ['#282a36', '#21222c', '#bd93f9', '#f8f8f2'],
};

interface ThemeSwatchProps {
  id: string;
  label: string;
  themeType: 'dark' | 'light';
  dots: [string, string, string, string];
  selected: boolean;
  onSelect: () => void;
  onRemove?: () => void;
}

function ThemeSwatch({ label, themeType, dots, selected, onSelect, onRemove }: ThemeSwatchProps) {
  return (
    <button
      type="button"
      role="button"
      aria-label={`${label} theme, ${themeType}`}
      aria-pressed={selected}
      onClick={onSelect}
      className={[
        'relative flex flex-col p-2 rounded-md border text-left transition-colors duration-150 w-full',
        selected
          ? 'ring-2 ring-primary ring-offset-1 border-primary'
          : 'border-border bg-card hover:border-primary/50 hover:bg-accent/30',
      ].join(' ')}
    >
      <div className="flex items-center justify-between">
        <div className="flex gap-1">
          {dots.map((color, i) => (
            <span
              key={i}
              className="w-3 h-3 rounded-full inline-block flex-shrink-0"
              style={{ backgroundColor: color }}
            />
          ))}
        </div>
        <span className="text-[10px] leading-none text-muted-foreground ml-1">
          {themeType === 'dark' ? '🌙' : '☀️'}
        </span>
      </div>
      <span className="text-[11px] mt-1 truncate leading-[1.3] pr-4">{label}</span>
      {onRemove && (
        <button
          type="button"
          aria-label={`Remove ${label} theme`}
          onClick={(e) => { e.stopPropagation(); onRemove(); }}
          className="absolute bottom-2 right-2 text-[14px] leading-none text-muted-foreground hover:text-destructive transition-colors"
        >
          <X size={12} />
        </button>
      )}
    </button>
  );
}

interface FontPickerCardProps {
  fontFamily: string;
  label: string;
  selected: boolean;
  onSelect: () => void;
}

function FontPickerCard({ fontFamily, label, selected, onSelect }: FontPickerCardProps) {
  return (
    <button
      type="button"
      aria-pressed={selected}
      onClick={onSelect}
      className={[
        'flex flex-col p-2 rounded-md border text-left transition-colors duration-150 w-full min-h-[56px]',
        selected
          ? 'ring-2 ring-primary ring-offset-1 ring-offset-background border-primary'
          : 'border-border bg-card hover:border-primary/50 hover:bg-accent/30',
      ].join(' ')}
    >
      <span style={{ fontFamily }} className="text-sm font-medium truncate leading-[1.4]">{label}</span>
      <span style={{ fontFamily }} className="text-[11px] text-muted-foreground mt-0.5 leading-[1.3]">
        ABC abc 012
      </span>
    </button>
  );
}

export interface SettingsPageProps {
  theme: string;
  onThemeChange: (theme: string) => void;
  onResetAllData?: () => Promise<void>;
  customThemes?: CustomTheme[];
  onImportTheme?: (theme: CustomTheme) => void;
  onRemoveCustomTheme?: (themeId: string) => void;
  uiFont?: string;
  monoFont?: string;
  density?: string;
}

const SettingsPage: React.FC<SettingsPageProps> = ({
  theme,
  onThemeChange,
  onResetAllData,
  customThemes,
  onImportTheme,
  onRemoveCustomTheme,
  uiFont = 'Inter',
  monoFont = 'JetBrains Mono',
  density = 'comfortable',
}) => {
  const { t, i18n } = useTranslation();
  const [locale, setLocale] = useState('en');
  const [confirmReset, setConfirmReset] = useState(false);
  const fileInputRef = useRef<HTMLInputElement>(null);
  const customThemesStrRef = useRef('[]');

  const [draftTheme, setDraftTheme] = useState(theme);
  const [draftUiFont, setDraftUiFont] = useState(uiFont);
  const [draftMonoFont, setDraftMonoFont] = useState(monoFont);
  const [draftDensity, setDraftDensity] = useState(density);

  const [savedTheme, setSavedTheme] = useState(theme);
  const [savedUiFont, setSavedUiFont] = useState(uiFont);
  const [savedMonoFont, setSavedMonoFont] = useState(monoFont);
  const [savedDensity, setSavedDensity] = useState(density);
  const [draftWorkingDir, setDraftWorkingDir] = useState('');
  const [currentOS, setCurrentOS] = useState<OSKey>('unknown');
  const [shellIntegration, setShellIntegrationState] = useState(true);

  // Refs track the latest values so the async GetSettings → merge → SetSettings
  // chain always reads the most recent state, avoiding stale-closure races when
  // the user changes multiple settings in rapid succession.
  const localeRef = useRef(locale);
  const draftThemeRef = useRef(draftTheme);
  const draftUiFontRef = useRef(draftUiFont);
  const draftMonoFontRef = useRef(draftMonoFont);
  const draftDensityRef = useRef(draftDensity);
  const shellIntegrationRef = useRef(shellIntegration);

  // Sync refs after every render so async callbacks always read the latest.
  useEffect(() => {
    localeRef.current = locale;
    draftThemeRef.current = draftTheme;
    draftUiFontRef.current = draftUiFont;
    draftMonoFontRef.current = draftMonoFont;
    draftDensityRef.current = draftDensity;
    shellIntegrationRef.current = shellIntegration;
  });

  // Tracks whether the user has edited any draft field. While true, the
  // async GetSettings resolver below must NOT overwrite draft/saved values.
  const userTouchedRef = useRef(false);
  const markTouched = useCallback(() => { userTouchedRef.current = true; }, []);

  // Persist the current in-memory settings to the store, merging with
  // any per-field override passed by the caller.  All values are read
  // from refs so rapid back-to-back changes always reflect the latest.
  const persistSettings = useCallback((override?: Record<string, unknown>) => {
    markTouched();
    GetSettings().then(current => {
      const merged = {
        locale: localeRef.current,
        theme: draftThemeRef.current,
        lastDarkTheme: current?.lastDarkTheme || 'vscode-dark',
        lastLightTheme: current?.lastLightTheme || 'vscode-light',
        customThemes: current?.customThemes || customThemesStrRef.current,
        uiFont: draftUiFontRef.current,
        monoFont: draftMonoFontRef.current,
        density: draftDensityRef.current,
        defaultWorkingDir: current?.defaultWorkingDir || {},
        shellIntegration: shellIntegrationRef.current,
        ...override,
      };
      SetSettings(JSON.stringify(merged)).catch(() => {});
      Events.Emit(eventNames.settingsChanged, merged);
    }).catch(() => {});
  }, [markTouched]);

  // Draft change helpers. In standalone mode SettingsPage drives its own
  // DOM preview via useEffects above, so it must NOT invoke the parent
  // callbacks during editing (those callbacks trigger the modal parent's
  // auto-save and the prop round-trip that broke dirty state). Parent
  // callbacks run only on Save (in handleSave below).
  const changeTheme = useCallback((v: string) => {
    setDraftTheme(v);
    persistSettings({ theme: v });
  }, [persistSettings]);

  const changeDensity = useCallback((v: string) => {
    setDraftDensity(v);
    persistSettings({ density: v });
  }, [persistSettings]);

  const changeUiFont = useCallback((v: string) => {
    setDraftUiFont(v);
    persistSettings({ uiFont: v });
  }, [persistSettings]);

  const changeMonoFont = useCallback((v: string) => {
    setDraftMonoFont(v);
    persistSettings({ monoFont: v });
  }, [persistSettings]);

  const changeLocale = useCallback((v: string) => {
    setLocale(v);
    i18n.changeLanguage(v).catch(() => {});
    persistSettings({ locale: v });
  }, [persistSettings, i18n]);

  const changeShellIntegration = useCallback((v: boolean) => {
    markTouched();
    setShellIntegrationState(v);
    persistSettings({ shellIntegration: v });
  }, [markTouched, persistSettings]);

  const changeWorkingDir = useCallback((v: string) => {
    markTouched();
    setDraftWorkingDir(v);
  }, [markTouched]);

  const persistWorkingDir = useCallback((path: string) => {
    if (currentOS === 'unknown') return;
    GetSettings().then(current => {
      const wd = setOSPath(current?.defaultWorkingDir, currentOS, path);
      persistSettings({ defaultWorkingDir: wd });
    }).catch(() => {});
  }, [currentOS, persistSettings]);

  const handleWorkingDirBlur = useCallback(() => {
    persistWorkingDir(draftWorkingDir);
  }, [persistWorkingDir, draftWorkingDir]);

  useEffect(() => {
    Promise.all([GetSettings(), GetOS()])
      .then(([s, os]) => {
        if (!s) return;
        if (userTouchedRef.current) return;
        setCurrentOS(normalizeOS(os));
        const wd = getOSPath(s.defaultWorkingDir, normalizeOS(os));
        setDraftWorkingDir(wd);
        const loc = s?.locale || i18n.language || 'en';
        setLocale(loc);
        setSavedTheme(s.theme || 'vscode-dark');
        setDraftTheme(s.theme || 'vscode-dark');
        setSavedUiFont(s.uiFont || 'Inter');
        setDraftUiFont(s.uiFont || 'Inter');
        setSavedMonoFont(s.monoFont || 'JetBrains Mono');
        setDraftMonoFont(s.monoFont || 'JetBrains Mono');
        setSavedDensity(s.density || 'comfortable');
        setDraftDensity(s.density || 'comfortable');
        setShellIntegrationState(s.shellIntegration ?? true);
      })
      .catch(() => {});
  }, []); // eslint-disable-line react-hooks/exhaustive-deps

  // In standalone mode, SettingsPage owns the live DOM preview so the
  // parent (SettingsWindow) never needs to round-trip through its own
  // `theme`/`uiFont`/... state. That round-trip was the source of the
  // prop-sync loop that zeroed out the dirty state.
  useEffect(() => {
    const custom = customThemes?.find(c => c.id === draftTheme);
    applyTheme(draftTheme, custom?.colors ?? null);
  }, [draftTheme, customThemes]);

  useEffect(() => {
    applyDensity(draftDensity);
  }, [draftDensity]);

  useEffect(() => {
    applyFonts(draftUiFont, draftMonoFont);
  }, [draftUiFont, draftMonoFont]);

  // In standalone mode, if the component unmounts (e.g. window closes)
  // with unsaved changes, revert DOM preview to the saved values so the
  // next open starts clean. A synced ref captures the latest saved state
  // so the cleanup only runs on actual unmount, not on every dep change.
  const savedSettingsRef = useSyncedRef({ savedTheme, savedDensity, savedUiFont, savedMonoFont, customThemes });

  useEffect(() => {
    return () => {
      // eslint-disable-next-line react-hooks/exhaustive-deps -- syncedRef.current is intentionally read in cleanup for latest value
      const { savedTheme: st, savedDensity: sd, savedUiFont: suf, savedMonoFont: smf, customThemes: ct } = savedSettingsRef.current;
      const custom = ct?.find(c => c.id === st);
      applyTheme(st, custom?.colors ?? null);
      applyDensity(sd);
      applyFonts(suf, smf);
    };
  }, []); // eslint-disable-line react-hooks/exhaustive-deps

  useEffect(() => {
    if (customThemes && customThemes.length > 0) {
      customThemesStrRef.current = JSON.stringify(customThemes);
    } else {
      customThemesStrRef.current = '[]';
    }
  }, [customThemes]);

  const handleImportFile = useCallback((e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (!file) return;
    e.target.value = '';
    const reader = new FileReader();
    reader.onload = (ev) => {
      try {
        const data = JSON.parse(ev.target?.result as string);
        if (
          typeof data.name !== 'string' ||
          (data.type !== 'dark' && data.type !== 'light') ||
          typeof data.colors?.background !== 'string'
        ) {
          toast.error(t('settings.themeInvalidFields'));
          return;
        }
        const allVarKeys = [
          'background', 'foreground', 'card', 'card-foreground', 'popover', 'popover-foreground',
          'primary', 'primary-foreground', 'secondary', 'secondary-foreground', 'muted', 'muted-foreground',
          'accent', 'accent-foreground', 'destructive', 'destructive-foreground', 'success', 'success-foreground',
          'border', 'input', 'ring', 'tab-bar-bg', 'tab-active-bg', 'tab-inactive-bg',
          'tab-active-indicator', 'status-bar-bg', 'status-bar-fg',
        ];
        const defaultTheme = data.type === 'dark'
          ? { background: '#1e1e1e', foreground: '#d4d4d4', card: '#252526', primary: '#007acc', accent: '#2a2d2e', border: 'rgba(255,255,255,0.1)', muted: '#3c3c3c', 'muted-foreground': '#858585', ring: '#007acc', destructive: '#f44747', success: '#4ec9b0', 'tab-bar-bg': '#2d2d2d', 'tab-active-bg': '#1e1e1e', 'tab-active-indicator': '#007acc', 'status-bar-bg': '#007acc' }
          : { background: '#ffffff', foreground: '#1f1f1f', card: '#f3f3f3', primary: '#0078d4', accent: '#e5e5e5', border: 'rgba(0,0,0,0.1)', muted: '#f0f0f0', 'muted-foreground': '#616161', ring: '#0078d4', destructive: '#d13438', success: '#0f7b0f', 'tab-bar-bg': '#f3f3f3', 'tab-active-bg': '#ffffff', 'tab-active-indicator': '#0078d4', 'status-bar-bg': '#0078d4' };
        const filledColors: Record<string, string> = {};
        for (const key of allVarKeys) {
          if (typeof data.colors?.[key] === 'string') {
            filledColors[key] = data.colors[key];
          } else if (defaultTheme[key as keyof typeof defaultTheme]) {
            filledColors[key] = defaultTheme[key as keyof typeof defaultTheme];
          }
        }
        const newTheme: CustomTheme = {
          id: `custom-${Date.now()}-${Math.random().toString(36).slice(2, 7)}`,
          name: data.name,
          type: data.type,
          colors: filledColors,
        };
        onImportTheme?.(newTheme);
        onThemeChange(newTheme.id);
        setDraftTheme(newTheme.id);
        setSavedTheme(newTheme.id);
        toast.success(t('settings.themeApplied', { name: data.name }));
      } catch {
        toast.error(t('settings.themeInvalidJson'));
      }
    };
    reader.readAsText(file);
  }, [onImportTheme, onThemeChange, t]);

  const handleDownloadTemplate = useCallback(async () => {
    try {
      await SaveThemeTemplate();
    } catch (err) {
      toast.error(String(err));
    }
  }, []);

  return (
    <div className="p-4 space-y-4">
      <Tabs defaultValue="appearance" className="w-full">
        <TabsList className="w-full justify-start rounded-none bg-transparent p-0 border-b border-border h-auto mb-0">
          <TabsTrigger
            value="appearance"
            className="rounded-none border-b-2 border-transparent px-3 py-2 text-sm data-[state=active]:border-primary data-[state=active]:text-foreground data-[state=active]:font-medium data-[state=inactive]:text-muted-foreground data-[state=inactive]:hover:text-foreground bg-transparent shadow-none"
          >
            {t('settings.tabs.appearance')}
          </TabsTrigger>
          <TabsTrigger
            value="typography"
            className="rounded-none border-b-2 border-transparent px-3 py-2 text-sm data-[state=active]:border-primary data-[state=active]:text-foreground data-[state=active]:font-medium data-[state=inactive]:text-muted-foreground data-[state=inactive]:hover:text-foreground bg-transparent shadow-none"
          >
            {t('settings.tabs.typography')}
          </TabsTrigger>
          <TabsTrigger
            value="general"
            className="rounded-none border-b-2 border-transparent px-3 py-2 text-sm data-[state=active]:border-primary data-[state=active]:text-foreground data-[state=active]:font-medium data-[state=inactive]:text-muted-foreground data-[state=inactive]:hover:text-foreground bg-transparent shadow-none"
          >
            {t('settings.tabs.general')}
          </TabsTrigger>
        </TabsList>

        <TabsContent value="appearance" className="space-y-4 pt-4">
          <div className="space-y-2">
            <p className="text-[11px] text-muted-foreground">{t('settings.theme')}</p>
            <div
              role="group"
              aria-label="Theme selection"
              className="grid grid-cols-4 gap-2 p-3 max-h-[200px] overflow-y-auto scrollbar-hide"
            >
              {THEMES.map(th => (
                <ThemeSwatch
                  key={th.id}
                  id={th.id}
                  label={th.label}
                  themeType={th.type}
                  dots={THEME_DOTS[th.id] ?? ['#888', '#666', '#aaa', '#ccc']}
                  selected={draftTheme === th.id}
                  onSelect={() => changeTheme(th.id)}
                />
              ))}
            </div>
          </div>

          {(customThemes?.length ?? 0) > 0 && (
            <div className="pt-3 border-t border-border space-y-2">
              <p className="text-[11px] text-muted-foreground">{t('settings.customThemes')}</p>
              <div role="group" aria-label="Custom themes" className="grid grid-cols-2 gap-2">
                {customThemes!.map(ct => (
                  <ThemeSwatch
                    key={ct.id}
                    id={ct.id}
                    label={ct.name}
                    themeType={ct.type}
                    dots={[
                      ct.colors.background ?? '#888',
                      ct.colors.card ?? '#666',
                      ct.colors.primary ?? '#aaa',
                      ct.colors.foreground ?? '#ccc',
                    ]}
                    selected={draftTheme === ct.id}
                    onSelect={() => changeTheme(ct.id)}
                    onRemove={() => onRemoveCustomTheme?.(ct.id)}
                  />
                ))}
              </div>
            </div>
          )}

          <div className="flex gap-2">
            <Button
              type="button"
              variant="outline"
              size="sm"
              className="flex-1"
              onClick={() => fileInputRef.current?.click()}
            >
              <Upload size={14} className="mr-1" />
              {t('settings.importTheme')}
            </Button>
            <Button
              type="button"
              variant="ghost"
              size="sm"
              className="flex-1"
              onClick={handleDownloadTemplate}
            >
              <Download size={14} className="mr-1.5" />
              {t('settings.downloadTemplate')}
            </Button>
          </div>

          <input
            ref={fileInputRef}
            type="file"
            accept=".json"
            aria-hidden="true"
            className="hidden"
            onChange={handleImportFile}
          />

          <div className="pt-3 border-t border-border space-y-2">
            <p className="text-sm font-medium">{t('settings.densityLabel')}</p>
            <ToggleGroup
              type="single"
              variant="outline"
              size="sm"
              value={draftDensity}
              onValueChange={(v) => { if (v) changeDensity(v); }}
              className="w-full"
            >
              <ToggleGroupItem value="compact" className="flex-1">
                {t('settings.densityCompact')}
              </ToggleGroupItem>
              <ToggleGroupItem value="comfortable" className="flex-1">
                {t('settings.densityComfortable')}
              </ToggleGroupItem>
              <ToggleGroupItem value="spacious" className="flex-1">
                {t('settings.densitySpacious')}
              </ToggleGroupItem>
            </ToggleGroup>
          </div>
        </TabsContent>

        <TabsContent value="typography" className="space-y-5 pt-4">
          <div className="space-y-2">
            <Label className="text-xs font-semibold">{t('settings.uiFontLabel')}</Label>
            <div role="group" aria-label="UI font selection" className="grid grid-cols-3 gap-2">
              {UI_FONTS.map(font => (
                <FontPickerCard
                  key={font.id}
                  fontFamily={font.fontFamily}
                  label={font.label}
                  selected={draftUiFont === font.id}
                  onSelect={() => changeUiFont(font.id)}
                />
              ))}
            </div>
          </div>

          <div className="space-y-2">
            <Label className="text-xs font-semibold">{t('settings.monoFontLabel')}</Label>
            <div role="group" aria-label="Editor font selection" className="grid grid-cols-3 gap-2">
              {MONO_FONTS.map(font => (
                <FontPickerCard
                  key={font.id}
                  fontFamily={font.fontFamily}
                  label={font.label}
                  selected={draftMonoFont === font.id}
                  onSelect={() => changeMonoFont(font.id)}
                />
              ))}
            </div>
          </div>
        </TabsContent>

        <TabsContent value="general" className="space-y-4 pt-4">
          <div className="space-y-2">
            <Label>{t('settings.language')}</Label>
            <Select value={locale} onValueChange={changeLocale}>
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {LANGUAGES.map(lang => (
                  <SelectItem key={lang.code} value={lang.code}>{lang.label}</SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          <div className="space-y-2">
            <Label>{t('settings.workingDirectory')}</Label>
            <div className="flex gap-2">
              <input
                type="text"
                value={draftWorkingDir}
                onChange={(e) => changeWorkingDir(e.target.value)}
                onBlur={handleWorkingDirBlur}
                placeholder={t('settings.workingDirectoryPlaceholder')}
                className="flex-1 h-9 rounded-md border border-input bg-transparent px-3 py-1 text-sm shadow-sm transition-colors file:border-0 file:bg-transparent file:text-sm file:font-medium placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-50"
              />
              <Button
                type="button"
                variant="outline"
                size="sm"
                disabled={currentOS === 'unknown'}
                onClick={async () => {
                  try {
                    const selected = await PickDirectory(draftWorkingDir);
                    if (selected) {
                      changeWorkingDir(selected);
                      persistWorkingDir(selected);
                    }
                  } catch (err) {
                    console.error('Directory picker error:', err);
                    toast.error(t('settings.pickDirectoryFailed', { message: String(err) }));
                  }
                }}
              >
                <FolderOpen size={14} className="mr-1" />
                {t('settings.browse')}
              </Button>
              {draftWorkingDir && (
                <Button
                  type="button"
                  variant="ghost"
                  size="sm"
                  onClick={() => {
                  changeWorkingDir('');
                  persistWorkingDir('');
                }}
                >
                  <X size={14} />
                </Button>
              )}
            </div>
            <p className="text-[11px] text-muted-foreground">
              {t('settings.workingDirectoryHint')}
            </p>
          </div>

          <div className="flex items-center justify-between gap-4">
            <div className="space-y-0.5">
              <Label htmlFor="shell-integration-toggle">{t('settings.shellIntegration')}</Label>
              <p className="text-[11px] text-muted-foreground">
                {t('settings.shellIntegrationHint')}
              </p>
            </div>
            <Switch
              id="shell-integration-toggle"
              checked={shellIntegration}
              onCheckedChange={changeShellIntegration}
            />
          </div>

          {onResetAllData && (
            <div className="border-t border-border pt-4 mt-2">
              <Label className="text-destructive text-xs font-semibold uppercase tracking-wide">{t('settings.dangerZone')}</Label>
              {confirmReset ? (
                <div className="mt-2 p-3 rounded-md border border-destructive/40 bg-destructive/5 space-y-2">
                  <p className="text-sm text-destructive">{t('settings.resetConfirm')}</p>
                  <div className="flex gap-2">
                    <Button
                      variant="destructive"
                      size="sm"
                      className="flex-1"
                      data-testid="danger-zone-confirm"
                      onClick={async () => {
                        await onResetAllData();
                        GetSettings().then(s => {
                          if (!s) return;
                          const t = s.theme || 'vscode-dark';
                          setSavedTheme(t);
                          setDraftTheme(t);
                          setSavedUiFont(s.uiFont || 'Inter');
                          setDraftUiFont(s.uiFont || 'Inter');
                          setSavedMonoFont(s.monoFont || 'JetBrains Mono');
                          setDraftMonoFont(s.monoFont || 'JetBrains Mono');
                          setSavedDensity(s.density || 'comfortable');
                          setDraftDensity(s.density || 'comfortable');
                          const wd = getOSPath(s.defaultWorkingDir, currentOS);
                          setDraftWorkingDir(wd);
                          setLocale(s.locale || 'en');
                          setShellIntegrationState(s.shellIntegration ?? true);
                        }).catch(() => {});
                        userTouchedRef.current = false;
                        setConfirmReset(false);
                        if (fileInputRef.current) fileInputRef.current.value = '';
                      }}
                    >
                      {t('settings.resetConfirmYes')}
                    </Button>
                    <Button
                      variant="outline"
                      size="sm"
                      className="flex-1"
                      data-testid="danger-zone-cancel"
                      onClick={() => setConfirmReset(false)}
                    >
                      {t('settings.resetConfirmNo')}
                    </Button>
                  </div>
                </div>
              ) : (
                <Button
                  variant="outline"
                  size="sm"
                  className="mt-2 text-destructive border-destructive/40 hover:bg-destructive/10 w-full"
                  data-testid="danger-zone-reset-button"
                  onClick={() => setConfirmReset(true)}
                >
                  {t('settings.resetAllData')}
                </Button>
              )}
            </div>
          )}
        </TabsContent>
      </Tabs>
    </div>
  );
};

export default SettingsPage;
