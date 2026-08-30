import React, { useRef, useEffect } from 'react';
import { X } from 'lucide-react';

export interface Tab {
  id: string;
  title: string;
  isDirty?: boolean;
}

interface TabBarProps {
  tabs: Tab[];
  activeTabId: string | null;
  onSelectTab: (id: string) => void;
  onCloseTab: (id: string) => void;
}

const TabBar: React.FC<TabBarProps> = ({ tabs, activeTabId, onSelectTab, onCloseTab }) => {
  const activeRef = useRef<HTMLDivElement | null>(null);

  useEffect(() => {
    activeRef.current?.scrollIntoView({ inline: 'nearest', block: 'nearest' });
  }, [activeTabId]);

  if (tabs.length === 0) return null;

  return (
    <div className="tab-bar" data-testid="tab-bar">
      {tabs.map((tab) => {
        const isActive = tab.id === activeTabId;
        return (
          <div
            key={tab.id}
            ref={isActive ? activeRef : null}
            className={`tab-item${isActive ? ' active' : ''}`}
            data-testid={`tab-${tab.id}`}
            onClick={() => onSelectTab(tab.id)}
          >
            <span className="tab-title" title={tab.title}>{tab.title}</span>
            {tab.isDirty && <span className="tab-dirty-dot" title="Unsaved changes" data-testid={`tab-dirty-dot-${tab.id}`} />}
            <span
              className="tab-close"
              role="button"
              aria-label="Close tab"
              onClick={(e) => {
                e.stopPropagation();
                onCloseTab(tab.id);
              }}
            >
              <X size={12} />
            </span>
          </div>
        );
      })}
    </div>
  );
};

export default TabBar;
