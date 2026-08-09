import { useState, useCallback, useRef, useEffect } from 'react';
import { copyText } from '../utils/clipboard';

export function useCopyToClipboard(resetMs = 1500) {
  const [copied, setCopied] = useState(false);
  const timerRef = useRef<ReturnType<typeof setTimeout>>(undefined);

  const copy = useCallback(async (text: string) => {
    try {
      await copyText(text);
      setCopied(true);
      clearTimeout(timerRef.current);
      timerRef.current = setTimeout(() => setCopied(false), resetMs);
    } catch (err) {
      console.error('Copy failed:', err);
      throw err;
    }
  }, [resetMs]);

  useEffect(() => () => clearTimeout(timerRef.current), []);

  return { copied, copy };
}
