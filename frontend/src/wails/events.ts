import { toast } from 'sonner';

export const eventNames = {
    openSettings: 'open-settings',
    openShortcuts: 'open-shortcuts',
    settingsChanged: 'settings-changed',
    settingsWindowClosing: 'settings-window-closing',
    dataReset: 'data-reset',
};

export async function initEventNames(): Promise<void> {
    try {
        const { GetEventNames } = await import('../../bindings/cmdex/eventservice');
        const names = await GetEventNames();
        eventNames.openSettings = names.openSettings;
        eventNames.openShortcuts = names.openShortcuts;
        eventNames.settingsChanged = names.settingsChanged;
        eventNames.settingsWindowClosing = names.settingsWindowClosing;
        eventNames.dataReset = names.dataReset;
    } catch (err) {
        console.error('Failed to init event names:', err);
        toast.error('Failed to initialize events. Using fallback event names.');
    }
}