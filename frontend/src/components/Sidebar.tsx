import React, { useState, useEffect, useCallback, useRef } from 'react';
import { useTranslation } from 'react-i18next';
import { toast } from 'sonner';
import {
  DndContext,
  DragOverlay,
  PointerSensor,
  KeyboardSensor,
  useSensor,
  useSensors,
  closestCenter,
  useDroppable,
  type DragStartEvent,
  type DragEndEvent,
  type DragOverEvent,
} from '@dnd-kit/core';
import {
  SortableContext,
  verticalListSortingStrategy,
  sortableKeyboardCoordinates,
  useSortable,
} from '@dnd-kit/sortable';
import { CSS } from '@dnd-kit/utilities';
import { type Category, type Command } from '../types';
import { getCommandDisplayTitle } from '../utils/tab';
import { Button } from '@/components/ui/button';
import { Collapsible, CollapsibleTrigger, CollapsibleContent } from '@/components/ui/collapsible';
import { Tooltip, TooltipTrigger, TooltipContent } from '@/components/ui/tooltip';
import {
  ContextMenu,
  ContextMenuTrigger,
  ContextMenuContent,
  ContextMenuItem,
} from '@/components/ui/context-menu';
import { Plus, Pencil, X, ChevronRight, Terminal, Settings, Group, Trash2, Download, Upload } from 'lucide-react';
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
import { ExportCommands, ImportCommands } from '../../bindings/cmdex/importexportservice';
import { MainLogo } from '@/assets/images/main-logo';

const STORAGE_KEY = 'cmdex-expanded-categories';

/** Normalize null/undefined/'' to '' for uncategorized bucket */
const normCatId = (id: string | null | undefined): string => id || '';

interface SidebarProps {
  categories: Category[];
  commands: Command[];
  selectedCommandId: string | null;
  onSelectCommand: (cmd: Command) => void;
  onAddCategory: () => void;
  onEditCategory: (cat: Category) => void;
  onDeleteCategory: (catId: string) => void;
  onAddCommand: (categoryId?: string) => void;
  onDeleteCommand: (cmd: Command) => void;
  onReorderCommand: (id: string, newPosition: number, newCategoryId: string) => void;
  onOpenSettings: () => void;
  onImport?: () => void;
}

interface SortableCommandItemProps {
  cmd: Command;
  isSelected: boolean;
  onSelect: () => void;
  onDelete: () => void;
  isPendingDelete: boolean;
  onRequestDelete: () => void;
  onCancelDelete: () => void;
}

const SortableCommandItem: React.FC<SortableCommandItemProps> = ({
  cmd,
  isSelected,
  onSelect,
  onDelete,
  isPendingDelete,
  onRequestDelete,
  onCancelDelete,
}) => {
  const { t } = useTranslation();
  const [isHovered, setIsHovered] = useState(false);
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = useSortable({ id: cmd.id });

  const style: React.CSSProperties = {
    transform: CSS.Transform.toString(transform),
    transition,
    opacity: isDragging ? 0.4 : 1,
  };

  return (
    <div
      ref={setNodeRef}
      style={style}
      className={`command-item ${isSelected ? 'active' : ''} ${isDragging ? 'dragging' : ''}`}
      data-testid={`command-item-${cmd.id}`}
      {...attributes}
      {...listeners}
      onClick={onSelect}
      onMouseEnter={() => setIsHovered(true)}
      onMouseLeave={() => setIsHovered(false)}
    >
      <Terminal className="size-3.5 shrink-0 text-muted-foreground" />
      <span className="cmd-body">
        <span className="cmd-title">{getCommandDisplayTitle(cmd)}</span>
        {cmd.tags && cmd.tags.length > 0 && (
          <span className="cmd-tags-row">
            {cmd.tags.slice(0, 3).map((tag) => (
              <span key={tag} className="cmd-tag-chip">#{tag}</span>
            ))}
          </span>
        )}
      </span>
      <span className="cmd-actions">
        {isPendingDelete ? (
          <>
          <button
              className="cmd-cancel-delete-icon-btn"
              onClick={(e) => { e.stopPropagation(); onCancelDelete(); }}
              title={t('common.cancel')}
            >
              <X className="size-3" />
            </button>
            <button
              className="cmd-delete-icon-btn text-destructive"
              onClick={(e) => { e.stopPropagation(); onDelete(); }}
              title={t('common.delete')}
            >
              <Trash2 className="size-3" />
            </button>
            
          </>
        ) : isHovered ? (
          <button
            className="cmd-trash-btn"
            onClick={(e) => { e.stopPropagation(); onRequestDelete(); }}
            title={t('sidebar.contextMenu.delete')}
          >
            <Trash2 className="size-3" />
          </button>
        ) : null}
      </span>
    </div>
  );
};

const DroppableCategoryZone: React.FC<{ categoryId: string; children: React.ReactNode }> = ({ categoryId, children }) => {
  const { setNodeRef } = useDroppable({ id: `category-drop-${categoryId}` });
  return (
    <div ref={setNodeRef} className="sidebar-commands" style={{ minHeight: 8 }}>
      {children}
    </div>
  );
};

const DroppableCategoryHeader: React.FC<{ categoryId: string; children: React.ReactNode }> = ({ categoryId, children }) => {
  const { setNodeRef } = useDroppable({ id: `category-header-${categoryId}` });
  return <div ref={setNodeRef}>{children}</div>;
};

const Sidebar: React.FC<SidebarProps> = ({
  categories,
  commands,
  selectedCommandId,
  onSelectCommand,
  onAddCategory,
  onEditCategory,
  onDeleteCategory,
  onAddCommand,
  onDeleteCommand,
  onReorderCommand,
  onOpenSettings,
  onImport,
}) => {
  const { t } = useTranslation();
  const prevCatIdsRef = useRef<string[]>([]);

  const [openCategories, setOpenCategories] = useState<Set<string>>(() => {
    try {
      const stored = localStorage.getItem(STORAGE_KEY);
      if (stored) return new Set(JSON.parse(stored) as string[]);
    } catch { /* ignore */ }
    const initial = new Set(categories.map(c => c.id));
    initial.add('__uncategorized__');
    return initial;
  });

  useEffect(() => {
    const prevIds = prevCatIdsRef.current;
    const currentIds = categories.map(c => c.id);
    const newIds = currentIds.filter(id => !prevIds.includes(id));
    if (newIds.length > 0) {
      setOpenCategories(prev => {
        const next = new Set(prev);
        newIds.forEach(id => next.add(id));
        return next;
      });
    }
    prevCatIdsRef.current = currentIds;
  }, [categories]);

  const persistExpanded = useCallback((expanded: Set<string>) => {
    try { localStorage.setItem(STORAGE_KEY, JSON.stringify([...expanded])); } catch { /* ignore */ }
  }, []);

  const toggleCategory = (catId: string) => {
    setOpenCategories(prev => {
      const next = new Set(prev);
      if (next.has(catId)) next.delete(catId);
      else next.add(catId);
      persistExpanded(next);
      return next;
    });
  };

  const getCommandsForCategory = (categoryId: string) =>
    commands.filter(cmd => cmd.categoryId === categoryId).sort((a, b) => a.position - b.position);

  const uncategorizedCommands = commands
    .filter(cmd => !cmd.categoryId || cmd.categoryId === '')
    .sort((a, b) => a.position - b.position);

  // Inline delete confirmation state
  const [pendingDeleteCmd, setPendingDeleteCmd] = useState<string | null>(null);
  const [pendingDeleteCat, setPendingDeleteCat] = useState<string | null>(null);

  // DnD state
  const [activeCommand, setActiveCommand] = useState<Command | null>(null);
  const [overCategoryId, setOverCategoryId] = useState<string | null>(null);

  const sensors = useSensors(
    useSensor(PointerSensor, { activationConstraint: { distance: 5 } }),
    useSensor(KeyboardSensor, { coordinateGetter: sortableKeyboardCoordinates })
  );

  const handleDragStart = useCallback((event: DragStartEvent) => {
    setActiveCommand(commands.find(c => c.id === String(event.active.id)) ?? null);
  }, [commands]);

  const parseCategoryDropId = (id: string): string | null => {
    if (id.startsWith('category-drop-')) return id.slice('category-drop-'.length);
    if (id.startsWith('category-header-')) return id.slice('category-header-'.length);
    return null;
  };

  const handleDragOver = useCallback((event: DragOverEvent) => {
    const overId = event.over?.id;
    if (!overId) { setOverCategoryId(null); return; }
    const overStr = String(overId);
    const catId = parseCategoryDropId(overStr);
    if (catId !== null) {
      setOverCategoryId(catId);
    } else if (categories.some(c => c.id === overId)) {
      setOverCategoryId(overStr);
    } else {
      const cmd = commands.find(c => c.id === overId);
      setOverCategoryId(normCatId(cmd?.categoryId));
    }
  }, [categories, commands]);

  const handleDragEnd = useCallback((event: DragEndEvent) => {
    const { active, over } = event;
    setActiveCommand(null);
    setOverCategoryId(null);
    if (!over) return;

    const activeId = String(active.id);
    const overId = String(over.id);
    if (activeId === overId) return;

    const activeCmd = commands.find(c => c.id === activeId);
    if (!activeCmd) return;

    const catDropId = parseCategoryDropId(overId);
    const isCatDrop = catDropId !== null || categories.some(c => c.id === overId);
    let targetCategoryId: string;
    let targetIndex: number;

    if (isCatDrop) {
      targetCategoryId = catDropId !== null ? catDropId : overId;
      const catCmds = commands.filter(c => normCatId(c.categoryId) === normCatId(targetCategoryId));
      targetIndex = catCmds.length;
    } else {
      const overCmd = commands.find(c => c.id === overId);
      targetCategoryId = normCatId(overCmd?.categoryId);

      if (normCatId(targetCategoryId) !== normCatId(activeCmd.categoryId)) {
        const catCmds = commands.filter(c => normCatId(c.categoryId) === normCatId(targetCategoryId));
        targetIndex = catCmds.length;
      } else {
        const catCmds = commands
          .filter(c => normCatId(c.categoryId) === normCatId(targetCategoryId))
          .sort((a, b) => a.position - b.position);
        targetIndex = catCmds.findIndex(c => c.id === overId);
        if (targetIndex === -1) targetIndex = catCmds.length;
      }
    }

    onReorderCommand(activeId, targetIndex, targetCategoryId);
  }, [commands, categories, onReorderCommand]);

  const isUncatDropTarget = overCategoryId === '';
  const isUncatOpen = openCategories.has('__uncategorized__');

  return (
    <div className="sidebar">
      {/* Header — outside context menu trigger so right-click doesn't fire */}
      <div className="sidebar-header">
        <div className="sidebar-logo">
          <div className="logo-icon">
            <MainLogo width="32" height="32" />
          </div>
          <h1>CmDex</h1>
          <Tooltip>
            <TooltipTrigger asChild>
              <Button
                size="icon-sm"
                variant="ghost"
                onClick={() => onAddCommand()}
                className="ml-auto"
                data-testid="sidebar-add-command"
              >
                <Plus className="size-4" />
              </Button>
            </TooltipTrigger>
            <TooltipContent>{t('sidebar.newCommand')}</TooltipContent>
          </Tooltip>
          <Tooltip>
            <TooltipTrigger asChild>
              <Button variant="ghost" size="icon-sm" onClick={onOpenSettings} data-testid="sidebar-settings">
                <Settings className="size-4" />
              </Button>
            </TooltipTrigger>
            <TooltipContent>{t('sidebar.settings')}</TooltipContent>
          </Tooltip>
        </div>
      </div>

      {/* Command list with DnD — context menu scoped to scroll area only */}
      <DndContext
        sensors={sensors}
        collisionDetection={closestCenter}
        onDragStart={handleDragStart}
        onDragOver={handleDragOver}
        onDragEnd={handleDragEnd}
      >
        <ContextMenu>
          <ContextMenuTrigger asChild>
            <div className="sidebar-content overflow-y-auto scrollbar-thin scrollbar-w-1 scrollbar-thumb-border scrollbar-track-transparent">
              {categories.map(cat => {
                const catCommands = getCommandsForCategory(cat.id);
                const isOpen = openCategories.has(cat.id);
                const isDropTarget = overCategoryId === cat.id;

                return (
                  <Collapsible key={cat.id} open={isOpen} onOpenChange={() => toggleCategory(cat.id)}>
                    <DroppableCategoryHeader categoryId={cat.id}>
                    <div className={`sidebar-section ${isDropTarget ? 'drop-target' : ''}`}>
                      <ContextMenu>
                        <ContextMenuTrigger asChild onContextMenu={(e) => e.stopPropagation()}>
                          <CollapsibleTrigger asChild>
                            <div className="sidebar-section-header" data-category-id={cat.id}>
                              <div className="section-left">
                                <ChevronRight className={`size-3.5 transition-transform ${isOpen ? 'rotate-90' : ''}`} />
                                <span className="category-dot" style={{ backgroundColor: cat.color || '#7c6aef' }} />
                                <span className="text-sm">{cat.name}</span>
                              </div>
                              <div className="section-right" />
                            </div>
                          </CollapsibleTrigger>
                        </ContextMenuTrigger>
                        <ContextMenuContent>
                          <ContextMenuItem onSelect={() => onAddCommand(cat.id)}>
                            <Plus className="size-3.5" /> {t('sidebar.contextMenu.newCommand')}
                          </ContextMenuItem>
                          <ContextMenuItem onSelect={onAddCategory}>
                            <ChevronRight className="size-3.5" /> {t('sidebar.contextMenu.newGroup')}
                          </ContextMenuItem>
                          <ContextMenuItem onSelect={() => onEditCategory(cat)}>
                            <Pencil className="size-3.5" /> {t('sidebar.editCategory')}
                          </ContextMenuItem>
                          <ContextMenuItem onSelect={async () => {
                            try {
                              await ExportCommands(commands.filter(c => c.categoryId === cat.id).map(c => c.id));
                            } catch (e) { console.error('Export failed:', e); toast.error(t('toast.exportFailed')); }
                          }}>
                            <Download className="size-3.5" /> {t('sidebar.exportCommands')}
                          </ContextMenuItem>
                          <ContextMenuItem onSelect={async () => {
                            try {
                              const imported = await ImportCommands();
                              if (imported && imported.length > 0 && onImport) onImport();
                            } catch (e) { console.error('Import failed:', e); toast.error(t('toast.importFailed')); }
                          }}>
                            <Upload className="size-3.5" /> {t('sidebar.importCommands')}
                          </ContextMenuItem>
                          <ContextMenuItem
                            onSelect={() => setPendingDeleteCat(cat.id)}
                            className="text-destructive focus:text-destructive"
                          >
                            <Trash2 className="size-3.5" /> {t('sidebar.contextMenu.delete')}
                          </ContextMenuItem>
                        </ContextMenuContent>
                      </ContextMenu>

                      <CollapsibleContent>
                        <SortableContext
                          items={catCommands.map(c => c.id)}
                          strategy={verticalListSortingStrategy}
                        >
                          <DroppableCategoryZone categoryId={cat.id}>
                            {catCommands.map(cmd => (
                              <SortableCommandItem
                                key={cmd.id}
                                cmd={cmd}
                                isSelected={selectedCommandId === cmd.id}
                                onSelect={() => onSelectCommand(cmd)}
                                onDelete={() => { onDeleteCommand(cmd); setPendingDeleteCmd(null); }}
                                isPendingDelete={pendingDeleteCmd === cmd.id}
                                onRequestDelete={() => setPendingDeleteCmd(cmd.id)}
                                onCancelDelete={() => setPendingDeleteCmd(null)}
                              />
                            ))}
                            {catCommands.length === 0 && (
                              <div className="command-item opacity-60" style={{ fontStyle: 'italic', cursor: 'default' }}>
                                <Plus className="size-3.5 shrink-0 opacity-30" />
                                <span className="cmd-title cursor-pointer" onClick={() => onAddCommand(cat.id)}>
                                  {t('sidebar.addACommand')}
                                </span>
                              </div>
                            )}
                          </DroppableCategoryZone>
                        </SortableContext>
                      </CollapsibleContent>
                    </div>
                    </DroppableCategoryHeader>
                  </Collapsible>
                );
              })}

              {/* Uncategorized — always rendered so it's a valid drop target */}
              <Collapsible open={isUncatOpen} onOpenChange={() => toggleCategory('__uncategorized__')}>
                <DroppableCategoryHeader categoryId="">
                <div className={`sidebar-section ${isUncatDropTarget ? 'drop-target' : ''}`}>
                  <ContextMenu>
                    <ContextMenuTrigger asChild onContextMenu={(e) => e.stopPropagation()}>
                      <CollapsibleTrigger asChild>
                        <div className="sidebar-section-header">
                          <div className="section-left">
                            <ChevronRight className={`size-3.5 transition-transform ${isUncatOpen ? 'rotate-90' : ''}`} />
                            <span className="category-dot" style={{ backgroundColor: '#6c6c88' }} />
                            <span className="text-xs uppercase">{t('sidebar.uncategorized')}</span>
                          </div>
                        </div>
                      </CollapsibleTrigger>
                    </ContextMenuTrigger>
                    <ContextMenuContent>
                      <ContextMenuItem onSelect={() => onAddCommand()}>
                        <Plus className="size-3.5" /> {t('sidebar.contextMenu.newCommand')}
                      </ContextMenuItem>
                      <ContextMenuItem onSelect={onAddCategory}>
                        <ChevronRight className="size-3.5" /> {t('sidebar.contextMenu.newGroup')}
                      </ContextMenuItem>
                    </ContextMenuContent>
                  </ContextMenu>
                  <CollapsibleContent>
                    <SortableContext
                      items={uncategorizedCommands.map(c => c.id)}
                      strategy={verticalListSortingStrategy}
                    >
                      <DroppableCategoryZone categoryId="">
                        {uncategorizedCommands.map(cmd => (
                          <SortableCommandItem
                            key={cmd.id}
                            cmd={cmd}
                            isSelected={selectedCommandId === cmd.id}
                            onSelect={() => onSelectCommand(cmd)}
                            onDelete={() => { onDeleteCommand(cmd); setPendingDeleteCmd(null); }}
                            isPendingDelete={pendingDeleteCmd === cmd.id}
                            onRequestDelete={() => setPendingDeleteCmd(cmd.id)}
                            onCancelDelete={() => setPendingDeleteCmd(null)}
                          />
                        ))}
                      </DroppableCategoryZone>
                    </SortableContext>
                  </CollapsibleContent>
                </div>
                </DroppableCategoryHeader>
              </Collapsible>
            </div>
          </ContextMenuTrigger>
          {/* Empty-space context menu (scoped to scroll area) */}
          <ContextMenuContent>
            <ContextMenuItem onSelect={() => onAddCommand()}>
              <Terminal className="size-3.5" /> {t('sidebar.contextMenu.newCommand')}
            </ContextMenuItem>
            <ContextMenuItem onSelect={onAddCategory}>
              <Group className="size-3.5" /> {t('sidebar.contextMenu.newGroup')}
            </ContextMenuItem>
            <ContextMenuItem onSelect={async () => {
              try {
                await ExportCommands(commands.map(c => c.id));
              } catch (e) { console.error('Export failed:', e); toast.error(t('toast.exportFailed')); }
            }}>
              <Download className="size-3.5" /> {t('sidebar.exportCommands')}
            </ContextMenuItem>
            <ContextMenuItem onSelect={async () => {
              try {
                const imported = await ImportCommands();
                if (imported && imported.length > 0 && onImport) onImport();
              } catch (e) { console.error('Import failed:', e); toast.error(t('toast.importFailed')); }
            }}>
              <Upload className="size-3.5" /> {t('sidebar.importCommands')}
            </ContextMenuItem>
          </ContextMenuContent>
        </ContextMenu>

        {/* Drag overlay ghost */}
        <DragOverlay>
          {activeCommand && (
            <div className="command-item dragging-ghost">
              <Terminal className="size-3.5 shrink-0 text-muted-foreground" />
              <span className="cmd-title">{getCommandDisplayTitle(activeCommand)}</span>
            </div>
          )}
        </DragOverlay>
      </DndContext>

      {/* Category delete — AlertDialog renders as body portal, covers full window */}
      {(() => {
        const catToDelete = categories.find(c => c.id === pendingDeleteCat);
        return (
          <AlertDialog open={pendingDeleteCat !== null} onOpenChange={(open) => { if (!open) setPendingDeleteCat(null); }}>
            <AlertDialogContent className="max-w-xs" data-testid="confirm-delete-category-dialog">
              <AlertDialogHeader>
                <AlertDialogTitle>{t('sidebar.deleteCategory')}</AlertDialogTitle>
                <AlertDialogDescription>
                  {t('common.deleteCategoryDescription')}
                </AlertDialogDescription>
              </AlertDialogHeader>
              <AlertDialogFooter>
                <AlertDialogCancel data-testid="confirm-delete-category-cancel" onClick={() => setPendingDeleteCat(null)}>
                  {t('common.cancel')}
                </AlertDialogCancel>
                <AlertDialogAction
                  variant="destructive"
                  data-testid="confirm-delete-category-confirm"
                  onClick={() => {
                    if (pendingDeleteCat) onDeleteCategory(pendingDeleteCat);
                    setPendingDeleteCat(null);
                  }}
                >
                  {catToDelete ? `${t('common.deleteWithName', { name: catToDelete.name })}` : t('common.delete')}
                </AlertDialogAction>
              </AlertDialogFooter>
            </AlertDialogContent>
          </AlertDialog>
        );
      })()}
    </div>
  );
};

export default Sidebar;
