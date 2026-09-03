import React, { useCallback, useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { toast } from 'sonner';
import { AlertTriangle, CheckCircle2, Keyboard } from 'lucide-react';

import { Label } from '@/components/ui/label';
import { Button } from '@/components/ui/button';
import { Switch } from '@/components/ui/switch';
import { SetSettings } from '../../bindings/cmdex/settingsservice';
import { createLatestAsyncQueue, type AsyncQueueResult } from '../utils/asyncQueue';
import { clearPendingShortcutIfCurrent } from '../utils/launcher';
import {
  ApplySettings,
  GetStatus,
  SetLaunchAtLogin,
  ValidateShortcut,
} from '../../bindings/cmdex/launcherservice';

interface LauncherStatus {
  supported: boolean;
  enabled: boolean;
  registered: boolean;
  shortcut: string;
  error: string;
  warning: string;
  launchAtLogin: boolean;
  platform: string;
}

type QueuedSettingsResult<T> = AsyncQueueResult<T>;

/** Turn a keydown into the accelerator format the Go side parses. */
function acceleratorFromEvent(e: React.KeyboardEvent<HTMLInputElement>): string | null {
  const key = e.key;
  // Ignore presses that are only modifiers — wait for a real key.
  if (['Control', 'Shift', 'Alt', 'Meta', 'CapsLock', 'Dead'].includes(key)) return null;

  const parts: string[] = [];
  if (e.ctrlKey) parts.push('Ctrl');
  if (e.altKey) parts.push('Alt');
  if (e.shiftKey) parts.push('Shift');
  if (e.metaKey) parts.push('Cmd');
  if (parts.length === 0) return null;

  let name = key.length === 1 ? key.toUpperCase() : key;
  if (name === ' ') name = 'Space';
  if (name === 'Escape') return null; // reserved for cancelling capture

  return [...parts, name].join('+');
}

/**
 * Settings for the system-wide quick launcher: on/off, its shortcut, and
 * whether CmDex starts at login. Registration state is read back from the
 * backend so a shortcut that the OS refused is visibly reported rather than
 * silently ignored.
 */
const LauncherSettings: React.FC = () => {
  const { t } = useTranslation();
  const [status, setStatus] = useState<LauncherStatus | null>(null);
  const [capturing, setCapturing] = useState(false);
  const [pendingShortcut, setPendingShortcut] = useState('');
  const captureRef = useRef<HTMLInputElement>(null);
  const [settingsQueue] = useState(createLatestAsyncQueue);
  const latestSettingsOperationRef = useRef(0);

  // Wails calls are asynchronous and settings updates are partial merges. A
  // small FIFO makes rapid toggles/shortcut captures last-write-wins, while
  // the generation check prevents an older ApplySettings response from
  // repainting the current status.
  const enqueueSettings = useCallback(<T,>(
    operation: () => Promise<T>,
    onCurrentFailure?: (operation: number) => Promise<void>,
  ): Promise<QueuedSettingsResult<T>> => {
    const operationNumber = ++latestSettingsOperationRef.current;
    return settingsQueue.enqueue(operation).catch(async (error) => {
      // A current failure is not followed by a successful result that can
      // repaint status. Refresh the backend state so an older success that
      // was correctly suppressed cannot leave the controls stale. The second
      // generation check belongs after the fetch as a newer operation may
      // have started while GetStatus was in flight.
      if (operationNumber === latestSettingsOperationRef.current && onCurrentFailure) {
        await onCurrentFailure(operationNumber);
      }
      throw error;
    });
  }, [settingsQueue]);

  const refreshStatusAfterFailure = useCallback(async (operationNumber: number) => {
    if (operationNumber !== latestSettingsOperationRef.current) return;
    try {
      const next = await GetStatus() as LauncherStatus;
      if (operationNumber === latestSettingsOperationRef.current) setStatus(next);
    } catch (err) {
      console.error('launcher settings: recovery GetStatus failed', err);
    }
  }, []);

  const refresh = useCallback(async () => {
    // Invalidate the component-level operation token alongside the queue
    // generation. Otherwise a failure from an already queued operation can
    // still start a recovery GetStatus after this refresh began.
    latestSettingsOperationRef.current += 1;
    const generation = settingsQueue.invalidate();
    try {
      const next = await GetStatus() as LauncherStatus;
      if (settingsQueue.isCurrent(generation)) setStatus(next);
    } catch (err) {
      console.error('launcher settings: GetStatus failed', err);
    }
  }, [settingsQueue]);

  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect -- async fetch; state is set from the promise callback
    refresh();
  }, [refresh]);

  useEffect(() => {
    if (capturing) captureRef.current?.focus();
  }, [capturing]);

  const persist = useCallback(async (
    patch: Record<string, unknown>,
    validate?: () => Promise<void>,
  ) => enqueueSettings(async () => {
    if (validate) await validate();
    await SetSettings(JSON.stringify(patch));
    return await ApplySettings() as LauncherStatus;
  }, refreshStatusAfterFailure).then(result => {
    if (result.current) setStatus(result.value);
    return result;
  }), [enqueueSettings, refreshStatusAfterFailure]);

  const toggleEnabled = useCallback(async (enabled: boolean) => {
    try {
      const result = await persist({ launcherEnabled: enabled });
      if (result.current && enabled && !result.value.registered && result.value.error) {
        toast.error(result.value.error);
      }
    } catch (err) {
      toast.error(t('settings.launcherSaveFailed', { message: String(err) }));
    }
  }, [persist, t]);

  // On any failure the pending capture is discarded so displayShortcut falls
  // back to status.shortcut — otherwise the field would keep advertising an
  // accelerator that was never actually applied.
  const commitShortcut = useCallback(async (accelerator: string) => {
    try {
      const result = await persist(
        { launcherShortcut: accelerator },
        () => ValidateShortcut(accelerator),
      );
      setPendingShortcut((current) => clearPendingShortcutIfCurrent(current, accelerator));
      if (result.current) {
        if (result.value.enabled && !result.value.registered && result.value.error) {
          toast.error(result.value.error);
        }
      }
    } catch (err) {
      const message = (err as { message?: unknown })?.message ?? err;
      toast.error(t('settings.launcherSaveFailed', { message: String(message) }));
      setPendingShortcut((current) => clearPendingShortcutIfCurrent(current, accelerator));
    }
  }, [persist, t]);

  const toggleLaunchAtLogin = useCallback(async (enabled: boolean) => {
    try {
      const result = await enqueueSettings(async () => {
        await SetLaunchAtLogin(enabled);
        return await GetStatus() as LauncherStatus;
      }, refreshStatusAfterFailure);
      if (result.current) setStatus(result.value);
    } catch (err) {
      toast.error(t('settings.launchAtLoginFailed', { message: String(err) }));
    }
  }, [enqueueSettings, refreshStatusAfterFailure, t]);

  const handleCaptureKeyDown = useCallback((e: React.KeyboardEvent<HTMLInputElement>) => {
    e.preventDefault();
    if (e.key === 'Escape') {
      setCapturing(false);
      setPendingShortcut('');
      return;
    }
    const accelerator = acceleratorFromEvent(e);
    if (!accelerator) return;
    setPendingShortcut(accelerator);
    setCapturing(false);
    commitShortcut(accelerator);
  }, [commitShortcut]);

  if (!status) return null;

  const displayShortcut = pendingShortcut || status.shortcut;

  return (
    <div className="border-t border-border pt-4 mt-2 space-y-3">
      <div className="flex items-center justify-between">
        <div className="space-y-0.5 pr-4">
          <Label>{t('settings.launcherEnabled')}</Label>
          <p className="text-[11px] text-muted-foreground">
            {t('settings.launcherEnabledHint')}
          </p>
        </div>
        <Switch
          checked={status.enabled}
          disabled={!status.supported}
          onCheckedChange={toggleEnabled}
        />
      </div>

      <div className="space-y-2">
        <Label>{t('settings.launcherShortcut')}</Label>
        <div className="flex gap-2 items-center">
          <input
            ref={captureRef}
            readOnly
            value={capturing ? t('settings.launcherShortcutCapturing') : displayShortcut}
            onKeyDown={handleCaptureKeyDown}
            onBlur={() => setCapturing(false)}
            onClick={() => setCapturing(true)}
            disabled={!status.supported || !status.enabled}
            className="flex-1 h-9 rounded-md border border-input bg-transparent px-3 py-1 text-sm font-mono shadow-sm transition-colors placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-50"
          />
          <Button
            type="button"
            variant="outline"
            size="sm"
            disabled={!status.supported || !status.enabled}
            onClick={() => setCapturing(true)}
          >
            <Keyboard size={14} className="mr-1" />
            {t('settings.launcherShortcutChange')}
          </Button>
        </div>

        {status.enabled && status.supported && (
          status.registered ? (
            <p className="text-[11px] text-muted-foreground flex items-center gap-1">
              <CheckCircle2 size={12} className="text-[var(--success)]" />
              {t('settings.launcherRegistered')}
            </p>
          ) : (
            <p className="text-[11px] text-destructive flex items-start gap-1">
              <AlertTriangle size={12} className="mt-0.5 shrink-0" />
              {status.error || t('settings.launcherNotRegistered')}
            </p>
          )
        )}

        {!status.supported && (
          <p className="text-[11px] text-destructive flex items-start gap-1">
            <AlertTriangle size={12} className="mt-0.5 shrink-0" />
            {t('settings.launcherUnsupported')}
          </p>
        )}

        {status.warning && (
          <p className="text-[11px] text-muted-foreground flex items-start gap-1">
            <AlertTriangle size={12} className="mt-0.5 shrink-0" />
            {status.warning}
          </p>
        )}
      </div>

      <div className="flex items-center justify-between">
        <div className="space-y-0.5 pr-4">
          <Label>{t('settings.launchAtLogin')}</Label>
          <p className="text-[11px] text-muted-foreground">
            {t('settings.launchAtLoginHint')}
          </p>
        </div>
        <Switch checked={status.launchAtLogin} onCheckedChange={toggleLaunchAtLogin} />
      </div>
    </div>
  );
};

export default LauncherSettings;
