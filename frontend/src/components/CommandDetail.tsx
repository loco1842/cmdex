import React, {
  useState,
  useCallback,
  useMemo,
  useEffect,
  useLayoutEffect,
  useRef,
} from 'react';
import {
  DndContext,
  closestCenter,
  PointerSensor,
  useSensor,
  useSensors,
  type DragEndEvent,
} from '@dnd-kit/core';
import {
  SortableContext,
  horizontalListSortingStrategy,
  useSortable,
} from '@dnd-kit/sortable';
import { CSS } from '@dnd-kit/utilities';
import { useTranslation } from 'react-i18next';
import type { Command, TabDraft, VariablePrompt, OSPathMap, OSKey } from '../types';
import { getOSPath, setOSPath, shortenPath } from '../utils/path';
import { useCopyToClipboard } from '../hooks/useCopyToClipboard';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { Textarea } from '@/components/ui/textarea';
import {
  Tooltip,
  TooltipTrigger,
  TooltipContent,
} from '@/components/ui/tooltip';
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
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from '@/components/ui/dialog';
import {
  ContextMenu,
  ContextMenuContent,
  ContextMenuItem,
  ContextMenuTrigger,
} from '@/components/ui/context-menu';
import {
  Copy,
  Check,
  Play,
  Plus,
  Loader2,
  Pencil,
  X,
  ALargeSmall,
  Hash,
  LayoutTemplate,
  ScanEye,
  FolderOpen,
} from 'lucide-react';
import { PickDirectory } from '../../bindings/cmdex/app';
import { toast } from 'sonner';
import { ShortcutLabel, ShortcutHint } from '@/components/ui/kbd';
import { Heading } from '@/components/ui/heading';

import { cn } from '@/lib/utils';

interface HighlightedTextareaProps {
  value: string;
  onChange: (value: string) => void;
  onKeyDown?: (e: React.KeyboardEvent<HTMLTextAreaElement>) => void;
  onBlur?: (e: React.FocusEvent<HTMLTextAreaElement>) => void;
  onFocus?: (e: React.FocusEvent<HTMLTextAreaElement>) => void;
  autoFocus?: boolean;
  placeholder?: string;
  className?: string;
  'data-testid'?: string;
}

const HighlightedTextarea: React.FC<HighlightedTextareaProps> = ({
  value,
  onChange,
  onKeyDown,
  onBlur,
  onFocus,
  autoFocus,
  placeholder,
  className = '',
  'data-testid': dataTestId,
}) => {
  const textareaRef = useRef<HTMLTextAreaElement>(null);
  const backdropRef = useRef<HTMLDivElement>(null);

  const syncScroll = useCallback(() => {
    if (textareaRef.current && backdropRef.current) {
      backdropRef.current.scrollTop = textareaRef.current.scrollTop;
      backdropRef.current.scrollLeft = textareaRef.current.scrollLeft;
    }
  }, []);

  const highlighted = useMemo(() => {
    const parts = value.split(/(\{\{\w+\}\})/g);
    return parts.map((part, i) => {
      if (/^\{\{\w+\}\}$/.test(part)) {
        return <mark key={i} className="var-highlight">{part}</mark>;
      }
      return <span key={i}>{part}</span>;
    });
  }, [value]);

  return (
    <div className={`highlighted-textarea-wrap ${className}`} data-testid={dataTestId}>
      <div ref={backdropRef} className="highlighted-textarea-backdrop" aria-hidden>
        <code>{highlighted}{'\n'}</code>
      </div>
      <Textarea
        ref={textareaRef}
        className="highlighted-textarea-input"
        data-testid={dataTestId ? `${dataTestId}-textarea` : undefined}
        value={value}
        onChange={(e) => onChange(e.target.value)}
        onKeyDown={onKeyDown}
        onBlur={onBlur}
        onFocus={onFocus}
        onScroll={syncScroll}
        autoFocus={autoFocus}
        placeholder={placeholder}
      />
    </div>
  );
};

interface SortablePresetChipProps {
  id: string;
  name: string;
  isActive: boolean;
  isRenaming: boolean;
  renamingDraft: string;
  onSelect: () => void;
  onDoubleClick: () => void;
  onSetRenaming: (id: string, name: string) => void;
  onRenameChange: (val: string) => void;
  onCommitRename: () => void;
  onConfirmDelete: (id: string) => void;
  renameLabel: string;
  deleteLabel: string;
  presetNamePlaceholder: string;
}

const SortablePresetChip: React.FC<SortablePresetChipProps> = ({
  id,
  name,
  isActive,
  isRenaming,
  renamingDraft,
  onSelect,
  onDoubleClick,
  onSetRenaming,
  onRenameChange,
  onCommitRename,
  onConfirmDelete,
  renameLabel,
  deleteLabel,
  presetNamePlaceholder,
}) => {
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = useSortable({ id });

  const style: React.CSSProperties = {
    transform: CSS.Transform.toString(transform),
    transition,
    opacity: isDragging ? 0.5 : undefined,
    display: 'inline-flex',
  };

  if (isRenaming) {
    return (
      <div ref={setNodeRef} style={style}>
        <input
          className="preset-chip preset-chip-renaming"
          data-testid={`preset-chip-rename-${id}`}
          autoFocus
          placeholder={presetNamePlaceholder}
          value={renamingDraft}
          onChange={(e) => onRenameChange(e.target.value.slice(0, 30))}
          onBlur={onCommitRename}
          onKeyDown={(e) => {
            if (e.key === 'Enter') { e.preventDefault(); onCommitRename(); }
            if (e.key === 'Escape') onCommitRename();
          }}
          onClick={(e) => e.stopPropagation()}
        />
      </div>
    );
  }

  return (
    <div ref={setNodeRef} style={style} {...attributes} {...listeners}>
      <ContextMenu>
        <ContextMenuTrigger asChild>
          <button
            type="button"
            className={`preset-chip${isActive ? ' active' : ''}`}
            data-testid={`preset-chip-${id}`}
            onClick={onSelect}
            onDoubleClick={(e) => { e.preventDefault(); onDoubleClick(); }}
            onKeyDown={(e) => {
              if (e.key === 'F2') { e.preventDefault(); onDoubleClick(); }
            }}
          >
            {name}
          </button>
        </ContextMenuTrigger>
        <ContextMenuContent>
          <ContextMenuItem onClick={() => { onSetRenaming(id, name); onSelect(); }}>
            {renameLabel}
          </ContextMenuItem>
          <ContextMenuItem
            className="text-destructive focus:text-destructive"
            onClick={() => onConfirmDelete(id)}
          >
            {deleteLabel}
          </ContextMenuItem>
        </ContextMenuContent>
      </ContextMenu>
    </div>
  );
};

export interface CommandDetailProps {
  command: Command;
  draft: TabDraft;
  baselineScriptBody: string;
  onDraftChange: (partial: Partial<TabDraft>) => void;
  isNewCommand: boolean;
  isExecuting: boolean;
  variables: VariablePrompt[];
  onExecute: (values: Record<string, string>) => void;
  onFillVariables: (initialValues: Record<string, string>) => void;
  onRenamePreset: (presetId: string, newName: string) => Promise<void>;
  onDeletePreset: (presetId: string) => Promise<void>;
  onAddPreset: (initialValues?: Record<string, string>) => Promise<string>;
  onSavePresetValues: (presetId: string, values: Record<string, string>) => Promise<void>;
  onReorderPresets: (presetIds: string[]) => Promise<void>;
  onResolvedValuesChange?: (values: Record<string, string>) => void;
  onSaveScript?: (scriptBody: string) => Promise<void>;
  currentOS?: OSKey;
  defaultWorkingDir?: OSPathMap;
}

const CommandDetail: React.FC<CommandDetailProps> = ({
  command,
  draft,
  baselineScriptBody,
  onDraftChange,
  isNewCommand,
  isExecuting,
  variables,
  onExecute,
  onFillVariables,
  onRenamePreset,
  onDeletePreset,
  onAddPreset,
  onSavePresetValues,
  onReorderPresets,
  onResolvedValuesChange,
  onSaveScript,
  currentOS,
  defaultWorkingDir,
}) => {
  const { t } = useTranslation();
  const commandWD = getOSPath(draft.workingDir, currentOS);
  const defaultWD = getOSPath(defaultWorkingDir, currentOS);
  const effectiveWD = commandWD || defaultWD;
  const { copied, copy } = useCopyToClipboard();
  const [previewOpen, setPreviewOpen] = useState(false);
  const showPreview = previewOpen;
  const [selectedPresetId, setSelectedPresetId] = useState<string>('');
  const [focusedVarName, setFocusedVarName] = useState<string | null>(null);
  const [overrides, setOverrides] = useState<Record<string, string>>({});
  const [tagInput, setTagInput] = useState('');
  const [editingTagIndex, setEditingTagIndex] = useState<number | null>(null);
  const [editingTagDraft, setEditingTagDraft] = useState('');
  const [addingTag, setAddingTag] = useState(false);
  const [scriptEditor, setScriptEditor] = useState(() => isNewCommand);
  const [scriptHintOpen, setScriptHintOpen] = useState(false);
  const [renamingChipId, setRenamingChipId] = useState<string | null>(null);
  const [renamingChipDraft, setRenamingChipDraft] = useState('');
  const [confirmDeletePresetId, setConfirmDeletePresetId] = useState<string | null>(null);
  const [deletingPresetId, setDeletingPresetId] = useState<string | null>(null);
  const [newlyCreatedPresetId, setNewlyCreatedPresetId] = useState<string | null>(null);
  const preAddPresetIdRef = useRef<string>('');
  const [scriptEditDraft, setScriptEditDraft] = useState('');
  const [showScriptDiscardConfirm, setShowScriptDiscardConfirm] = useState(false);
  const [workingDirDialogOpen, setWorkingDirDialogOpen] = useState(false);
  const [workingDirDraft, setWorkingDirDraft] = useState('');
  const scriptWrapRef = useRef<HTMLDivElement>(null);
  const scriptEditDraftRef = useRef('');
  const scriptBodyRef = useRef('');

  const presetSensors = useSensors(
    useSensor(PointerSensor, { activationConstraint: { distance: 5 } }),
  );

  const handlePresetDragEnd = useCallback(
    (event: DragEndEvent) => {
      const { active, over } = event;
      if (!over || active.id === over.id || !command.presets) return;
      const oldIndex = command.presets.findIndex((p) => p.id === active.id);
      const newIndex = command.presets.findIndex((p) => p.id === over.id);
      if (oldIndex === -1 || newIndex === -1) return;
      const ids = command.presets.map((p) => p.id);
      ids.splice(oldIndex, 1);
      ids.splice(newIndex, 0, active.id as string);
      onReorderPresets(ids);
    },
    [command.presets, onReorderPresets],
  );

  const presetIds = useMemo(
    () => (command.presets || []).map((p) => p.id),
    [command.presets],
  );

  const scriptBody = draft.scriptBody;

  const titleHeadingRef = useRef<HTMLHeadingElement>(null);

  useEffect(() => {
    scriptEditDraftRef.current = scriptEditDraft;
    scriptBodyRef.current = scriptBody;
  }, [scriptEditDraft, scriptBody]);

  useLayoutEffect(() => {
    const el = titleHeadingRef.current;
    if (!el) return;
    if (document.activeElement === el) return;
    const next = draft.title ?? '';
    if (el.textContent !== next) {
      el.textContent = next;
    }
  }, [draft.title, command.id]);

  const handleTitleInput = useCallback(
    (e: React.FormEvent<HTMLHeadingElement>) => {
      const el = e.currentTarget;
      const normalized = (el.textContent ?? '').replace(/\r?\n/g, ' ');
      if (normalized !== (el.textContent ?? '')) {
        el.textContent = normalized;
        const range = document.createRange();
        const sel = window.getSelection();
        range.selectNodeContents(el);
        range.collapse(false);
        sel?.removeAllRanges();
        sel?.addRange(range);
      }
      onDraftChange({ title: normalized });
    },
    [onDraftChange],
  );

  const handleTitleKeyDown = useCallback((e: React.KeyboardEvent<HTMLHeadingElement>) => {
    if (e.key === 'Enter') e.preventDefault();
  }, []);

  const handleTitlePaste = useCallback(
    (e: React.ClipboardEvent<HTMLHeadingElement>) => {
      e.preventDefault();
      const pasted = e.clipboardData.getData('text/plain').replace(/\r?\n/g, ' ');
      const el = titleHeadingRef.current;
      if (!el) return;
      const sel = window.getSelection();
      if (!sel?.rangeCount) return;
      const range = sel.getRangeAt(0);
      range.deleteContents();
      range.insertNode(document.createTextNode(pasted));
      range.collapse(false);
      sel.removeAllRanges();
      sel.addRange(range);
      onDraftChange({ title: el.textContent ?? '' });
    },
    [onDraftChange],
  );

  useEffect(() => {
    /* eslint-disable react-hooks/set-state-in-effect -- auto-open editor for new commands with empty body */
    if (isNewCommand) {
      setScriptEditor(!draft.scriptBody.trim());
    } else {
      setScriptEditor(false);
    }
    /* eslint-enable react-hooks/set-state-in-effect */
  }, [command.id, isNewCommand]); // eslint-disable-line react-hooks/exhaustive-deps -- draft.scriptBody intentionally excluded: only auto-open on command switch

  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect -- reset overrides when command or preset changes
    setOverrides({});
  }, [command.id, selectedPresetId]);

  useEffect(() => {
    if (command.presets && command.presets.length > 0) {
      const isValidPreset = command.presets.some((p) => p.id === selectedPresetId);
      if (!isValidPreset) {
        const newId = command.presets[0].id;
        // eslint-disable-next-line react-hooks/set-state-in-effect -- fallback to first preset when selected becomes invalid
        setSelectedPresetId(newId);
      }
    }
    // selectedPresetId intentionally excluded: effect should only re-run when the command
    // or its presets change, not when the user explicitly deselects a chip.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [command.id, command.presets]);

  // Auto-switch to Preview when a preset is selected
  useEffect(() => {
    if (selectedPresetId) {
      // eslint-disable-next-line react-hooks/set-state-in-effect -- auto-open preview when preset is selected
      setPreviewOpen(true);
    }
  }, [selectedPresetId, command.id]);

  const commitChipRename = async () => {
    if (!renamingChipId) return;
    const trimmed = renamingChipDraft.trim().slice(0, 30);
    if (!trimmed) {
      if (renamingChipId === newlyCreatedPresetId) {
        await onDeletePreset(renamingChipId);
        setSelectedPresetId(preAddPresetIdRef.current);
        setNewlyCreatedPresetId(null);
      }
      setRenamingChipId(null);
      return;
    }
    await onRenamePreset(renamingChipId, trimmed);
    if (renamingChipId === newlyCreatedPresetId) setNewlyCreatedPresetId(null);
    setRenamingChipId(null);
  };

  const reveal = useCallback(
    (key: keyof TabDraft['revealed']) => {
      onDraftChange({
        revealed: { ...draft.revealed, [key]: true },
      });
    },
    [draft.revealed, onDraftChange],
  );

  const handleScriptBodyChange = useCallback(
    (body: string) => {
      onDraftChange({
        scriptBody: body,
      });
    },
    [onDraftChange],
  );

  useEffect(() => {
    if (!scriptEditor || isNewCommand) return;
    const handler = (e: MouseEvent) => {
      const target = e.target as Node;
      if (scriptWrapRef.current?.contains(target)) return;
      if ((target as HTMLElement).closest?.('.command-text-box-glow')) return;
      if ((target as HTMLElement).closest?.('[role="alertdialog"]')) return;
      if (scriptEditDraftRef.current === scriptBodyRef.current) {
        setScriptEditor(false);
      } else {
        setShowScriptDiscardConfirm(true);
      }
    };
    document.addEventListener('mousedown', handler);
    return () => document.removeEventListener('mousedown', handler);
  }, [scriptEditor, isNewCommand]);

  const enterScriptEdit = useCallback(() => {
    setScriptEditDraft(scriptBody);
    setScriptEditor(true);
  }, [scriptBody, setScriptEditDraft, setScriptEditor]);

  const doSaveScriptEdit = useCallback(() => {
    handleScriptBodyChange(scriptEditDraft);
    setScriptEditor(false);
    if (!isNewCommand && onSaveScript) {
      onSaveScript(scriptEditDraft);
    }
  }, [scriptEditDraft, handleScriptBodyChange, isNewCommand, onSaveScript, setScriptEditor]);

  const saveScriptEdit = useCallback(() => {
    doSaveScriptEdit();
  }, [doSaveScriptEdit]);

  const discardScriptEdit = useCallback(() => {
    setScriptEditDraft(baselineScriptBody);
    setScriptEditor(false);
  }, [baselineScriptBody, setScriptEditDraft, setScriptEditor]);

  const hasScriptChanges = scriptEditor && !isNewCommand && scriptEditDraft !== scriptBody;

  const resolvedValues = useMemo(() => {
    const vals: Record<string, string> = {};
    if (selectedPresetId) {
      const preset = command.presets.find((p) => p.id === selectedPresetId);
      if (preset) {
        variables.forEach((v) => {
          vals[v.name] = preset.values[v.name] ?? v.defaultValue ?? '';
        });
        return { ...vals, ...overrides };
      }
    }
    variables.forEach((v) => {
      vals[v.name] = v.defaultValue ?? '';
    });
    return { ...vals, ...overrides };
  }, [selectedPresetId, command.presets, variables, overrides]);

  const prevResolvedRef = useRef<Record<string, string>>({});
  useEffect(() => {
    // Only call onResolvedValuesChange when values actually change,
    // preventing infinite loops when parent re-renders create new object references.
    const keys = new Set([...Object.keys(prevResolvedRef.current), ...Object.keys(resolvedValues)]);
    let changed = false;
    for (const k of keys) {
      if (prevResolvedRef.current[k] !== resolvedValues[k]) {
        changed = true;
        break;
      }
    }
    if (changed) {
      onResolvedValuesChange?.(resolvedValues);
      prevResolvedRef.current = resolvedValues;
    }
  }, [resolvedValues, onResolvedValuesChange]);

  const hasUnsavedChanges = useMemo(() => {
    if (!selectedPresetId) return false;
    const preset = command.presets.find((p) => p.id === selectedPresetId);
    if (!preset) return false;
    return Object.entries(overrides).some(([k, v]) => {
      const stored = preset.values[k] ?? variables.find((x) => x.name === k)?.defaultValue ?? '';
      return v !== stored;
    });
  }, [selectedPresetId, overrides, command.presets, variables]);

  const scriptParts = useMemo(
    () => (scriptBody ? scriptBody.split(/(\{\{\w+\}\})/g) : null),
    [scriptBody],
  );



  const renderScriptUnified = useMemo(() => {
    if (!scriptParts) return null;
    if (!showPreview) {
      // Template mode: same as renderScriptWithVars
      return scriptParts.map((part, i) => {
        if (/^\{\{\w+\}\}$/.test(part)) {
          const varName = part.slice(2, -2);
          return (
            <span key={i} className="var-missing" title={varName}>
              {part}
            </span>
          );
        }
        return <span key={i}>{part}</span>;
      });
    }
    // Preview mode: resolved values or dimmed placeholder [varName]
    return scriptParts.map((part, i) => {
      if (/^\{\{\w+\}\}$/.test(part)) {
        const varName = part.slice(2, -2);
        const val = resolvedValues[varName];
        const isFocused = focusedVarName === varName;
        if (val) {
          return (
            <span key={i} className={`var-filled${isFocused ? ' var-focused' : ''}`} title={`${varName}=${val}`}>
              {val}
            </span>
          );
        }
        // No value: show dimmed [varName] placeholder (per D-03)
        return (
          <span key={i} className={`var-placeholder-muted${isFocused ? ' var-focused' : ''}`} title={varName}>
            [{varName}]
          </span>
        );
      }
      return <span key={i}>{part}</span>;
    });
  }, [scriptParts, showPreview, resolvedValues, focusedVarName]);

  const getResolvedScript = useMemo(() => {
    if (!scriptBody) return '';
    return scriptBody.replace(/\{\{(\w+)\}\}/g, (_match, varName) => {
      return resolvedValues[varName] || `{{${varName}}}`;
    });
  }, [scriptBody, resolvedValues]);

  const handleCopy = useCallback(() => {
    const text = showPreview ? getResolvedScript : scriptBody;
    copy(text).catch(() => {
      toast.error(t('commandDetail.copyFailed'));
    });
  }, [showPreview, getResolvedScript, scriptBody, copy, t]);

  const TAG_REGEX = /^[a-zA-Z0-9-]+$/;

  const commitNewTag = () => {
    const trimmed = tagInput.trim();
    if (trimmed && TAG_REGEX.test(trimmed) && !draft.tags.includes(trimmed)) {
      onDraftChange({ tags: [...draft.tags, trimmed] });
    }
    setTagInput('');
    setAddingTag(false);
  };

  const commitEditTag = () => {
    if (editingTagIndex === null) return;
    const trimmed = editingTagDraft.trim();
    if (!trimmed || !TAG_REGEX.test(trimmed)) {
      setEditingTagIndex(null);
      return;
    }
    const updated = [...draft.tags];
    if (draft.tags.includes(trimmed) && draft.tags[editingTagIndex] !== trimmed) {
      setEditingTagIndex(null);
      return;
    }
    updated[editingTagIndex] = trimmed;
    onDraftChange({ tags: updated });
    setEditingTagIndex(null);
  };

  const showHeaderBlock =
    draft.revealed.title ||
    draft.revealed.description ||
    draft.revealed.tags;

  const showTitle = draft.revealed.title;
  const showDescription = draft.revealed.description;
  const showTags = draft.revealed.tags;

  return (
    // Spacing controlled by .main-body CSS (24px 24px) — no inline padding overrides in this component
    <div className="command-detail">
      {showHeaderBlock && (
        <div className="detail-header">
          {showTitle && (
            <div className="hover-actions-host detail-header-title-wrap inline-icon-field">
              <Heading
                ref={titleHeadingRef}
                level={1}
                className={cn(
                  'text-center title-contenteditable w-full min-w-0 cursor-text outline-none focus-visible:ring-2 focus-visible:ring-ring/50 focus-visible:ring-offset-2 focus-visible:ring-offset-background rounded-sm',
                  !draft.title.trim() && 'title-contenteditable--empty',
                )}
                contentEditable
                suppressContentEditableWarning
                data-testid="command-title"
                aria-label={t('commandEditor.title')}
                data-placeholder={t('commandEditor.titlePlaceholder')}
                onInput={handleTitleInput}
                onKeyDown={handleTitleKeyDown}
                onPaste={handleTitlePaste}
              />
              {(!draft.revealed.description || !draft.revealed.tags) && (
                <div className="add-field-pill-anchor">
                  {!draft.revealed.description && (
                    <button
                      type="button"
                      className="add-title-pill"
                      onClick={(e) => { e.stopPropagation(); reveal('description'); }}
                    >
                      <ALargeSmall className="size-3 shrink-0" />
                      <span className="add-title-pill-label">{t('commandDetail.addDescription')}</span>
                    </button>
                  )}
                  {!draft.revealed.tags && (
                    <button
                      type="button"
                      className="add-title-pill"
                      onClick={(e) => { e.stopPropagation(); reveal('tags'); }}
                    >
                      <Hash className="size-3 shrink-0" />
                      <span className="add-title-pill-label">{t('commandDetail.addTags')}</span>
                    </button>
                  )}
                </div>
              )}
            </div>
          )}

          {showTags && (
            <div className="inline-icon-field pt-0!">
              <Tooltip>
                <TooltipTrigger asChild>
                  <Hash className="inline-icon-field-icon" color="var(--primary)" />
                </TooltipTrigger>
                <TooltipContent>{t('commandDetail.tagsTooltip')}</TooltipContent>
              </Tooltip>
              <div className="tags-badge-row">
                {draft.tags.map((tag, idx) => (
                  <Badge key={tag} variant="outline-default" className="tag-badge group">
                    {editingTagIndex === idx ? (
                      <input
                        className="tag-edit-input"
                        autoFocus
                        style={{ width: '23ch' }}
                        value={editingTagDraft}
                        onChange={(e) => {
                          let trimmed = e.target.value.replace(/[^a-zA-Z0-9-]/g, '');
                          if (trimmed.length > 30) {
                            trimmed = trimmed.slice(0, 30);
                          }
                          setEditingTagDraft(trimmed);
                        }}
                        onBlur={commitEditTag}
                        onKeyDown={(e) => {
                          if (e.key === 'Enter') { e.preventDefault(); commitEditTag(); }
                          if (e.key === 'Escape') setEditingTagIndex(null);
                        }}
                      />
                    ) : (
                      <>
                        <span
                          className="tag-name"
                          onClick={() => { setEditingTagIndex(idx); setEditingTagDraft(tag); }}
                        >
                          {tag}
                        </span>
                        <button
                          type="button"
                          className="tag-remove-btn"
                          onClick={() => onDraftChange({ tags: draft.tags.filter((x) => x !== tag) })}
                        >
                        <X className="size-2.5" />
                        </button>
                      </>
                    )}
                  </Badge>
                ))}
                {addingTag ? (
                  <Badge variant="outline" className="tag-badge w-fit">
                    <input
                      className="tag-edit-input"
                      autoFocus
                      value={tagInput}
                      style={{ width: '23ch' }}
                      onChange={(e) => {
                        let trimmed = e.target.value.replace(/[^a-zA-Z0-9-]/g, '');
                        if (trimmed.length > 30) {
                          trimmed = trimmed.slice(0, 30);
                        }
                        setTagInput(trimmed);
                      }}
                      onBlur={commitNewTag}
                      onKeyDown={(e) => {
                        if (e.key === 'Enter') { e.preventDefault(); commitNewTag(); }
                        if (e.key === 'Escape') { setTagInput(''); setAddingTag(false); }
                      }}
                      placeholder={t('commandDetail.tagNamePlaceholder')}
                    />
                  </Badge>
                ) : (
                  <Tooltip>
                    <TooltipTrigger asChild>
                      <button
                        type="button"
                        className="tag-add-btn"
                        onClick={() => { setTagInput(''); setAddingTag(true); }}
                      >
                        <Plus className="size-3" />
                      </button>
                    </TooltipTrigger>
                    <TooltipContent>{t('commandDetail.addTag')}</TooltipContent>
                  </Tooltip>
                )}
              </div>
            </div>
          )}

          {showDescription && (
            <div className="inline-icon-field mt-1">
              <Tooltip>
                <TooltipTrigger asChild>
                  <ALargeSmall className="inline-icon-field-icon mt-0.5" />
                </TooltipTrigger>
                <TooltipContent>{t('commandDetail.descriptionTooltip')}</TooltipContent>
              </Tooltip>
            <Textarea
              className="detail-description-textarea"
              data-testid="command-description"
              value={draft?.description}
              onChange={(e) => {
                onDraftChange({ description: e.target.value });
                const el = e.target;
                el.style.height = 'auto';
                el.style.height = Math.min(el.scrollHeight, 400) + 'px';
              }}
              onFocus={(e) => {
                const el = e.target;
                el.style.height = 'auto';
                el.style.height = Math.min(el.scrollHeight, 450) + 'px';
              }}
              onBlur={(e) => {
                onDraftChange({ description: e.target.value });
                const el = e.target;
                el.style.height = '';
                requestAnimationFrame(() => {
                  el.scrollTop = 0;
                  el.setSelectionRange(0, 0);
                });
              }}
              placeholder={t('commandEditor.descriptionPlaceholder')}
            />
            </div>
          )}

        </div>
      )}

      <div className="detail-section">
        <div className="detail-section-title">
          {t('commandDetail.command')}
          <Tooltip>
            <TooltipTrigger asChild>
              <button
                type="button"
                className="script-mode-toggle"
                hidden={isNewCommand || variables.length <= 0}
                onClick={() => setPreviewOpen(!showPreview)}
                aria-label={showPreview ? t('commandDetail.showTemplate') : t('commandDetail.showPreview')}
              >
                {showPreview ? (
                  <ScanEye className="size-3" />
                ) : (
                  <LayoutTemplate className="size-3" />
                )}
              </button>
            </TooltipTrigger>
            <TooltipContent>
              {showPreview ? t('commandDetail.showTemplate') : t('commandDetail.showPreview')}
            </TooltipContent>
          </Tooltip>
        </div>
        <div className="hover-actions-host script-area-hover command-text-box-glow">
          {!draft.revealed.title && scriptBody.trim().length > 0 && (
            <div className="add-title-pill-anchor">
              <button
                type="button"
                className="add-title-pill"
                onClick={(e) => { e.stopPropagation(); reveal('title'); }}
              >
                <Plus className="size-3 shrink-0" />
                <span className="add-title-pill-label">{t('commandDetail.addTitle')}</span>
              </button>
            </div>
          )}
          <div className="command-text-box-inner" ref={scriptWrapRef}>
            <div className="command-text-box-header">
              <div className="flex items-center gap-1.5">
                <Tooltip>
                  <TooltipTrigger asChild>
                    <Button
                      variant="ghost"
                      size="icon-xs"
                      className="cmd-header-dim-btn"
                      onClick={() => {
                        setWorkingDirDraft(getOSPath(draft.workingDir, currentOS));
                        setWorkingDirDialogOpen(true);
                      }}
                    >
                      <FolderOpen className="size-3.5" />
                    </Button>
                  </TooltipTrigger>
                  <TooltipContent>
                    {effectiveWD
                      ? shortenPath(effectiveWD)
                      : t('commandDetail.workingDirectoryNotSet')}
                  </TooltipContent>
                </Tooltip>
                <span className="command-text-box-label">
                  {showPreview ? t('commandDetail.preview') : t('commandDetail.template')}
                </span>
                {!isNewCommand && (
                  <Tooltip>
                    <TooltipTrigger asChild>
                      <Button
                        variant="ghost"
                        size="icon-xs"
                        className="text-primary hover:text-primary"
                        disabled={isExecuting}
                        data-testid="command-run-btn"
                        onClick={() => {
                          if (variables.length > 0) {
                            const hasEmpty = variables.some((v) => !resolvedValues[v.name]);
                            if (hasEmpty) {
                              onFillVariables(resolvedValues);
                            } else {
                              onExecute(resolvedValues);
                            }
                          } else {
                            onExecute({});
                          }
                        }}
                      >
                        {isExecuting ? (
                          <Loader2 className="size-3.5 animate-spin" />
                        ) : (
                          <Play 
                            className="size-3.5" 
                            style={{
                              filter: `
                                drop-shadow(0 0  4px rgb(from var(--primary) r g b / 1))
                                drop-shadow(0 0 16px rgb(from var(--primary) r g b / 0.9))
                                drop-shadow(0 0 32px rgb(from var(--primary) r g b / 0.5))
                              `
                            }}
                          />
                        )}
                      </Button>
                    </TooltipTrigger>
                    <TooltipContent side="right">
                      {isExecuting ? (
                        t('commandDetail.running')
                      ) : (
                        <ShortcutHint label={t('commandDetail.execute')} id="execute" />
                      )}
                    </TooltipContent>
                  </Tooltip>
                )}
              </div>
              <div className="command-text-box-header-actions">
                {scriptEditor && !isNewCommand && (
                  <Tooltip>
                    <TooltipTrigger asChild>
                      <Button variant="ghost" size="icon-xs" data-testid="script-edit-discard-btn" onClick={discardScriptEdit}>
                        <X className="size-3.5 text-destructive" />
                      </Button>
                    </TooltipTrigger>
                    <TooltipContent>{t('commandDetail.revertScript')}</TooltipContent>
                  </Tooltip>
                )}
                {hasScriptChanges && (
                  <Tooltip>
                    <TooltipTrigger asChild>
                      <Button variant="ghost" size="icon-xs" data-testid="script-edit-save-btn" onClick={saveScriptEdit}>
                        <Check className="size-3.5 text-success" />
                      </Button>
                    </TooltipTrigger>
                    <TooltipContent>{t('commandDetail.saveScript')}</TooltipContent>
                  </Tooltip>
                )}
                {!scriptEditor && (
                  <Tooltip>
                    <TooltipTrigger asChild>
                      <Button variant="ghost" size="icon-xs" data-testid="script-edit-enter-btn" onClick={enterScriptEdit}>
                        <Pencil className="size-3.5" />
                      </Button>
                    </TooltipTrigger>
                    <TooltipContent>{t('commandDetail.editScript')}</TooltipContent>
                  </Tooltip>
                )}
                <Tooltip>
                  <TooltipTrigger asChild>
                    <Button variant="ghost" size="icon-xs" onClick={handleCopy}>
                      {copied ? (
                        <Check className="size-3.5 text-success" />
                      ) : (
                        <Copy className="size-3.5" />
                      )}
                    </Button>
                  </TooltipTrigger>
                  <TooltipContent>
                    {copied ? t('commandDetail.copied') : t('commandDetail.copyCommand')}
                  </TooltipContent>
                </Tooltip>
              </div>
            </div>
            {scriptEditor ? (
              <Tooltip open={scriptHintOpen}>
                <TooltipTrigger asChild>
                  <div className="script-edit-wrap">
                    <HighlightedTextarea
                      className="detail-script-textarea"
                      data-testid="command-script"
                      autoFocus={!isNewCommand}
                      value={isNewCommand ? scriptBody : scriptEditDraft}
                      onChange={(val) => {
                        if (isNewCommand) {
                          handleScriptBodyChange(val);
                        } else {
                          setScriptEditDraft(val);
                        }
                      }}
                      onFocus={() => setScriptHintOpen(true)}
                      onBlur={() => setScriptHintOpen(false)}
                      onKeyDown={(e) => {
                        if (e.key === 'Escape' && !isNewCommand) {
                          e.preventDefault();
                          discardScriptEdit();
                          return;
                        }
                        if (e.key === 'Enter' && !e.shiftKey && !isNewCommand) {
                          e.preventDefault();
                          if (hasScriptChanges) {
                            saveScriptEdit();
                          } else {
                            setScriptEditor(false);
                          }
                        }
                      }}
                      placeholder={t('commandEditor.commandPlaceholder')}
                    />
                  </div>
                </TooltipTrigger>
                {!isNewCommand && (<TooltipContent side="right" sideOffset={5} className="script-edit-tooltip-content p-0">
                  <div className="script-edit-tooltip-card">
                    <div className="script-edit-tooltip-title">{t('common.keyboardShortcuts')}</div>
                    <div className="script-edit-tooltip-row">
                      <span>{t('commandDetail.scriptEditHintNewLine')}</span>
                      <ShortcutLabel id="scriptNewLine" />
                    </div>
                    <div className="script-edit-tooltip-row">
                      <span>{t('commandDetail.scriptEditHintSave')}</span>
                      <ShortcutLabel id="scriptSave" />
                    </div>
                    <div className="script-edit-tooltip-row">
                      <span>{t('commandDetail.scriptEditHintDiscard')}</span>
                      <ShortcutLabel id="escape" />
                    </div>
                  </div>
                </TooltipContent>)}
              </Tooltip>
            ) : (
              <div className="command-text-box script-preview-compact">
                <code className="whitespace-pre-wrap">{renderScriptUnified}</code>
              </div>
            )}
          </div>
        </div>
      </div>

      {!isNewCommand && variables.length > 0 && (
        <div className="detail-section mt-4">
          <div className="detail-section-title">{t('commandDetail.presets')}</div>

          <div className="preset-chips">
            <DndContext sensors={presetSensors} collisionDetection={closestCenter} onDragEnd={handlePresetDragEnd}>
              <SortableContext items={presetIds} strategy={horizontalListSortingStrategy}>
                {command.presets?.map((p) => (
                  <SortablePresetChip
                    key={p.id}
                    id={p.id}
                    name={p.name}
                    isActive={selectedPresetId === p.id}
                    isRenaming={renamingChipId === p.id}
                    renamingDraft={renamingChipDraft}
                    onSelect={() => setSelectedPresetId((prev) => (prev === p.id ? '' : p.id))}
                    onDoubleClick={() => {
                      setSelectedPresetId(p.id);
                      setRenamingChipId(p.id);
                      setRenamingChipDraft(p.name);
                    }}
                    onSetRenaming={(id, name) => {
                      setRenamingChipId(id);
                      setRenamingChipDraft(name);
                    }}
                    onRenameChange={setRenamingChipDraft}
                    onCommitRename={commitChipRename}
                    onConfirmDelete={setConfirmDeletePresetId}
                    renameLabel={t('commandDetail.rename')}
                    deleteLabel={t('commandDetail.delete')}
                    presetNamePlaceholder={t('commandDetail.presetNamePlaceholder')}
                  />
                ))}
              </SortableContext>
            </DndContext>
            <Tooltip>
              <TooltipTrigger asChild>
                <button
                  type="button"
                  className="preset-chip preset-chip-add"
                  data-testid="preset-chip-add"
                  onClick={async () => {
                    preAddPresetIdRef.current = selectedPresetId;
                    const hasValues = Object.values(resolvedValues).some((v) => v.trim());
                    const newId = await onAddPreset(hasValues ? resolvedValues : undefined);
                    setSelectedPresetId(newId);
                    setRenamingChipId(newId);
                    setNewlyCreatedPresetId(newId);
                    setRenamingChipDraft('');
                  }}
                >
                  <Plus size={12} />
                </button>
              </TooltipTrigger>
              <TooltipContent>{t('commandDetail.addPreset')}</TooltipContent>
            </Tooltip>
          </div>

          <div className="command-text-box-glow mt-3">
            <div className="command-text-box-inner">
              <div className="preset-vars-panel">
                <div className="preset-vars-list">
                  {variables.map((v) => {
                    const val = resolvedValues[v.name];
                    return (
                      <div
                        key={v.name}
                        className={`preset-var-row${val ? '' : ' preset-var-row-empty'}`}
                      >
                        <span className="preset-var-name" title={'{{' + v.name + '}}'}>
                          {v.name}
                        </span>
                        <input
                          className={`preset-var-input preset-var-value${val ? '' : ' empty'}`}
                          data-testid={`preset-var-input-${v.name}`}
                          autoComplete="off"
                          autoCorrect="off"
                          autoCapitalize="off"
                          spellCheck={false}
                          value={val}
                          onChange={(e) =>
                            setOverrides((prev) => ({ ...prev, [v.name]: e.target.value }))
                          }
                          onFocus={() => setFocusedVarName(v.name)}
                          onBlur={() =>
                            setFocusedVarName((current) => (current === v.name ? null : current))
                          }
                          onKeyDown={async (e) => {
                            if (e.key === 'Enter') {
                              e.preventDefault();
                              if (selectedPresetId) {
                                try {
                                  await onSavePresetValues(selectedPresetId, resolvedValues);
                                  setOverrides({});
                                } catch {
                                  toast.error(t('commandDetail.savePresetFailed'));
                                }
                              }
                            }
                            if (e.key === 'Escape') {
                              e.preventDefault();
                              setOverrides((prev) => {
                                const next = { ...prev };
                                delete next[v.name];
                                return next;
                              });
                            }
                          }}
                          title={t('commandDetail.clickToEdit')}
                          placeholder={t('commandDetail.clickToSet')}
                        />
                      </div>
                    );
                  })}
                </div>
              </div>
              {hasUnsavedChanges && (
                <div className="command-text-box-header-actions" style={{ justifyContent: 'flex-end', padding: '4px 8px 8px' }}>
                  <Tooltip>
                    <TooltipTrigger asChild>
                      <Button variant="ghost" size="icon-xs" data-testid="preset-values-revert" onClick={() => setOverrides({})}>
                        <X className="size-3.5 text-destructive" />
                      </Button>
                    </TooltipTrigger>
                    <TooltipContent>{t('commandDetail.revertChanges')}</TooltipContent>
                  </Tooltip>
                  <Tooltip>
                    <TooltipTrigger asChild>
                      <Button
                        variant="ghost"
                        size="icon-xs"
                        data-testid="preset-values-save"
                        onClick={async () => {
                          await onSavePresetValues(selectedPresetId, resolvedValues);
                          setOverrides({});
                        }}
                      >
                        <Check className="size-3.5 text-success" />
                      </Button>
                    </TooltipTrigger>
                    <TooltipContent>{t('commandDetail.savePresetValues')}</TooltipContent>
                  </Tooltip>
                </div>
              )}
            </div>
          </div>
        </div>
      )}

      <AlertDialog
        open={confirmDeletePresetId !== null}
        onOpenChange={(open) => {
          if (!open) {
            setConfirmDeletePresetId(null);
            setDeletingPresetId(null);
          }
        }}
      >
        <AlertDialogContent data-testid="confirm-delete-preset-dialog">
          <AlertDialogHeader>
            <AlertDialogTitle>{t('commandDetail.deletePresetTitle')}</AlertDialogTitle>
            <AlertDialogDescription>{t('commandDetail.deletePresetDescription')}</AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel data-testid="confirm-delete-preset-cancel">{t('commandDetail.cancel')}</AlertDialogCancel>
            <AlertDialogAction
              variant="destructive"
              data-testid="confirm-delete-preset-confirm"
              onClick={async () => {
                const id = deletingPresetId || confirmDeletePresetId;
                if (id) {
                  await onDeletePreset(id);
                  if (selectedPresetId === id) setSelectedPresetId('');
                }
                setConfirmDeletePresetId(null);
                setDeletingPresetId(null);
              }}
            >
              {t('commandDetail.delete')}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      <AlertDialog
        open={showScriptDiscardConfirm}
        onOpenChange={(open) => { if (!open) setShowScriptDiscardConfirm(false); }}
      >
        {/* NOTE: button semantics here are inverted from what "Cancel"/"Action"
            normally imply — Cancel discards the pending script edit, Action
            saves it. Tests must not assume Cancel is a no-op. */}
        <AlertDialogContent data-testid="script-discard-dialog">
          <AlertDialogHeader>
            <AlertDialogTitle>{t('app.discardTitle')}</AlertDialogTitle>
            <AlertDialogDescription>{t('app.discardDescription')}</AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel data-testid="script-discard-discard" onClick={() => {
              discardScriptEdit();
            }}>
              {t('commandEditor.discard')}
            </AlertDialogCancel>
            <AlertDialogAction data-testid="script-discard-save" onClick={() => {
              saveScriptEdit();
            }}>
              {t('commandDetail.saveScript')}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      <Dialog open={workingDirDialogOpen} onOpenChange={setWorkingDirDialogOpen}>
        <DialogContent className="sm:max-w-md" data-testid="working-directory-dialog">
          <DialogHeader>
            <DialogTitle>{t('commandDetail.workingDirectoryDialogTitle')}</DialogTitle>
            <DialogDescription>
              {t('commandDetail.workingDirectoryDialogDescription')}
            </DialogDescription>
          </DialogHeader>
          <div className="flex items-center gap-2 py-2">
            <input
              type="text"
              data-testid="working-directory-input"
              value={workingDirDraft}
              onChange={(e) => setWorkingDirDraft(e.target.value)}
              placeholder={commandWD ? t('commandDetail.workingDirectoryPlaceholder') : (defaultWD || t('commandDetail.workingDirectoryPlaceholder'))}
              className="flex-1 h-9 rounded-md border border-input bg-transparent px-3 py-1 text-sm shadow-sm transition-colors placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring"
            />
            <Button
              type="button"
              variant="outline"
              size="sm"
              data-testid="working-directory-browse"
              onClick={async () => {
                if (currentOS === 'unknown') return;
                try {
                  const selected = await PickDirectory(workingDirDraft || effectiveWD);
                  if (selected) {
                    setWorkingDirDraft(selected);
                  }
                } catch (err) {
                  console.error('Directory picker error:', err);
                }
              }}
              disabled={currentOS === 'unknown'}
            >
              <FolderOpen size={14} className="mr-1" />
              {t('commandDetail.browse')}
            </Button>
          </div>
          <DialogFooter>
            <Button
              type="button"
              variant="ghost"
              size="sm"
              data-testid="working-directory-clear"
              onClick={() => {
                setWorkingDirDraft('');
              }}
            >
              {t('commandDetail.clear')}
            </Button>
            <Button
              type="button"
              size="sm"
              data-testid="working-directory-apply"
              disabled={currentOS === 'unknown'}
              onClick={() => {
                if (currentOS === 'unknown') return;
                onDraftChange({ workingDir: setOSPath(draft.workingDir, currentOS, workingDirDraft) });
                setWorkingDirDialogOpen(false);
              }}
            >
              {t('commandDetail.apply')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

    </div>
  );
};

export default React.memo(CommandDetail);
