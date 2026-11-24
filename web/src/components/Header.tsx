import React from 'react';
import { Sun, Moon, ShieldCheck, Radio } from 'lucide-react';
import { isMockMode } from '../lib/firebase';
import { PublicStatus } from '../lib/types';

interface HeaderProps {
  darkMode: boolean;
  onToggleTheme: () => void;
  status: PublicStatus;
}

export const Header: React.FC<HeaderProps> = ({ darkMode, onToggleTheme, status }) => {
  const ageSeconds = status.updated_at
    ? Math.max(0, Math.floor((Date.now() - new Date(status.updated_at).getTime()) / 1000))
    : 0;
  const stale = ageSeconds > 180;

  return (
    <header className="topbar">
      <div className="brand-block">
        <div className="brand-mark" aria-hidden="true">
          <ShieldCheck />
          <span className={stale ? 'brand-pulse stale' : 'brand-pulse'} />
        </div>
        <div>
          <div className="brand-title-row">
            <h1>Pocket Sentinel</h1>
            <span className="edition">PUBLIC</span>
          </div>
          <p>vivo 1804 · Public operations surface</p>
        </div>
      </div>

      <div className="topbar-actions">
        <span className={isMockMode ? 'sync-badge preview' : stale ? 'sync-badge stale' : 'sync-badge live'}>
          <Radio />
          {isMockMode ? 'Preview' : stale ? 'Sync delayed' : 'Live sync'}
        </span>
        <button onClick={onToggleTheme} aria-label="สลับธีม" className="icon-button">
          {darkMode ? <Sun /> : <Moon />}
        </button>
      </div>
    </header>
  );
};
