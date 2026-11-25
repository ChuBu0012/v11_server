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
      <div className="brand-lockup">
        <div className="brand-mark" aria-hidden="true">
          <ShieldCheck />
        </div>
        <div>
          <div className="brand-title-row">
            <h1>Pocket Sentinel</h1>
            <span className="brand-badge">PUBLIC HUB</span>
          </div>
          <p>vivo 1804 · ศูนย์ติดตามสถานะและข่าวสาร</p>
        </div>
      </div>

      <div className="topbar-actions">
        <span className={isMockMode ? 'sync-pill preview' : stale ? 'sync-pill stale' : 'sync-pill live'}>
          <Radio />
          <span>{isMockMode ? 'โหมดตัวอย่าง' : stale ? 'ซิงก์ล่าช้า' : 'ออนไลน์'}</span>
        </span>
        <button onClick={onToggleTheme} aria-label="สลับธีม" className="theme-button" title="สลับโหมดมืด/สว่าง">
          {darkMode ? <Sun /> : <Moon />}
        </button>
      </div>
    </header>
  );
};
