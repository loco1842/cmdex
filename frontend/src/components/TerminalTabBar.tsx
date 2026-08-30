import React, { useRef, useEffect, useState } from 'react';
import { GripVertical, Plus, X } from 'lucide-react';
import {
  DndContext,
  closestCenter,
  PointerSensor,
  useSensor,
  useSensors,
  type DragEndEvent,
} from '@dnd-kit/core';
import { restrictToHorizontalAxis } from '@dnd-kit/modifiers';
import {
  arrayMove,
  SortableContext,
  useSortable,
  horizontalListSortingStrategy,
} from '@dnd-kit/sortable';
import { CSS } from '@dnd-kit/utilities';
import {
  ContextMenu,
  ContextMenuTrigger,
  ContextMenuContent,
  ContextMenuItem,
} from '@/components/ui/context-menu';
import { type SessionInfo } from '../types';

interface TerminalTabBarProps {
  sessions: SessionInfo[];
  activeSessionId: string;
  onSelectTab: (id: string) => void;
  onCloseTab: (id: string) => void;
  onReorderTabs: (sessions: SessionInfo[]) => void;
  onCreateSession: () => void;
  onRenameSession: (id: string, name: string) => void;
}

interface SortableTerminalTabProps {
  session: SessionInfo;
  isActive: boolean;
  isLastTab: boolean;
  onSelect: (id: string) => void;
  onClose: (id: string) => void;
  onRename: (id: string, name: string) => void;
}

function SortableTerminalTab({
  session,
  isActive,
  isLastTab,
  onSelect,
  onClose,
  onRename,
}: SortableTerminalTabProps) {
  const {
    attributes,
    listeners,
    setNodeRef,
    transform,
    transition,
  } = useSortable({ id: session.id });

  const style = {
    transform: CSS.Transform.toString(transform),
    transition,
  };

  const [isRenaming, setIsRenaming] = useState(false);
  const [renameValue, setRenameValue] = useState(session.name);
  const renameInputRef = useRef<HTMLInputElement>(null);

  const handleRename = () => {
    setRenameValue(session.name);
    setIsRenaming(true);
    // Focus the input after React commits the render
    requestAnimationFrame(() => {
      renameInputRef.current?.focus();
      renameInputRef.current?.select();
    });
  };

  const commitRename = () => {
    const trimmed = renameValue.trim();
    if (trimmed && trimmed !== session.name) {
      onRename(session.id, trimmed);
    }
    setIsRenaming(false);
  };

  const cancelRename = () => {
    setIsRenaming(false);
  };

  return (
    <ContextMenu>
      <ContextMenuTrigger asChild>
        <div
          ref={setNodeRef}
          style={style}
          className={`tab-item${isActive ? ' active' : ''}`}
          data-testid={`terminal-tab-${session.id}`}
          onClick={() => onSelect(session.id)}
        >
          <span className="tab-drag-handle" {...attributes} {...listeners}>
            <GripVertical size={12} />
          </span>
          <span
            className={`tab-status-dot ${session.running ? 'running' : 'stopped'}`}
            data-testid={`terminal-tab-status-${session.id}`}
          />
          {isRenaming ? (
            <input
              ref={renameInputRef}
              className="tab-rename-input"
              value={renameValue}
              onChange={(e) => setRenameValue(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === 'Enter') {
                  e.preventDefault();
                  commitRename();
                } else if (e.key === 'Escape') {
                  e.preventDefault();
                  cancelRename();
                }
              }}
              onBlur={commitRename}
              onClick={(e) => e.stopPropagation()}
            />
          ) : (
            <span className="tab-title" title={session.name}>
              {session.name}
            </span>
          )}
          {!isLastTab && (
            <span
              className="tab-close"
              role="button"
              aria-label={`Close ${session.name}`}
              onClick={(e) => {
                e.stopPropagation();
                onClose(session.id);
              }}
            >
              <X size={12} />
            </span>
          )}
        </div>
      </ContextMenuTrigger>
      <ContextMenuContent>
        <ContextMenuItem onSelect={handleRename}>
          Rename Session
        </ContextMenuItem>
        <ContextMenuItem
          disabled={isLastTab}
          onSelect={() => onClose(session.id)}
        >
          Close Session
        </ContextMenuItem>
      </ContextMenuContent>
    </ContextMenu>
  );
}

export default function TerminalTabBar({
  sessions,
  activeSessionId,
  onSelectTab,
  onCloseTab,
  onReorderTabs,
  onCreateSession,
  onRenameSession,
}: TerminalTabBarProps) {
  const activeRef = useRef<HTMLDivElement | null>(null);

  useEffect(() => {
    activeRef.current?.scrollIntoView({ inline: 'nearest', block: 'nearest' });
  }, [activeSessionId]);

  const sensors = useSensors(
    useSensor(PointerSensor, {
      activationConstraint: { distance: 5 },
    })
  );

  const handleDragEnd = (event: DragEndEvent) => {
    const { active, over } = event;
    if (!over || active.id === over.id) return;

    const oldIndex = sessions.findIndex((s) => s.id === String(active.id));
    const newIndex = sessions.findIndex((s) => s.id === String(over.id));

    if (oldIndex !== -1 && newIndex !== -1) {
      onReorderTabs(arrayMove(sessions, oldIndex, newIndex));
    }
  };

  if (sessions.length === 0) return null;

  const isLastTab = sessions.length <= 1;

  return (
    <div className="tab-bar" data-testid="terminal-tab-bar">
      <DndContext
        sensors={sensors}
        collisionDetection={closestCenter}
        modifiers={[restrictToHorizontalAxis]}
        onDragEnd={handleDragEnd}
      >
        <SortableContext
          items={sessions.map((s) => s.id)}
          strategy={horizontalListSortingStrategy}
        >
          {sessions.map((session) => {
            const isActive = session.id === activeSessionId;
            return (
              <SortableTerminalTab
                key={session.id}
                session={session}
                isActive={isActive}
                isLastTab={isLastTab}
                onSelect={onSelectTab}
                onClose={onCloseTab}
                onRename={onRenameSession}
              />
            );
          })}
        </SortableContext>
      </DndContext>
      <div
        className="tab-new-session-btn"
        data-testid="terminal-new-session-btn"
        role="button"
        aria-label="Create new terminal session"
        title="New Session (Ctrl+T)"
        onClick={onCreateSession}
      >
        <Plus size={14} />
      </div>
    </div>
  );
}
