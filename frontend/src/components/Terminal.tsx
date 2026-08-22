import React, { useEffect, useRef, forwardRef, useImperativeHandle } from 'react';
import { Terminal } from '@xterm/xterm';
import { FitAddon } from '@xterm/addon-fit';
import { WebglAddon } from '@xterm/addon-webgl';
import { WebLinksAddon } from '@xterm/addon-web-links';
import '@xterm/xterm/css/xterm.css';
import { Events } from '@wailsio/runtime';
import { Write, Resize, Start, Clear } from '../../bindings/cmdex/terminalservice';
import { toast } from 'sonner';

interface TerminalComponentProps {
  isVisible: boolean;
  theme: string;
  sessionId: string;
  // Whether the backend already reports this session's shell as running at
  // mount time (e.g. the session TerminalService.ServiceStartup or
  // CreateSession already spawned). Only consulted once, on mount — see the
  // startCalledRef effect below. Without this, every mount unconditionally
  // called Start() even for an already-running session, which tears down
  // the healthy PTY (startSessionLocked has no "already running" guard, only
  // a re-entrant "already starting" one) and spawns a brand new shell,
  // producing a spurious extra shell/prompt on every session mount.
  initiallyRunning?: boolean;
  onShellExit?: () => void;
}

export interface TerminalHandle {
    clear: () => void;
    getSelection: () => string;
    getLastOutput: () => string;
    focus: () => void;
}

const TerminalComponent = forwardRef<TerminalHandle, TerminalComponentProps>(
    ({ isVisible, theme, sessionId, initiallyRunning, onShellExit }, ref) => {
  const containerRef = useRef<HTMLDivElement>(null);
  const terminalRef = useRef<Terminal | null>(null);
  const fitAddonRef = useRef<FitAddon | null>(null);
  const isFirstMountRef = useRef(true);
  const backendAvailableRef = useRef(true);
  const startCalledRef = useRef(false);
  const sessionIdRef = useRef(sessionId);
  const onShellExitRef = useRef(onShellExit);
  const initiallyRunningRef = useRef(initiallyRunning);
  useEffect(() => {
    sessionIdRef.current = sessionId;
  }, [sessionId]);
  useEffect(() => {
    onShellExitRef.current = onShellExit;
  }, [onShellExit]);
  useEffect(() => {
    initiallyRunningRef.current = initiallyRunning;
  }, [initiallyRunning]);

    function hexToRgba(hex: string, alpha: number): string {
        hex = hex.replace('#', '');
        if (hex.length === 3) {
            hex = hex[0] + hex[0] + hex[1] + hex[1] + hex[2] + hex[2];
        }
        const r = parseInt(hex.substring(0, 2), 16);
        const g = parseInt(hex.substring(2, 4), 16);
        const b = parseInt(hex.substring(4, 6), 16);
        return `rgba(${r}, ${g}, ${b}, ${alpha})`;
    }

  useImperativeHandle(ref, () => ({
      clear: () => {
        if (!backendAvailableRef.current) {
          terminalRef.current?.clear();
          return;
        }
        Clear(sessionIdRef.current).catch((err) => {
          console.error('clear failed:', err);
          if (backendAvailableRef.current) {
            backendAvailableRef.current = false;
          }
        });
        terminalRef.current?.clear();
      },
      getSelection: () => terminalRef.current?.getSelection() || '',
      getLastOutput: () => {
          const buffer = terminalRef.current?.buffer.active;
          if (!buffer) return '';

          const stripAnsi = (str: string) => str
              // eslint-disable-next-line no-control-regex
              .replace(/\x1B\[[0-9;]*[mGKHFJA-Za-z]/g, '')
              // eslint-disable-next-line no-control-regex
              .replace(/\x1B\][^\x07]*\x07/g, '')
              .replace(/\r/g, '');

          const promptRegex = /[$#%❯>➤λ→⟩»◇](\s|$)/;

          const isLineContent = (i: number): boolean => {
              const l = buffer.getLine(i);
              if (!l) return false;
              if (l.isWrapped) {
                  let p = i - 1;
                  while (p >= 0 && buffer.getLine(p)?.isWrapped) p--;
                  if (p >= 0) {
                      const pl = buffer.getLine(p);
                      if (pl && promptRegex.test(pl.translateToString(true).trim())) return false;
                  }
              }
              return stripAnsi(l.translateToString(true)).trim().length > 0;
          };

          const cursorPos = buffer.cursorY + buffer.baseY;
          let promptIdx = -1;
          let prevPromptIdx = -1;
          let scanFrom = cursorPos;

          while (scanFrom >= 0) {
              let found = -1;
              for (let i = scanFrom; i >= 0; i--) {
                  const line = buffer.getLine(i);
                  if (!line) continue;
                  // A wrapped row is a continuation of the row above it, never
                  // the start of a new prompt — testing it against promptRegex
                  // misfires on echoed command text that happens to wrap and
                  // end in a prompt-like character (e.g. a long script line
                  // ending in "$" or ">" when the terminal is narrow).
                  if (line.isWrapped) continue;
                  if (promptRegex.test(line.translateToString(true).trim())) {
                      found = i;
                      break;
                  }
              }

              if (found === -1) break;

              if (promptIdx === -1) {
                  promptIdx = found;
                  scanFrom = found - 1;
                  continue;
              }

              let hasContent = false;
              for (let i = found + 1; i < promptIdx; i++) {
                  if (isLineContent(i)) { hasContent = true; break; }
              }

              if (hasContent) {
                  prevPromptIdx = found;
                  break;
              }

              promptIdx = found;
              scanFrom = found - 1;
          }

          if (promptIdx === -1) return '';

          const outputStart = prevPromptIdx !== -1 ? prevPromptIdx + 1 : promptIdx + 1;
          const outputEnd = prevPromptIdx !== -1 ? promptIdx - 1 : cursorPos;

          const outputLines: string[] = [];
          for (let i = outputStart; i <= outputEnd; i++) {
              const line = buffer.getLine(i);
              if (!line) continue;
              const stripped = stripAnsi(line.translateToString(true));
              if (stripped.length === 0) continue;

              if (line.isWrapped) {
                  let parent = i - 1;
                  while (parent >= 0 && buffer.getLine(parent)?.isWrapped) parent--;
                  if (parent >= 0) {
                      const parentLine = buffer.getLine(parent);
                      if (parentLine && promptRegex.test(parentLine.translateToString(true).trim())) continue;
                  }
                  if (outputLines.length > 0) {
                      outputLines[outputLines.length - 1] += stripped;
                  }
                  continue;
              }

              outputLines.push(stripped);
          }

          return outputLines.join('\n');
      },
      focus: () => {
        terminalRef.current?.focus();
      },
  }));

  useEffect(() => {
    const skipTransition = isFirstMountRef.current;
    if (isFirstMountRef.current) {
        isFirstMountRef.current = false;
    }

    const container = containerRef.current;
    if (!skipTransition && container) {
        container.style.opacity = '0';
        container.style.transition = 'opacity var(--transition-fast)';
    }

    const term = new Terminal({
      cursorBlink: true,
      cursorStyle: 'block',
      fontSize: 14,
      fontFamily: 'JetBrains Mono, Fira Code, monospace',
      fontWeight: '400',
      scrollback: 5000,
      convertEol: true,
      allowProposedApi: true,
      allowTransparency: false,
      theme: {
        background: '#1e1e1e',
        foreground: '#d4d4d4',
        cursor: '#d4d4d4',
        cursorAccent: '#1e1e1e',
        selectionBackground: '#264f78',
        black: '#000000',
        red: '#cd3131',
        green: '#0dbc79',
        yellow: '#e5e510',
        blue: '#2472c8',
        magenta: '#bc3fbc',
        cyan: '#11a8cd',
        white: '#e5e5e5',
        brightBlack: '#666666',
        brightRed: '#f44747',
        brightGreen: '#4ec9b0',
        brightYellow: '#d7ba7d',
        brightBlue: '#569cd6',
        brightMagenta: '#c586c0',
        brightCyan: '#4ec9b0',
        brightWhite: '#ffffff',
      },
    });

    const fitAddon = new FitAddon();
    term.loadAddon(fitAddon);
    fitAddonRef.current = fitAddon;

    const webLinksAddon = new WebLinksAddon((event, uri) => {
      window.open(uri, '_blank');
    });
    term.loadAddon(webLinksAddon);

    try {
      const webglAddon = new WebglAddon();
      webglAddon.onContextLoss(() => webglAddon.dispose());
      term.loadAddon(webglAddon);
    } catch (webglErr) {
      // WebGL unavailable (e.g. headless or no GPU) — fall back to canvas renderer
      console.debug('WebGL addon not available:', webglErr);
    }

    if (containerRef.current) {
      term.open(containerRef.current);
      requestAnimationFrame(() => {
        fitAddon.fit();
        if (!skipTransition && containerRef.current) {
            containerRef.current.style.opacity = '1';
        }
      });
    }

    terminalRef.current = term;

    const inputDisposable = term.onData((data) => {
      if (!backendAvailableRef.current) return;
      Write(sessionIdRef.current, data).catch((err) => {
        console.error('TerminalService.Write failed:', err);
        if (backendAvailableRef.current) {
          backendAvailableRef.current = false;
        }
      });
    });

    const resizeDisposable = term.onResize(({ cols, rows }) => {
      if (!backendAvailableRef.current) return;
      Resize(sessionIdRef.current, cols, rows).catch((err) => {
        console.error('resize failed:', err);
        if (backendAvailableRef.current) {
          backendAvailableRef.current = false;
        }
      });
    });

    const observer = new ResizeObserver(() => {
      fitAddonRef.current?.fit();
    });
    if (containerRef.current) {
      observer.observe(containerRef.current);
    }

    return () => {
      inputDisposable.dispose();
      resizeDisposable.dispose();
      observer.disconnect();
      if (terminalRef.current === term) {
        term.dispose();
        terminalRef.current = null;
        fitAddonRef.current = null;
      }
      startCalledRef.current = false;
    };
  }, []);

  useEffect(() => {
    const term = terminalRef.current;
    if (!term || !sessionId) return;

    if (!startCalledRef.current) {
      startCalledRef.current = true;
      // The backend may have already started this session's shell (e.g. the
      // one TerminalService.ServiceStartup/CreateSession spawns) before this
      // component ever mounts. startSessionLocked has no "already running"
      // guard — only a re-entrant "already starting" one — so calling Start
      // again here would tear down that healthy PTY and spawn a redundant
      // second shell, showing up as an extra prompt the moment the terminal
      // tab opens.
      if (!initiallyRunningRef.current) {
        requestAnimationFrame(() => {
          const current = terminalRef.current;
          if (!current) return;
          Start(sessionId, current.cols, current.rows).catch((err) => {
            console.error('terminal start failed:', err);
            if (backendAvailableRef.current) {
              backendAvailableRef.current = false;
              toast.error('Terminal start failed');
            }
          });
        });
      }
    }

    const ptyOutputEvent = 'pty-output:' + sessionId;
    const ptyExitEvent = 'pty-exit:' + sessionId;
    const ptyClearedEvent = 'pty-cleared:' + sessionId;

    const cleanupOutput = Events.On(ptyOutputEvent, (event: { data: { data: string } }) => {
      const output = event?.data?.data;
      if (output) {
        backendAvailableRef.current = true;
        if (process.env.NODE_ENV === 'development') {
          console.log({output});
        }
        terminalRef.current?.write(output);
      }
    });

    const cleanupExit = Events.On(ptyExitEvent, (event: { data: { exitCode: number; wasIntentional: boolean } }) => {
      const { exitCode, wasIntentional } = event?.data ?? {};
      if (process.env.NODE_ENV === 'development') {
        console.log(`Shell exited: code=${exitCode}, intentional=${wasIntentional}`);
      }
      if (wasIntentional) {
        onShellExitRef.current?.();
      }
    });

    const cleanupCleared = Events.On(ptyClearedEvent, () => {
      terminalRef.current?.clear();
    });

    return () => {
      cleanupOutput();
      cleanupExit();
      cleanupCleared();
    };
  }, [sessionId]);

  useEffect(() => {
    const term = terminalRef.current;
    if (!term) return;

    requestAnimationFrame(() => {
        const current = terminalRef.current;
        if (!current) return;

        const styles = getComputedStyle(document.documentElement);
        const background = styles.getPropertyValue('--background').trim();
        const foreground = styles.getPropertyValue('--foreground').trim();
        const primary = styles.getPropertyValue('--primary').trim();
        const cursorAccent = background;
        const selectionBg = hexToRgba(primary, 0.4);

        current.options.theme = {
            ...current.options.theme,
            background,
            foreground,
            cursor: primary,
            cursorAccent,
            selectionBackground: selectionBg,
        };
    });
  }, [theme]);

  return (
    <div
      ref={containerRef}
      className="terminal-container"
      style={{ display: isVisible ? '' : 'none' }}
    />
  );
  }
);

export default TerminalComponent;
