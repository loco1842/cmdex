import { toast } from 'sonner';

export const eventNames = {
    openSettings: 'open-settings',
    openShortcuts: 'open-shortcuts',
    settingsChanged: 'settings-changed',
    settingsWindowClosing: 'settings-window-closing',
    dataReset: 'data-reset',
    launcherShown: 'launcher-shown',
    launcherHidden: 'launcher-hidden',
};

export type EventNameKey = keyof typeof eventNames;

/** Merge backend names without allowing a partial/malformed payload to erase fallbacks. */
export function mergeEventNames(names: Partial<Record<EventNameKey, unknown>> | null | undefined): void {
    if (!names || typeof names !== 'object') return;
    (Object.keys(eventNames) as EventNameKey[]).forEach((key) => {
        const name = names[key];
        if (typeof name === 'string' && name.length > 0) {
            eventNames[key] = name;
        }
    });
}

export async function initEventNames(): Promise<void> {
    try {
        const { GetEventNames } = await import('../../bindings/cmdex/eventservice');
        const names = await GetEventNames();
        mergeEventNames(names);
    } catch (err) {
        console.error('Failed to init event names:', err);
        toast.error('Failed to initialize events. Using fallback event names.');
    }
}
