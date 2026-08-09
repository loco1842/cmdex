/**
 * Copy text to the system clipboard.
 * Prefer the Clipboard API; fall back to execCommand for Wails/webview
 * contexts that deny navigator.clipboard (NotAllowedError).
 */
export async function copyText(text: string): Promise<void> {
  if (typeof navigator !== 'undefined' && navigator.clipboard?.writeText) {
    try {
      await navigator.clipboard.writeText(text);
      return;
    } catch {
      // Fall through — common in desktop webviews without clipboard permission.
    }
  }

  const ta = document.createElement('textarea');
  ta.value = text;
  ta.setAttribute('readonly', '');
  ta.style.position = 'fixed';
  ta.style.top = '0';
  ta.style.left = '-9999px';
  document.body.appendChild(ta);
  try {
    ta.focus();
    ta.select();
    ta.setSelectionRange(0, text.length);
    if (!document.execCommand('copy')) {
      throw new Error('Copy to clipboard failed');
    }
  } finally {
    document.body.removeChild(ta);
  }
}
