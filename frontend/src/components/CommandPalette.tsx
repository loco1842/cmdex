import React, {
  useState,
  useMemo,
  useRef,
  useEffect,
  useCallback,
} from 'react';
import { type Command, type Category } from '../types';
import { getCommandDisplayTitle } from '../utils/tab';
import { filterCommands, scriptSnippet } from '../utils/commandSearch';
import { Kbd, ShortcutLabel } from './ui/kbd';
import { isCmdOrCtrl } from '../lib/shortcuts';
import { FileText, Search, X } from 'lucide-react';

interface CommandPaletteProps {
  open: boolean;
  commands: Command[];
  categories: Category[];
  onClose: () => void;
  onOpen: (cmd: Command) => void;
  onExecute: (cmd: Command) => void;
}

/** Highlight substring matches in text */
function Highlight({ text, query }: { text: string; query: string }) {
  if (!query) return <>{text}</>;
  const idx = text.toLowerCase().indexOf(query.toLowerCase());
  if (idx === -1) return <>{text}</>;
  return (
    <>
      {text.slice(0, idx)}
      <mark className="palette-match">{text.slice(idx, idx + query.length)}</mark>
      {text.slice(idx + query.length)}
    </>
  );
}

const CommandPalette: React.FC<CommandPaletteProps> = ({
  open,
  commands,
  categories,
  onClose,
  onOpen,
  onExecute,
}) => {
  const [query, setQuery] = useState('');
  const [activeIndex, setActiveIndex] = useState(0);
  const inputRef = useRef<HTMLInputElement>(null);
  const listRef = useRef<HTMLDivElement>(null);

  const catMap = useMemo(() => {
    const m: Record<string, string> = {};
    categories.forEach((c) => { m[c.id] = c.name; });
    return m;
  }, [categories]);

  const filtered = useMemo(() => filterCommands(query, commands), [query, commands]);

  // Reset selection when results change
  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect
    setActiveIndex(0);
  }, [filtered]);

  // Focus input when opened
  useEffect(() => {
    if (open) {
      const t = setTimeout(() => inputRef.current?.focus(), 30);
      // eslint-disable-next-line react-hooks/set-state-in-effect -- reset state on open
      setQuery('');
      setActiveIndex(0);
      return () => clearTimeout(t);
    }
  }, [open]);

  // Scroll active item into view
  useEffect(() => {
    if (!listRef.current) return;
    const el = listRef.current.querySelector<HTMLElement>(`[data-idx="${activeIndex}"]`);
    el?.scrollIntoView({ block: 'nearest' });
  }, [activeIndex]);

  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent<HTMLInputElement>) => {
      if (e.key === 'ArrowDown') {
        e.preventDefault();
        setActiveIndex((i) => Math.min(i + 1, filtered.length - 1));
      } else if (e.key === 'ArrowUp') {
        e.preventDefault();
        setActiveIndex((i) => Math.max(i - 1, 0));
      } else if (e.key === 'Enter') {
        e.preventDefault();
        const cmd = filtered[activeIndex];
        if (!cmd) return;
        if (isCmdOrCtrl(e)) {
          onExecute(cmd);
        } else {
          onOpen(cmd);
        }
        onClose();
      } else if (e.key === 'Escape') {
        onClose();
      }
    },
    [filtered, activeIndex, onOpen, onExecute, onClose],
  );

  if (!open) return null;

  return (
    <div className="palette-overlay" data-testid="command-palette" onMouseDown={onClose}>
      <div className="palette-modal" onMouseDown={(e) => e.stopPropagation()}>

        {/* Search row */}
        <div className="palette-search">
          <Search size={15} className="palette-search-icon" />
          <input
            ref={inputRef}
            className="palette-input"
            data-testid="palette-input"
            placeholder="Search commands…"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            onKeyDown={handleKeyDown}
            autoComplete="off"
            spellCheck={false}
          />
          {query && (
            <button className="palette-clear" onClick={() => { setQuery(''); inputRef.current?.focus(); }}>
              <X size={13} />
            </button>
          )}
          <div className="palette-shortcut-badge">
            <ShortcutLabel id="palette" />
          </div>
        </div>

        {/* Results */}
        <div className="palette-results" ref={listRef}>
          {filtered.length === 0 ? (
            <div className="palette-empty" data-testid="palette-empty">No commands match "{query}"</div>
          ) : (
            filtered.map((cmd, i) => {
              const catName = cmd.categoryId ? catMap[cmd.categoryId] : null;
              const isActive = i === activeIndex;
              return (
                <div
                  key={cmd.id}
                  data-idx={i}
                  data-testid={`palette-item-${cmd.id}`}
                  className={`palette-item${isActive ? ' active' : ''}`}
                  onMouseEnter={() => setActiveIndex(i)}
                  onClick={() => { onOpen(cmd); onClose(); }}
                >
                  <FileText size={13} className="palette-item-icon" />
                  <div className="palette-item-body">
                    <span className="palette-item-title">
                      <Highlight text={getCommandDisplayTitle(cmd)} query={query.trim()} />
                    </span>
                    {cmd.description?.Valid && (
                      <span className="palette-item-desc">
                        <Highlight text={cmd.description.String} query={query.trim()} />
                      </span>
                    )}
                    {cmd.scriptContent && (
                      <span className="palette-item-script">
                        {scriptSnippet(cmd.scriptContent)}
                      </span>
                    )}
                  </div>
                  <div className="palette-item-meta">
                    {catName && <span className="palette-cat-badge">{catName}</span>}
                    {(cmd.tags || []).slice(0, 2).map((tag) => (
                      <span key={tag} className="palette-tag-badge">#{tag}</span>
                    ))}
                  </div>
                </div>
              );
            })
          )}
        </div>

        {/* Footer hints */}
        <div className="palette-footer">
          <span className="palette-hint"><Kbd>↑</Kbd><Kbd>↓</Kbd> navigate</span>
          <span className="palette-hint"><Kbd>↩</Kbd> open</span>
          <span className="palette-hint"><ShortcutLabel id="execute" /> execute</span>
          <span className="palette-hint"><Kbd>Esc</Kbd> close</span>
          <span className="palette-hint" style={{ marginLeft: 'auto', opacity: 0.5 }}>
            {filtered.length} result{filtered.length !== 1 ? 's' : ''}
          </span>
        </div>
      </div>
    </div>
  );
};

export default CommandPalette;
